package volt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSConfig(t *testing.T) {
	t.Run("DefaultCORSConfig has permissive defaults", func(t *testing.T) {
		cfg := DefaultCORSConfig()

		assertEqual(t, 1, len(cfg.AllowOrigins))
		assertEqual(t, "*", cfg.AllowOrigins[0])
		assertTrue(t, len(cfg.AllowMethods) > 0)
		assertTrue(t, len(cfg.AllowHeaders) > 0)
		assertEqual(t, 86400, cfg.MaxAge)
	})
}

func TestSecureHeaders(t *testing.T) {
	t.Run("adds security headers", func(t *testing.T) {
		handler := SecureHeaders()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assertEqual(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
		assertEqual(t, "DENY", rec.Header().Get("X-Frame-Options"))
		assertEqual(t, "1; mode=block", rec.Header().Get("X-XSS-Protection"))
		assertEqual(t, "strict-origin-when-cross-origin", rec.Header().Get("Referrer-Policy"))
	})
}

func TestMaxBodySize(t *testing.T) {
	t.Run("wraps request body with size limit", func(t *testing.T) {
		var bodyReader http.Handler
		handler := MaxBodySize(1024)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			bodyReader = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
			_ = bodyReader
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("POST", "/", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assertEqual(t, http.StatusOK, rec.Code)
	})
}

func TestCacheControl(t *testing.T) {
	t.Run("sets cache control header", func(t *testing.T) {
		handler := CacheControl("max-age=3600")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assertEqual(t, "max-age=3600", rec.Header().Get("Cache-Control"))
	})
}

func TestNoCache(t *testing.T) {
	t.Run("sets no-cache headers", func(t *testing.T) {
		handler := NoCache()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assertEqual(t, "no-cache, no-store, must-revalidate", rec.Header().Get("Cache-Control"))
	})
}

func TestAuthConfig(t *testing.T) {
	t.Run("has correct default values", func(t *testing.T) {
		cfg := AuthConfig{}

		// Defaults are set in the Auth function, not in the struct
		// This test documents expected behavior
		assertEqual(t, "", cfg.Header)
		assertEqual(t, "", cfg.Prefix)
	})
}

func TestRateLimitConfig(t *testing.T) {
	t.Run("InMemoryRateLimitStore allows requests", func(t *testing.T) {
		store := &InMemoryRateLimitStore{}

		allowed, remaining, _ := store.Allow("test-key", 10, 0)

		assertTrue(t, allowed)
		assertEqual(t, 10, remaining)
	})
}

// =============================================================================
// Integration-style middleware tests
// =============================================================================

func TestMiddlewareChaining(t *testing.T) {
	t.Run("middlewares execute in order", func(t *testing.T) {
		var order []string

		mw1 := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "mw1-before")
				next.ServeHTTP(w, r)
				order = append(order, "mw1-after")
			})
		}

		mw2 := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "mw2-before")
				next.ServeHTTP(w, r)
				order = append(order, "mw2-after")
			})
		}

		handler := mw1(mw2(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "handler")
		})))

		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assertEqual(t, 5, len(order))
		assertEqual(t, "mw1-before", order[0])
		assertEqual(t, "mw2-before", order[1])
		assertEqual(t, "handler", order[2])
		assertEqual(t, "mw2-after", order[3])
		assertEqual(t, "mw1-after", order[4])
	})
}

func TestCORSMiddleware(t *testing.T) {
	t.Run("handles preflight OPTIONS request", func(t *testing.T) {
		cfg := CORSConfig{
			AllowOrigins: []string{"https://example.com"},
			AllowMethods: []string{"GET", "POST"},
			AllowHeaders: []string{"Content-Type"},
			MaxAge:       3600,
		}

		handlerCalled := false
		handler := CORS(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
		}))

		req := httptest.NewRequest("OPTIONS", "/", nil)
		req.Header.Set("Origin", "https://example.com")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assertEqual(t, http.StatusNoContent, rec.Code)
		assertEqual(t, "https://example.com", rec.Header().Get("Access-Control-Allow-Origin"))
		assertTrue(t, !handlerCalled) // Preflight should not call handler
	})

	t.Run("passes through non-preflight requests", func(t *testing.T) {
		cfg := CORSConfig{
			AllowOrigins: []string{"https://example.com"},
		}

		handlerCalled := false
		handler := CORS(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Origin", "https://example.com")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assertEqual(t, http.StatusOK, rec.Code)
		assertTrue(t, handlerCalled)
		assertEqual(t, "https://example.com", rec.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("handles wildcard origin", func(t *testing.T) {
		cfg := CORSConfig{
			AllowOrigins: []string{"*"},
		}

		handler := CORS(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Origin", "https://any-origin.com")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assertEqual(t, "https://any-origin.com", rec.Header().Get("Access-Control-Allow-Origin"))
	})
}

func TestAuthMiddleware(t *testing.T) {
	t.Run("skips authentication for skip paths", func(t *testing.T) {
		cfg := AuthConfig{
			Header:    "Authorization",
			Prefix:    "Bearer ",
			SkipPaths: []string{"/health", "/docs"},
			Validator: func(ctx context.Context, token string) (any, error) {
				t.Error("validator should not be called for skip paths")
				return nil, nil
			},
		}

		handlerCalled := false
		handler := Auth(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/health", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assertEqual(t, http.StatusOK, rec.Code)
		assertTrue(t, handlerCalled)
	})

	t.Run("returns 401 when authorization header missing", func(t *testing.T) {
		cfg := AuthConfig{
			Validator: func(ctx context.Context, token string) (any, error) {
				return nil, nil
			},
		}

		handler := Auth(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("handler should not be called")
		}))

		req := httptest.NewRequest("GET", "/protected", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assertEqual(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("returns 401 for invalid token format", func(t *testing.T) {
		cfg := AuthConfig{
			Header: "Authorization",
			Prefix: "Bearer ",
			Validator: func(ctx context.Context, token string) (any, error) {
				return nil, nil
			},
		}

		handler := Auth(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("handler should not be called")
		}))

		req := httptest.NewRequest("GET", "/protected", nil)
		req.Header.Set("Authorization", "Basic dXNlcjpwYXNz") // Wrong prefix
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assertEqual(t, http.StatusUnauthorized, rec.Code)
	})
}
