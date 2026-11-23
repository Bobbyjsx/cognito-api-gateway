package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"cognito-api-gateway/internal/config"
)

func TestUpstreamProxyForwarding(t *testing.T) {
	upstreamHit := false
	var receivedPath string
	var receivedAuth string

	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHit = true
		receivedPath = r.URL.Path
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Access-Control-Allow-Origin", "https://upstream.internal")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer upstreamServer.Close()

	targetURL, _ := url.Parse(upstreamServer.URL)
	cfg := &config.Config{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 5,
		IdleConnTimeout:     30 * time.Second,
		UpstreamTimeout:     5 * time.Second,
	}

	proxy := NewUpstreamProxy("test-upstream", targetURL, cfg, NewBufferPool())
	defer proxy.CloseIdleConnections()

	req := httptest.NewRequest(http.MethodGet, "/agent/chat", nil)
	req.Header.Set("Authorization", "Bearer test-jwt-token")
	rec := httptest.NewRecorder()

	proxy.ServeHTTP(rec, req)

	if !upstreamHit {
		t.Fatal("expected upstream to receive request")
	}
	if receivedPath != "/agent/chat" {
		t.Errorf("expected /agent/chat, got %s", receivedPath)
	}
	if receivedAuth != "Bearer test-jwt-token" {
		t.Errorf("expected Bearer test-jwt-token, got %s", receivedAuth)
	}
	// Upstream CORS header should have been stripped
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("expected upstream Access-Control-Allow-Origin to be stripped, got %s", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestUpstreamProxyError(t *testing.T) {
	// Point to unreachable port
	unreachableURL, _ := url.Parse("http://127.0.0.1:54321")
	cfg := &config.Config{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 5,
		IdleConnTimeout:     30 * time.Second,
		UpstreamTimeout:     100 * time.Millisecond,
	}

	proxy := NewUpstreamProxy("unreachable", unreachableURL, cfg, NewBufferPool())
	defer proxy.CloseIdleConnections()

	req := httptest.NewRequest(http.MethodGet, "/agent/sessions", nil)
	rec := httptest.NewRecorder()

	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("expected status 502 Bad Gateway, got %d", rec.Code)
	}

	var errResp ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to decode JSON error response: %v", err)
	}
	if errResp.Error != "Bad Gateway" {
		t.Errorf("expected Bad Gateway, got %s", errResp.Error)
	}
}
