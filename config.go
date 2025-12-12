package volt

import (
	"fmt"
	"log/slog"
	"os"
	"time"
)

// Config holds all application configuration.
type Config struct {
	Name        string
	Version     string
	Description string
	Environment string

	Server  ServerConfig
	OTEL    OTELConfig
	OpenAPI OpenAPIConfig

	Logger *slog.Logger
}

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Host            string
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	RequestTimeout  time.Duration
	ShutdownTimeout time.Duration
}

// OTELConfig holds OpenTelemetry configuration.
type OTELConfig struct {
	Enabled        bool
	ServiceName    string
	ServiceVersion string
	Environment    string
	CollectorURL   string // gRPC endpoint, e.g., "localhost:4317"

	// Sampling configuration
	TraceSampleRate float64

	// Resource attributes
	Attributes map[string]string

	// Feature flags
	EnableTraces  bool
	EnableMetrics bool
	EnableLogs    bool
}

// OpenAPIConfig holds OpenAPI documentation configuration.
type OpenAPIConfig struct {
	Enabled     bool
	DocsPath    string // Default: /docs
	SpecPath    string // Default: /openapi.json
	Servers     []OpenAPIServer
	SecurityDef map[string]SecurityScheme
}

// OpenAPIServer represents an API server for the OpenAPI spec.
type OpenAPIServer struct {
	URL         string
	Description string
}

// SecurityScheme represents an OpenAPI security scheme.
type SecurityScheme struct {
	Type         string
	Scheme       string
	BearerFormat string
	In           string
	Name         string
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Name:        "volt-app",
		Version:     "0.0.1",
		Description: "A Volt application",
		Environment: getEnv("ENVIRONMENT", "development"),

		Server: ServerConfig{
			Host:            getEnv("HOST", "0.0.0.0"),
			Port:            getEnvInt("PORT", 8080),
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    30 * time.Second,
			IdleTimeout:     120 * time.Second,
			RequestTimeout:  30 * time.Second,
			ShutdownTimeout: 30 * time.Second,
		},

		OTEL: OTELConfig{
			Enabled:         getEnvBool("OTEL_ENABLED", false),
			ServiceName:     getEnv("OTEL_SERVICE_NAME", "volt-app"),
			ServiceVersion:  getEnv("OTEL_SERVICE_VERSION", "0.0.1"),
			Environment:     getEnv("OTEL_ENVIRONMENT", "development"),
			CollectorURL:    getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
			TraceSampleRate: 1.0,
			EnableTraces:    true,
			EnableMetrics:   true,
			EnableLogs:      true,
		},

		OpenAPI: OpenAPIConfig{
			Enabled:  true,
			DocsPath: "/docs",
			SpecPath: "/openapi.json",
		},

		Logger: slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})),
	}
}

// Option is a function that modifies Config.
type Option func(*Config)

// WithName sets the application name.
func WithName(name string) Option {
	return func(c *Config) {
		c.Name = name
		c.OTEL.ServiceName = name
	}
}

// WithVersion sets the application version.
func WithVersion(version string) Option {
	return func(c *Config) {
		c.Version = version
		c.OTEL.ServiceVersion = version
	}
}

// WithDescription sets the application description.
func WithDescription(desc string) Option {
	return func(c *Config) {
		c.Description = desc
	}
}

// WithPort sets the server port.
func WithPort(port int) Option {
	return func(c *Config) {
		c.Server.Port = port
	}
}

// WithHost sets the server host.
func WithHost(host string) Option {
	return func(c *Config) {
		c.Server.Host = host
	}
}

// WithOTEL configures OpenTelemetry.
func WithOTEL(otelCfg OTELConfig) Option {
	return func(c *Config) {
		c.OTEL = otelCfg
		c.OTEL.Enabled = true
	}
}

// WithOTELCollector enables OTEL and sets the collector URL.
func WithOTELCollector(url string) Option {
	return func(c *Config) {
		c.OTEL.Enabled = true
		c.OTEL.CollectorURL = url
	}
}

// WithLogger sets a custom logger.
func WithLogger(logger *slog.Logger) Option {
	return func(c *Config) {
		c.Logger = logger
	}
}

// WithOpenAPIServers sets the OpenAPI servers.
func WithOpenAPIServers(servers ...OpenAPIServer) Option {
	return func(c *Config) {
		c.OpenAPI.Servers = servers
	}
}

// WithRequestTimeout sets the request timeout.
func WithRequestTimeout(d time.Duration) Option {
	return func(c *Config) {
		c.Server.RequestTimeout = d
	}
}

// Environment helpers

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		var i int
		if _, err := fmt.Sscanf(val, "%d", &i); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		return val == "true" || val == "1" || val == "yes"
	}
	return defaultVal
}
