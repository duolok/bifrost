package api

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"log/slog"
	"net/http"
	"strings"

	"duolok/bifrost/gateway/internal/errors"
	"duolok/bifrost/gateway/internal/models"
	"duolok/bifrost/gateway/internal/pubsub"
	"duolok/bifrost/gateway/internal/validator"
	"duolok/bifrost/gateway/pkg/apiutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (h *Handler) TriggerDeploy(c *gin.Context) {
	projectID, ok := apiutil.ParseID(c, "id", resourceProject)
	if !ok {
		return
	}

	var req models.TriggerDeployRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiutil.RespondError(c, errors.InvalidInput(err.Error()))
		return
	}

	var exists bool
	err := h.pool.QueryRow(c.Request.Context(),
		`SELECT EXISTS(SELECT 1 FROM projects WHERE id = $1 AND status = $2)`,
		projectID, models.ProjectActive,
	).Scan(&exists)

	if err != nil {
		apiutil.RespondError(c, errors.Internal("failed to verify project", err))
		return
	}

	if !exists {
		apiutil.RespondError(c, errors.NotFound(resourceProject, projectID))
		return
	}

	var d models.Deployment
	err = h.pool.QueryRow(c.Request.Context(),
		`INSERT INTO deployments (project_id, commit_sha, branch, triggered_by, status)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (project_id, commit_sha) DO NOTHING
		 RETURNING id, project_id, commit_sha, branch, triggered_by, status, created_at`,
		projectID, req.CommitSHA, apiutil.NilIfEmpty(req.Branch), triggerAPI, models.StatusQueued,
	).Scan(&d.ID, &d.ProjectID, &d.CommitSHA, &d.Branch, &d.TriggeredBy, &d.Status, &d.CreatedAt)

	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			apiutil.RespondError(c, errors.AlreadyExists(resourceDeployment, req.CommitSHA))
			return
		}
		apiutil.RespondError(c, errors.Internal("failed to create deployment", err))
		return
	}

	h.audit(c, auditDeployTriggered, resourceDeployment, d.ID, gin.H{
		"project_id": projectID,
		"commit_sha": req.CommitSHA,
	})

	projectName, repoURL := h.fetchProjectMeta(c.Request.Context(), projectID)
	h.validateAndBuild(c, d.ID, projectName, repoURL, req.CommitSHA)
	h.emitter.Emit("deploy.created", d.ID.String(), projectName, "Deployment queued")

	c.JSON(http.StatusCreated, d)
}

func (h *Handler) ListDeployments(c *gin.Context) {
	projectID, ok := apiutil.ParseID(c, "id", resourceProject)
	if !ok {
		return
	}

	rows, err := h.pool.Query(c.Request.Context(),
		deploymentSelectSQL+` WHERE project_id = $1 ORDER BY created_at DESC`, projectID)
	if err != nil {
		apiutil.RespondError(c, errors.Internal("failed to list deployments", err))
		return
	}
	defer rows.Close()

	deployments := []models.Deployment{}
	for rows.Next() {
		var d models.Deployment
		if err := scanDeployment(rows, &d); err != nil {
			apiutil.RespondError(c, errors.Internal("failed to scan deployment", err))
			return
		}
		deployments = append(deployments, d)
	}

	c.JSON(http.StatusOK, gin.H{"deployments": deployments})
}

func (h *Handler) GetDeployment(c *gin.Context) {
	id, ok := apiutil.ParseID(c, "id", resourceDeployment)
	if !ok {
		return
	}

	d, err := h.fetchDeployment(c.Request.Context(), id)
	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			apiutil.RespondError(c, errors.NotFound(resourceDeployment, id))
			return
		}
		apiutil.RespondError(c, errors.Internal("failed to get deployment", err))
		return
	}

	c.JSON(http.StatusOK, d)
}

func (h *Handler) DeployBuilt(c *gin.Context) {
	if h.deployer == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "k8s deployer not configured"})
		return
	}

	id, ok := apiutil.ParseID(c, "id", resourceDeployment)
	if !ok {
		return
	}

	d, err := h.fetchDeployment(c.Request.Context(), id)
	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			apiutil.RespondError(c, errors.NotFound(resourceDeployment, id))
			return
		}
		apiutil.RespondError(c, errors.Internal("failed to get deployment", err))
		return
	}

	if !models.CanTransition(d.Status, models.StatusDeploying) {
		apiutil.RespondError(c, errors.InvalidInput(
			"cannot deploy from status "+string(d.Status)+"; expected "+string(models.StatusBuilt),
		))
		return
	}

	if d.ImageURI == nil || *d.ImageURI == "" {
		apiutil.RespondError(c, errors.InvalidInput("deployment has no image_uri; build must complete first"))
		return
	}

	var p models.Project
	err = h.pool.QueryRow(c.Request.Context(),
		`SELECT id, name, repo_url, default_branch, config, status, created_at, updated_at
		 FROM projects
		 WHERE id = $1`, d.ProjectID,
	).Scan(&p.ID, &p.Name, &p.RepoURL, &p.DefaultBranch, &p.Config, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		apiutil.RespondError(c, errors.Internal("failed to get project for deployment", err))
		return
	}

	if err := h.deployer.Deploy(c.Request.Context(), d, p); err != nil {
		h.audit(c, auditDeployFailed, resourceDeployment, d.ID, gin.H{"error": err.Error()})
		apiutil.RespondError(c, errors.DeployFailed("k8s deployment failed", err))
		return
	}

	h.audit(c, auditDeployComplete, resourceDeployment, d.ID, gin.H{"project": p.Name})

	d, _ = h.fetchDeployment(c.Request.Context(), id)
	c.JSON(http.StatusOK, d)
}

