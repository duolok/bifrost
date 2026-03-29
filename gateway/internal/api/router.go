package api

import (
	"duolok/bifrost/gateway/internal/middleware"

	"github.com/gin-gonic/gin"
)

func NewRouter(deps HandlerDeps) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.RequestLogger())

	h := NewHandler(deps)

	r.GET("/health", h.CheckHealth)
	apiGroup := r.Group("/api/v1")
	{
		apiGroup.POST("/project", h.CreateProject)
		apiGroup.GET("/projects", h.ListProjects)
		apiGroup.GET("/project/:id", h.GetProject)
		apiGroup.DELETE("/project/:id", h.DeleteProject)

		apiGroup.POST("/projects/:id/deploy", h.TriggerDeploy)
		apiGroup.GET("/projects/:id/deployments", h.ListDeployments)
		apiGroup.GET("/deployments/:id", h.GetDeployment)
		apiGroup.POST("/deployments/:id/deploy", h.DeployBuilt)
		apiGroup.POST("/deployments/:id/retry", h.RetryDeploy)
		apiGroup.POST("/deployments/:id/health", h.ReportHealth)
		apiGroup.POST("/projects/:id/rollback", h.Rollback)

		apiGroup.GET("/projects/:id/secrets", h.ListSecrets)
		apiGroup.POST("/projects/:id/secrets", h.SetSecret)
		apiGroup.DELETE("/projects/:id/secrets/:key", h.DeleteSecret)

		apiGroup.POST("/webhook/github", h.HandleGitHubWebhook)
	}

	return r
}
