# Volt ⚡

A Go web framework for developers who want speed without the straitjacket.

## Philosophy

Volt is heavily inspired by [GoFr](https://gofr.dev) and [Huma](https://huma.rocks), taking the best ideas from both while staying radically open:

- **Huma-style operations**: Define your API with structs, get OpenAPI for free
- **OTEL-native**: Traces, metrics, and logs flow to your collector out of the box
- **Bring your own everything**: ORM, HTTP clients, caches - we instrument, you choose
- **Standing on giants**: chi, huma, otel-*, slog - proven libraries, not reinvented wheels

## Quick Start

```go
package main

import (
    "context"
    "github.com/yourorg/volt"
)

type GreetInput struct {
    Name string `path:"name" doc:"Name to greet"`
}

type GreetOutput struct {
    Body struct {
        Message string `json:"message"`
    }
}

func main() {
    app := volt.New()
    
    // Register an operation - OpenAPI generated automatically
    volt.Register(app, volt.Operation{
        Method:      "GET",
        Path:        "/greet/{name}",
        Summary:     "Greet someone",
        Tags:        []string{"greetings"},
    }, func(ctx context.Context, input *GreetInput) (*GreetOutput, error) {
        return &GreetOutput{
            Body: struct{ Message string }{
                Message: "Hello, " + input.Name + "!",
            },
        }, nil
    })
    
    app.Run()
}
```

## Core Concepts

### 1. Operations (Huma-style)

Operations are the heart of Volt. Define input/output structs with tags, and Volt:
- Validates requests automatically
- Generates OpenAPI 3.1 specs
- Handles content negotiation
- Provides type-safe handlers

### 2. Services (Instrumented Dependencies)

Register any HTTP-based service client:

```go
// Your existing client library
type GitLabClient struct {
    http *http.Client
    base string
}

func NewGitLabClient(httpClient *http.Client, baseURL string) *GitLabClient {
    return &GitLabClient{http: httpClient, base: baseURL}
}

// Register it - Volt provides an instrumented http.Client
volt.RegisterHTTPService(app, "gitlab", func(client *http.Client) any {
    return NewGitLabClient(client, "https://gitlab.com/api/v4")
})

// Use it in handlers
func (ctx context.Context, input *MyInput) (*MyOutput, error) {
    gitlab := volt.Use[*GitLabClient](ctx, "gitlab")
    // client already has tracing, metrics, retries...
}
```

### 3. Databases (Instrumented Connections)

```go
// Register any database - you get an instrumented *sql.DB
volt.RegisterDatabase(app, "primary", volt.DatabaseConfig{
    Driver: "postgres",
    DSN:    "postgres://...",
})

// Use with your favorite ORM
import "gorm.io/gorm"

volt.RegisterDatabaseService(app, "gorm", func(db *sql.DB) any {
    gormDB, _ := gorm.Open(postgres.New(postgres.Config{Conn: db}))
    return gormDB
})

// In handlers
func handler(ctx context.Context, input *Input) (*Output, error) {
    db := volt.Use[*gorm.DB](ctx, "gorm")
    // All queries are traced automatically
}
```

### 4. OTEL Integration

```go
app := volt.New(
    volt.WithOTEL(volt.OTELConfig{
        ServiceName:    "my-service",
        ServiceVersion: "1.0.0",
        CollectorURL:   "otel-collector:4317",
        // Logs, traces, metrics all configured
    }),
)
```

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         volt.App                                 │
├─────────────────────────────────────────────────────────────────┤
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │   Router     │  │   OpenAPI    │  │    OTEL      │          │
│  │   (chi)      │  │   (huma)     │  │  Provider    │          │
│  └──────────────┘  └──────────────┘  └──────────────┘          │
├─────────────────────────────────────────────────────────────────┤
│                    Service Registry                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │ HTTP Clients │  │  Databases   │  │   Caches     │          │
│  │ (instrumented)│ │ (instrumented)│ │ (instrumented)│          │
│  └──────────────┘  └──────────────┘  └──────────────┘          │
├─────────────────────────────────────────────────────────────────┤
│                    Middleware Stack                              │
│  ┌─────┐ ┌─────────┐ ┌────────┐ ┌──────────┐ ┌───────┐        │
│  │OTEL │→│Recovery │→│Logging │→│RequestID │→│ Your  │        │
│  └─────┘ └─────────┘ └────────┘ └──────────┘ └───────┘        │
└─────────────────────────────────────────────────────────────────┘
```

## Giants We Stand On

| Component | Library | Why |
|-----------|---------|-----|
| Router | [chi](https://github.com/go-chi/chi) | Fast, idiomatic, middleware-friendly |
| OpenAPI | [huma](https://github.com/danielgtaylor/huma) | Best-in-class struct-to-OpenAPI |
| Tracing | [otel](https://opentelemetry.io/docs/go/) | Industry standard |
| HTTP Instrumentation | [otelhttp](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp) | Automatic span propagation |
| SQL Instrumentation | [otelsql](https://github.com/XSAM/otelsql) | Database observability |
| Logging | [slog](https://pkg.go.dev/log/slog) + [otelslog](https://pkg.go.dev/go.opentelemetry.io/contrib/bridges/otelslog) | Structured logs to OTEL |
| Validation | [huma validators](https://huma.rocks/features/request-validation/) | Schema-driven |

## Directory Structure

```
volt/
├── app.go              # Core application struct and lifecycle
├── config.go           # Configuration handling
├── context.go          # Enhanced context with service access
├── database.go         # Database registration and instrumentation
├── http_service.go     # HTTP client registration and instrumentation
├── middleware.go       # Built-in middleware
├── otel.go             # OpenTelemetry setup
├── operation.go        # Huma-style operation registration
├── registry.go         # Service registry (DI container)
└── server.go           # HTTP server with graceful shutdown
```

## License

MIT
