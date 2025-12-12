package volt

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// Operation defines metadata for an API operation.
// This follows Huma's operation model for automatic OpenAPI generation.
type Operation struct {
	// HTTP method (GET, POST, PUT, PATCH, DELETE, etc.)
	Method string

	// URL path with parameters, e.g., "/users/{id}"
	Path string

	// Short summary for OpenAPI
	Summary string

	// Detailed description for OpenAPI
	Description string

	// OpenAPI tags for grouping
	Tags []string

	// Unique operation ID (auto-generated if empty)
	OperationID string

	// Whether the operation is deprecated
	Deprecated bool

	// Security requirements (references security schemes)
	Security []map[string][]string

	// Maximum request body size in bytes (0 = default)
	MaxBodyBytes int64

	// Request body timeout
	BodyReadTimeout int

	// Custom metadata
	Metadata map[string]any
}

// Handler is the function signature for operation handlers.
// I is the input type, O is the output type.
type Handler[I, O any] func(ctx context.Context, input *I) (*O, error)

// Register registers an operation with the application.
// The input and output types are used to generate OpenAPI schemas.
//
// Example:
//
//	type CreateUserInput struct {
//	    Body struct {
//	        Name  string `json:"name" required:"true" doc:"User's name"`
//	        Email string `json:"email" format:"email" doc:"User's email"`
//	    }
//	}
//
//	type CreateUserOutput struct {
//	    Body struct {
//	        ID   string `json:"id"`
//	        Name string `json:"name"`
//	    }
//	}
//
//	volt.Register(app, volt.Operation{
//	    Method:  "POST",
//	    Path:    "/users",
//	    Summary: "Create a new user",
//	    Tags:    []string{"users"},
//	}, func(ctx context.Context, input *CreateUserInput) (*CreateUserOutput, error) {
//	    // Implementation
//	})
func Register[I, O any](app *App, op Operation, handler Handler[I, O]) {
	// Build huma operation
	humaOp := huma.Operation{
		Method:       op.Method,
		Path:         op.Path,
		Summary:      op.Summary,
		Description:  op.Description,
		Tags:         op.Tags,
		OperationID:  op.OperationID,
		Deprecated:   op.Deprecated,
		Security:     op.Security,
		MaxBodyBytes: op.MaxBodyBytes,
	}

	if op.Metadata != nil {
		humaOp.Metadata = op.Metadata
	}

	// Register with Huma, wrapping our handler
	huma.Register(app.api, humaOp, func(ctx context.Context, input *I) (*O, error) {
		// Inject our enhanced context with service access
		voltCtx := &Context{
			Context:  ctx,
			registry: app.registry,
			logger:   app.logger,
		}

		return handler(voltCtx, input)
	})
}

// RegisterSimple registers a simple handler without input/output types.
// Useful for health checks, metrics endpoints, etc.
func RegisterSimple(app *App, method, path string, handler http.HandlerFunc) {
	app.router.Method(method, path, handler)
}

// Group creates a route group with shared middleware or prefix.
type Group struct {
	app    *App
	prefix string
	mw     []func(http.Handler) http.Handler
}

// Group creates a new route group.
func (a *App) Group(prefix string, middlewares ...func(http.Handler) http.Handler) *Group {
	return &Group{
		app:    a,
		prefix: prefix,
		mw:     middlewares,
	}
}

// RegisterGroup registers an operation within the group.
func RegisterGroup[I, O any](g *Group, op Operation, handler Handler[I, O]) {
	// Prepend group prefix to path
	op.Path = g.prefix + op.Path
	Register(g.app, op, handler)
}

// --- Common Input/Output Patterns ---

// EmptyInput represents a request with no input.
type EmptyInput struct{}

// StatusOutput represents a simple status response.
type StatusOutput struct {
	Body struct {
		Status  string `json:"status" example:"ok" doc:"Status message"`
		Message string `json:"message,omitempty" doc:"Optional message"`
	}
}

// ErrorModel represents an error response.
type ErrorModel struct {
	Status  int    `json:"status" doc:"HTTP status code"`
	Title   string `json:"title" doc:"Error title"`
	Detail  string `json:"detail,omitempty" doc:"Detailed error message"`
	TraceID string `json:"trace_id,omitempty" doc:"Trace ID for debugging"`
}

// PaginatedInput provides common pagination parameters.
type PaginatedInput struct {
	Page     int `query:"page" default:"1" minimum:"1" doc:"Page number"`
	PageSize int `query:"page_size" default:"20" minimum:"1" maximum:"100" doc:"Items per page"`
}

// PaginatedMeta provides pagination metadata for responses.
type PaginatedMeta struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	TotalItems int `json:"total_items"`
	TotalPages int `json:"total_pages"`
}

// --- Health Check ---

// HealthInput is empty for health checks.
type HealthInput struct{}

// HealthOutput represents health check response.
type HealthOutput struct {
	Body struct {
		Status    string            `json:"status" example:"healthy"`
		Version   string            `json:"version,omitempty"`
		Checks    map[string]string `json:"checks,omitempty"`
		Timestamp string            `json:"timestamp,omitempty"`
	}
}

// RegisterHealthCheck adds a health check endpoint.
func RegisterHealthCheck(app *App, path string, checks ...HealthChecker) {
	if path == "" {
		path = "/health"
	}

	Register(app, Operation{
		Method:  "GET",
		Path:    path,
		Summary: "Health check",
		Tags:    []string{"health"},
	}, func(ctx context.Context, input *HealthInput) (*HealthOutput, error) {
		out := &HealthOutput{}
		out.Body.Status = "healthy"
		out.Body.Version = app.config.Version
		out.Body.Checks = make(map[string]string)

		for _, check := range checks {
			name, status := check.Check(ctx)
			out.Body.Checks[name] = status
			if status != "ok" && status != "healthy" {
				out.Body.Status = "degraded"
			}
		}

		return out, nil
	})
}

// HealthChecker interface for custom health checks.
type HealthChecker interface {
	Check(ctx context.Context) (name string, status string)
}

// HealthCheckFunc is a function adapter for HealthChecker.
type HealthCheckFunc func(ctx context.Context) (name string, status string)

func (f HealthCheckFunc) Check(ctx context.Context) (string, string) {
	return f(ctx)
}
