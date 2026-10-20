package domain

import (
	"errors"
	"fmt"
	"net/http"
)

// ErrorCode is the stable, machine-readable error identifier returned to API
// clients and to the model as tool error codes.
type ErrorCode string

const (
	CodeBadRequest          ErrorCode = "bad_request"
	CodeValidation          ErrorCode = "validation_error"
	CodeUnauthorized        ErrorCode = "unauthorized"
	CodeForbidden           ErrorCode = "forbidden"
	CodeNotFound            ErrorCode = "not_found"
	CodeConflict            ErrorCode = "conflict"
	CodeRateLimited         ErrorCode = "rate_limited"
	CodeConfirmationNeeded  ErrorCode = "confirmation_required"
	CodeActionExpired       ErrorCode = "action_expired"
	CodeLLMUnavailable      ErrorCode = "llm_unavailable"
	CodeToolExecutionFailed ErrorCode = "tool_execution_failed"
	CodeInternal            ErrorCode = "internal_error"
)

// HTTPStatus maps a stable error code onto the HTTP status defined in the spec.
func HTTPStatus(code ErrorCode) int {
	switch code {
	case CodeBadRequest, CodeValidation:
		return http.StatusBadRequest
	case CodeUnauthorized:
		return http.StatusUnauthorized
	case CodeForbidden:
		return http.StatusForbidden
	case CodeNotFound:
		return http.StatusNotFound
	case CodeConflict, CodeConfirmationNeeded:
		return http.StatusConflict
	case CodeRateLimited:
		return http.StatusTooManyRequests
	case CodeActionExpired:
		return http.StatusGone
	case CodeLLMUnavailable:
		return http.StatusServiceUnavailable
	case CodeToolExecutionFailed:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}

// AppError is the only error shape crossing the service -> HTTP boundary.
// It never carries SQL text, stack traces or secrets in Message.
type AppError struct {
	Code    ErrorCode      `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
	// Err keeps the internal cause for logs; it is never serialized.
	Err error `json:"-"`
}

func (e *AppError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error { return e.Err }

// WithDetail returns a copy carrying one extra debug-safe detail.
func (e *AppError) WithDetail(key string, value any) *AppError {
	cp := *e
	cp.Details = map[string]any{}
	for k, v := range e.Details {
		cp.Details[k] = v
	}
	cp.Details[key] = value
	return &cp
}

// Wrap attaches an internal cause for logging purposes.
func (e *AppError) Wrap(err error) *AppError {
	cp := *e
	cp.Err = err
	return &cp
}

// NewError builds a new AppError.
func NewError(code ErrorCode, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

// Convenience constructors for the most common business failures.
func ValidationError(message string) *AppError {
	return NewError(CodeValidation, message)
}

func NotFoundError(message string) *AppError {
	return NewError(CodeNotFound, message)
}

func ForbiddenError(message string) *AppError {
	return NewError(CodeForbidden, message)
}

func ConflictError(message string) *AppError {
	return NewError(CodeConflict, message)
}

// Repository-level sentinel errors. Repositories translate driver errors into
// these; services translate them into AppError with user-safe messages.
var (
	ErrNotFound = errors.New("domain: resource not found")
	ErrConflict = errors.New("domain: resource conflict")
	// ErrVersionConflict signals an optimistic-locking mismatch.
	ErrVersionConflict = errors.New("domain: version conflict")
)

// AsAppError extracts an *AppError, converting unknown errors to internal_error.
func AsAppError(err error) *AppError {
	if err == nil {
		return nil
	}
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	switch {
	case errors.Is(err, ErrNotFound):
		return NewError(CodeNotFound, "未找到对应资源").Wrap(err)
	case errors.Is(err, ErrConflict), errors.Is(err, ErrVersionConflict):
		return NewError(CodeConflict, "资源状态已变化，请重试").Wrap(err)
	default:
		return NewError(CodeInternal, "服务内部错误").Wrap(err)
	}
}
