package volt

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestAuthzRequirement(t *testing.T) {
	t.Run("Authz creates requirement with standard fields", func(t *testing.T) {
		req := Authz("collection", "id", "admin")

		assertEqual(t, "collection", req.Resource)
		assertEqual(t, "id", req.ResourceIDParam)
		assertEqual(t, "admin", req.Permission)
		assertNil(t, req.Extra)
	})

	t.Run("AuthzExtra creates requirement with extra metadata", func(t *testing.T) {
		extra := map[string]any{
			"feature":      "ai_analysis",
			"subscription": "pro",
		}
		req := AuthzExtra("work", "workId", "editor", extra)

		assertEqual(t, "work", req.Resource)
		assertEqual(t, "workId", req.ResourceIDParam)
		assertEqual(t, "editor", req.Permission)
		assertEqual(t, "ai_analysis", req.Extra["feature"])
		assertEqual(t, "pro", req.Extra["subscription"])
	})
}

func TestWithAuthz(t *testing.T) {
	t.Run("adds authz to operation without existing metadata", func(t *testing.T) {
		op := Operation{
			Method: "GET",
			Path:   "/test",
		}

		result := WithAuthz(op, Authz("resource", "id", "viewer"))

		req, ok := result.Metadata["authz"].(AuthzRequirement)
		assertTrue(t, ok)
		assertEqual(t, "resource", req.Resource)
	})

	t.Run("preserves existing metadata", func(t *testing.T) {
		op := Operation{
			Method: "GET",
			Path:   "/test",
			Metadata: map[string]any{
				"custom": "value",
			},
		}

		result := WithAuthz(op, Authz("resource", "id", "viewer"))

		assertEqual(t, "value", result.Metadata["custom"])
		_, ok := result.Metadata["authz"].(AuthzRequirement)
		assertTrue(t, ok)
	})
}

func TestNoopPolicy(t *testing.T) {
	t.Run("allows all requests", func(t *testing.T) {
		ctx := context.Background()
		req := AuthzRequest{}
		requirement := Authz("anything", "id", "admin")

		err := NoopPolicy.Authorize(ctx, req, requirement)

		assertNil(t, err)
	})
}

func TestAuthenticatedPolicy(t *testing.T) {
	type user struct{ ID string }

	tests := []struct {
		name      string
		getUserFn func(ctx context.Context) (any, bool)
		wantErr   bool
	}{
		{
			name: "allows when user present",
			getUserFn: func(ctx context.Context) (any, bool) {
				return &user{ID: "123"}, true
			},
			wantErr: false,
		},
		{
			name: "denies when user not present",
			getUserFn: func(ctx context.Context) (any, bool) {
				return nil, false
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := AuthenticatedPolicy(tt.getUserFn)
			err := policy.Authorize(context.Background(), AuthzRequest{}, AuthzRequirement{})

			if tt.wantErr {
				assertNotNil(t, err)
				assertErrorStatus(t, err, http.StatusUnauthorized)
			} else {
				assertNil(t, err)
			}
		})
	}
}

func TestCompositePolicy(t *testing.T) {
	allowPolicy := AuthzPolicyFunc(func(ctx context.Context, req AuthzRequest, requirement AuthzRequirement) error {
		return nil
	})

	denyPolicy := AuthzPolicyFunc(func(ctx context.Context, req AuthzRequest, requirement AuthzRequirement) error {
		return ErrForbidden("denied")
	})

	tests := []struct {
		name     string
		policies []AuthzPolicy
		wantErr  bool
	}{
		{
			name:     "passes when all policies pass",
			policies: []AuthzPolicy{allowPolicy, allowPolicy, allowPolicy},
			wantErr:  false,
		},
		{
			name:     "fails when first policy fails",
			policies: []AuthzPolicy{denyPolicy, allowPolicy},
			wantErr:  true,
		},
		{
			name:     "fails when middle policy fails",
			policies: []AuthzPolicy{allowPolicy, denyPolicy, allowPolicy},
			wantErr:  true,
		},
		{
			name:     "fails when last policy fails",
			policies: []AuthzPolicy{allowPolicy, allowPolicy, denyPolicy},
			wantErr:  true,
		},
		{
			name:     "passes with empty policies",
			policies: []AuthzPolicy{},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := NewCompositePolicy(tt.policies...)
			err := policy.Authorize(context.Background(), AuthzRequest{}, AuthzRequirement{})

			if tt.wantErr {
				assertNotNil(t, err)
			} else {
				assertNil(t, err)
			}
		})
	}
}