func (h *Handler) RetryDeploy(c *gin.Context) {
	id, ok := apiutil.ParseID(c, "id", resourceDeployment)
	if !ok {
		return
	}

	ctx := c.Request.Context()

	d, err := h.fetchDeployment(ctx, id)
	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			apiutil.RespondError(c, errors.NotFound(resourceDeployment, id))
			return
		}
		apiutil.RespondError(c, errors.Internal("failed to get deployment", err))
		return
	}

	if !models.CanTransition(d.Status, models.StatusQueued) {
		apiutil.RespondError(c, errors.InvalidInput(
			"cannot retry from status "+string(d.Status)+"; only failed deployments can be retried",
		))
		return
	}

	_, err = h.pool.Exec(ctx,
		`UPDATE deployments
		 SET status = $1, status_message = NULL, config_snapshot = NULL,
		     build_started_at = NULL, build_finished_at = NULL,
		     deploy_started_at = NULL, deploy_finished_at = NULL
		 WHERE id = $2`,
		models.StatusQueued, id,
	)
	if err != nil {
		apiutil.RespondError(c, errors.Internal("failed to reset deployment", err))
		return
	}

	h.audit(c, auditDeployRetried, resourceDeployment, id, gin.H{
		"previous_status": string(d.Status),
	})

	projectName, repoURL := h.fetchProjectMeta(ctx, d.ProjectID)
	h.validateAndBuild(c, id, projectName, repoURL, d.CommitSHA)
	h.emitter.Emit("deploy.retried", id.String(), projectName, "Deployment retried")

	d, _ = h.fetchDeployment(ctx, id)
	c.JSON(http.StatusOK, d)
}

func (h *Handler) Rollback(c *gin.Context) {
	if h.deployer == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "k8s deployer not configured"})
		return
	}

	projectID, ok := apiutil.ParseID(c, "id", resourceProject)
	if !ok {
		return
	}

	ctx := c.Request.Context()

	// Find the last successful deployment for this project
	var prev models.Deployment
	err := scanDeployment(h.pool.QueryRow(ctx,
		deploymentSelectSQL+` WHERE project_id = $1 AND status IN ($2, $3)
		 ORDER BY deploy_finished_at DESC LIMIT 1`,
		projectID, models.StatusRunning, models.StatusHealthy,
	), &prev)

	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			apiutil.RespondError(c, errors.NotFound("healthy deployment", projectID))
			return
		}
		apiutil.RespondError(c, errors.Internal("failed to find previous deployment", err))
		return
	}

	if prev.ImageURI == nil || *prev.ImageURI == "" {
		apiutil.RespondError(c, errors.InvalidInput("previous deployment has no image_uri"))
		return
	}

	// Create a new deployment record at "built" status (skip build pipeline).
	// Use rollback:{original_id} as commit_sha to avoid UNIQUE(project_id, commit_sha) conflict.
	rollbackSHA := "rollback:" + prev.ID.String()[:8]
	var d models.Deployment
	err = h.pool.QueryRow(ctx,
		`INSERT INTO deployments (project_id, commit_sha, branch, triggered_by, image_uri, config_snapshot, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, project_id, commit_sha, branch, triggered_by, image_uri, status, created_at`,
		projectID, rollbackSHA, prev.Branch, triggerRollback, prev.ImageURI, prev.ConfigSnapshot, models.StatusBuilt,
	).Scan(&d.ID, &d.ProjectID, &d.CommitSHA, &d.Branch, &d.TriggeredBy, &d.ImageURI, &d.Status, &d.CreatedAt)

	if err != nil {
		apiutil.RespondError(c, errors.Internal("failed to create rollback deployment", err))
		return
	}

	// Fetch full project for deployer
	var p models.Project
	err = h.pool.QueryRow(ctx,
		`SELECT id, name, repo_url, default_branch, config, status, created_at, updated_at
		 FROM projects WHERE id = $1`, projectID,
	).Scan(&p.ID, &p.Name, &p.RepoURL, &p.DefaultBranch, &p.Config, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		apiutil.RespondError(c, errors.Internal("failed to get project", err))
		return
	}

	h.audit(c, auditDeployRollback, resourceDeployment, d.ID, gin.H{
		"project":         p.Name,
		"rollback_to":     prev.ID,
		"rollback_commit": prev.CommitSHA,
	})

	h.emitter.Emit("deploy.rollback", d.ID.String(), p.Name, "Rolling back to "+prev.CommitSHA[:8])

	// Deploy using the existing image
	if err := h.deployer.Deploy(ctx, d, p); err != nil {
		h.audit(c, auditDeployFailed, resourceDeployment, d.ID, gin.H{"error": err.Error()})
		apiutil.RespondError(c, errors.DeployFailed("rollback deployment failed", err))
		return
	}

	d, _ = h.fetchDeployment(ctx, d.ID)
	c.JSON(http.StatusOK, gin.H{
		"status":          "rolled_back",
		"deployment_id":   d.ID,
		"rolled_back_to":  prev.CommitSHA,
		"previous_deploy": prev.ID,
	})
}

