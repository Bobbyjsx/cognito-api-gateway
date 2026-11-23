package server

import (
	"encoding/json"
	"net/http"
	"time"

	"cognito-api-gateway/internal/config"
	"cognito-api-gateway/internal/probe"
)

func handleHealth() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":    "ok",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func handleReadiness(cfg *config.Config, pm *probe.ProbeManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		results, allHealthy := pm.CheckAll(r.Context())

		statusCode := http.StatusOK
		statusText := "ready"
		if !allHealthy {
			statusCode = http.StatusServiceUnavailable
			statusText = "degraded"
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":      statusText,
			"environment": cfg.Environment,
			"timestamp":   time.Now().UTC().Format(time.RFC3339),
			"upstreams":   results,
		})
	}
}

func handleIdentityProbe(cfg *config.Config, pm *probe.ProbeManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := pm.CheckIdentity(r.Context())
		code := http.StatusOK
		if status.Status != "healthy" {
			code = http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(status)
	}
}

func handleBackendProbe(cfg *config.Config, pm *probe.ProbeManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := pm.CheckBackend(r.Context())
		code := http.StatusOK
		if status.Status != "healthy" {
			code = http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(status)
	}
}
