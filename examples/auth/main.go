// Example demonstrating Volt's authorization system
// Shows progression from simple to complex use cases
package main

import (
	"context"
	"log"

	"github.com/bermos/volt"
	"github.com/google/uuid"
)

// =============================================================================
// Domain Types (your models)
// =============================================================================

type User struct {
	ID    uuid.UUID
	Email string
}

// =============================================================================
// Example 1: Simple Authentication-Only Policy
// =============================================================================

// For simple apps that just need "is user logged in?"
func simpleExample() {
	app := volt.New()

	// Simple policy: just check user is authenticated
	volt.SetAuthzPolicy(app, volt.AuthenticatedPolicy(func(ctx context.Context) (any, bool) {
		user, ok := volt.User[*User](ctx)
		return user, ok
	}))

	// Operations just declare they need auth - policy handles it
	volt.Register(app, volt.WithAuthz(volt.Operation{
		Method:  "GET",
		Path:    "/profile",
		Summary: "Get current user profile",
	}, volt.AuthzPermission("authenticated")), handleGetProfile)
}

// =============================================================================
// Example 2: Simple Role-Based Policy
// =============================================================================

// For apps with global roles (admin, user, etc.)
type SimpleRolePolicy struct {
	getUserFn func(ctx context.Context) (*User, bool)
	getRoleFn func(ctx context.Context, userID uuid.UUID) (string, error)
}

func (p *SimpleRolePolicy) Authorize(ctx context.Context, req volt.AuthzRequest, requirement volt.AuthzRequirement) error {
	user, ok := p.getUserFn(ctx)
	if !ok {
		return volt.ErrUnauthorized("authentication required")
	}

	// Get user's global role
	role, err := p.getRoleFn(ctx, user.ID)
	if err != nil {
		return volt.ErrInternal("failed to check permissions").WithCause(err)
	}

	// Simple role hierarchy check
	if !hasRole(role, requirement.Permission) {
		return volt.ErrForbidden("insufficient permissions")
	}

	return nil
}

func hasRole(userRole, required string) bool {
	hierarchy := map[string]int{"viewer": 1, "member": 2, "editor": 3, "admin": 4}
	return hierarchy[userRole] >= hierarchy[required]
}

func simpleRoleExample() {
	app := volt.New()

	volt.SetAuthzPolicy(app, &SimpleRolePolicy{
		getUserFn: func(ctx context.Context) (*User, bool) {
			return volt.User[*User](ctx)
		},
		getRoleFn: func(ctx context.Context, userID uuid.UUID) (string, error) {
			// Your role lookup logic
			return "editor", nil
		},
	})

	// Admin-only endpoint
	volt.Register(app, volt.WithAuthz(volt.Operation{
		Method:  "DELETE",
		Path:    "/users/{id}",
		Summary: "Delete a user",
	}, volt.Authz("", "", "admin")), handleDeleteUser)

	// Editor endpoint
	volt.Register(app, volt.WithAuthz(volt.Operation{
		Method:  "PUT",
		Path:    "/posts/{id}",
		Summary: "Update a post",
	}, volt.Authz("", "", "editor")), handleUpdatePost)
}

// =============================================================================
// Example 3: Resource-Based RBAC
// =============================================================================

// For apps where permissions are per-resource (collections, workspaces, etc.)
type ResourceRBACPolicy struct {
	getUserFn   func(ctx context.Context) (*User, bool)
	resolvers   map[string]ResourceResolver
	roleChecker func(ctx context.Context, userID, resourceID uuid.UUID, permission string) (bool, error)
}

// ResourceResolver resolves a resource ID to its authoritative parent
// (e.g., score -> work -> collection, returns collection ID)
type ResourceResolver func(ctx context.Context, resourceID uuid.UUID) (uuid.UUID, error)

