package handler

import (
	"net/http"
)

// healthResponse is the JSON body for GET /healthz.
type healthResponse struct {
	Status string `json:"status"`
}

// Healthz handles GET /healthz.
//
// @Summary      Health check
// @Description  Liveness/readiness check. No authentication required.
// @Tags         health
// @Produce      json
// @Success      200  {object}  healthResponse
// @Router       /healthz [get]
func Healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
