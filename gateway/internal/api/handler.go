package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	stderrors "errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"duolok/bifrost/gateway/internal/errors"
	"duolok/bifrost/gateway/internal/k8s"
	"duolok/bifrost/gateway/internal/models"
	"duolok/bifrost/gateway/internal/pubsub"
	"duolok/bifrost/gateway/internal/validator"
	"duolok/bifrost/gateway/pkg/apiutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	pool      *pgxpool.Pool
	deployer  *k8s.Deployer
	publisher *pubsub.Publisher
	validator *validator.Client
	startAt   time.Time
}

func NewHandler(pool *pgxpool.Pool, deployer *k8s.Deployer, publisher *pubsub.Publisher, validator *validator.Client) *Handler {
	return &Handler{
		pool:      pool,
		deployer:  deployer,
		publisher: publisher,
		validator: validator,
		startAt:   time.Now(),
	}
}

func (h *Handler) CreateProject(c *gin.Context) {
	var req models.CreateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiutil.RespondError(c, errors.InvalidInput(err.Error()))
		return
	}

	if req.DefaultBranch == "" {
		req.DefaultBranch = "main"
	}

	secret, err := apiutil.GenerateSecret(32)
	if err != nil {
		apiutil.RespondError(c, errors.Internal("failed to generate webhook secret", err))
		return
	}

	var p models.Project
	err = h.pool.QueryRow(c.Request.Context(),
		`INSERT INTO projects (name, repo_url, default_branch, webhook_secret)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, name, repo_url, default_branch, webhook_secret, config, status, created_at, updated_at`,
		req.Name, req.RepoURL, req.DefaultBranch, secret,
	).Scan(&p.ID, &p.Name, &p.RepoURL, &p.DefaultBranch, &p.WebhookSecret, &p.Config, &p.Status, &p.CreatedAt, &p.UpdatedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		if stderrors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			apiutil.RespondError(c, errors.AlreadyExists(resourceProject, req.Name))
			return
		}
		apiutil.RespondError(c, errors.Internal("failed to create project", err))
		return
	}

	h.audit(c, auditProjectCreated, resourceProject, p.ID, gin.H{"name": p.Name})

	// Include webhook_secret in creation response only (it's json:"-" on the model)
	c.JSON(http.StatusCreated, gin.H{
		"id":             p.ID,
		"name":           p.Name,
		"repo_url":       p.RepoURL,
		"default_branch": p.DefaultBranch,
		"webhook_secret": p.WebhookSecret,
		"config":         p.Config,
		"status":         p.Status,
		"created_at":     p.CreatedAt,
		"updated_at":     p.UpdatedAt,
	})
}

func (h *Handler) ListProjects(c *gin.Context) {
	rows, err := h.pool.Query(c.Request.Context(),
		`SELECT id, name, repo_url, default_branch, config, status, created_at, updated_at
		 FROM projects
		 WHERE status != $1
		 ORDER BY created_at DESC`, models.ProjectArchived)
	if err != nil {
		apiutil.RespondError(c, errors.Internal("failed to list projects", err))
		return
	}
	defer rows.Close()

	projects := []models.Project{}
	for rows.Next() {
		var p models.Project
		if err := rows.Scan(&p.ID, &p.Name, &p.RepoURL, &p.DefaultBranch, &p.Config, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
			apiutil.RespondError(c, errors.Internal("failed to scan project", err))
			return
		}
		projects = append(projects, p)
	}

	c.JSON(http.StatusOK, gin.H{"projects": projects})
}

func (h *Handler) GetProject(c *gin.Context) {
	id, ok := apiutil.ParseID(c, "id", resourceProject)
	if !ok {
		return
	}

	var p models.Project
	err := h.pool.QueryRow(c.Request.Context(),
		`SELECT id, name, repo_url, default_branch, config, status, created_at, updated_at
		 FROM projects
		 WHERE id = $1`, id,
	).Scan(&p.ID, &p.Name, &p.RepoURL, &p.DefaultBranch, &p.Config, &p.Status, &p.CreatedAt, &p.UpdatedAt)

	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			apiutil.RespondError(c, errors.NotFound(resourceProject, id))
			return
		}
		apiutil.RespondError(c, errors.Internal("failed to get project", err))
		return
	}

	c.JSON(http.StatusOK, p)
}

