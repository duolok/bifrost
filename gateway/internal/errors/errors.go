package errors

import (
	"fmt"
)

type Code string

const (
	ErrNotFound         Code = "NOT_FOUND"
	ErrAlreadyExists    Code = "ALREADY_EXISTS"
	ErrInvalidInput     Code = "INVALID_INPUT"
	ErrValidationFailed Code = "VALIDATION_FAILED"
	ErrBuildFailed      Code = "BUILD_FAILED"
	ErrDeployFailed     Code = "DEPLOY_FAILED"
	ErrUpstreamTimeout  Code = "UPSTREAM_TIMEOUT"
	ErrInternal         Code = "INTERNAL"
	ErrUnauthorized     Code = "UNAUTHORIZED"
	ErrForbidden        Code = "FORBIDDEN"
)

type AppError struct {
	Code    Code                   `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
	Cause   error                  `json:"-"`
}

func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error {
	return e.Cause
}

func NotFound(resource string, id interface{}) *AppError {
	return &AppError{
		Code:    ErrNotFound,
		Message: fmt.Sprintf("%s not found", resource),
		Details: map[string]interface{}{"resource": resource, "id": id},
	}
}

func AlreadyExists(resource, name string) *AppError {
	return &AppError{
		Code:    ErrAlreadyExists,
		Message: fmt.Sprintf("%s '%s' already exists", resource, name),
		Details: map[string]interface{}{"resource": resource, "name": name},
	}
}

func InvalidInput(message string) *AppError {
	return &AppError{
		Code:    ErrInvalidInput,
		Message: message,
	}
}

func Internal(message string, cause error) *AppError {
	return &AppError{
		Code:    ErrInternal,
		Message: message,
		Cause:   cause,
	}
}

func DeployFailed(message string, cause error) *AppError {
	return &AppError{
		Code:    ErrDeployFailed,
		Message: message,
		Cause:   cause,
	}
}

func Unauthorized(message string) *AppError {
	return &AppError{
		Code:    ErrUnauthorized,
		Message: message,
	}
}

func Forbidden(message string) *AppError {
	return &AppError{
		Code:    ErrForbidden,
		Message: message,
	}
}

func StatusCode(err *AppError) int {
	switch err.Code {
	case ErrNotFound:
		return 404
	case ErrAlreadyExists:
		return 409
	case ErrInvalidInput, ErrValidationFailed:
		return 400
	case ErrUnauthorized:
		return 401
	case ErrForbidden:
		return 403
	case ErrUpstreamTimeout:
		return 504
	default:
		return 500
	}
}
