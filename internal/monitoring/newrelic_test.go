package monitoring

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"cognito-api-gateway/internal/config"
)

func TestInitNewRelicDisabled(t *testing.T) {
	cfg := &config.Config{
		NewRelicEnabled: false,
	}
	app, err := InitNewRelic(cfg)
	if err != nil {
		t.Fatalf("unexpected error with disabled New Relic: %v", err)
	}
	if app != nil {
		t.Errorf("expected nil app when disabled")
	}
}

func TestLogWriter(t *testing.T) {
	buf := &bytes.Buffer{}
	writer := &LogWriter{
		Output: buf,
		App:    nil,
	}

	testData := []byte("test log message\n")
	n, err := writer.Write(testData)
	if err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}
	if n != len(testData) {
		t.Errorf("expected %d bytes written, got %d", len(testData), n)
	}
	if buf.String() != string(testData) {
		t.Errorf("expected %s, got %s", string(testData), buf.String())
	}
}

func TestProbeEndpointFiltering(t *testing.T) {
	probes := []string{
		"/health",
		"/healthz",
		"/ready",
		"/health/identity",
		"/ready/backend",
	}

	for _, p := range probes {
		if !isProbeEndpoint(p) {
			t.Errorf("expected %s to be classified as probe endpoint", p)
		}
	}

	nonProbes := []string{
		"/",
		"/agent/chat",
		"/agent/sessions",
		"/api/v1/auth/login",
	}

	for _, np := range nonProbes {
		if isProbeEndpoint(np) {
			t.Errorf("expected %s NOT to be classified as probe endpoint", np)
		}
	}
}

func TestMiddlewareNoApp(t *testing.T) {
	mw := Middleware(nil, "test")
	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("expected downstream handler to be called when New Relic app is nil")
	}
}
