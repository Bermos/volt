package volt

import (
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	t.Run("returns config with sensible defaults", func(t *testing.T) {
		cfg := DefaultConfig()

		assertEqual(t, "volt-app", cfg.Name)
		assertEqual(t, "0.0.1", cfg.Version)
		assertEqual(t, "A Volt application", cfg.Description)
		assertEqual(t, "0.0.0.0", cfg.Server.Host)
		assertEqual(t, 8080, cfg.Server.Port)
		assertEqual(t, 30*time.Second, cfg.Server.ReadTimeout)
		assertEqual(t, 30*time.Second, cfg.Server.WriteTimeout)
		assertEqual(t, 30*time.Second, cfg.Server.ShutdownTimeout)
		assertTrue(t, !cfg.OTEL.Enabled)
		assertTrue(t, cfg.OpenAPI.Enabled)
		assertEqual(t, "/docs", cfg.OpenAPI.DocsPath)
		assertEqual(t, "/openapi.json", cfg.OpenAPI.SpecPath)
	})
}

func TestConfigOptions(t *testing.T) {
	t.Run("WithName sets name and OTEL service name", func(t *testing.T) {
		cfg := DefaultConfig()
		WithName("my-api")(cfg)

		assertEqual(t, "my-api", cfg.Name)
		assertEqual(t, "my-api", cfg.OTEL.ServiceName)
	})

	t.Run("WithVersion sets version and OTEL service version", func(t *testing.T) {
		cfg := DefaultConfig()
		WithVersion("2.0.0")(cfg)

		assertEqual(t, "2.0.0", cfg.Version)
		assertEqual(t, "2.0.0", cfg.OTEL.ServiceVersion)
	})

	t.Run("WithDescription sets description", func(t *testing.T) {
		cfg := DefaultConfig()
		WithDescription("My awesome API")(cfg)

		assertEqual(t, "My awesome API", cfg.Description)
	})

	t.Run("WithPort sets server port", func(t *testing.T) {
		cfg := DefaultConfig()
		WithPort(3000)(cfg)

		assertEqual(t, 3000, cfg.Server.Port)
	})

	t.Run("WithHost sets server host", func(t *testing.T) {
		cfg := DefaultConfig()
		WithHost("127.0.0.1")(cfg)

		assertEqual(t, "127.0.0.1", cfg.Server.Host)
	})

	t.Run("WithRequestTimeout sets request timeout", func(t *testing.T) {
		cfg := DefaultConfig()
		WithRequestTimeout(60 * time.Second)(cfg)

		assertEqual(t, 60*time.Second, cfg.Server.RequestTimeout)
	})

	t.Run("WithOTELCollector enables OTEL and sets collector URL", func(t *testing.T) {
		cfg := DefaultConfig()
		WithOTELCollector("otel-collector:4317")(cfg)

		assertTrue(t, cfg.OTEL.Enabled)
		assertEqual(t, "otel-collector:4317", cfg.OTEL.CollectorURL)
	})

	t.Run("WithOTEL sets full OTEL config", func(t *testing.T) {
		cfg := DefaultConfig()
		otelCfg := OTELConfig{
			ServiceName:     "custom-service",
			ServiceVersion:  "1.0.0",
			CollectorURL:    "localhost:4317",
			TraceSampleRate: 0.5,
			EnableTraces:    true,
			EnableMetrics:   false,
			EnableLogs:      true,
		}
		WithOTEL(otelCfg)(cfg)

		assertTrue(t, cfg.OTEL.Enabled)
		assertEqual(t, "custom-service", cfg.OTEL.ServiceName)
		assertEqual(t, 0.5, cfg.OTEL.TraceSampleRate)
		assertTrue(t, !cfg.OTEL.EnableMetrics)
	})

	t.Run("WithLogger sets custom logger", func(t *testing.T) {
		cfg := DefaultConfig()
		logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
		WithLogger(logger)(cfg)

		assertEqual(t, logger, cfg.Logger)
	})

	t.Run("WithOpenAPIServers sets servers", func(t *testing.T) {
		cfg := DefaultConfig()
		WithOpenAPIServers(
			OpenAPIServer{URL: "http://localhost:8080", Description: "Local"},
			OpenAPIServer{URL: "https://api.example.com", Description: "Production"},
		)(cfg)

		assertEqual(t, 2, len(cfg.OpenAPI.Servers))
		assertEqual(t, "http://localhost:8080", cfg.OpenAPI.Servers[0].URL)
		assertEqual(t, "https://api.example.com", cfg.OpenAPI.Servers[1].URL)
	})
}

func TestEnvHelpers(t *testing.T) {
	t.Run("getEnv returns default when env not set", func(t *testing.T) {
		result := getEnv("VOLT_TEST_UNSET_VAR", "default")
		assertEqual(t, "default", result)
	})

	t.Run("getEnv returns env value when set", func(t *testing.T) {
		os.Setenv("VOLT_TEST_VAR", "custom")
		defer os.Unsetenv("VOLT_TEST_VAR")

		result := getEnv("VOLT_TEST_VAR", "default")
		assertEqual(t, "custom", result)
	})

	t.Run("getEnvInt returns default when env not set", func(t *testing.T) {
		result := getEnvInt("VOLT_TEST_UNSET_INT", 42)
		assertEqual(t, 42, result)
	})

	t.Run("getEnvInt returns parsed int when set", func(t *testing.T) {
		os.Setenv("VOLT_TEST_INT", "100")
		defer os.Unsetenv("VOLT_TEST_INT")

		result := getEnvInt("VOLT_TEST_INT", 42)
		assertEqual(t, 100, result)
	})

	t.Run("getEnvInt returns default for invalid int", func(t *testing.T) {
		os.Setenv("VOLT_TEST_INVALID_INT", "not-a-number")
		defer os.Unsetenv("VOLT_TEST_INVALID_INT")

		result := getEnvInt("VOLT_TEST_INVALID_INT", 42)
		assertEqual(t, 42, result)
	})

	t.Run("getEnvBool returns default when env not set", func(t *testing.T) {
		result := getEnvBool("VOLT_TEST_UNSET_BOOL", true)
		assertTrue(t, result)
	})

	t.Run("getEnvBool parses true values", func(t *testing.T) {
		trueValues := []string{"true", "1", "yes"}
		for _, v := range trueValues {
			os.Setenv("VOLT_TEST_BOOL", v)
			result := getEnvBool("VOLT_TEST_BOOL", false)
			assertTrue(t, result)
		}
		os.Unsetenv("VOLT_TEST_BOOL")
	})

	t.Run("getEnvBool parses false values", func(t *testing.T) {
		os.Setenv("VOLT_TEST_BOOL", "false")
		defer os.Unsetenv("VOLT_TEST_BOOL")

		result := getEnvBool("VOLT_TEST_BOOL", true)
		assertTrue(t, !result)
	})
}
