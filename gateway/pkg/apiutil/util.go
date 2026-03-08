package apiutil

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"

	"duolok/bifrost/gateway/internal/errors"
	"duolok/bifrost/gateway/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func RespondError(c *gin.Context, err *errors.AppError) {
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

func GenerateSecret(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	return hex.EncodeToString(bytes), nil
}

func NilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ParseID extracts a UUID path param, returning an error response on failure.
// Returns the parsed UUID and true on success, or zero UUID and false on failure.
func ParseID(c *gin.Context, param string, resource string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(param))
	if err != nil {
		RespondError(c, errors.InvalidInput("invalid "+resource+" ID"))
		return uuid.UUID{}, false
	}
	return id, true
}
