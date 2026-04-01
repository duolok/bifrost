package api

import (
	"duolok/bifrost/gateway/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewRouter(deps HandlerDeps, jwtSecret []byte, pool *pgxpool.Pool) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.RequestLogger())

	h := NewHandler(deps)

	r.GET("/health", h.CheckHealth)
	authGroup := r.Group("/api/v1/auth")
	{
		authGroup.POST("/register", h.Register)
		authGroup.POST("/login", h.Login)
	}

	r.POST("/api/v1/webhook/github", h.HandleGitHubWebhook)

	apiGroup := r.Group("/api/v1")
	apiGroup.Use(middleware.AuthRequired(jwtSecret, pool))
	{
		apiGroup.GET("/auth/me", h.GetMe)

		// Team management
		apiGroup.GET("/team", h.GetTeam)
		apiGroup.PUT("/team", middleware.RequireRole("admin"), h.UpdateTeam)
		apiGroup.GET("/team/members", h.ListTeamMembers)
		apiGroup.POST("/team/invite", middleware.RequireRole("admin"), h.InviteTeamMember)
		apiGroup.DELETE("/team/members/:id", middleware.RequireRole("admin"), h.RemoveTeamMember)
		apiGroup.PUT("/team/members/:id/role", middleware.RequireRole("admin"), h.UpdateMemberRole)

		// API keys
		apiGroup.GET("/api-keys", h.ListAPIKeys)
		apiGroup.POST("/api-keys", middleware.RequireRole("deployer"), h.CreateAPIKey)
		apiGroup.DELETE("/api-keys/:id", middleware.RequireRole("deployer"), h.DeleteAPIKey)

		// Projects (scoped to team)
		apiGroup.POST("/project", middleware.RequireRole("deployer"), h.CreateProject)
		apiGroup.GET("/projects", h.ListProjects)
		apiGroup.GET("/project/:id", h.GetProject)
		apiGroup.DELETE("/project/:id", middleware.RequireRole("admin"), h.DeleteProject)

		// Deployments
		apiGroup.POST("/projects/:id/deploy", middleware.RequireRole("deployer"), h.TriggerDeploy)
		apiGroup.GET("/projects/:id/deployments", h.ListDeployments)
		apiGroup.GET("/deployments/:id", h.GetDeployment)
		apiGroup.POST("/deployments/:id/deploy", middleware.RequireRole("deployer"), h.DeployBuilt)
		apiGroup.POST("/deployments/:id/retry", middleware.RequireRole("deployer"), h.RetryDeploy)
		apiGroup.POST("/deployments/:id/health", h.ReportHealth)
		apiGroup.POST("/projects/:id/rollback", middleware.RequireRole("deployer"), h.Rollback)

		// Secrets
		apiGroup.GET("/projects/:id/secrets", middleware.RequireRole("deployer"), h.ListSecrets)
		apiGroup.POST("/projects/:id/secrets", middleware.RequireRole("deployer"), h.SetSecret)
		apiGroup.DELETE("/projects/:id/secrets/:key", middleware.RequireRole("admin"), h.DeleteSecret)
	}

	return r
}
