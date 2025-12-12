package volt

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Registry manages all registered services and their lifecycle.
type Registry struct {
	mu       sync.RWMutex
	services map[string]*serviceEntry
	order    []string // Track registration order for init/shutdown

	// Factories for lazy initialization
	httpServices map[string]*httpServiceFactory
	dbServices   map[string]*dbServiceFactory
}

type serviceEntry struct {
	name     string
	instance any
	shutdown func(context.Context) error
}

type httpServiceFactory struct {
	name       string
	baseURL    string
	factory    func(client *http.Client) any
	config     HTTPServiceConfig
	instance   any
	httpClient *http.Client
}

type dbServiceFactory struct {
	name     string
	config   DatabaseConfig
	factory  func(db *sql.DB) any
	instance any
	db       *sql.DB
}

// NewRegistry creates a new service registry.
func NewRegistry() *Registry {
	return &Registry{
		services:     make(map[string]*serviceEntry),
		httpServices: make(map[string]*httpServiceFactory),
		dbServices:   make(map[string]*dbServiceFactory),
	}
}

// Register adds a service to the registry.
func (r *Registry) Register(name string, instance any, shutdown func(context.Context) error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.services[name] = &serviceEntry{
		name:     name,
		instance: instance,
		shutdown: shutdown,
	}
	r.order = append(r.order, name)
}

// Get retrieves a service by name.
func (r *Registry) Get(name string) (any, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if entry, ok := r.services[name]; ok {
		return entry.instance, true
	}
	if factory, ok := r.httpServices[name]; ok && factory.instance != nil {
		return factory.instance, true
	}
	if factory, ok := r.dbServices[name]; ok && factory.instance != nil {
		return factory.instance, true
	}
	return nil, false
}

// MustGet retrieves a service or panics if not found.
func (r *Registry) MustGet(name string) any {
	svc, ok := r.Get(name)
	if !ok {
		panic(fmt.Sprintf("service %q not found", name))
	}
	return svc
}

// Initialize initializes all registered services.
func (r *Registry) Initialize(ctx context.Context, app *App) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Initialize HTTP services
	for name, factory := range r.httpServices {
		client := r.createInstrumentedHTTPClient(app, factory.config, name)
		factory.httpClient = client
		factory.instance = factory.factory(client)

		app.logger.Info("initialized HTTP service", "name", name, "base_url", factory.baseURL)
	}

	// Initialize database services
	for name, factory := range r.dbServices {
		db, err := r.createInstrumentedDB(ctx, app, factory.config, name)
		if err != nil {
			return fmt.Errorf("failed to initialize database %q: %w", name, err)
		}
		factory.db = db

		if factory.factory != nil {
			factory.instance = factory.factory(db)
		} else {
			factory.instance = db
		}

		app.logger.Info("initialized database service", "name", name, "driver", factory.config.Driver)
	}

	return nil
}

