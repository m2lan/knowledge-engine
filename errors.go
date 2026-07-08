package knowledge

import (
	"errors"
	"fmt"
	"net/http"
)

// Sentinel errors for common cases.
var (
	ErrNotFound         = &AppError{Code: http.StatusNotFound, Message: "resource not found"}
	ErrUnauthorized     = &AppError{Code: http.StatusUnauthorized, Message: "unauthorized"}
	ErrForbidden        = &AppError{Code: http.StatusForbidden, Message: "forbidden"}
	ErrValidation       = &AppError{Code: http.StatusBadRequest, Message: "validation failed"}
	ErrConflict         = &AppError{Code: http.StatusConflict, Message: "resource conflict"}
	ErrInternal         = &AppError{Code: http.StatusInternalServerError, Message: "internal error"}
	ErrTooManyRequests  = &AppError{Code: http.StatusTooManyRequests, Message: "too many requests"}
	ErrServiceUnavail   = &AppError{Code: http.StatusServiceUnavailable, Message: "service unavailable"}
)

// AppError is a structured application error with an HTTP status code.
type AppError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
	Err     error  `json:"-"`
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	if e.Detail != "" {
		return fmt.Sprintf("%s: %s", e.Message, e.Detail)
	}
	return e.Message
}

func (e *AppError) Unwrap() error {
	return e.Err
}

// New creates a new AppError.
func NewAppError(code int, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

// Wrap wraps an existing error with an AppError.
func Wrap(err error, message string) *AppError {
	return &AppError{Code: http.StatusInternalServerError, Message: message, Err: err}
}

// WrapWithCode wraps an error with a specific HTTP code.
func WrapWithCode(err error, code int, message string) *AppError {
	return &AppError{Code: code, Message: message, Err: err}
}

// NotFound creates a "not found" error for a resource.
func NotFound(resource string) *AppError {
	return &AppError{
		Code:    http.StatusNotFound,
		Message: fmt.Sprintf("%s not found", resource),
	}
}

// Validation creates a validation error.
func Validation(detail string) *AppError {
	return &AppError{
		Code:    http.StatusBadRequest,
		Message: "validation failed",
		Detail:  detail,
	}
}

// Conflict creates a conflict error.
func Conflict(detail string) *AppError {
	return &AppError{
		Code:    http.StatusConflict,
		Message: "resource conflict",
		Detail:  detail,
	}
}

// Code extracts the HTTP status code from an error.
// Returns 500 for unknown errors.
func Code(err error) int {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return http.StatusInternalServerError
}
