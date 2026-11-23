package config

import (
	"os"
	"testing"
	"time"
)

func TestConfigLoadDefaults(t *testing.T) {
	// Clear env vars
	os.Unsetenv("BACKEND_SERVICE_URL")
	os.Unsetenv("IDENTITY_SERVICE_URL")
	os.Unsetenv("PORT")
	os.Unsetenv("ENVIRONMENT")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error loading default config, got %v", err)
	}

	if cfg.Port != 8000 {
		t.Errorf("expected default port 8000, got %d", cfg.Port)
	}
	if cfg.Environment != "development" {
		t.Errorf("expected default environment development, got %s", cfg.Environment)
	}
	if cfg.BackendServiceURL.String() != "http://localhost:8002" {
		t.Errorf("expected default backend URL http://localhost:8002, got %s", cfg.BackendServiceURL.String())
	}
	if cfg.IdentityServiceURL.String() != "https://vm-id.bobslab.xyz" {
		t.Errorf("expected default identity URL https://vm-id.bobslab.xyz, got %s", cfg.IdentityServiceURL.String())
	}
	if cfg.ReadTimeout != 15*time.Second {
		t.Errorf("expected 15s read timeout, got %v", cfg.ReadTimeout)
	}
}

func TestConfigLoadCustom(t *testing.T) {
	t.Setenv("BACKEND_SERVICE_URL", "http://backend:9000")
	t.Setenv("IDENTITY_SERVICE_URL", "https://identity:9001")
	t.Setenv("PORT", "8080")
	t.Setenv("ENVIRONMENT", "staging")
	t.Setenv("RATE_LIMIT_ENABLED", "false")
	t.Setenv("NEW_RELIC_ENABLED", "true")
	t.Setenv("NEW_RELIC_LICENSE_KEY", "test-key")
	t.Setenv("NEW_RELIC_APP_NAME", "cognito-gateway-staging")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error loading custom config, got %v", err)
	}

	if cfg.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Port)
	}
	if cfg.Environment != "staging" {
		t.Errorf("expected environment staging, got %s", cfg.Environment)
	}
	if cfg.BackendServiceURL.String() != "http://backend:9000" {
		t.Errorf("expected backend URL http://backend:9000, got %s", cfg.BackendServiceURL.String())
	}
	if cfg.RateLimitEnabled {
		t.Errorf("expected rate limit disabled, got true")
	}
	if !cfg.NewRelicEnabled {
		t.Errorf("expected New Relic enabled, got false")
	}
}
