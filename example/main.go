// Example application demonstrating Volt framework features
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/bermos/volt"
)

// =============================================================================
// Domain Types
// =============================================================================

type User struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// =============================================================================
// API Input/Output Types (Huma-style)
// =============================================================================

// --- Users ---

type ListUsersInput struct {
	volt.PaginatedInput        // Embeds page, page_size
	Search              string `query:"search" doc:"Search term for filtering users"`
}

type ListUsersOutput struct {
	Body struct {
		Users []User             `json:"users"`
		Meta  volt.PaginatedMeta `json:"meta"`
	}
}

type GetUserInput struct {
	ID string `path:"id" doc:"User ID" example:"usr_abc123"`
}

type GetUserOutput struct {
	Body User
}

type CreateUserInput struct {
	Body struct {
		Name  string `json:"name" required:"true" minLength:"1" maxLength:"100" doc:"User's full name"`
		Email string `json:"email" required:"true" format:"email" doc:"User's email address"`
	}
}

type CreateUserOutput struct {
	Body User
}

type UpdateUserInput struct {
	ID   string `path:"id" doc:"User ID"`
	Body struct {
		Name  string `json:"name,omitempty" minLength:"1" maxLength:"100"`
		Email string `json:"email,omitempty" format:"email"`
	}
}

type UpdateUserOutput struct {
	Body User
}

type DeleteUserInput struct {
	ID string `path:"id" doc:"User ID"`
}

type DeleteUserOutput struct {
	Body struct {
		Deleted bool `json:"deleted"`
	}
}

// =============================================================================
// External Service Clients (to demonstrate DI)
// =============================================================================

// GitLabClient - example external service
type GitLabClient struct {
	http    *http.Client
	baseURL string
}

func NewGitLabClient(httpClient *http.Client, baseURL string) *GitLabClient {
	return &GitLabClient{
		http:    httpClient,
		baseURL: baseURL,
	}
}

func (c *GitLabClient) GetProject(ctx context.Context, id string) (map[string]any, error) {
	// The http.Client is already instrumented with OTEL tracing
	resp, err := c.http.Get(fmt.Sprintf("%s/projects/%s", c.baseURL, id))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	// ... parse response
	return map[string]any{"id": id}, nil
}

// NotificationService - another example
type NotificationService struct {
	http *http.Client
}

func NewNotificationService(httpClient *http.Client) *NotificationService {
	return &NotificationService{http: httpClient}
}

func (s *NotificationService) SendEmail(ctx context.Context, to, subject, body string) error {
	// Implementation using instrumented client
	return nil
}

// =============================================================================
// Handlers
// =============================================================================

func handleListUsers(ctx context.Context, input *ListUsersInput) (*ListUsersOutput, error) {
	// Access logger with trace context
	logger := volt.Logger(ctx)
	logger.Info("listing users", "page", input.Page, "search", input.Search)

	// Start a custom span for business logic
	ctx, span := volt.StartSpan(ctx, "users.list")
	defer span.End()

	// Example: access registered services
	// gitlab := volt.Use[*GitLabClient](ctx, "gitlab")
	// db := volt.Use[*gorm.DB](ctx, "gorm")

	// Mock response
	users := []User{
		{ID: "usr_1", Name: "Alice", Email: "alice@example.com", CreatedAt: time.Now()},
		{ID: "usr_2", Name: "Bob", Email: "bob@example.com", CreatedAt: time.Now()},
	}

	return &ListUsersOutput{
		Body: struct {
			Users []User             `json:"users"`
			Meta  volt.PaginatedMeta `json:"meta"`
		}{
			Users: users,
			Meta: volt.PaginatedMeta{
				Page:       input.Page,
				PageSize:   input.PageSize,
				TotalItems: 2,
				TotalPages: 1,
			},
		},
	}, nil
}

func handleGetUser(ctx context.Context, input *GetUserInput) (*GetUserOutput, error) {
	logger := volt.Logger(ctx)
	logger.Info("getting user", "id", input.ID)

	user := User{
		ID:        input.ID,
		Name:      "Alice",
		Email:     "alice@example.com",
		CreatedAt: time.Now(),
	}

	return &GetUserOutput{Body: user}, nil
}

func handleCreateUser(ctx context.Context, input *CreateUserInput) (*CreateUserOutput, error) {
	logger := volt.Logger(ctx)
	logger.Info("creating user", "name", input.Body.Name, "email", input.Body.Email)

	// Example: send notification using registered service
	// notifier := volt.Use[*NotificationService](ctx, "notifications")
	// notifier.SendEmail(ctx, input.Body.Email, "Welcome!", "...")

	user := User{
		ID:        "usr_new",
		Name:      input.Body.Name,
		Email:     input.Body.Email,
		CreatedAt: time.Now(),
	}

	return &CreateUserOutput{Body: user}, nil
}