func (p *ResourceRBACPolicy) Authorize(ctx context.Context, req volt.AuthzRequest, requirement volt.AuthzRequirement) error {
	user, ok := p.getUserFn(ctx)
	if !ok {
		return volt.ErrUnauthorized("authentication required")
	}

	// No resource specified = just needs authentication
	if requirement.Resource == "" {
		return nil
	}

	// Get resource ID from request parameters
	resourceIDStr, ok := req.PathParams[requirement.ResourceIDParam]
	if !ok {
		return volt.ErrBadRequest("missing resource identifier")
	}

	resourceID, err := uuid.Parse(resourceIDStr)
	if err != nil {
		return volt.ErrBadRequest("invalid resource identifier")
	}

	// Resolve to authoritative resource (e.g., score -> collection)
	resolver, ok := p.resolvers[requirement.Resource]
	if !ok {
		return volt.ErrInternal("unknown resource type: " + requirement.Resource)
	}

	authoritativeID, err := resolver(ctx, resourceID)
	if err != nil {
		return volt.ErrNotFound(requirement.Resource)
	}

	// Check role on the authoritative resource
	hasPermission, err := p.roleChecker(ctx, user.ID, authoritativeID, requirement.Permission)
	if err != nil {
		return volt.ErrInternal("authorization check failed").WithCause(err)
	}

	if !hasPermission {
		return volt.ErrForbidden("insufficient permissions for this " + requirement.Resource)
	}

	return nil
}

func resourceRBACExample() {
	app := volt.New()

	// Your services (would be injected via DI in real app)
	// collectionService := ...
	// workService := ...
	// scoreService := ...
	// authzService := ...

	volt.SetAuthzPolicy(app, &ResourceRBACPolicy{
		getUserFn: func(ctx context.Context) (*User, bool) {
			return volt.User[*User](ctx)
		},
		resolvers: map[string]ResourceResolver{
			// Collection is the root - returns itself
			"collection": func(ctx context.Context, id uuid.UUID) (uuid.UUID, error) {
				return id, nil
			},
			// Work resolves to its parent collection
			"work": func(ctx context.Context, id uuid.UUID) (uuid.UUID, error) {
				// return workService.GetCollectionID(ctx, id)
				return uuid.New(), nil // placeholder
			},
			// Score resolves through work to collection
			"score": func(ctx context.Context, id uuid.UUID) (uuid.UUID, error) {
				// workID, _ := scoreService.GetWorkID(ctx, id)
				// return workService.GetCollectionID(ctx, workID)
				return uuid.New(), nil // placeholder
			},
			// Program resolves to its parent collection
			"program": func(ctx context.Context, id uuid.UUID) (uuid.UUID, error) {
				// return programService.GetCollectionID(ctx, id)
				return uuid.New(), nil // placeholder
			},
		},
		roleChecker: func(ctx context.Context, userID, collectionID uuid.UUID, permission string) (bool, error) {
			// return authzService.HasCollectionRole(ctx, userID, collectionID, permission)
			return true, nil // placeholder
		},
	})

	// Now operations are clean and declarative:

	// Collection endpoints
	volt.Register(app, volt.WithAuthz(volt.Operation{
		Method:  "GET",
		Path:    "/collections/{id}",
		Summary: "Get collection details",
	}, volt.Authz("collection", "id", "viewer")), handleGetCollection)

	volt.Register(app, volt.WithAuthz(volt.Operation{
		Method:  "PUT",
		Path:    "/collections/{id}",
		Summary: "Update collection",
	}, volt.Authz("collection", "id", "editor")), handleUpdateCollection)

	volt.Register(app, volt.WithAuthz(volt.Operation{
		Method:  "DELETE",
		Path:    "/collections/{id}",
		Summary: "Delete collection",
	}, volt.Authz("collection", "id", "admin")), handleDeleteCollection)

	// Work endpoints - authorization cascades to parent collection
	volt.Register(app, volt.WithAuthz(volt.Operation{
		Method:  "POST",
		Path:    "/collections/{collectionId}/works",
		Summary: "Create work in collection",
	}, volt.Authz("collection", "collectionId", "editor")), handleCreateWork)

	volt.Register(app, volt.WithAuthz(volt.Operation{
		Method:  "PUT",
		Path:    "/works/{id}",
		Summary: "Update work",
	}, volt.Authz("work", "id", "editor")), handleUpdateWork)

	// Score endpoints - authorization cascades through work to collection
	volt.Register(app, volt.WithAuthz(volt.Operation{
		Method:  "GET",
		Path:    "/scores/{id}",
		Summary: "Get score details",
	}, volt.Authz("score", "id", "viewer")), handleGetScore)

	volt.Register(app, volt.WithAuthz(volt.Operation{
		Method:  "DELETE",
		Path:    "/scores/{id}",
		Summary: "Delete score",
	}, volt.Authz("score", "id", "editor")), handleDeleteScore)
}

// =============================================================================
// Example 4: Complex Policy with Extra Metadata
// =============================================================================

