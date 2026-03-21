package api

import (
	"duolok/bifrost/gateway/internal/k8s"
	"duolok/bifrost/gateway/internal/middleware"
	"duolok/bifrost/gateway/internal/pubsub"
	"duolok/bifrost/gateway/internal/validator"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewRouter(pool *pgxpool.Pool, deployer *k8s.Deployer, publisher *pubsub.Publisher, validator *validator.Client) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.RequestLogger())

	h := NewHandler(pool, deployer, publisher, validator)

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

		apiGroup.POST("/webhook/github", h.HandleGitHubWebhook)
	}

	return r
}
