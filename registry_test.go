package volt

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestRegistry(t *testing.T) {
	t.Run("Register and Get service", func(t *testing.T) {
		r := NewRegistry()

		type MyService struct{ Name string }
		svc := &MyService{Name: "test"}

		r.Register("myservice", svc, nil)

		got, ok := r.Get("myservice")
		assertTrue(t, ok)
		assertEqual(t, svc, got.(*MyService))
	})

	t.Run("Get returns false for unknown service", func(t *testing.T) {
		r := NewRegistry()

		_, ok := r.Get("unknown")
		assertTrue(t, !ok)
	})

	t.Run("MustGet panics for unknown service", func(t *testing.T) {
		r := NewRegistry()

		defer func() {
			if recover() == nil {
				t.Error("expected panic")
			}
		}()

		r.MustGet("unknown")
	})

	t.Run("MustGet returns service when found", func(t *testing.T) {
		r := NewRegistry()
		r.Register("test", "value", nil)

		got := r.MustGet("test")
		assertEqual(t, "value", got.(string))
	})

	t.Run("Shutdown calls shutdown functions in reverse order", func(t *testing.T) {
		r := NewRegistry()
		var order []string

		r.Register("first", "a", func(ctx context.Context) error {
			order = append(order, "first")
			return nil
		})
		r.Register("second", "b", func(ctx context.Context) error {
			order = append(order, "second")
			return nil
		})
		r.Register("third", "c", func(ctx context.Context) error {
			order = append(order, "third")
			return nil
		})

		err := r.Shutdown(context.Background())

		assertNil(t, err)
		assertEqual(t, 3, len(order))
		assertEqual(t, "third", order[0])
		assertEqual(t, "second", order[1])
		assertEqual(t, "first", order[2])
	})
}

func TestHTTPServiceConfig(t *testing.T) {
	t.Run("DefaultHTTPServiceConfig has sensible defaults", func(t *testing.T) {
		cfg := DefaultHTTPServiceConfig()

		assertEqual(t, 30*time.Second, cfg.Timeout)
		assertEqual(t, 3, cfg.MaxRetries)
		assertEqual(t, 100, cfg.MaxIdleConns)
		assertEqual(t, 10, cfg.MaxIdleConnsPerHost)
		assertTrue(t, len(cfg.RetryOnStatus) > 0)
	})

	t.Run("WithHTTPTimeout sets timeout", func(t *testing.T) {
		cfg := DefaultHTTPServiceConfig()
		WithHTTPTimeout(60 * time.Second)(&cfg)

		assertEqual(t, 60*time.Second, cfg.Timeout)
	})

	t.Run("WithHTTPRetries configures retry behavior", func(t *testing.T) {
		cfg := DefaultHTTPServiceConfig()
		WithHTTPRetries(5, 200*time.Millisecond, 5*time.Second)(&cfg)

		assertEqual(t, 5, cfg.MaxRetries)
		assertEqual(t, 200*time.Millisecond, cfg.RetryWaitMin)
		assertEqual(t, 5*time.Second, cfg.RetryWaitMax)
	})

	t.Run("WithHTTPHeaders sets default headers", func(t *testing.T) {
		cfg := DefaultHTTPServiceConfig()
		headers := map[string]string{"Authorization": "Bearer token"}
		WithHTTPHeaders(headers)(&cfg)

		assertEqual(t, "Bearer token", cfg.DefaultHeaders["Authorization"])
	})

	t.Run("WithHTTPBaseURL sets base URL", func(t *testing.T) {
		cfg := DefaultHTTPServiceConfig()
		WithHTTPBaseURL("https://api.example.com")(&cfg)

		assertEqual(t, "https://api.example.com", cfg.BaseURL)
	})
}

func TestDatabaseConfig(t *testing.T) {
	t.Run("DefaultDatabaseConfig has sensible defaults", func(t *testing.T) {
		cfg := DefaultDatabaseConfig()

		assertEqual(t, 25, cfg.MaxOpenConns)
		assertEqual(t, 10, cfg.MaxIdleConns)
		assertEqual(t, 5*time.Minute, cfg.ConnMaxLifetime)
		assertEqual(t, 5*time.Minute, cfg.ConnMaxIdleTime)
	})
}

