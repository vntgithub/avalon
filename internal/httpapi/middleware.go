package httpapi

import (
	"net/http"
	"strconv"

	"github.com/vntrieu/avalon/internal/ratelimit"
)

// RateLimitMiddleware returns a middleware that limits by key extracted from the request (e.g. IP).
// When over limit, responds with 429 and optional Retry-After header.
func RateLimitMiddleware(limiter ratelimit.Limiter, keyFunc func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyFunc(r)
			if key == "" {
				key = "unknown"
			}
			allowed, retryAfter := limiter.Allow(key)
			if !allowed {
				if retryAfter > 0 {
					w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				}
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RateLimitKeyByIP returns the client IP from the request (using X-Real-IP / X-Forwarded-For when set).
func RateLimitKeyByIP(r *http.Request) string {
	if x := r.Header.Get("X-Real-IP"); x != "" {
		return x
	}
	if x := r.Header.Get("X-Forwarded-For"); x != "" {
		return x
	}
	return r.RemoteAddr
}

// MaxBytesReader wraps the request body with a limit so decode does not read more than maxBytes.
// Use for JSON endpoints to prevent large payloads. Call before decoding body.
const DefaultMaxBodyBytes = 1 << 20 // 1MB

// LimitRequestBody returns middleware that limits request body size; over-size requests get 413.
func LimitRequestBody(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