func (h *Handler) DeleteProject(c *gin.Context) {
	id, ok := apiutil.ParseID(c, "id", resourceProject)
	if !ok {
		return
	}

	tag, err := h.pool.Exec(c.Request.Context(),
		`UPDATE projects SET status = $1, updated_at = NOW()
		 WHERE id = $2 AND status != $1`, models.ProjectArchived, id)
	if err != nil {
		apiutil.RespondError(c, errors.Internal("failed to delete project", err))
		return
	}

	if tag.RowsAffected() == 0 {
		apiutil.RespondError(c, errors.NotFound(resourceProject, id))
		return
	}

	h.audit(c, auditProjectDeleted, resourceProject, id, nil)
	c.Status(http.StatusNoContent)
}

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

	// Verify project exists and is active
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

	// Fetch project details for validation and build
	var projectName, repoURL string
	_ = h.pool.QueryRow(c.Request.Context(),
		`SELECT name, repo_url FROM projects WHERE id = $1`, projectID,
	).Scan(&projectName, &repoURL)

	h.validateAndBuild(c, d.ID, projectName, repoURL, req.CommitSHA)

	c.JSON(http.StatusCreated, d)
}

func (h *Handler) ListDeployments(c *gin.Context) {
	projectID, ok := apiutil.ParseID(c, "id", resourceProject)
	if !ok {
		return
	}

	rows, err := h.pool.Query(c.Request.Context(),
		`SELECT id, project_id, commit_sha, branch, triggered_by, image_uri,
		        status, status_message, config_snapshot,
		        build_started_at, build_finished_at, deploy_started_at, deploy_finished_at, created_at
		 FROM deployments
		 WHERE project_id = $1
		 ORDER BY created_at DESC`, projectID)
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

	var d models.Deployment
	err := h.pool.QueryRow(c.Request.Context(),
		`SELECT id, project_id, commit_sha, branch, triggered_by, image_uri,
		        status, status_message, config_snapshot,
		        build_started_at, build_finished_at, deploy_started_at, deploy_finished_at, created_at
		 FROM deployments
		 WHERE id = $1`, id,
	).Scan(
		&d.ID, &d.ProjectID, &d.CommitSHA, &d.Branch, &d.TriggeredBy, &d.ImageURI,
		&d.Status, &d.StatusMessage, &d.ConfigSnapshot,
		&d.BuildStartedAt, &d.BuildFinishedAt, &d.DeployStartedAt, &d.DeployFinishedAt, &d.CreatedAt,
	)

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

	// Fetch the deployment
	var d models.Deployment
	err := h.pool.QueryRow(c.Request.Context(),
		`SELECT id, project_id, commit_sha, branch, triggered_by, image_uri,
		        status, status_message, config_snapshot,
		        build_started_at, build_finished_at, deploy_started_at, deploy_finished_at, created_at
		 FROM deployments
		 WHERE id = $1`, id,
	).Scan(
		&d.ID, &d.ProjectID, &d.CommitSHA, &d.Branch, &d.TriggeredBy, &d.ImageURI,
		&d.Status, &d.StatusMessage, &d.ConfigSnapshot,
		&d.BuildStartedAt, &d.BuildFinishedAt, &d.DeployStartedAt, &d.DeployFinishedAt, &d.CreatedAt,
	)
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

	h.audit(c, auditDeployComplete, resourceDeployment, d.ID, gin.H{
		"project": p.Name,
	})

	h.pool.QueryRow(c.Request.Context(),
		`SELECT id, project_id, commit_sha, branch, triggered_by, image_uri,
		        status, status_message, config_snapshot,
		        build_started_at, build_finished_at, deploy_started_at, deploy_finished_at, created_at
		 FROM deployments
		 WHERE id = $1`, id,
	).Scan(
		&d.ID, &d.ProjectID, &d.CommitSHA, &d.Branch, &d.TriggeredBy, &d.ImageURI,
		&d.Status, &d.StatusMessage, &d.ConfigSnapshot,
		&d.BuildStartedAt, &d.BuildFinishedAt, &d.DeployStartedAt, &d.DeployFinishedAt, &d.CreatedAt,
	)

	c.JSON(http.StatusOK, d)
}