func TestRetryRoundTripper(t *testing.T) {
	t.Run("retries on configured status codes", func(t *testing.T) {
		attempts := 0
		transport := &mockRoundTripper{
			roundTripFn: func(req *http.Request) (*http.Response, error) {
				attempts++
				if attempts < 3 {
					return &http.Response{StatusCode: 503}, nil
				}
				return &http.Response{StatusCode: 200}, nil
			},
		}

		rt := &retryRoundTripper{
			base:       transport,
			maxRetries: 3,
			minWait:    1 * time.Millisecond,
			maxWait:    10 * time.Millisecond,
			retryOn:    []int{503},
		}

		req, _ := http.NewRequest("GET", "http://test", nil)
		resp, err := rt.RoundTrip(req)

		assertNil(t, err)
		assertEqual(t, 200, resp.StatusCode)
		assertEqual(t, 3, attempts)
	})

	t.Run("does not retry on success", func(t *testing.T) {
		attempts := 0
		transport := &mockRoundTripper{
			roundTripFn: func(req *http.Request) (*http.Response, error) {
				attempts++
				return &http.Response{StatusCode: 200}, nil
			},
		}

		rt := &retryRoundTripper{
			base:       transport,
			maxRetries: 3,
			minWait:    1 * time.Millisecond,
			maxWait:    10 * time.Millisecond,
			retryOn:    []int{503},
		}

		req, _ := http.NewRequest("GET", "http://test", nil)
		resp, err := rt.RoundTrip(req)

		assertNil(t, err)
		assertEqual(t, 200, resp.StatusCode)
		assertEqual(t, 1, attempts)
	})

	t.Run("stops after max retries", func(t *testing.T) {
		attempts := 0
		transport := &mockRoundTripper{
			roundTripFn: func(req *http.Request) (*http.Response, error) {
				attempts++
				return &http.Response{StatusCode: 503}, nil
			},
		}

		rt := &retryRoundTripper{
			base:       transport,
			maxRetries: 2,
			minWait:    1 * time.Millisecond,
			maxWait:    10 * time.Millisecond,
			retryOn:    []int{503},
		}

		req, _ := http.NewRequest("GET", "http://test", nil)
		resp, _ := rt.RoundTrip(req)

		assertEqual(t, 503, resp.StatusCode)
		assertEqual(t, 3, attempts) // initial + 2 retries
	})
}

func TestHeaderRoundTripper(t *testing.T) {
	t.Run("adds default headers", func(t *testing.T) {
		var capturedReq *http.Request
		transport := &mockRoundTripper{
			roundTripFn: func(req *http.Request) (*http.Response, error) {
				capturedReq = req
				return &http.Response{StatusCode: 200}, nil
			},
		}

		rt := &headerRoundTripper{
			base: transport,
			headers: map[string]string{
				"X-Custom-Header": "custom-value",
				"Authorization":   "Bearer default",
			},
		}

		req, _ := http.NewRequest("GET", "http://test", nil)
		_, err := rt.RoundTrip(req)

		assertNil(t, err)
		assertEqual(t, "custom-value", capturedReq.Header.Get("X-Custom-Header"))
		assertEqual(t, "Bearer default", capturedReq.Header.Get("Authorization"))
	})

	t.Run("does not override existing headers", func(t *testing.T) {
		var capturedReq *http.Request
		transport := &mockRoundTripper{
			roundTripFn: func(req *http.Request) (*http.Response, error) {
				capturedReq = req
				return &http.Response{StatusCode: 200}, nil
			},
		}

		rt := &headerRoundTripper{
			base: transport,
			headers: map[string]string{
				"Authorization": "Bearer default",
			},
		}

		req, _ := http.NewRequest("GET", "http://test", nil)
		req.Header.Set("Authorization", "Bearer override")
		_, err := rt.RoundTrip(req)

		assertNil(t, err)
		assertEqual(t, "Bearer override", capturedReq.Header.Get("Authorization"))
	})
}

// =============================================================================
// Mocks
// =============================================================================

type mockRoundTripper struct {
	roundTripFn func(req *http.Request) (*http.Response, error)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTripFn(req)
}
