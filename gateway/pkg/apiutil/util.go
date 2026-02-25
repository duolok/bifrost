package apiutil

import (
	"crypto/rand"
	"duolok/bifrost/gateway/internal/errors"
	"duolok/bifrost/gateway/internal/middleware"
	"encoding/hex"
	"log/slog"

	"github.com/gin-gonic/gin"
)

func respondError(c *gin.Context, err *errors.AppError) {
	traceID := middleware.GetTraceID(c)

	if err.Cause != nil {
		slog.Error(err.Message,
			"trace_id", traceID,
			"code", err.Code,
			"cause", err.Cause.Error(),
		)
	}

	c.JSON(errors.StatusCode(err), gin.H{
		"error":    err,
		"trace_id": traceID,
	})

}

func generateSecret(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	return hex.EncodeToString(bytes), nil
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
