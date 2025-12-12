package volt

import (
	"context"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// Context extends context.Context with Volt-specific functionality.
// It provides type-safe access to registered services.
type Context struct {
	context.Context
	registry *Registry
	logger   *slog.Logger
}

// Use retrieves a typed service from the registry.
// Panics if the service is not found or has the wrong type.
//
// Example:
//
//	gitlab := volt.Use[*gitlab.Client](ctx, "gitlab")
func Use[T any](ctx context.Context, name string) T {
	voltCtx, ok := ctx.(*Context)
	if !ok {
		panic("Use called with non-Volt context")
	}

	svc, ok := voltCtx.registry.Get(name)
	if !ok {
		panic("service not found: " + name)
	}

	typed, ok := svc.(T)
	if !ok {
		var zero T
		panic("service type mismatch for " + name + ": expected " + typeName(zero))
	}

	return typed
}

// TryUse retrieves a typed service from the registry.
// Returns (zero value, false) if the service is not found or has the wrong type.
//
// Example:
//
//	if gitlab, ok := volt.TryUse[*gitlab.Client](ctx, "gitlab"); ok {
//	    // use gitlab
//	}
func TryUse[T any](ctx context.Context, name string) (T, bool) {
	var zero T

	voltCtx, ok := ctx.(*Context)
	if !ok {
		return zero, false
	}

	svc, ok := voltCtx.registry.Get(name)
	if !ok {
		return zero, false
	}

	typed, ok := svc.(T)
	if !ok {
		return zero, false
	}

	return typed, true
}

// Logger returns the context's logger with trace information attached.
func Logger(ctx context.Context) *slog.Logger {
	voltCtx, ok := ctx.(*Context)
	if !ok {
		return slog.Default()
	}

	// Add trace context if available
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().HasTraceID() {
		return voltCtx.logger.With(
			"trace_id", span.SpanContext().TraceID().String(),
			"span_id", span.SpanContext().SpanID().String(),
		)
	}

	return voltCtx.logger
}

// Span returns the current trace span from context.
func Span(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// TraceID returns the trace ID from context, or empty string if none.
func TraceID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().HasTraceID() {
		return span.SpanContext().TraceID().String()
	}
	return ""
}

// typeName returns a readable type name for error messages.
func typeName(v any) string {
	if v == nil {
		return "nil"
	}
	return fmt.Sprintf("%T", v)
}

// --- Context Keys ---

type contextKey int

const (
	requestIDKey contextKey = iota
	userKey
)

// WithRequestID adds a request ID to the context.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestID retrieves the request ID from context.
func RequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// WithUser adds user information to the context.
func WithUser(ctx context.Context, user any) context.Context {
	return context.WithValue(ctx, userKey, user)
}

// User retrieves user information from context.
func User[T any](ctx context.Context) (T, bool) {
	var zero T
	if user, ok := ctx.Value(userKey).(T); ok {
		return user, true
	}
	return zero, false
}
