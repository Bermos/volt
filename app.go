// Package volt provides a batteries-included web framework for Go that combines
// the ergonomics of Huma with the philosophy of GoFr, while remaining radically
// open to developer choice.
package volt

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// App is the main application container that holds all configuration,
// services, and the HTTP server.
type App struct {
	config   *Config
	router   chi.Router
	api      huma.API
	registry *Registry
	otel     *OTELProvider
	logger   *slog.Logger
	server   *http.Server

	// Authorization
	authzPolicy AuthzPolicy

	// Lifecycle hooks
	onStart []func(context.Context) error
	onStop  []func(context.Context) error
}

// New creates a new Volt application with the given options.
func New(opts ...Option) *App {
	cfg := DefaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	// Initialize router
	r := chi.NewRouter()

	// Create app
	app := &App{
		config:   cfg,
		router:   r,
		registry: NewRegistry(),
		logger:   cfg.Logger,
	}

	// Setup OTEL if configured
	if cfg.OTEL.Enabled {
		provider, err := NewOTELProvider(cfg.OTEL)
		if err != nil {
			app.logger.Error("failed to initialize OTEL", "error", err)
		} else {
			app.otel = provider
			app.logger = provider.Logger() // Use OTEL-bridged logger
		}
	}

	// Setup default middleware stack
	app.setupMiddleware()

	// Create Huma API for OpenAPI generation
	humaConfig := huma.DefaultConfig(cfg.Name, cfg.Version)
	humaConfig.Info.Description = cfg.Description

	// Configure OpenAPI servers
	if len(cfg.OpenAPI.Servers) > 0 {
		humaConfig.Servers = make([]*huma.Server, len(cfg.OpenAPI.Servers))
		for i, s := range cfg.OpenAPI.Servers {
			humaConfig.Servers[i] = &huma.Server{
				URL:         s.URL,
				Description: s.Description,
			}
		}
	}

	app.api = humachi.New(r, humaConfig)

	return app
}

// setupMiddleware configures the default middleware stack.
func (a *App) setupMiddleware() {
	// Request ID for tracing correlation
	a.router.Use(middleware.RequestID)

	// Real IP detection
	a.router.Use(middleware.RealIP)

	// Recovery from panics
	a.router.Use(middleware.Recoverer)

	// OTEL HTTP instrumentation
	if a.otel != nil {
		a.router.Use(func(next http.Handler) http.Handler {
			return otelhttp.NewHandler(next, "http.request",
				otelhttp.WithTracerProvider(a.otel.TracerProvider()),
				otelhttp.WithMeterProvider(a.otel.MeterProvider()),
			)
		})
	}

	// Structured logging
	a.router.Use(a.loggingMiddleware())

	// Timeout
	if a.config.Server.RequestTimeout > 0 {
		a.router.Use(middleware.Timeout(a.config.Server.RequestTimeout))
	}
}

// loggingMiddleware creates a structured logging middleware.
func (a *App) loggingMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func() {
				a.logger.Info("request completed",
					"method", r.Method,
					"path", r.URL.Path,
					"status", ww.Status(),
					"duration_ms", time.Since(start).Milliseconds(),
					"bytes", ww.BytesWritten(),
					"request_id", middleware.GetReqID(r.Context()),
				)
			}()

			next.ServeHTTP(ww, r)
		})
	}
}

// Router returns the underlying chi router for advanced customization.
func (a *App) Router() chi.Router {
	return a.router
}

// API returns the Huma API for direct access to OpenAPI features.
func (a *App) API() huma.API {
	return a.api
}

// Logger returns the application logger.
func (a *App) Logger() *slog.Logger {
	return a.logger
}

// Registry returns the service registry.
func (a *App) Registry() *Registry {
	return a.registry
}

// Use adds middleware to the router.
func (a *App) Use(middlewares ...func(http.Handler) http.Handler) {
	a.router.Use(middlewares...)
}

// OnStart registers a function to be called when the application starts.
func (a *App) OnStart(fn func(context.Context) error) {
	a.onStart = append(a.onStart, fn)
}

// OnStop registers a function to be called when the application stops.
func (a *App) OnStop(fn func(context.Context) error) {
	a.onStop = append(a.onStop, fn)
}

// Run starts the application and blocks until shutdown.
func (a *App) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run start hooks
	for _, fn := range a.onStart {
		if err := fn(ctx); err != nil {
			return fmt.Errorf("start hook failed: %w", err)
		}
	}

	// Initialize registered services
	if err := a.registry.Initialize(ctx, a); err != nil {
		return fmt.Errorf("service initialization failed: %w", err)
	}

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", a.config.Server.Host, a.config.Server.Port)
	a.server = &http.Server{
		Addr:         addr,
		Handler:      a.router,
		ReadTimeout:  a.config.Server.ReadTimeout,
		WriteTimeout: a.config.Server.WriteTimeout,
		IdleTimeout:  a.config.Server.IdleTimeout,
	}

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		a.logger.Info("starting server",
			"addr", addr,
			"name", a.config.Name,
			"version", a.config.Version,
		)
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errChan:
		return err
	case sig := <-sigChan:
		a.logger.Info("received shutdown signal", "signal", sig)
	}

	return a.Shutdown(ctx)
}

// Shutdown gracefully shuts down the application.
func (a *App) Shutdown(ctx context.Context) error {
	shutdownCtx, cancel := context.WithTimeout(ctx, a.config.Server.ShutdownTimeout)
	defer cancel()

	a.logger.Info("shutting down server")

	// Shutdown HTTP server
	if a.server != nil {
		if err := a.server.Shutdown(shutdownCtx); err != nil {
			a.logger.Error("server shutdown error", "error", err)
		}
	}

	// Run stop hooks in reverse order
	for i := len(a.onStop) - 1; i >= 0; i-- {
		if err := a.onStop[i](shutdownCtx); err != nil {
			a.logger.Error("stop hook failed", "error", err)
		}
	}

	// Shutdown services
	if err := a.registry.Shutdown(shutdownCtx); err != nil {
		a.logger.Error("service shutdown error", "error", err)
	}

	// Shutdown OTEL
	if a.otel != nil {
		if err := a.otel.Shutdown(shutdownCtx); err != nil {
			a.logger.Error("OTEL shutdown error", "error", err)
		}
	}

	a.logger.Info("shutdown complete")
	return nil
}
