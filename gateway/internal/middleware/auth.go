package middleware

import (
	"net/http"
	"strings"

	"duolok/bifrost/gateway/internal/auth"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func authError(c *gin.Context, message string) {
	c.JSON(http.StatusUnauthorized, gin.H{
		"error":    gin.H{"code": "UNAUTHORIZED", "message": message},
		"trace_id": GetTraceID(c),
	})
	c.Abort()
}

func AuthRequired(jwtSecret []byte, pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			authError(c, "missing or invalid authorization header")
			return
		}
		token := strings.TrimPrefix(header, "Bearer ")

		claims, err := auth.ValidateJWT(token, jwtSecret)
		if err == nil {
			c.Set("user_id", claims.UserID.String())
			c.Set("team_id", claims.TeamID.String())
			c.Set("role", claims.Role)
			c.Next()
			return
		}

		keyHash := auth.HashAPIKey(token)
		var userID, teamID string
		row := pool.QueryRow(c.Request.Context(),
			`SELECT ak.user_id, ak.team_id, tm.role
			 FROM api_keys ak
			 JOIN team_members tm ON tm.user_id = ak.user_id AND tm.team_id = ak.team_id
			 WHERE ak.key_hash = $1
			   AND (ak.expires_at IS NULL OR ak.expires_at > NOW())`,
			keyHash,
		)
		var role string
		if err := row.Scan(&userID, &teamID, &role); err != nil {
			authError(c, "invalid or expired token")
			return
		}

		c.Set("user_id", userID)
		c.Set("team_id", teamID)
		c.Set("role", role)
		c.Next()
	}
}

func GetUserID(c *gin.Context) string {
	return c.GetString("user_id")
}

func GetTeamID(c *gin.Context) string {
	return c.GetString("team_id")
}

func GetRole(c *gin.Context) string {
	return c.GetString("role")
}

func RequireRole(minRole string) gin.HandlerFunc {
	order := map[string]int{"viewer": 0, "deployer": 1, "admin": 2}
	return func(c *gin.Context) {
		role := c.GetString("role")
		if order[role] < order[minRole] {
			c.JSON(http.StatusForbidden, gin.H{
				"code":     "FORBIDDEN",
				"message":  "insufficient permissions, requires " + minRole + " role",
				"trace_id": GetTraceID(c),
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
