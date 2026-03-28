package api

import (
	stderrors "errors"
	"log/slog"
	"net/http"

	"duolok/bifrost/gateway/internal/errors"
	"duolok/bifrost/gateway/internal/models"
	"duolok/bifrost/gateway/pkg/apiutil"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

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

	// Return struct with webhook_secret included (json:"-" on model hides it otherwise).
	// This is the only time the secret is shown to the user.
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

	projectName, _ := h.fetchProjectMeta(c.Request.Context(), id)

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

	if h.deployer != nil && projectName != "" {
		if err := h.deployer.Delete(c.Request.Context(), projectName); err != nil {
			slog.Warn("failed to delete k8s resources", "project", projectName, "error", err)
		}
	}

	h.audit(c, auditProjectDeleted, resourceProject, id, nil)
	c.Status(http.StatusNoContent)
}
