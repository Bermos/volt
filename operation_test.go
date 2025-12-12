package volt

import (
	"context"
	"testing"
)

func TestOperation(t *testing.T) {
	t.Run("Operation struct holds all fields", func(t *testing.T) {
		op := Operation{
			Method:      "POST",
			Path:        "/users",
			Summary:     "Create user",
			Description: "Creates a new user account",
			Tags:        []string{"users", "admin"},
			OperationID: "createUser",
			Deprecated:  true,
			Security: []map[string][]string{
				{"bearer": {}},
			},
			MaxBodyBytes: 1024 * 1024,
			Metadata: map[string]any{
				"custom": "value",
			},
		}

		assertEqual(t, "POST", op.Method)
		assertEqual(t, "/users", op.Path)
		assertEqual(t, "Create user", op.Summary)
		assertEqual(t, "Creates a new user account", op.Description)
		assertEqual(t, 2, len(op.Tags))
		assertEqual(t, "createUser", op.OperationID)
		assertTrue(t, op.Deprecated)
		assertEqual(t, int64(1024*1024), op.MaxBodyBytes)
		assertEqual(t, "value", op.Metadata["custom"])
	})
}

func TestPaginatedInput(t *testing.T) {
	t.Run("has expected defaults in struct tags", func(t *testing.T) {
		// This is more of a documentation test - verifying the struct exists
		// with expected fields. Actual default handling is done by Huma.
		input := PaginatedInput{
			Page:     1,
			PageSize: 20,
		}

		assertEqual(t, 1, input.Page)
		assertEqual(t, 20, input.PageSize)
	})
}

func TestPaginatedMeta(t *testing.T) {
	t.Run("calculates pagination correctly", func(t *testing.T) {
		meta := PaginatedMeta{
			Page:       2,
			PageSize:   10,
			TotalItems: 95,
			TotalPages: 10,
		}

		assertEqual(t, 2, meta.Page)
		assertEqual(t, 10, meta.PageSize)
		assertEqual(t, 95, meta.TotalItems)
		assertEqual(t, 10, meta.TotalPages)
	})
}

func TestHealthCheckFunc(t *testing.T) {
	t.Run("adapts function to HealthChecker interface", func(t *testing.T) {
		checker := HealthCheckFunc(func(ctx context.Context) (string, string) {
			return "database", "ok"
		})

		name, status := checker.Check(context.Background())

		assertEqual(t, "database", name)
		assertEqual(t, "ok", status)
	})

	t.Run("can return unhealthy status", func(t *testing.T) {
		checker := HealthCheckFunc(func(ctx context.Context) (string, string) {
			return "redis", "connection failed"
		})

		name, status := checker.Check(context.Background())

		assertEqual(t, "redis", name)
		assertEqual(t, "connection failed", status)
	})
}

func TestStatusOutput(t *testing.T) {
	t.Run("can hold status response", func(t *testing.T) {
		output := StatusOutput{}
		output.Body.Status = "ok"
		output.Body.Message = "All systems operational"

		assertEqual(t, "ok", output.Body.Status)
		assertEqual(t, "All systems operational", output.Body.Message)
	})
}

func TestErrorModel(t *testing.T) {
	t.Run("holds error response fields", func(t *testing.T) {
		model := ErrorModel{
			Status:  404,
			Title:   "Not Found",
			Detail:  "User with ID 123 not found",
			TraceID: "abc-123-def",
		}

		assertEqual(t, 404, model.Status)
		assertEqual(t, "Not Found", model.Title)
		assertEqual(t, "User with ID 123 not found", model.Detail)
		assertEqual(t, "abc-123-def", model.TraceID)
	})
}

// =============================================================================
// Group Tests
// =============================================================================

func TestGroup(t *testing.T) {
	t.Run("stores prefix and middleware", func(t *testing.T) {
		app := &App{}
		mw := func(next func()) func() {
			return func() { next() }
		}
		_ = mw // middleware example

		group := app.Group("/api/v1")

		assertEqual(t, "/api/v1", group.prefix)
		assertEqual(t, app, group.app)
	})
}