type gitHubPushEvent struct {
	Ref        string `json:"ref"`
	After      string `json:"after"`
	Repository struct {
		CloneURL string `json:"clone_url"`
		HTMLURL  string `json:"html_url"`
	} `json:"repository"`
	Pusher struct {
		Name string `json:"name"`
	} `json:"pusher"`
}

func (h *Handler) HandleGitHubWebhook(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		apiutil.RespondError(c, errors.InvalidInput("failed to read request body"))
		return
	}

	var event gitHubPushEvent
	if err := json.Unmarshal(body, &event); err != nil {
		apiutil.RespondError(c, errors.InvalidInput("invalid JSON payload"))
		return
	}

	repoURL := event.Repository.CloneURL
	var p models.Project
	err = h.pool.QueryRow(c.Request.Context(),
		`SELECT id, name, repo_url, default_branch, webhook_secret
		 FROM projects
		 WHERE (repo_url = $1 OR repo_url = $2) AND status = $3`,
		repoURL, strings.TrimSuffix(repoURL, ".git"), models.ProjectActive,
	).Scan(&p.ID, &p.Name, &p.RepoURL, &p.DefaultBranch, &p.WebhookSecret)

	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			apiutil.RespondError(c, errors.NotFound(resourceProject, repoURL))
			return
		}
		apiutil.RespondError(c, errors.Internal("failed to look up project", err))
		return
	}

	// Verify HMAC-SHA256 signature
	signature := c.GetHeader("X-Hub-Signature-256")
	if !verifyHMAC(p.WebhookSecret, body, signature) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
		return
	}

	branch := strings.TrimPrefix(event.Ref, "refs/heads/")
	commitSHA := event.After
	triggeredBy := triggerGitHubWebhook
	if event.Pusher.Name != "" {
		triggeredBy = triggerGitHubWebhook + ":" + event.Pusher.Name
	}

	var d models.Deployment
	err = h.pool.QueryRow(c.Request.Context(),
		`INSERT INTO deployments (project_id, commit_sha, branch, triggered_by, status)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (project_id, commit_sha) DO NOTHING
		 RETURNING id, project_id, commit_sha, branch, triggered_by, status, created_at`,
		p.ID, commitSHA, branch, triggeredBy, models.StatusQueued,
	).Scan(&d.ID, &d.ProjectID, &d.CommitSHA, &d.Branch, &d.TriggeredBy, &d.Status, &d.CreatedAt)

	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusOK, gin.H{"status": "already_processed", "commit_sha": commitSHA})
			return
		}
		apiutil.RespondError(c, errors.Internal("failed to create deployment", err))
		return
	}

	h.audit(c, auditWebhookReceived, resourceProject, p.ID, gin.H{
		"commit_sha": commitSHA,
		"branch":     branch,
	})

	h.validateAndBuild(c, d.ID, p.Name, p.RepoURL, commitSHA)

	c.JSON(http.StatusOK, gin.H{
		"status":        "accepted",
		"deployment_id": d.ID,
	})
}

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
	// Transition queued → validating
	_, err := h.pool.Exec(ctx,
		`UPDATE deployments SET status = $1 WHERE id = $2 AND status = $3`,
		models.StatusValidating, deployID, models.StatusQueued,
	)
	if err != nil {
		slog.Error("failed to transition to validating", "deploy_id", deployID, "error", err)
		return
	}

	slog.Info("validating deployment config", "deploy_id", deployID, "project", projectName)

	// Fetch deploy.toml from repo
	configRaw, err := validator.FetchDeployToml(ctx, repoURL, commitSHA)
	if err != nil {
		slog.Error("failed to fetch deploy.toml", "deploy_id", deployID, "error", err)
		h.failDeploy(ctx, deployID, "failed to fetch deploy.toml: "+err.Error())
		return
	}

	// Call the OCaml validator
	result, err := h.validator.Validate(ctx, configRaw)
	if err != nil {
		slog.Error("validator call failed", "deploy_id", deployID, "error", err)
		h.failDeploy(ctx, deployID, "validator unavailable: "+err.Error())
		return
	}

	if result.Status != "valid" {
		// Build a readable error message from validation errors
		msgs := make([]string, len(result.Errors))
		for i, e := range result.Errors {
			msgs[i] = e.Field + ": " + e.Message
		}
		errMsg := "config validation failed: " + strings.Join(msgs, "; ")

		slog.Warn("config validation failed", "deploy_id", deployID, "errors", msgs)
		h.failDeploy(ctx, deployID, errMsg)
		return
	}

	// Store the raw config as config_snapshot on the deployment
	configJSON, _ := json.Marshal(map[string]string{"raw": configRaw})
	h.pool.Exec(ctx,
		`UPDATE deployments SET config_snapshot = $1 WHERE id = $2`,
		configJSON, deployID,
	)

	slog.Info("config validation passed", "deploy_id", deployID, "project", projectName)
}

