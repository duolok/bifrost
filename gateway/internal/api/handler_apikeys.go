package api

import (
	"net/http"
	"time"

	"duolok/bifrost/gateway/internal/auth"
	appErrors "duolok/bifrost/gateway/internal/errors"
	"duolok/bifrost/gateway/internal/middleware"
	"duolok/bifrost/gateway/pkg/apiutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (h *Handler) ListAPIKeys(c *gin.Context) {
	userID := middleware.GetUserID(c)

	rows, err := h.pool.Query(c.Request.Context(),
		`SELECT id, name, scopes, expires_at, created_at
		 FROM api_keys WHERE user_id = $1 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		apiutil.RespondError(c, appErrors.Internal("failed to list API keys", err))
		return
	}
	defer rows.Close()

	type keyResponse struct {
		ID        uuid.UUID  `json:"id"`
		Name      string     `json:"name"`
		Scopes    []string   `json:"scopes"`
		ExpiresAt *time.Time `json:"expires_at,omitempty"`
		CreatedAt time.Time  `json:"created_at"`
	}

	var keys []keyResponse
	for rows.Next() {
		var k keyResponse
		if err := rows.Scan(&k.ID, &k.Name, &k.Scopes, &k.ExpiresAt, &k.CreatedAt); err != nil {
			apiutil.RespondError(c, appErrors.Internal("failed to scan key", err))
			return
		}
		keys = append(keys, k)
	}
	if keys == nil {
		keys = []keyResponse{}
	}
	c.JSON(http.StatusOK, keys)
}

func (h *Handler) CreateAPIKey(c *gin.Context) {
	userID := middleware.GetUserID(c)
	teamID := middleware.GetTeamID(c)

	var req struct {
		Name string `json:"name" binding:"required,min=1,max=255"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiutil.RespondError(c, appErrors.InvalidInput(err.Error()))
		return
	}

	plaintext, keyHash, err := auth.GenerateAPIKey()
	if err != nil {
		apiutil.RespondError(c, appErrors.Internal("failed to generate key", err))
		return
	}

	var keyID uuid.UUID
	err = h.pool.QueryRow(c.Request.Context(),
		`INSERT INTO api_keys (user_id, team_id, key_hash, name) VALUES ($1, $2, $3, $4)
		 RETURNING id`,
		userID, teamID, keyHash, req.Name,
	).Scan(&keyID)
	if err != nil {
		apiutil.RespondError(c, appErrors.Internal("failed to create API key", err))
		return
	}

	h.audit(c, auditAPIKeyCreated, "api_key", keyID, gin.H{"name": req.Name})

	// Return plaintext key only once — it's never stored
	c.JSON(http.StatusCreated, gin.H{
		"id":   keyID,
		"name": req.Name,
		"key":  plaintext,
	})
}

func (h *Handler) DeleteAPIKey(c *gin.Context) {
	userID := middleware.GetUserID(c)
	keyID, ok := apiutil.ParseID(c, "id", "API key")
	if !ok {
		return
	}

	tag, err := h.pool.Exec(c.Request.Context(),
		`DELETE FROM api_keys WHERE id = $1 AND user_id = $2`,
		keyID, userID,
	)
	if err != nil || tag.RowsAffected() == 0 {
		apiutil.RespondError(c, appErrors.NotFound("API key", keyID))
		return
	}

	h.audit(c, auditAPIKeyRevoked, "api_key", keyID, nil)

	c.JSON(http.StatusOK, gin.H{"message": "API key revoked"})
}