// For apps that need feature flags, subscription checks, etc.
type FullPolicy struct {
	authz               *ResourceRBACPolicy
	featureChecker      func(ctx context.Context, userID uuid.UUID, feature string) bool
	subscriptionChecker func(ctx context.Context, userID uuid.UUID, tier string) bool
}

func (p *FullPolicy) Authorize(ctx context.Context, req volt.AuthzRequest, requirement volt.AuthzRequirement) error {
	// First, do standard RBAC check
	if err := p.authz.Authorize(ctx, req, requirement); err != nil {
		return err
	}

	user, _ := volt.User[*User](ctx)

	// Check feature flag if specified
	if feature, ok := requirement.Extra["feature"].(string); ok {
		if !p.featureChecker(ctx, user.ID, feature) {
			return volt.ErrForbidden("feature not available")
		}
	}

	// Check subscription tier if specified
	if tier, ok := requirement.Extra["subscription"].(string); ok {
		if !p.subscriptionChecker(ctx, user.ID, tier) {
			return volt.ErrForbidden("subscription upgrade required").
				WithCode("UPGRADE_REQUIRED").
				WithDetail("This feature requires the " + tier + " plan")
		}
	}

	return nil
}

func complexExample() {
	app := volt.New()

	// Assume we have the full policy configured...

	// Operation with feature flag requirement
	volt.Register(app, volt.WithAuthz(volt.Operation{
		Method:  "POST",
		Path:    "/collections/{collectionId}/scores",
		Summary: "Upload a score",
	}, volt.AuthzExtra("collection", "collectionId", "editor", map[string]any{
		"feature": "score_upload",
	})), handleUploadScore)

	// Operation with subscription requirement
	volt.Register(app, volt.WithAuthz(volt.Operation{
		Method:  "POST",
		Path:    "/collections/{collectionId}/export",
		Summary: "Export collection as PDF",
	}, volt.AuthzExtra("collection", "collectionId", "viewer", map[string]any{
		"subscription": "pro",
	})), handleExportCollection)

	// Operation with both
	volt.Register(app, volt.WithAuthz(volt.Operation{
		Method:  "POST",
		Path:    "/collections/{collectionId}/ai-analyze",
		Summary: "AI analysis of collection",
	}, volt.AuthzExtra("collection", "collectionId", "editor", map[string]any{
		"feature":      "ai_analysis",
		"subscription": "enterprise",
	})), handleAIAnalysis)
}

// =============================================================================
// Handler Stubs
// =============================================================================

func handleGetProfile(ctx context.Context, input *struct{}) (*struct{ Body User }, error) {
	return nil, nil
}
func handleDeleteUser(ctx context.Context, input *struct {
	ID string `path:"id"`
}) (*struct{}, error) {
	return nil, nil
}
func handleUpdatePost(ctx context.Context, input *struct {
	ID string `path:"id"`
}) (*struct{}, error) {
	return nil, nil
}
func handleGetCollection(ctx context.Context, input *struct {
	ID string `path:"id"`
}) (*struct{}, error) {
	return nil, nil
}
func handleUpdateCollection(ctx context.Context, input *struct {
	ID string `path:"id"`
}) (*struct{}, error) {
	return nil, nil
}
func handleDeleteCollection(ctx context.Context, input *struct {
	ID string `path:"id"`
}) (*struct{}, error) {
	return nil, nil
}
func handleCreateWork(ctx context.Context, input *struct {
	CollectionID string `path:"collectionId"`
}) (*struct{}, error) {
	return nil, nil
}
func handleUpdateWork(ctx context.Context, input *struct {
	ID string `path:"id"`
}) (*struct{}, error) {
	return nil, nil
}
func handleGetScore(ctx context.Context, input *struct {
	ID string `path:"id"`
}) (*struct{}, error) {
	return nil, nil
}
func handleDeleteScore(ctx context.Context, input *struct {
	ID string `path:"id"`
}) (*struct{}, error) {
	return nil, nil
}
func handleUploadScore(ctx context.Context, input *struct {
	CollectionID string `path:"collectionId"`
}) (*struct{}, error) {
	return nil, nil
}
func handleExportCollection(ctx context.Context, input *struct {
	CollectionID string `path:"collectionId"`
}) (*struct{}, error) {
	return nil, nil
}
func handleAIAnalysis(ctx context.Context, input *struct {
	CollectionID string `path:"collectionId"`
}) (*struct{}, error) {
	return nil, nil
}

func main() {
	log.Println("This is an example file demonstrating authorization patterns")
	log.Println("See the individual example functions for different complexity levels")
}
