package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"cognito-api-gateway/internal/config"
)

func TestProbeManager(t *testing.T) {
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer backendServer.Close()

	identityServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/jwks.json" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"keys":[]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer identityServer.Close()

	bURL, _ := url.Parse(backendServer.URL)
	iURL, _ := url.Parse(identityServer.URL)

	pm := NewProbeManager(&config.Config{}, bURL, iURL)

	results, allHealthy := pm.CheckAll(context.Background())
	if !allHealthy {
		t.Fatalf("expected all upstreams healthy, got results: %+v", results)
	}

	if results["backend"].Status != "healthy" {
		t.Errorf("expected backend healthy, got %s", results["backend"].Status)
	}
	if results["identity"].Status != "healthy" {
		t.Errorf("expected identity healthy, got %s", results["identity"].Status)
	}
}
