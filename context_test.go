package volt

import (
	"context"
	"testing"
)

func TestUse(t *testing.T) {
	type TestService struct {
		Name string
	}

	t.Run("retrieves typed service from volt context", func(t *testing.T) {
		registry := NewRegistry()
		svc := &TestService{Name: "test"}
		registry.Register("myservice", svc, nil)

		ctx := &Context{
			Context:  context.Background(),
			registry: registry,
		}

		result := Use[*TestService](ctx, "myservice")
		assertEqual(t, "test", result.Name)
	})

	t.Run("panics with non-volt context", func(t *testing.T) {
		defer func() {
			r := recover()
			if r == nil {
				t.Error("expected panic")
			}
			if r != "Use called with non-Volt context" {
				t.Errorf("unexpected panic message: %v", r)
			}
		}()

		Use[*TestService](context.Background(), "myservice")
	})

	t.Run("panics when service not found", func(t *testing.T) {
		registry := NewRegistry()
		ctx := &Context{
			Context:  context.Background(),
			registry: registry,
		}

		defer func() {
			r := recover()
			if r == nil {
				t.Error("expected panic")
			}
		}()

		Use[*TestService](ctx, "unknown")
	})

	t.Run("panics on type mismatch", func(t *testing.T) {
		registry := NewRegistry()
		registry.Register("myservice", "a string", nil)

		ctx := &Context{
			Context:  context.Background(),
			registry: registry,
		}

		defer func() {
			r := recover()
			if r == nil {
				t.Error("expected panic")
			}
		}()

		Use[*TestService](ctx, "myservice")
	})
}

func TestTryUse(t *testing.T) {
	type TestService struct {
		Name string
	}

	t.Run("returns service and true when found", func(t *testing.T) {
		registry := NewRegistry()
		svc := &TestService{Name: "test"}
		registry.Register("myservice", svc, nil)

		ctx := &Context{
			Context:  context.Background(),
			registry: registry,
		}

		result, ok := TryUse[*TestService](ctx, "myservice")
		assertTrue(t, ok)
		assertEqual(t, "test", result.Name)
	})

	t.Run("returns false for non-volt context", func(t *testing.T) {
		result, ok := TryUse[*TestService](context.Background(), "myservice")
		assertTrue(t, !ok)
		assertNil(t, result)
	})

	t.Run("returns false when service not found", func(t *testing.T) {
		registry := NewRegistry()
		ctx := &Context{
			Context:  context.Background(),
			registry: registry,
		}

		result, ok := TryUse[*TestService](ctx, "unknown")
		assertTrue(t, !ok)
		assertNil(t, result)
	})

	t.Run("returns false on type mismatch", func(t *testing.T) {
		registry := NewRegistry()
		registry.Register("myservice", "a string", nil)

		ctx := &Context{
			Context:  context.Background(),
			registry: registry,
		}

		result, ok := TryUse[*TestService](ctx, "myservice")
		assertTrue(t, !ok)
		assertNil(t, result)
	})
}

func TestRequestID(t *testing.T) {
	t.Run("WithRequestID and RequestID round-trip", func(t *testing.T) {
		ctx := context.Background()
		ctx = WithRequestID(ctx, "req-123")

		result := RequestID(ctx)
		assertEqual(t, "req-123", result)
	})

	t.Run("RequestID returns empty string when not set", func(t *testing.T) {
		result := RequestID(context.Background())
		assertEqual(t, "", result)
	})
}

func TestUser(t *testing.T) {
	type TestUser struct {
		ID    string
		Email string
	}

	t.Run("WithUser and User round-trip", func(t *testing.T) {
		ctx := context.Background()
		user := &TestUser{ID: "123", Email: "test@example.com"}
		ctx = WithUser(ctx, user)

		result, ok := User[*TestUser](ctx)
		assertTrue(t, ok)
		assertEqual(t, "123", result.ID)
		assertEqual(t, "test@example.com", result.Email)
	})

	t.Run("User returns false when not set", func(t *testing.T) {
		result, ok := User[*TestUser](context.Background())
		assertTrue(t, !ok)
		assertNil(t, result)
	})

	t.Run("User returns false on type mismatch", func(t *testing.T) {
		ctx := WithUser(context.Background(), "not a user struct")

		result, ok := User[*TestUser](ctx)
		assertTrue(t, !ok)
		assertNil(t, result)
	})
}

func TestTypeName(t *testing.T) {
	t.Run("returns nil for nil", func(t *testing.T) {
		result := typeName(nil)
		assertEqual(t, "nil", result)
	})

	t.Run("returns type name for values", func(t *testing.T) {
		result := typeName("hello")
		assertEqual(t, "string", result)

		result = typeName(42)
		assertEqual(t, "int", result)

		type Custom struct{}
		result = typeName(&Custom{})
		assertEqual(t, "*volt.Custom", result)
	})
}
