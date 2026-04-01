package api

import (
	"net/http"

	appErrors "duolok/bifrost/gateway/internal/errors"
	"duolok/bifrost/gateway/internal/middleware"
	"duolok/bifrost/gateway/internal/models"
	"duolok/bifrost/gateway/pkg/apiutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (h *Handler) GetTeam(c *gin.Context) {
	teamID := middleware.GetTeamID(c)

	var team models.Team
	err := h.pool.QueryRow(c.Request.Context(),
		`SELECT id, name, created_at, updated_at FROM teams WHERE id = $1`, teamID,
	).Scan(&team.ID, &team.Name, &team.CreatedAt, &team.UpdatedAt)
	if err != nil {
		apiutil.RespondError(c, appErrors.NotFound("team", teamID))
		return
	}
	c.JSON(http.StatusOK, team)
}

func (h *Handler) UpdateTeam(c *gin.Context) {
	teamID := middleware.GetTeamID(c)

	var req struct {
		Name string `json:"name" binding:"required,min=1,max=255"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		apiutil.RespondError(c, appErrors.InvalidInput(err.Error()))
		return
	}

	var team models.Team
	err := h.pool.QueryRow(c.Request.Context(),
		`UPDATE teams SET name = $1, updated_at = NOW() WHERE id = $2
		 RETURNING id, name, created_at, updated_at`,
		req.Name, teamID,
	).Scan(&team.ID, &team.Name, &team.CreatedAt, &team.UpdatedAt)
	if err != nil {
		apiutil.RespondError(c, appErrors.Internal("failed to update team", err))
		return
	}
	c.JSON(http.StatusOK, team)
}

func (h *Handler) ListTeamMembers(c *gin.Context) {
	teamID := middleware.GetTeamID(c)

	rows, err := h.pool.Query(c.Request.Context(),
		`SELECT tm.id, tm.team_id, tm.user_id, u.email, u.name, tm.role, tm.created_at
		 FROM team_members tm
		 JOIN users u ON u.id = tm.user_id
		 WHERE tm.team_id = $1
		 ORDER BY tm.created_at`,
		teamID,
	)
	if err != nil {
		apiutil.RespondError(c, appErrors.Internal("failed to list members", err))
		return
	}
	defer rows.Close()

	var members []models.TeamMember
	for rows.Next() {
		var m models.TeamMember
		if err := rows.Scan(&m.ID, &m.TeamID, &m.UserID, &m.Email, &m.Name, &m.Role, &m.CreatedAt); err != nil {
			apiutil.RespondError(c, appErrors.Internal("failed to scan member", err))
			return
		}
		members = append(members, m)
	}
	if members == nil {
		members = []models.TeamMember{}
	}

	c.JSON(http.StatusOK, members)
}

func (h *Handler) InviteTeamMember(c *gin.Context) {
	teamID := middleware.GetTeamID(c)

	var req struct {
		Email string `json:"email" binding:"required,email"`
		Role  string `json:"role" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		apiutil.RespondError(c, appErrors.InvalidInput(err.Error()))
		return
	}

	role := models.Role(req.Role)
	if role != models.RoleAdmin && role != models.RoleDeployer && role != models.RoleViewer {
		apiutil.RespondError(c, appErrors.InvalidInput("role must be admin, deployer, or viewer"))
		return
	}

	ctx := c.Request.Context()

	var userID uuid.UUID
	err := h.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE email = $1`, req.Email,
	).Scan(&userID)
	if err == pgx.ErrNoRows {
		apiutil.RespondError(c, appErrors.NotFound("user", req.Email))
		return
	}
	if err != nil {
		apiutil.RespondError(c, appErrors.Internal("failed to find user", err))
		return
	}

	var memberID uuid.UUID
	err = h.pool.QueryRow(ctx,
		`INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, $3)
		 RETURNING id`,
		teamID, userID, role,
	).Scan(&memberID)
	if err != nil {
		apiutil.RespondError(c, appErrors.AlreadyExists("team member", req.Email))
		return
	}

	h.audit(c, auditTeamInvite, "team", uuid.MustParse(teamID), gin.H{
		"invited_email": req.Email,
		"role":          req.Role,
	})

	c.JSON(http.StatusCreated, gin.H{
		"id":      memberID,
		"user_id": userID,
		"team_id": teamID,
		"role":    role,
	})
}

func (h *Handler) RemoveTeamMember(c *gin.Context) {
	teamID := middleware.GetTeamID(c)
	memberID, ok := apiutil.ParseID(c, "id", "team member")
	if !ok {
		return
	}

	tag, err := h.pool.Exec(c.Request.Context(),
		`DELETE FROM team_members WHERE id = $1 AND team_id = $2`,
		memberID, teamID,
	)
	if err != nil || tag.RowsAffected() == 0 {
		apiutil.RespondError(c, appErrors.NotFound("team member", memberID))
		return
	}

	h.audit(c, auditTeamRemove, "team", uuid.MustParse(teamID), gin.H{
		"member_id": memberID,
	})

	c.JSON(http.StatusOK, gin.H{"message": "member removed"})
}

func (h *Handler) UpdateMemberRole(c *gin.Context) {
	teamID := middleware.GetTeamID(c)
	memberID, ok := apiutil.ParseID(c, "id", "team member")
	if !ok {
		return
	}

	var req struct {
		Role string `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiutil.RespondError(c, appErrors.InvalidInput(err.Error()))
		return
	}

	role := models.Role(req.Role)
	if role != models.RoleAdmin && role != models.RoleDeployer && role != models.RoleViewer {
		apiutil.RespondError(c, appErrors.InvalidInput("role must be admin, deployer, or viewer"))
		return
	}

	tag, err := h.pool.Exec(c.Request.Context(),
		`UPDATE team_members SET role = $1 WHERE id = $2 AND team_id = $3`,
		role, memberID, teamID,
	)
	if err != nil || tag.RowsAffected() == 0 {
		apiutil.RespondError(c, appErrors.NotFound("team member", memberID))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "role updated", "role": role})
}
