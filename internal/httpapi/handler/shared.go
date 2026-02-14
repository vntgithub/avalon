package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

// requestID returns the request ID from chi's context for logging.
func requestID(r *http.Request) string {
	if id, ok := r.Context().Value(middleware.RequestIDKey).(string); ok {
		return id
	}
	return ""
}
