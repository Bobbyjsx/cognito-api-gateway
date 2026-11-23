package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/newrelic/go-agent/v3/newrelic"

	"cognito-api-gateway/internal/config"
	"cognito-api-gateway/internal/middleware"
	"cognito-api-gateway/internal/monitoring"
	"cognito-api-gateway/internal/probe"
	"cognito-api-gateway/internal/proxy"
)

// Server wraps the gateway router, proxies, probe manager, and dependencies.
type Server struct {
	router        chi.Router
	cfg           *config.Config
	backendProxy  *proxy.UpstreamProxy
	identityProxy *proxy.UpstreamProxy
	probeManager  *probe.ProbeManager
	nrApp         *newrelic.Application
}

// New creates and configures the API gateway server.
func New(cfg *config.Config, nrApp ...*newrelic.Application) *Server {
	var app *newrelic.Application
	if len(nrApp) > 0 {
		app = nrApp[0]
	}

	bufPool := proxy.NewBufferPool()

	backendProxy := proxy.NewUpstreamProxy("cognito-backend", cfg.BackendServiceURL, cfg, bufPool)
	identityProxy := proxy.NewUpstreamProxy("identity-service", cfg.IdentityServiceURL, cfg, bufPool)

	rateLimiter := middleware.NewRateLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst, cfg.RateLimitEnabled)
	probeManager := probe.NewProbeManager(cfg, cfg.BackendServiceURL, cfg.IdentityServiceURL)

	r := chi.NewRouter()

	// Global high-performance middleware stack
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(monitoring.Middleware(app, cfg.Environment))
	r.Use(middleware.Logger)
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.CORS(cfg.CORSAllowedOrigins))
	r.Use(rateLimiter.Middleware)

	// Gateway Native Endpoints
	r.Get("/health", handleHealth())
	r.Get("/healthz", handleHealth())
	r.Get("/ready", handleReadiness(cfg, probeManager))
	r.Get("/health/identity", handleIdentityProbe(cfg, probeManager))
	r.Get("/ready/identity", handleIdentityProbe(cfg, probeManager))
	r.Get("/health/backend", handleBackendProbe(cfg, probeManager))
	r.Get("/ready/backend", handleBackendProbe(cfg, probeManager))
	r.Get("/", handleRoot(cfg))

	// Proxy Upstream Routes
	// 1. Centralized Identity Service Routes (Auth, OAuth 2.0 / OIDC discovery)
	r.Handle("/api/v1/oauth/*", identityProxy)
	r.Handle("/api/v1/oauth", identityProxy)
	r.Handle("/api/v1/auth/*", identityProxy)
	r.Handle("/api/v1/auth", identityProxy)
	r.Handle("/.well-known/*", identityProxy)
	r.Handle("/.well-known", identityProxy)

	// 2. Cognito Backend Routes
	// Core AI & Chat Agent surface
	r.Handle("/agent/*", backendProxy)
	r.Handle("/agent", backendProxy)

	// User auth, settings, & config
	r.Handle("/auth/*", backendProxy)
	r.Handle("/auth", backendProxy)
	r.Handle("/config/*", backendProxy)
	r.Handle("/config", backendProxy)

	// Voice / Speech-to-text
	r.Handle("/stt/*", backendProxy)
	r.Handle("/stt", backendProxy)

	// Background Tasks & Workers
	r.Handle("/tasks/*", backendProxy)
	r.Handle("/tasks", backendProxy)

	// Paystack Billing & Webhooks
	r.Handle("/billing/*", backendProxy)
	r.Handle("/billing", backendProxy)
	r.Handle("/webhooks/*", backendProxy)
	r.Handle("/webhooks", backendProxy)

	// Custom Prompts
	r.Handle("/prompts/*", backendProxy)
	r.Handle("/prompts", backendProxy)

	// API Documentation & Schema
	r.Handle("/docs", backendProxy)
	r.Handle("/docs/*", backendProxy)
	r.Handle("/redoc", backendProxy)
	r.Handle("/redoc/*", backendProxy)
	r.Handle("/openapi.json", backendProxy)

	// Catch-all general API routes
	r.Handle("/api/*", backendProxy)

	return &Server{
		router:        r,
		cfg:           cfg,
		backendProxy:  backendProxy,
		identityProxy: identityProxy,
		probeManager:  probeManager,
		nrApp:         app,
	}
}

// Handler returns the http.Handler for the gateway.
func (s *Server) Handler() http.Handler {
	return s.router
}

// CloseIdleConnections gracefully cleans up upstream persistent connection pools.
func (s *Server) CloseIdleConnections() {
	if s.backendProxy != nil {
		s.backendProxy.CloseIdleConnections()
	}
	if s.identityProxy != nil {
		s.identityProxy.CloseIdleConnections()
	}
}

func handleRoot(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"service":     "cognito-api-gateway",
			"description": "Cognito AI - High-performance Unified API Gateway",
			"version":     "0.1.0",
			"environment": cfg.Environment,
			"routes": []string{
				"/health",
				"/ready",
				"/health/identity",
				"/ready/identity",
				"/health/backend",
				"/ready/backend",
				"/api/v1/oauth/*",
				"/api/v1/auth/*",
				"/.well-known/*",
				"/agent/*",
				"/auth/*",
				"/config/*",
				"/stt/*",
				"/tasks/*",
				"/billing/*",
				"/webhooks/*",
				"/prompts/*",
				"/docs",
				"/redoc",
				"/openapi.json",
			},
		})
	}
}