// validateAndBuild runs OCaml validation then publishes a build request.
func (h *Handler) validateAndBuild(c *gin.Context, deployID uuid.UUID, projectName, repoURL, commitSHA string) {
	ctx := c.Request.Context()

	if h.validator != nil {
		h.runValidation(ctx, deployID, projectName, repoURL, commitSHA)
	}

	if h.publisher != nil {
		h.publishBuild(ctx, deployID, projectName, repoURL, commitSHA)
	}
}

func (h *Handler) runValidation(ctx context.Context, deployID uuid.UUID, projectName, repoURL, commitSHA string) {
	h.emitter.Emit("deploy.validating", deployID.String(), projectName, "Validating config")

	_, err := h.pool.Exec(ctx,
		`UPDATE deployments SET status = $1 WHERE id = $2 AND status = $3`,
		models.StatusValidating, deployID, models.StatusQueued,
	)
	if err != nil {
		slog.Error("failed to transition to validating", "deploy_id", deployID, "error", err)
		return
	}

	slog.Info("validating deployment config", "deploy_id", deployID, "project", projectName)

	configRaw, err := validator.FetchDeployToml(ctx, repoURL, commitSHA)
	if err != nil {
		slog.Error("failed to fetch deploy.toml", "deploy_id", deployID, "error", err)
		h.failDeploy(ctx, deployID, "failed to fetch deploy.toml: "+err.Error())
		return
	}

	result, err := h.validator.Validate(ctx, configRaw)
	if err != nil {
		slog.Error("validator call failed", "deploy_id", deployID, "error", err)
		h.failDeploy(ctx, deployID, "validator unavailable: "+err.Error())
		return
	}

	if result.Status != "valid" {
		msgs := make([]string, len(result.Errors))
		for i, e := range result.Errors {
			msgs[i] = e.Field + ": " + e.Message
		}
		errMsg := "config validation failed: " + strings.Join(msgs, "; ")

		slog.Warn("config validation failed", "deploy_id", deployID, "errors", msgs)
		h.failDeploy(ctx, deployID, errMsg)
		return
	}

	configJSON, _ := json.Marshal(map[string]string{"raw": configRaw})
	h.pool.Exec(ctx,
		`UPDATE deployments SET config_snapshot = $1 WHERE id = $2`,
		configJSON, deployID,
	)

	h.emitter.Emit("deploy.validated", deployID.String(), projectName, "Config validation passed")
	slog.Info("config validation passed", "deploy_id", deployID, "project", projectName)
}

func (h *Handler) failDeploy(ctx context.Context, deployID uuid.UUID, message string) {
	_, err := h.pool.Exec(ctx,
		`UPDATE deployments SET status = $1, status_message = $2 WHERE id = $3`,
		models.StatusFailed, message, deployID,
	)

	h.emitter.Emit("deploy.failed", deployID.String(), "", message)

	if err != nil {
		slog.Error("failed to mark deployment as failed", "deploy_id", deployID, "error", err)
	}
}

func (h *Handler) publishBuild(ctx context.Context, deployID uuid.UUID, projectName, repoURL, commitSHA string) {
	tag, err := h.pool.Exec(ctx,
		`UPDATE deployments SET status = $1, build_started_at = NOW()
		 WHERE id = $2 AND status IN ($3, $4)`,
		models.StatusBuilding, deployID, models.StatusQueued, models.StatusValidating,
	)
	if err != nil {
		slog.Error("failed to transition to building", "deploy_id", deployID, "error", err)
		return
	}

	if tag.RowsAffected() == 0 {
		return
	}

	h.emitter.Emit("deploy.building", deployID.String(), projectName, "Build started")

	req := pubsub.BuildRequest{
		DeployID:    deployID.String(),
		ProjectName: projectName,
		RepoURL:     repoURL,
		CommitSHA:   commitSHA,
		ImageURI:    h.publisher.ImageURI(projectName, commitSHA),
	}

	if err := h.publisher.PublishBuildRequest(ctx, req); err != nil {
		slog.Error("failed to publish build request", "deploy_id", deployID, "error", err)
		h.pool.Exec(ctx,
			`UPDATE deployments SET status = $1, build_started_at = NULL WHERE id = $2`,
			models.StatusQueued, deployID,
		)
	}
}
