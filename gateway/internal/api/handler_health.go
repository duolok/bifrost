package api

import (
	"net/http"
	"time"

	"duolok/bifrost/gateway/internal/errors"
	"duolok/bifrost/gateway/internal/notify"
	"duolok/bifrost/gateway/internal/rules"
	"duolok/bifrost/gateway/pkg/apiutil"

	"github.com/gin-gonic/gin"
)

func (h *Handler) ReportHealth(c *gin.Context) {
	deployID, ok := apiutil.ParseID(c, "id", resourceDeployment)
	if !ok {
		return
	}

	var body struct {
		Healthy         bool    `json:"healthy"`
		ResponseTimeMs  int     `json:"response_time_ms"`
		MemoryUsedKB    int64   `json:"memory_used_kb"`
		MemoryTotalKB   int64   `json:"memory_total_kb"`
		CpuUsagePercent float64 `json:"cpu_usage_percent"`
		OpenFds         int     `json:"open_fds"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		apiutil.RespondError(c, errors.InvalidInput(err.Error()))
		return
	}

	status := "healthy"
	if !body.Healthy {
		status = "unhealthy"
	}

	memPercent := float64(0)
	if body.MemoryTotalKB > 0 {
		memPercent = float64(body.MemoryUsedKB) / float64(body.MemoryTotalKB) * 100
	}

	var unhealthyCount int
	h.pool.QueryRow(c.Request.Context(),
		`SELECT COUNT(*) FROM (
			SELECT status FROM health_checks
			WHERE deployment_id = $1
			ORDER BY checked_at DESC LIMIT 10
		) sub WHERE status = 'unhealthy'`,
		deployID,
	).Scan(&unhealthyCount)

	_, err := h.pool.Exec(c.Request.Context(),
		`INSERT INTO health_checks (deployment_id, status, response_time_ms, cpu_percent, memory_bytes, memory_percent, fd_count)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		deployID, status, body.ResponseTimeMs, body.CpuUsagePercent, body.MemoryUsedKB*1024, memPercent, body.OpenFds,
	)
	if err != nil {
		apiutil.RespondError(c, errors.Internal("failed to store health check", err))
		return
	}

	if h.rules != nil {
		var projectName string
		h.pool.QueryRow(c.Request.Context(),
			`SELECT p.name FROM projects p JOIN deployments d ON d.project_id = p.id WHERE d.id = $1`,
			deployID,
		).Scan(&projectName)

		event := rules.HealthEvent{
			DeployID:       deployID.String(),
			Project:        projectName,
			Healthy:        body.Healthy,
			ResponseTimeMs: body.ResponseTimeMs,
			CpuPercent:     body.CpuUsagePercent,
			MemoryPercent:  memPercent,
			UnhealthyCount: unhealthyCount,
		}

		actions := h.rules.Evaluate(event, defaultRulesScript)
		for _, action := range actions {
			if action.Type == "notify" && h.notifier != nil {
				h.notifier.PublishEmail(notify.EmailMessage{
					To:       action.To,
					Subject:  "[Bifrost] " + action.Severity + ": " + projectName,
					Body:     action.Message,
					Severity: action.Severity,
					DeployID: deployID.String(),
					Project:  projectName,
				})
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
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
