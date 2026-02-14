package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vntrieu/avalon/internal/ratelimit"
)

// denyAllLimiter denies every request (for testing 429).
type denyAllLimiter struct{}

func (denyAllLimiter) Allow(key string) (bool, int) { return false, 60 }

func TestRateLimitMiddleware_Returns429WhenDenied(t *testing.T) {
	var lim ratelimit.Limiter = denyAllLimiter{}
	handler := RateLimitMiddleware(lim, RateLimitKeyByIP)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") != "60" {
		t.Errorf("expected Retry-After 60, got %q", w.Header().Get("Retry-After"))
	}
}

func TestRateLimitMiddleware_ProxiesWhenAllowed(t *testing.T) {
	handler := RateLimitMiddleware(&ratelimit.Noop{}, RateLimitKeyByIP)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Errorf("expected body ok, got %q", w.Body.String())
	}
}