func handleUpdateUser(ctx context.Context, input *UpdateUserInput) (*UpdateUserOutput, error) {
	user := User{
		ID:        input.ID,
		Name:      input.Body.Name,
		Email:     input.Body.Email,
		CreatedAt: time.Now(),
	}

	return &UpdateUserOutput{Body: user}, nil
}

func handleDeleteUser(ctx context.Context, input *DeleteUserInput) (*DeleteUserOutput, error) {
	return &DeleteUserOutput{
		Body: struct {
			Deleted bool `json:"deleted"`
		}{Deleted: true},
	}, nil
}

// =============================================================================
// Main
// =============================================================================

func main() {
	// Create application with configuration
	app := volt.New(
		volt.WithName("example-api"),
		volt.WithVersion("1.0.0"),
		volt.WithDescription("Example API demonstrating Volt framework"),
		volt.WithPort(8080),

		// Enable OTEL (comment out if no collector available)
		// volt.WithOTELCollector("localhost:4317"),

		// Configure OpenAPI servers
		volt.WithOpenAPIServers(
			volt.OpenAPIServer{URL: "http://localhost:8080", Description: "Local development"},
			volt.OpenAPIServer{URL: "https://api.example.com", Description: "Production"},
		),
	)

	// ==========================================================================
	// Register External Services (DI)
	// ==========================================================================

	// Register GitLab client - receives instrumented http.Client
	volt.RegisterHTTPService(app, "gitlab", func(client *http.Client) *GitLabClient {
		return NewGitLabClient(client, "https://gitlab.com/api/v4")
	},
		volt.WithHTTPTimeout(30*time.Second),
		volt.WithHTTPRetries(3, 100*time.Millisecond, 2*time.Second),
		volt.WithHTTPHeaders(map[string]string{
			"PRIVATE-TOKEN": "your-token-here",
		}),
	)

	// Register notification service
	volt.RegisterHTTPService(app, "notifications", func(client *http.Client) *NotificationService {
		return NewNotificationService(client)
	},
		volt.WithHTTPBaseURL("https://notifications.internal"),
	)

	// ==========================================================================
	// Register Databases (with ORM example)
	// ==========================================================================

	// Option 1: Direct sql.DB access
	// volt.RegisterDatabase(app, "primary", volt.DatabaseConfig{
	// 	Driver: "postgres",
	// 	DSN:    "postgres://user:pass@localhost/mydb?sslmode=disable",
	// })

	// Option 2: With ORM wrapper (GORM example)
	// volt.RegisterDatabaseService(app, "gorm", volt.DatabaseConfig{
	// 	Driver: "postgres",
	// 	DSN:    "postgres://user:pass@localhost/mydb?sslmode=disable",
	// }, func(db *sql.DB) *gorm.DB {
	// 	gormDB, _ := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{})
	// 	return gormDB
	// })

	// ==========================================================================
	// Register API Operations (Huma-style)
	// ==========================================================================

	// Health check (built-in)
	volt.RegisterHealthCheck(app, "/health",
		volt.HealthCheckFunc(func(ctx context.Context) (string, string) {
			return "database", "ok"
		}),
		volt.HealthCheckFunc(func(ctx context.Context) (string, string) {
			return "gitlab", "ok"
		}),
	)

	// Users CRUD
	volt.Register(app, volt.Operation{
		Method:      "GET",
		Path:        "/api/v1/users",
		Summary:     "List users",
		Description: "Returns a paginated list of users with optional filtering",
		Tags:        []string{"users"},
	}, handleListUsers)

	volt.Register(app, volt.Operation{
		Method:      "GET",
		Path:        "/api/v1/users/{id}",
		Summary:     "Get user",
		Description: "Returns a single user by ID",
		Tags:        []string{"users"},
	}, handleGetUser)

	volt.Register(app, volt.Operation{
		Method:      "POST",
		Path:        "/api/v1/users",
		Summary:     "Create user",
		Description: "Creates a new user",
		Tags:        []string{"users"},
	}, handleCreateUser)

	volt.Register(app, volt.Operation{
		Method:      "PUT",
		Path:        "/api/v1/users/{id}",
		Summary:     "Update user",
		Description: "Updates an existing user",
		Tags:        []string{"users"},
	}, handleUpdateUser)

	volt.Register(app, volt.Operation{
		Method:      "DELETE",
		Path:        "/api/v1/users/{id}",
		Summary:     "Delete user",
		Description: "Deletes a user",
		Tags:        []string{"users"},
	}, handleDeleteUser)

	// ==========================================================================
	// Lifecycle Hooks
	// ==========================================================================

	app.OnStart(func(ctx context.Context) error {
		app.Logger().Info("application starting up")
		// Run migrations, warm caches, etc.
		return nil
	})

	app.OnStop(func(ctx context.Context) error {
		app.Logger().Info("application shutting down")
		// Cleanup resources
		return nil
	})

	// ==========================================================================
	// Run
	// ==========================================================================

	log.Println("Starting server on :8080")
	log.Println("OpenAPI docs: http://localhost:8080/docs")
	log.Println("OpenAPI spec: http://localhost:8080/openapi.json")

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
