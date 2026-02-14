package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

// contextKey type for request context keys (avoids collisions with other packages).
type contextKey string

// UserIDContextKey is the context key for the authenticated user's ID (set by OptionalUser/RequireUser middleware).
const UserIDContextKey contextKey = "user_id"

// UserIDFromRequest returns the user ID from the request context if set by user auth middleware; otherwise empty.
func UserIDFromRequest(r *http.Request) *string {
	v := r.Context().Value(UserIDContextKey)
	if v == nil {
		return nil
	}
	if id, ok := v.(string); ok && id != "" {
		return &id
	}
	return nil
}

// requestID returns the request ID from chi's context for logging.
func requestID(r *http.Request) string {
	if id, ok := r.Context().Value(middleware.RequestIDKey).(string); ok {
		return id
	}
	return ""
}
