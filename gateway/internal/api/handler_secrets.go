package api

import (
	stderrors "errors"
	"net/http"
	"time"

	"duolok/bifrost/gateway/internal/errors"
	"duolok/bifrost/gateway/pkg/apiutil"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
)

type secretRequest struct {
	KeyName   string `json:"key_name" binding:"required"`
	SecretRef string `json:"secret_ref" binding:"required"`
}

type secretResponse struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	KeyName   string    `json:"key_name"`
	SecretRef string    `json:"secret_ref"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *Handler) ListSecrets(c *gin.Context) {
	projectID, ok := apiutil.ParseID(c, "id", resourceProject)
	if !ok {
		return
	}

	rows, err := h.pool.Query(c.Request.Context(),
		`SELECT id, project_id, key_name, secret_ref, created_at
		 FROM project_secrets
		 WHERE project_id = $1
		 ORDER BY key_name`, projectID)
	if err != nil {
		apiutil.RespondError(c, errors.Internal("failed to list secrets", err))
		return
	}
	defer rows.Close()

	secrets := []secretResponse{}
	for rows.Next() {
		var s secretResponse
		if err := rows.Scan(&s.ID, &s.ProjectID, &s.KeyName, &s.SecretRef, &s.CreatedAt); err != nil {
			apiutil.RespondError(c, errors.Internal("failed to scan secret", err))
			return
		}
		secrets = append(secrets, s)
	}

	c.JSON(http.StatusOK, gin.H{"secrets": secrets})
}

func (h *Handler) SetSecret(c *gin.Context) {
	projectID, ok := apiutil.ParseID(c, "id", resourceProject)
	if !ok {
		return
	}

	var req secretRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiutil.RespondError(c, errors.InvalidInput(err.Error()))
		return
	}

	var s secretResponse
	err := h.pool.QueryRow(c.Request.Context(),
		`INSERT INTO project_secrets (project_id, key_name, secret_ref)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (project_id, key_name) DO UPDATE SET secret_ref = EXCLUDED.secret_ref
		 RETURNING id, project_id, key_name, secret_ref, created_at`,
		projectID, req.KeyName, req.SecretRef,
	).Scan(&s.ID, &s.ProjectID, &s.KeyName, &s.SecretRef, &s.CreatedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		if stderrors.As(err, &pgErr) && pgErr.Code == "23503" {
			apiutil.RespondError(c, errors.NotFound(resourceProject, projectID))
			return
		}
		apiutil.RespondError(c, errors.Internal("failed to set secret", err))
		return
	}

	h.audit(c, "secret.set", resourceProject, projectID, gin.H{"key": req.KeyName})
	c.JSON(http.StatusOK, s)
}

func (h *Handler) DeleteSecret(c *gin.Context) {
	projectID, ok := apiutil.ParseID(c, "id", resourceProject)
	if !ok {
		return
	}

	keyName := c.Param("key")
	if keyName == "" {
		apiutil.RespondError(c, errors.InvalidInput("key name is required"))
		return
	}

	tag, err := h.pool.Exec(c.Request.Context(),
		`DELETE FROM project_secrets WHERE project_id = $1 AND key_name = $2`,
		projectID, keyName)
	if err != nil {
		apiutil.RespondError(c, errors.Internal("failed to delete secret", err))
		return
	}

	if tag.RowsAffected() == 0 {
		apiutil.RespondError(c, errors.NotFound("secret", keyName))
		return
	}

	h.audit(c, "secret.deleted", resourceProject, projectID, gin.H{"key": keyName})
	c.Status(http.StatusNoContent)
}