func TestAuthzPolicyFunc(t *testing.T) {
	t.Run("adapts function to policy interface", func(t *testing.T) {
		called := false
		policy := AuthzPolicyFunc(func(ctx context.Context, req AuthzRequest, requirement AuthzRequirement) error {
			called = true
			return nil
		})

		err := policy.Authorize(context.Background(), AuthzRequest{}, AuthzRequirement{})

		assertNil(t, err)
		assertTrue(t, called)
	})
}

// =============================================================================
// Example: Resource-Based RBAC Policy Tests
// =============================================================================

// TestResourceRBACPolicy demonstrates testing a real-world policy implementation
func TestResourceRBACPolicy(t *testing.T) {
	type User struct {
		ID uuid.UUID
	}

	type ctxKey struct{}

	// Test implementation of a resource-based policy
	type testPolicy struct {
		users       map[uuid.UUID]*User
		resolvers   map[string]func(uuid.UUID) (uuid.UUID, error)
		permissions map[string]map[uuid.UUID]string // resourceID -> userID -> role
	}

	newTestPolicy := func() *testPolicy {
		return &testPolicy{
			users:       make(map[uuid.UUID]*User),
			resolvers:   make(map[string]func(uuid.UUID) (uuid.UUID, error)),
			permissions: make(map[string]map[uuid.UUID]string),
		}
	}

	hasRole := func(userRole, required string) bool {
		hierarchy := map[string]int{"viewer": 1, "member": 2, "editor": 3, "admin": 4}
		return hierarchy[userRole] >= hierarchy[required]
	}

	authorize := func(p *testPolicy, ctx context.Context, req AuthzRequest, requirement AuthzRequirement) error {
		// Get user from context
		user, ok := ctx.Value(ctxKey{}).(*User)
		if !ok {
			return ErrUnauthorized("not authenticated")
		}

		// No resource = just needs auth
		if requirement.Resource == "" {
			return nil
		}

		// Get resource ID
		resourceIDStr, ok := req.PathParams[requirement.ResourceIDParam]
		if !ok {
			return ErrBadRequest("missing resource ID")
		}
		resourceID, err := uuid.Parse(resourceIDStr)
		if err != nil {
			return ErrBadRequest("invalid resource ID")
		}

		// Resolve to authoritative resource
		resolver, ok := p.resolvers[requirement.Resource]
		if !ok {
			return ErrInternal("unknown resource type")
		}
		authID, err := resolver(resourceID)
		if err != nil {
			return ErrNotFound(requirement.Resource)
		}

		// Check permission
		perms, ok := p.permissions[authID.String()]
		if !ok {
			return ErrForbidden("no access")
		}
		role, ok := perms[user.ID]
		if !ok {
			return ErrForbidden("no access")
		}
		if !hasRole(role, requirement.Permission) {
			return ErrForbidden("insufficient permissions")
		}

		return nil
	}

	// Setup test data
	userID := uuid.New()
	otherUserID := uuid.New()
	collectionID := uuid.New()
	workID := uuid.New()
	scoreID := uuid.New()

	policy := newTestPolicy()
	policy.users[userID] = &User{ID: userID}
	policy.users[otherUserID] = &User{ID: otherUserID}

	// User is editor on collection
	policy.permissions[collectionID.String()] = map[uuid.UUID]string{
		userID: "editor",
	}

	// Resolvers
	policy.resolvers["collection"] = func(id uuid.UUID) (uuid.UUID, error) {
		if id == collectionID {
			return collectionID, nil
		}
		return uuid.Nil, errors.New("not found")
	}
	policy.resolvers["work"] = func(id uuid.UUID) (uuid.UUID, error) {
		if id == workID {
			return collectionID, nil // work belongs to collection
		}
		return uuid.Nil, errors.New("not found")
	}
	policy.resolvers["score"] = func(id uuid.UUID) (uuid.UUID, error) {
		if id == scoreID {
			return collectionID, nil // score belongs to collection (via work)
		}
		return uuid.Nil, errors.New("not found")
	}

	tests := []struct {
		name        string
		userID      *uuid.UUID // nil = no user in context
		requirement AuthzRequirement
		pathParams  map[string]string
		wantErr     bool
		wantStatus  int
	}{
		{
			name:        "allows editor to view collection",
			userID:      &userID,
			requirement: Authz("collection", "id", "viewer"),
			pathParams:  map[string]string{"id": collectionID.String()},
			wantErr:     false,
		},
		{
			name:        "allows editor to edit collection",
			userID:      &userID,
			requirement: Authz("collection", "id", "editor"),
			pathParams:  map[string]string{"id": collectionID.String()},
			wantErr:     false,
		},
		{
			name:        "denies editor admin access to collection",
			userID:      &userID,
			requirement: Authz("collection", "id", "admin"),
			pathParams:  map[string]string{"id": collectionID.String()},
			wantErr:     true,
			wantStatus:  http.StatusForbidden,
		},
		{
			name:        "allows editor to edit work in collection",
			userID:      &userID,
			requirement: Authz("work", "id", "editor"),
			pathParams:  map[string]string{"id": workID.String()},
			wantErr:     false,
		},
		{
			name:        "allows editor to view score in collection",
			userID:      &userID,
			requirement: Authz("score", "id", "viewer"),
			pathParams:  map[string]string{"id": scoreID.String()},
			wantErr:     false,
		},
		{
			name:        "denies unauthenticated user",
			userID:      nil,
			requirement: Authz("collection", "id", "viewer"),
			pathParams:  map[string]string{"id": collectionID.String()},
			wantErr:     true,
			wantStatus:  http.StatusUnauthorized,
		},
		{
			name:        "denies user without collection access",
			userID:      &otherUserID,
			requirement: Authz("collection", "id", "viewer"),
			pathParams:  map[string]string{"id": collectionID.String()},
			wantErr:     true,
			wantStatus:  http.StatusForbidden,
		},
		{
			name:        "returns not found for unknown resource",
			userID:      &userID,
			requirement: Authz("collection", "id", "viewer"),
			pathParams:  map[string]string{"id": uuid.New().String()},
			wantErr:     true,
			wantStatus:  http.StatusNotFound,
		},
		{
			name:        "returns bad request for missing resource ID",
			userID:      &userID,
			requirement: Authz("collection", "id", "viewer"),
			pathParams:  map[string]string{}, // missing "id"
			wantErr:     true,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "returns bad request for invalid UUID",
			userID:      &userID,
			requirement: Authz("collection", "id", "viewer"),
			pathParams:  map[string]string{"id": "not-a-uuid"},
			wantErr:     true,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "allows authenticated user when no resource required",
			userID:      &userID,
			requirement: Authz("", "", ""),
			pathParams:  map[string]string{},
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.userID != nil {
				ctx = context.WithValue(ctx, ctxKey{}, policy.users[*tt.userID])
			}

			req := AuthzRequest{PathParams: tt.pathParams}
			err := authorize(policy, ctx, req, tt.requirement)

			if tt.wantErr {
				assertNotNil(t, err)
				if tt.wantStatus != 0 {
					assertErrorStatus(t, err, tt.wantStatus)
				}
			} else {
				assertNil(t, err)
			}
		})
	}
}

// =============================================================================
// Test Helpers
// =============================================================================

func assertEqual[T comparable](t *testing.T, expected, actual T) {
	t.Helper()
	if expected != actual {
		t.Errorf("expected %v, got %v", expected, actual)
	}
}

func assertTrue(t *testing.T, condition bool) {
	t.Helper()
	if !condition {
		t.Error("expected true, got false")
	}
}

func assertNil(t *testing.T, v any) {
	t.Helper()
	if v != nil {
		// Handle error interface specially
		if err, ok := v.(error); ok && err != nil {
			t.Errorf("expected nil, got error: %v", err)
			return
		}
	}
}

func assertNotNil(t *testing.T, v any) {
	t.Helper()
	if v == nil {
		t.Error("expected non-nil, got nil")
	}
}

func assertErrorStatus(t *testing.T, err error, expectedStatus int) {
	t.Helper()
	var voltErr *Error
	if errors.As(err, &voltErr) {
		if voltErr.status != expectedStatus {
			t.Errorf("expected status %d, got %d", expectedStatus, voltErr.status)
		}
		return
	}
	t.Errorf("error is not a *volt.Error: %T", err)
}
