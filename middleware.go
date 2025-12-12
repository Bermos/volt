package volt

import (
	"context"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// --- Authentication Middleware ---

// AuthConfig configures authentication middleware.
type AuthConfig struct {
	// Header to read token from (default: Authorization)
	Header string

	// Prefix to strip from header value (default: Bearer )
	Prefix string

	// Skip authentication for these paths
	SkipPaths []string

	// Validator function - returns user info or error
	Validator func(ctx context.Context, token string) (any, error)
}

// Auth creates an authentication middleware.
func Auth(config AuthConfig) func(http.Handler) http.Handler {
	if config.Header == "" {
		config.Header = "Authorization"
	}
	if config.Prefix == "" {
		config.Prefix = "Bearer "
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check skip paths
			for _, path := range config.SkipPaths {
				if strings.HasPrefix(r.URL.Path, path) {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Get token from header
			authHeader := r.Header.Get(config.Header)
			if authHeader == "" {
				http.Error(w, "missing authorization header", http.StatusUnauthorized)
				return
			}

			// Strip prefix
			token := strings.TrimPrefix(authHeader, config.Prefix)
			if token == authHeader {
				http.Error(w, "invalid authorization format", http.StatusUnauthorized)
				return
			}

			// Validate token
			user, err := config.Validator(r.Context(), token)
			if err != nil {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			// Add user to context
			ctx := WithUser(r.Context(), user)

			// Add to span attributes
			span := trace.SpanFromContext(ctx)
			if span.IsRecording() {
				// Add user ID if available (adjust based on your user type)
				span.SetAttributes(attribute.String("user.authenticated", "true"))
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// --- CORS Middleware ---

// CORSConfig configures CORS middleware.
type CORSConfig struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	ExposeHeaders    []string
	AllowCredentials bool
	MaxAge           int
}

// DefaultCORSConfig returns a permissive CORS config for development.
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
		MaxAge:       86400,
	}
}

// CORS creates a CORS middleware.
func CORS(config CORSConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			allowed := false
			for _, o := range config.AllowOrigins {
				if o == "*" || o == origin {
					allowed = true
					break
				}
			}

			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}

			if config.AllowCredentials {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			// Preflight request
			if r.Method == "OPTIONS" {
				w.Header().Set("Access-Control-Allow-Methods", strings.Join(config.AllowMethods, ", "))
				w.Header().Set("Access-Control-Allow-Headers", strings.Join(config.AllowHeaders, ", "))
				if config.MaxAge > 0 {
					w.Header().Set("Access-Control-Max-Age", string(rune(config.MaxAge)))
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			if len(config.ExposeHeaders) > 0 {
				w.Header().Set("Access-Control-Expose-Headers", strings.Join(config.ExposeHeaders, ", "))
			}

			next.ServeHTTP(w, r)
		})
	}
}

// --- Rate Limiting Middleware ---

// RateLimitConfig configures rate limiting.
type RateLimitConfig struct {
	// Requests per window
	Requests int

	// Time window
	Window time.Duration

	// Key extractor (default: IP address)
	KeyFunc func(r *http.Request) string

	// Store for rate limit state (default: in-memory)
	Store RateLimitStore
}

// RateLimitStore interface for rate limit state storage.
type RateLimitStore interface {
	// Allow checks if request is allowed and increments counter
	Allow(key string, limit int, window time.Duration) (allowed bool, remaining int, resetAt time.Time)
}

// InMemoryRateLimitStore is a simple in-memory rate limit store.
type InMemoryRateLimitStore struct {
	// Implementation would use sync.Map with cleanup
}

func (s *InMemoryRateLimitStore) Allow(key string, limit int, window time.Duration) (bool, int, time.Time) {
	// Simplified - real implementation would track per-key counts
	return true, limit, time.Now().Add(window)
}

// RateLimit creates a rate limiting middleware.
func RateLimit(config RateLimitConfig) func(http.Handler) http.Handler {
	if config.KeyFunc == nil {
		config.KeyFunc = func(r *http.Request) string {
			return r.RemoteAddr
		}
	}
	if config.Store == nil {
		config.Store = &InMemoryRateLimitStore{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := config.KeyFunc(r)
			allowed, remaining, resetAt := config.Store.Allow(key, config.Requests, config.Window)

			w.Header().Set("X-RateLimit-Limit", string(rune(config.Requests)))
			w.Header().Set("X-RateLimit-Remaining", string(rune(remaining)))
			w.Header().Set("X-RateLimit-Reset", resetAt.Format(time.RFC3339))

			if !allowed {
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// --- Security Headers Middleware ---

// SecureHeaders adds security headers to responses.
func SecureHeaders() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Prevent MIME type sniffing
			w.Header().Set("X-Content-Type-Options", "nosniff")

			// Prevent clickjacking
			w.Header().Set("X-Frame-Options", "DENY")

			// XSS protection
			w.Header().Set("X-XSS-Protection", "1; mode=block")

			// Referrer policy
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

			// Content Security Policy (customize as needed)
			// w.Header().Set("Content-Security-Policy", "default-src 'self'")

			next.ServeHTTP(w, r)
		})
	}
}

// --- Request Size Limit Middleware ---

// MaxBodySize limits the maximum request body size.
func MaxBodySize(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// --- Compression Middleware ---
// Note: For compression, use github.com/go-chi/chi/v5/middleware.Compress
// or github.com/klauspost/compress/gzhttp for better performance

// --- Cache Control Middleware ---

// CacheControl sets cache control headers.
func CacheControl(directive string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", directive)
			next.ServeHTTP(w, r)
		})
	}
}

// NoCache is a convenience middleware that disables caching.
func NoCache() func(http.Handler) http.Handler {
	return CacheControl("no-cache, no-store, must-revalidate")
}