func (h *Handler) failDeploy(ctx context.Context, deployID uuid.UUID, message string) {
	_, err := h.pool.Exec(ctx,
		`UPDATE deployments SET status = $1, status_message = $2 WHERE id = $3`,
		models.StatusFailed, message, deployID,
	)
	if err != nil {
		slog.Error("failed to mark deployment as failed", "deploy_id", deployID, "error", err)
	}
}

func (h *Handler) publishBuild(ctx context.Context, deployID uuid.UUID, projectName, repoURL, commitSHA string) {
	// Accept transition from both queued (no validator) and validating (with validator)
	tag, err := h.pool.Exec(ctx,
		`UPDATE deployments SET status = $1, build_started_at = NOW()
		 WHERE id = $2 AND status IN ($3, $4)`,
		models.StatusBuilding, deployID, models.StatusQueued, models.StatusValidating,
	)
	if err != nil {
		slog.Error("failed to transition to building", "deploy_id", deployID, "error", err)
		return
	}

	// If no rows updated, deployment was already failed (e.g. validation failed)
	if tag.RowsAffected() == 0 {
		return
	}

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

func (h *Handler) CheckHealth(c *gin.Context) {
	health := gin.H{
		"status":  "ok",
		"service": "bifrost-gateway",
		"uptime":  time.Since(h.startAt).String(),
	}

	var dbOK bool
	if err := h.pool.QueryRow(c.Request.Context(), "SELECT true").Scan(&dbOK); err != nil {
		health["status"] = "degraded"
		health["database"] = "unavailable"
		c.JSON(http.StatusServiceUnavailable, health)
		return
	}

	stats := h.pool.Stat()
	health["database"] = gin.H{
		"status":      "connected",
		"total_conns": stats.TotalConns(),
		"idle_conns":  stats.IdleConns(),
	}

	c.JSON(http.StatusOK, health)
}

func (h *Handler) audit(c *gin.Context, action, resourceType string, resourceID uuid.UUID, details gin.H) {
	go func() {
		_, err := h.pool.Exec(context.Background(),
			`INSERT INTO audit_log (actor, action, resource_type, resource_id, details)
			 VALUES ($1, $2, $3, $4, $5)`,
			"system", action, resourceType, resourceID, details,
		)
		if err != nil {
			slog.Error("failed to write audit log", "action", action, "error", err)
		}
	}()
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

func verifyHMAC(secret string, body []byte, signatureHeader string) bool {
	if !strings.HasPrefix(signatureHeader, "sha256=") {
		return false
	}

	sigHex := strings.TrimPrefix(signatureHeader, "sha256=")
	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := mac.Sum(nil)
	return hmac.Equal(sigBytes, expected)
}