// Shutdown gracefully shuts down all services.
func (r *Registry) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []error

	// Shutdown in reverse order
	for i := len(r.order) - 1; i >= 0; i-- {
		name := r.order[i]
		if entry, ok := r.services[name]; ok && entry.shutdown != nil {
			if err := entry.shutdown(ctx); err != nil {
				errs = append(errs, fmt.Errorf("shutdown %q: %w", name, err))
			}
		}
	}

	// Close database connections
	for name, factory := range r.dbServices {
		if factory.db != nil {
			if err := factory.db.Close(); err != nil {
				errs = append(errs, fmt.Errorf("close database %q: %w", name, err))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}
	return nil
}

// --- HTTP Service Registration ---

// HTTPServiceConfig configures an HTTP service client.
type HTTPServiceConfig struct {
	BaseURL string
	Timeout time.Duration

	// Retry configuration
	MaxRetries     int
	RetryWaitMin   time.Duration
	RetryWaitMax   time.Duration
	RetryOnStatus  []int // HTTP status codes to retry on

	// Custom transport options
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	IdleConnTimeout     time.Duration

	// Headers to add to all requests
	DefaultHeaders map[string]string
}

// DefaultHTTPServiceConfig returns sensible defaults.
func DefaultHTTPServiceConfig() HTTPServiceConfig {
	return HTTPServiceConfig{
		Timeout:             30 * time.Second,
		MaxRetries:          3,
		RetryWaitMin:        100 * time.Millisecond,
		RetryWaitMax:        2 * time.Second,
		RetryOnStatus:       []int{502, 503, 504},
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}
}

// RegisterHTTPService registers an HTTP-based service client.
// The factory function receives an instrumented http.Client.
//
// Example:
//
//	volt.RegisterHTTPService(app, "gitlab", func(client *http.Client) any {
//	    return gitlab.NewClient(client, "https://gitlab.com/api/v4")
//	})
func RegisterHTTPService[T any](app *App, name string, factory func(client *http.Client) T, opts ...HTTPServiceOption) {
	config := DefaultHTTPServiceConfig()
	for _, opt := range opts {
		opt(&config)
	}

	app.registry.mu.Lock()
	defer app.registry.mu.Unlock()

	app.registry.httpServices[name] = &httpServiceFactory{
		name:    name,
		baseURL: config.BaseURL,
		factory: func(client *http.Client) any {
			return factory(client)
		},
		config: config,
	}
}

// HTTPServiceOption configures an HTTP service.
type HTTPServiceOption func(*HTTPServiceConfig)

// WithHTTPBaseURL sets the base URL for the service.
func WithHTTPBaseURL(url string) HTTPServiceOption {
	return func(c *HTTPServiceConfig) {
		c.BaseURL = url
	}
}

// WithHTTPTimeout sets the request timeout.
func WithHTTPTimeout(d time.Duration) HTTPServiceOption {
	return func(c *HTTPServiceConfig) {
		c.Timeout = d
	}
}

// WithHTTPRetries configures retry behavior.
func WithHTTPRetries(max int, minWait, maxWait time.Duration) HTTPServiceOption {
	return func(c *HTTPServiceConfig) {
		c.MaxRetries = max
		c.RetryWaitMin = minWait
		c.RetryWaitMax = maxWait
	}
}

// WithHTTPHeaders sets default headers.
func WithHTTPHeaders(headers map[string]string) HTTPServiceOption {
	return func(c *HTTPServiceConfig) {
		c.DefaultHeaders = headers
	}
}

// createInstrumentedHTTPClient creates an HTTP client with OTEL instrumentation.
func (r *Registry) createInstrumentedHTTPClient(app *App, config HTTPServiceConfig, name string) *http.Client {
	transport := &http.Transport{
		MaxIdleConns:        config.MaxIdleConns,
		MaxIdleConnsPerHost: config.MaxIdleConnsPerHost,
		IdleConnTimeout:     config.IdleConnTimeout,
	}

	// Wrap with OTEL instrumentation if available
	var rt http.RoundTripper = transport
	if app.otel != nil {
		rt = otelhttp.NewTransport(transport,
			otelhttp.WithTracerProvider(app.otel.TracerProvider()),
			otelhttp.WithMeterProvider(app.otel.MeterProvider()),
		)
	}

	// Wrap with retry logic
	if config.MaxRetries > 0 {
		rt = &retryRoundTripper{
			base:       rt,
			maxRetries: config.MaxRetries,
			minWait:    config.RetryWaitMin,
			maxWait:    config.RetryWaitMax,
			retryOn:    config.RetryOnStatus,
		}
	}

	// Wrap with default headers
	if len(config.DefaultHeaders) > 0 {
		rt = &headerRoundTripper{
			base:    rt,
			headers: config.DefaultHeaders,
		}
	}

	return &http.Client{
		Transport: rt,
		Timeout:   config.Timeout,
	}
}

// --- Database Registration ---

// DatabaseConfig configures a database connection.
type DatabaseConfig struct {
	Driver string
	DSN    string

	// Connection pool settings
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// DefaultDatabaseConfig returns sensible defaults.
func DefaultDatabaseConfig() DatabaseConfig {
	return DatabaseConfig{
		MaxOpenConns:    25,
		MaxIdleConns:    10,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 5 * time.Minute,
	}
}

// RegisterDatabase registers a database connection.
// Returns the instrumented *sql.DB directly.
//
// Example:
//
//	volt.RegisterDatabase(app, "primary", volt.DatabaseConfig{
//	    Driver: "postgres",
//	    DSN:    "postgres://user:pass@localhost/db",
//	})
func RegisterDatabase(app *App, name string, config DatabaseConfig) {
	app.registry.mu.Lock()
	defer app.registry.mu.Unlock()

	app.registry.dbServices[name] = &dbServiceFactory{
		name:   name,
		config: config,
	}
}

// RegisterDatabaseService registers a database with a custom factory.
// Useful for wrapping with an ORM.
//
// Example:
//
//	volt.RegisterDatabaseService(app, "gorm", config, func(db *sql.DB) any {
//	    gormDB, _ := gorm.Open(postgres.New(postgres.Config{Conn: db}))
//	    return gormDB
//	})
func RegisterDatabaseService[T any](app *App, name string, config DatabaseConfig, factory func(db *sql.DB) T) {
	app.registry.mu.Lock()
	defer app.registry.mu.Unlock()

	app.registry.dbServices[name] = &dbServiceFactory{
		name:   name,
		config: config,
		factory: func(db *sql.DB) any {
			return factory(db)
		},
	}
}

// createInstrumentedDB creates a database connection with OTEL instrumentation.
func (r *Registry) createInstrumentedDB(ctx context.Context, app *App, config DatabaseConfig, name string) (*sql.DB, error) {
	// Note: In production, you'd use otelsql here
	// db, err := otelsql.Open(config.Driver, config.DSN)
	db, err := sql.Open(config.Driver, config.DSN)
	if err != nil {
		return nil, err
	}

	// Configure pool
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxIdleConns)
	db.SetConnMaxLifetime(config.ConnMaxLifetime)
	db.SetConnMaxIdleTime(config.ConnMaxIdleTime)

	// Test connection
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping failed: %w", err)
	}

	return db, nil
}

// --- Round Trippers ---

type retryRoundTripper struct {
	base       http.RoundTripper
	maxRetries int
	minWait    time.Duration
	maxWait    time.Duration
	retryOn    []int
}

func (rt *retryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error

	for attempt := 0; attempt <= rt.maxRetries; attempt++ {
		resp, err = rt.base.RoundTrip(req)

		if err == nil && !rt.shouldRetry(resp.StatusCode) {
			return resp, nil
		}

		if attempt < rt.maxRetries {
			wait := rt.minWait * time.Duration(1<<attempt)
			if wait > rt.maxWait {
				wait = rt.maxWait
			}
			time.Sleep(wait)
		}
	}

	return resp, err
}

func (rt *retryRoundTripper) shouldRetry(status int) bool {
	for _, s := range rt.retryOn {
		if status == s {
			return true
		}
	}
	return false
}

type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (rt *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range rt.headers {
		if req.Header.Get(k) == "" {
			req.Header.Set(k, v)
		}
	}
	return rt.base.RoundTrip(req)
}
