package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"cognito-api-gateway/internal/config"
)

func TestServerRouting(t *testing.T) {
	backendCalled := false
	identityCalled := false

	backendMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendCalled = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"backend":"ok"}`))
	}))
	defer backendMock.Close()

	identityMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identityCalled = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"identity":"ok"}`))
	}))
	defer identityMock.Close()

	bURL, _ := url.Parse(backendMock.URL)
	iURL, _ := url.Parse(identityMock.URL)

	cfg := &config.Config{
		Environment:        "test",
		Port:               8000,
		BackendServiceURL:  bURL,
		IdentityServiceURL: iURL,
		RateLimitEnabled:   false,
	}

	srv := New(cfg)
	defer srv.CloseIdleConnections()

	// 1. Test native /health
	reqHealth := httptest.NewRequest(http.MethodGet, "/health", nil)
	recHealth := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recHealth, reqHealth)

	if recHealth.Code != http.StatusOK {
		t.Errorf("expected 200 OK from /health, got %d", recHealth.Code)
	}

	// 2. Test native /
	reqRoot := httptest.NewRequest(http.MethodGet, "/", nil)
	recRoot := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recRoot, reqRoot)

	if recRoot.Code != http.StatusOK {
		t.Errorf("expected 200 OK from /, got %d", recRoot.Code)
	}
	var rootData map[string]any
	if err := json.Unmarshal(recRoot.Body.Bytes(), &rootData); err != nil {
		t.Fatalf("failed to parse / response: %v", err)
	}
	if rootData["service"] != "cognito-api-gateway" {
		t.Errorf("expected cognito-api-gateway service name, got %v", rootData["service"])
	}

	// 3. Test routing to backend: /agent/chat
	reqAgent := httptest.NewRequest(http.MethodPost, "/agent/chat", nil)
	recAgent := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recAgent, reqAgent)

	if !backendCalled {
		t.Error("expected backendMock to be called for /agent/chat")
	}

	// 4. Test routing to identity: /.well-known/jwks.json
	reqIdentity := httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil)
	recIdentity := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recIdentity, reqIdentity)

	if !identityCalled {
		t.Error("expected identityMock to be called for /.well-known/jwks.json")
	}
}
