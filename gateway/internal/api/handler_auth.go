package api

import (
	"fmt"
	"net/http"

	"duolok/bifrost/gateway/internal/auth"
	appErrors "duolok/bifrost/gateway/internal/errors"
	"duolok/bifrost/gateway/internal/middleware"
	"duolok/bifrost/gateway/internal/models"
	"duolok/bifrost/gateway/pkg/apiutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (h *Handler) Register(c *gin.Context) {
	var req models.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiutil.RespondError(c, appErrors.InvalidInput(err.Error()))
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		apiutil.RespondError(c, appErrors.Internal("failed to hash password", err))
		return
	}

	ctx := c.Request.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		apiutil.RespondError(c, appErrors.Internal("failed to start transaction", err))
		return
	}
	defer tx.Rollback(ctx)

	var userID uuid.UUID
	err = tx.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, name) VALUES ($1, $2, $3)
		 RETURNING id`,
		req.Email, hash, req.Name,
	).Scan(&userID)
	if err != nil {
		apiutil.RespondError(c, appErrors.AlreadyExists("user", req.Email))
		return
	}

	teamName := fmt.Sprintf("%s's team", req.Name)
	var teamID uuid.UUID
	err = tx.QueryRow(ctx,
		`INSERT INTO teams (name) VALUES ($1) RETURNING id`,
		teamName,
	).Scan(&teamID)
	if err != nil {
		apiutil.RespondError(c, appErrors.Internal("failed to create team", err))
		return
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, $3)`,
		teamID, userID, models.RoleAdmin,
	)

	if err != nil {
		apiutil.RespondError(c, appErrors.Internal("failed to add team member", err))
		return
	}

	if err := tx.Commit(ctx); err != nil {
		apiutil.RespondError(c, appErrors.Internal("failed to commit", err))
		return
	}

	token, err := auth.GenerateJWT(userID, teamID, string(models.RoleAdmin), h.jwtSecret)
	if err != nil {
		apiutil.RespondError(c, appErrors.Internal("failed to generate token", err))
		return
	}

	h.audit(c, auditUserRegister, "user", userID, gin.H{"email": req.Email})

	c.JSON(http.StatusCreated, gin.H{
		"token":   token,
		"user_id": userID,
		"team_id": teamID,
		"role":    models.RoleAdmin,
	})
}

func (h *Handler) Login(c *gin.Context) {
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiutil.RespondError(c, appErrors.InvalidInput(err.Error()))
		return
	}

	ctx := c.Request.Context()
	var user models.User
	err := h.pool.QueryRow(ctx,
		`SELECT id, email, password_hash, name, created_at, updated_at FROM users WHERE email = $1`,
		req.Email,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Name, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		apiutil.RespondError(c, appErrors.Unauthorized("invalid email or password"))
		return
	}

	if !auth.CheckPassword(user.PasswordHash, req.Password) {
		apiutil.RespondError(c, appErrors.Unauthorized("invalid email or password"))
		return
	}

	// Get the user's team and role
	var teamID uuid.UUID
	var role string
	err = h.pool.QueryRow(ctx,
		`SELECT team_id, role FROM team_members WHERE user_id = $1 LIMIT 1`,
		user.ID,
	).Scan(&teamID, &role)
	if err != nil {
		apiutil.RespondError(c, appErrors.Internal("user has no team membership", err))
		return
	}

	token, err := auth.GenerateJWT(user.ID, teamID, role, h.jwtSecret)
	if err != nil {
		apiutil.RespondError(c, appErrors.Internal("failed to generate token", err))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":   token,
		"user_id": user.ID,
		"team_id": teamID,
		"role":    role,
	})
}

func (h *Handler) GetMe(c *gin.Context) {
	userID := middleware.GetUserID(c)
	teamID := middleware.GetTeamID(c)

	ctx := c.Request.Context()
	var user models.User
	err := h.pool.QueryRow(ctx,
		`SELECT id, email, name, created_at, updated_at FROM users WHERE id = $1`,
		userID,
	).Scan(&user.ID, &user.Email, &user.Name, &user.CreatedAt, &user.UpdatedAt)
	if err == pgx.ErrNoRows {
		apiutil.RespondError(c, appErrors.NotFound("user", userID))
		return
	}
	if err != nil {
		apiutil.RespondError(c, appErrors.Internal("failed to fetch user", err))
		return
	}

	var team models.Team
	_ = h.pool.QueryRow(ctx,
		`SELECT id, name, created_at, updated_at FROM teams WHERE id = $1`,
		teamID,
	).Scan(&team.ID, &team.Name, &team.CreatedAt, &team.UpdatedAt)

	c.JSON(http.StatusOK, gin.H{
		"user": user,
		"team": team,
		"role": middleware.GetRole(c),
	})
}
