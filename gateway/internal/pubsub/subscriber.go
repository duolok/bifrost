package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"duolok/bifrost/gateway/internal/events"
	"duolok/bifrost/gateway/internal/k8s"
	"duolok/bifrost/gateway/internal/models"

	gcppubsub "cloud.google.com/go/pubsub"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type BuildComplete struct {
	DeployID     string `json:"deploy_id"`
	ImageURI     string `json:"image_uri"`
	Success      bool   `json:"success"`
	ErrorMessage string `json:"error_message"`
}

type Subscriber struct {
	pool     *pgxpool.Pool
	deployer *k8s.Deployer
	emitter  *events.Emitter
	sub      *gcppubsub.Subscription
}

func NewSubscriber(ctx context.Context, pool *pgxpool.Pool, deployer *k8s.Deployer, emitter *events.Emitter, gcpProject, subName string) (*Subscriber, error) {
	client, err := gcppubsub.NewClient(ctx, gcpProject)
	if err != nil {
		return nil, fmt.Errorf("create pubsub client: %w", err)
	}

	return &Subscriber{
		pool:     pool,
		deployer: deployer,
		emitter:  emitter,
		sub:      client.Subscription(subName),
	}, nil
}

func (s *Subscriber) Start(ctx context.Context) error {
	slog.Info("build-complete subscriber started")
	return s.sub.Receive(ctx, func(ctx context.Context, msg *gcppubsub.Message) {
		var bc BuildComplete
		if err := json.Unmarshal(msg.Data, &bc); err != nil {
			slog.Error("failed to parse build-complete message", "error", err, "data", string(msg.Data))
			msg.Ack()
			return
		}

		slog.Info("received build-complete", "deploy_id", bc.DeployID, "success", bc.Success)

		if err := s.handleBuildComplete(ctx, bc); err != nil {
			slog.Error("failed to handle build-complete", "deploy_id", bc.DeployID, "error", err)
			msg.Nack()
			return
		}

		msg.Ack()
	})
}

func (s *Subscriber) handleBuildComplete(ctx context.Context, bc BuildComplete) error {
	deployID, err := uuid.Parse(bc.DeployID)
	if err != nil {
		return fmt.Errorf("invalid deploy_id %q: %w", bc.DeployID, err)
	}

	if !bc.Success {
		_, err := s.pool.Exec(ctx,
			`UPDATE deployments SET status = $1, status_message = $2, build_finished_at = NOW()
			 WHERE id = $3 AND status = $4`,
			models.StatusFailed, bc.ErrorMessage, deployID, models.StatusBuilding,
		)
		s.emitter.Emit("deploy.build_failed", bc.DeployID, "", bc.ErrorMessage)
		return err
	}

	tag, err := s.pool.Exec(ctx,
		`UPDATE deployments SET status = $1, image_uri = $2, build_finished_at = NOW()
		 WHERE id = $3 AND status = $4`,
		models.StatusBuilt, bc.ImageURI, deployID, models.StatusBuilding,
	)

	if err != nil {
		return fmt.Errorf("update deployment to built: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("deployment %s not in building state", deployID)
	}

	s.emitter.Emit("deploy.built", bc.DeployID, "", "Build complete")

	if s.deployer == nil {
		slog.Warn("k8s deployer not configured, stopping at built", "deploy_id", deployID)
		return nil
	}

	var d models.Deployment
	err = s.pool.QueryRow(ctx,
		`SELECT id, project_id, commit_sha, branch, triggered_by, image_uri,
		        status, status_message, config_snapshot,
		        build_started_at, build_finished_at, deploy_started_at, deploy_finished_at, created_at
		 FROM deployments WHERE id = $1`, deployID,
	).Scan(
		&d.ID, &d.ProjectID, &d.CommitSHA, &d.Branch, &d.TriggeredBy, &d.ImageURI,
		&d.Status, &d.StatusMessage, &d.ConfigSnapshot,
		&d.BuildStartedAt, &d.BuildFinishedAt, &d.DeployStartedAt, &d.DeployFinishedAt, &d.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("fetch deployment: %w", err)
	}

	var p models.Project
	err = s.pool.QueryRow(ctx,
		`SELECT id, name, repo_url, default_branch, config, status, created_at, updated_at
		 FROM projects WHERE id = $1`, d.ProjectID,
	).Scan(&p.ID, &p.Name, &p.RepoURL, &p.DefaultBranch, &p.Config, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("fetch project: %w", err)
	}

	if err := s.deployer.Deploy(ctx, d, p); err != nil {
		return fmt.Errorf("k8s deploy: %w", err)
	}

	slog.Info("deployment complete", "deploy_id", deployID, "project", p.Name)
	return nil
}
