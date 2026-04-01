package api

import (
	"context"
	"log/slog"
	"time"

	"duolok/bifrost/gateway/internal/events"
	"duolok/bifrost/gateway/internal/k8s"
	"duolok/bifrost/gateway/internal/middleware"
	"duolok/bifrost/gateway/internal/models"
	"duolok/bifrost/gateway/internal/notify"
	"duolok/bifrost/gateway/internal/pubsub"
	"duolok/bifrost/gateway/internal/rules"
	"duolok/bifrost/gateway/internal/validator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	pool      *pgxpool.Pool
	deployer  *k8s.Deployer
	publisher *pubsub.Publisher
	validator *validator.Client
	emitter   *events.Emitter
	notifier  *notify.Publisher
	rules     *rules.Engine
	jwtSecret []byte
	oauth     *OAuthConfig
	startAt   time.Time
}

type HandlerDeps struct {
	Pool      *pgxpool.Pool
	Deployer  *k8s.Deployer
	Publisher *pubsub.Publisher
	Validator *validator.Client
	Emitter   *events.Emitter
	Notifier  *notify.Publisher
	Rules     *rules.Engine
	JWTSecret []byte
	OAuth     *OAuthConfig
}

func NewHandler(deps HandlerDeps) *Handler {
	return &Handler{
		pool:      deps.Pool,
		deployer:  deps.Deployer,
		publisher: deps.Publisher,
		validator: deps.Validator,
		emitter:   deps.Emitter,
		notifier:  deps.Notifier,
		rules:     deps.Rules,
		jwtSecret: deps.JWTSecret,
		oauth:     deps.OAuth,
		startAt:   time.Now(),
	}
}

const deploymentSelectSQL = `SELECT id, project_id, commit_sha, branch, triggered_by, image_uri,
			status, status_message, config_snapshot,
			build_started_at, build_finished_at, deploy_started_at, deploy_finished_at, created_at
			FROM deployments`

// fetchDeployment loads a deployment by ID.
func (h *Handler) fetchDeployment(ctx context.Context, id uuid.UUID) (models.Deployment, error) {
	var d models.Deployment
	err := scanDeployment(h.pool.QueryRow(ctx, deploymentSelectSQL+` WHERE id = $1`, id), &d)
	return d, err
}

// audit function is used for managing audit logs.
func (h *Handler) audit(c *gin.Context, action, resourceType string, resourceID uuid.UUID, details gin.H) {
	actor := middleware.GetUserID(c)
	if actor == "" {
		actor = "system"
	}
	go func() {
		_, err := h.pool.Exec(context.Background(),
			`INSERT INTO audit_log (actor, action, resource_type, resource_id, details)
			 VALUES ($1, $2, $3, $4, $5)`,
			actor, action, resourceType, resourceID, details,
		)
		if err != nil {
			slog.Error("failed to write audit log", "action", action, "error", err)
		}
	}()
}

// fetchProjectMeta loads just the name and repo_url for a project.
func (h *Handler) fetchProjectMeta(ctx context.Context, projectID uuid.UUID) (name, repoURL string) {
	_ = h.pool.QueryRow(ctx,
		`SELECT name, repo_url FROM projects WHERE id = $1`, projectID,
	).Scan(&name, &repoURL)
	return
}

// scanDeployment scans a full deployment row into the struct.
// Works with both pgx.Row and pgx.Rows via the scanner interface.
type scanner interface {
	Scan(dest ...any) error
}

func scanDeployment(s scanner, d *models.Deployment) error {
	return s.Scan(
		&d.ID, &d.ProjectID, &d.CommitSHA, &d.Branch, &d.TriggeredBy, &d.ImageURI,
		&d.Status, &d.StatusMessage, &d.ConfigSnapshot,
		&d.BuildStartedAt, &d.BuildFinishedAt, &d.DeployStartedAt, &d.DeployFinishedAt, &d.CreatedAt,
	)
}
