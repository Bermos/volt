package volt

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Error represents a structured API error that integrates with
// Huma's error handling and OpenTelemetry tracing.
type Error struct {
	status  int
	code    string
	message string
	detail  string
	cause   error
	attrs   []attribute.KeyValue
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.message, e.cause)
	}
	return e.message
}

// Unwrap returns the underlying cause.
func (e *Error) Unwrap() error {
	return e.cause
}

// GetStatus implements huma.StatusError.
func (e *Error) GetStatus() int {
	return e.status
}

// --- Error Constructors ---

// NewError creates a new Error with the given status and message.
func NewError(status int, message string) *Error {
	return &Error{
		status:  status,
		message: message,
	}
}

// Wrap wraps an existing error with additional context.
func Wrap(err error, message string) *Error {
	return &Error{
		status:  http.StatusInternalServerError,
		message: message,
		cause:   err,
	}
}

// --- Common Errors ---

// ErrNotFound creates a 404 Not Found error.
func ErrNotFound(resource string) *Error {
	return &Error{
		status:  http.StatusNotFound,
		code:    "NOT_FOUND",
		message: fmt.Sprintf("%s not found", resource),
	}
}

// ErrBadRequest creates a 400 Bad Request error.
func ErrBadRequest(message string) *Error {
	return &Error{
		status:  http.StatusBadRequest,
		code:    "BAD_REQUEST",
		message: message,
	}
}

// ErrUnauthorized creates a 401 Unauthorized error.
func ErrUnauthorized(message string) *Error {
	if message == "" {
		message = "unauthorized"
	}
	return &Error{
		status:  http.StatusUnauthorized,
		code:    "UNAUTHORIZED",
		message: message,
	}
}

// ErrForbidden creates a 403 Forbidden error.
func ErrForbidden(message string) *Error {
	if message == "" {
		message = "forbidden"
	}
	return &Error{
		status:  http.StatusForbidden,
		code:    "FORBIDDEN",
		message: message,
	}
}

// ErrConflict creates a 409 Conflict error.
func ErrConflict(message string) *Error {
	return &Error{
		status:  http.StatusConflict,
		code:    "CONFLICT",
		message: message,
	}
}

// ErrValidation creates a 422 Unprocessable Entity error.
func ErrValidation(message string) *Error {
	return &Error{
		status:  http.StatusUnprocessableEntity,
		code:    "VALIDATION_ERROR",
		message: message,
	}
}

// ErrInternal creates a 500 Internal Server Error.
func ErrInternal(message string) *Error {
	if message == "" {
		message = "internal server error"
	}
	return &Error{
		status:  http.StatusInternalServerError,
		code:    "INTERNAL_ERROR",
		message: message,
	}
}

// ErrServiceUnavailable creates a 503 Service Unavailable error.
func ErrServiceUnavailable(message string) *Error {
	if message == "" {
		message = "service unavailable"
	}
	return &Error{
		status:  http.StatusServiceUnavailable,
		code:    "SERVICE_UNAVAILABLE",
		message: message,
	}
}

// --- Error Methods ---

// WithCode adds an error code.
func (e *Error) WithCode(code string) *Error {
	e.code = code
	return e
}

// WithDetail adds detailed error information.
func (e *Error) WithDetail(detail string) *Error {
	e.detail = detail
	return e
}

// WithCause sets the underlying cause.
func (e *Error) WithCause(err error) *Error {
	e.cause = err
	return e
}

// WithAttr adds an attribute for tracing.
func (e *Error) WithAttr(key string, value string) *Error {
	e.attrs = append(e.attrs, attribute.String(key, value))
	return e
}

// --- OTEL Integration ---

// Record records the error on the current span.
func (e *Error) Record(ctx context.Context) *Error {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return e
	}

	// Set span status for errors
	if e.status >= 500 {
		span.SetStatus(codes.Error, e.message)
	}

	// Record error
	span.RecordError(e,
		trace.WithAttributes(
			attribute.Int("error.status", e.status),
			attribute.String("error.code", e.code),
		),
	)

	// Add custom attributes
	if len(e.attrs) > 0 {
		span.SetAttributes(e.attrs...)
	}

	return e
}

// --- Huma Integration ---

// ToHumaError converts to a Huma error model.
func (e *Error) ToHumaError(ctx context.Context) huma.StatusError {
	// Record on span
	e.Record(ctx)

	// Create Huma error detail
	detail := &huma.ErrorDetail{
		Message:  e.message,
		Location: e.code,
	}

	// Add trace ID if available
	_ = TraceID(ctx)

	return huma.NewError(e.status, e.message, detail).(*huma.ErrorModel)
	// Note: You might want to customize this based on your needs
	//_ = traceID // Use in custom error model if needed
}

// --- Error Checking Helpers ---

// IsNotFound checks if an error is a not found error.
func IsNotFound(err error) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.status == http.StatusNotFound
	}
	return false
}

// IsConflict checks if an error is a conflict error.
func IsConflict(err error) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.status == http.StatusConflict
	}
	return false
}

// IsValidation checks if an error is a validation error.
func IsValidation(err error) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.status == http.StatusUnprocessableEntity || e.status == http.StatusBadRequest
	}
	return false
}

// StatusFromError extracts the HTTP status from an error, defaulting to 500.
func StatusFromError(err error) int {
	var e *Error
	if errors.As(err, &e) {
		return e.status
	}

	var humaErr huma.StatusError
	if errors.As(err, &humaErr) {
		return humaErr.GetStatus()
	}

	return http.StatusInternalServerError
}
