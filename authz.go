package volt

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// =============================================================================
// Authorization Requirements (The "What")
// =============================================================================

// AuthzRequirement declares what authorization is needed for an operation.
// This is purely declarative - it says WHAT is required, not HOW to verify it.
// The Policy implementation decides how to interpret and enforce these requirements.
type AuthzRequirement struct {
	// Resource type being accessed (e.g., "collection", "work", "score")
	// Empty string means no resource-level authorization needed
	Resource string

	// Parameter name containing the resource ID (e.g., "id", "workId")
	// References a path, query, or header parameter from the operation input
	ResourceIDParam string

	// Required permission/role (e.g., "admin", "editor", "read", "write")
	// Interpretation is up to the Policy implementation
	Permission string

	// Custom metadata for complex authorization scenarios
	// Use this for anything that doesn't fit the standard fields
	Extra map[string]any
}

// =============================================================================
// Authorization Policy (The "How")
// =============================================================================

// AuthzPolicy defines how authorization requirements are enforced.
// Implement this interface to plug in your authorization logic.
type AuthzPolicy interface {
	// Authorize checks if the request meets the authorization requirements.
	//
	// Parameters:
	//   - ctx: Request context (contains authenticated user if auth middleware ran)
	//   - req: The HTTP request
	//   - requirement: The declared authorization requirement for this operation
	//
	// Returns nil if authorized, or an error (typically *volt.Error) if not.
	// The error will be returned to the client, so use appropriate status codes.
	Authorize(ctx context.Context, req AuthzRequest, requirement AuthzRequirement) error
}

// AuthzRequest provides access to request data needed for authorization decisions.
type AuthzRequest struct {
	// The underlying HTTP request
	HTTP *http.Request

	// Resolved path parameters (e.g., {"id": "abc-123", "workId": "def-456"})
	PathParams map[string]string

	// Query parameters
	QueryParams map[string][]string

	// Operation metadata (for custom authorization logic)
	Operation *huma.Operation
}

// =============================================================================
// Policy Implementations
// =============================================================================

// AuthzPolicyFunc is a function adapter for simple policies.
type AuthzPolicyFunc func(ctx context.Context, req AuthzRequest, requirement AuthzRequirement) error

func (f AuthzPolicyFunc) Authorize(ctx context.Context, req AuthzRequest, requirement AuthzRequirement) error {
	return f(ctx, req, requirement)
}

// NoopPolicy allows all requests. Use for development or public APIs.
var NoopPolicy AuthzPolicy = AuthzPolicyFunc(func(ctx context.Context, req AuthzRequest, requirement AuthzRequirement) error {
	return nil
})

// AuthenticatedPolicy only checks that a user is present in context.
// Pair with your auth middleware that sets the user.
func AuthenticatedPolicy(getUserFn func(ctx context.Context) (any, bool)) AuthzPolicy {
	return AuthzPolicyFunc(func(ctx context.Context, req AuthzRequest, requirement AuthzRequirement) error {
		if _, ok := getUserFn(ctx); !ok {
			return ErrUnauthorized("authentication required")
		}
		return nil
	})
}

// CompositePolicy chains multiple policies. All must pass.
type CompositePolicy struct {
	policies []AuthzPolicy
}

func NewCompositePolicy(policies ...AuthzPolicy) *CompositePolicy {
	return &CompositePolicy{policies: policies}
}

func (c *CompositePolicy) Authorize(ctx context.Context, req AuthzRequest, requirement AuthzRequirement) error {
	for _, policy := range c.policies {
		if err := policy.Authorize(ctx, req, requirement); err != nil {
			return err
		}
	}
	return nil
}

// =============================================================================
// Framework Integration
// =============================================================================

// SetAuthzPolicy registers the authorization policy for the application.
func SetAuthzPolicy(app *App, policy AuthzPolicy) {
	app.authzPolicy = policy
}

// authzMiddleware creates the Huma middleware that enforces authorization.
func (a *App) authzMiddleware() func(ctx huma.Context, next func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		// Skip if no policy configured
		if a.authzPolicy == nil {
			next(ctx)
			return
		}

		// Get requirement from operation metadata
		requirement, ok := ctx.Operation().Metadata["authz"].(AuthzRequirement)
		if !ok {
			// No authorization requirement declared - allow through
			next(ctx)
			return
		}

		// Build authorization request
		req := AuthzRequest{
			HTTP:        nil, // Will be set by adapter
			PathParams:  extractPathParams(ctx),
			QueryParams: extractQueryParams(ctx),
			Operation:   ctx.Operation(),
		}

		// Check authorization
		if err := a.authzPolicy.Authorize(ctx.Context(), req, requirement); err != nil {
			status := StatusFromError(err)
			_ = huma.WriteErr(a.api, ctx, status, err.Error())
			return
		}

		next(ctx)
	}
}

// extractPathParams extracts path parameters from the Huma context.
func extractPathParams(ctx huma.Context) map[string]string {
	params := make(map[string]string)
	op := ctx.Operation()
	if op == nil {
		return params
	}

	for _, param := range op.Parameters {
		if param.In == "path" {
			if value := ctx.Param(param.Name); value != "" {
				params[param.Name] = value
			}
		}
	}
	return params
}

// extractQueryParams extracts query parameters from the Huma context.
func extractQueryParams(ctx huma.Context) map[string][]string {
	params := make(map[string][]string)
	op := ctx.Operation()
	if op == nil {
		return params
	}

	for _, param := range op.Parameters {
		if param.In == "query" {
			if value := ctx.Query(param.Name); value != "" {
				params[param.Name] = []string{value}
			}
		}
	}
	return params
}

// =============================================================================
// Operation Helpers
// =============================================================================

// WithAuthz creates an Operation with authorization requirement.
// Convenience function for cleaner operation definitions.
func WithAuthz(op Operation, requirement AuthzRequirement) Operation {
	if op.Metadata == nil {
		op.Metadata = make(map[string]any)
	}
	op.Metadata["authz"] = requirement
	return op
}

// Authz creates an AuthzRequirement with the common fields.
// Use for simple resource + permission patterns.
func Authz(resource, resourceIDParam, permission string) AuthzRequirement {
	return AuthzRequirement{
		Resource:        resource,
		ResourceIDParam: resourceIDParam,
		Permission:      permission,
	}
}

// AuthzPermission creates an AuthzRequirement with the common fields and permission.
// Convenience function for simpler permission checks.
func AuthzPermission(permission string) AuthzRequirement {
	return Authz("", "", permission)
}

// AuthzExtra creates an AuthzRequirement with extra metadata.
// Use for complex authorization scenarios.
func AuthzExtra(resource, resourceIDParam, permission string, extra map[string]any) AuthzRequirement {
	return AuthzRequirement{
		Resource:        resource,
		ResourceIDParam: resourceIDParam,
		Permission:      permission,
		Extra:           extra,
	}
}
