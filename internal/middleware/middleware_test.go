package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestID(t *testing.T) {
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := GetRequestID(r.Context())
		if reqID == "" {
			t.Error("expected non-empty request ID in context")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	respID := rec.Header().Get(HeaderRequestID)
	if respID == "" {
		t.Error("expected X-Request-ID response header")
	}
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		remoteAddr string
		expectedIP string
	}{
		{
			name:       "Cloudflare Connecting IP",
			headers:    map[string]string{"CF-Connecting-IP": "203.0.113.195"},
			remoteAddr: "172.18.0.2:45678",
			expectedIP: "203.0.113.195",
		},
		{
			name:       "X-Real-IP fallback",
			headers:    map[string]string{"X-Real-IP": "198.51.100.42"},
			remoteAddr: "172.18.0.2:45678",
			expectedIP: "198.51.100.42",
		},
		{
			name:       "X-Forwarded-For chain",
			headers:    map[string]string{"X-Forwarded-For": "192.0.2.1, 10.0.0.1"},
			remoteAddr: "172.18.0.2:45678",
			expectedIP: "192.0.2.1",
		},
		{
			name:       "RemoteAddr fallback",
			headers:    map[string]string{},
			remoteAddr: "198.51.100.99:12345",
			expectedIP: "198.51.100.99",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.RemoteAddr = tc.remoteAddr
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			ip := GetClientIP(req)
			if ip != tc.expectedIP {
				t.Errorf("expected client IP %s, got %s", tc.expectedIP, ip)
			}
		})
	}
}

func TestCORS(t *testing.T) {
	corsMiddleware := CORS([]string{"https://cognito.bobslab.xyz", "http://localhost:3000", "*.bobslab.xyz"})
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Preflight OPTIONS test with custom headers
	reqOptions := httptest.NewRequest(http.MethodOptions, "/test", nil)
	reqOptions.Header.Set("Origin", "https://cognito.bobslab.xyz")
	reqOptions.Header.Set("Access-Control-Request-Method", "GET")
	reqOptions.Header.Set("Access-Control-Request-Headers", "authorization,x-refresh-token")
	recOptions := httptest.NewRecorder()
	handler.ServeHTTP(recOptions, reqOptions)

	if recOptions.Code != http.StatusNoContent {
		t.Errorf("expected 204 No Content for OPTIONS, got %d", recOptions.Code)
	}
	if recOptions.Header().Get("Access-Control-Allow-Origin") != "https://cognito.bobslab.xyz" {
		t.Errorf("expected reflected origin, got %s", recOptions.Header().Get("Access-Control-Allow-Origin"))
	}
	if recOptions.Header().Get("Vary") != "Origin" {
		t.Errorf("expected Vary: Origin, got %s", recOptions.Header().Get("Vary"))
	}
	allowHeaders := recOptions.Header().Get("Access-Control-Allow-Headers")
	if !strings.Contains(allowHeaders, "X-Refresh-Token") || !strings.Contains(allowHeaders, "x-refresh-token") {
		t.Errorf("expected X-Refresh-Token in allow-headers, got %s", allowHeaders)
	}

	// Subdomain wildcard matching
	reqWildcard := httptest.NewRequest(http.MethodGet, "/test", nil)
	reqWildcard.Header.Set("Origin", "https://preview.bobslab.xyz")
	recWildcard := httptest.NewRecorder()
	handler.ServeHTTP(recWildcard, reqWildcard)

	if recWildcard.Header().Get("Access-Control-Allow-Origin") != "https://preview.bobslab.xyz" {
		t.Errorf("expected subdomain wildcard to match, got %s", recWildcard.Header().Get("Access-Control-Allow-Origin"))
	}

	// Normal GET test with disallowed origin
	reqDisallowed := httptest.NewRequest(http.MethodGet, "/test", nil)
	reqDisallowed.Header.Set("Origin", "https://evil.com")
	recDisallowed := httptest.NewRecorder()
	handler.ServeHTTP(recDisallowed, reqDisallowed)

	if recDisallowed.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("expected no allow origin for unauthorized domain, got %s", recDisallowed.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestRecoverer(t *testing.T) {
	handler := Recoverer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("unexpected runtime panic")
	}))

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500 on panic, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "internal_server_error") {
		t.Errorf("expected JSON error response, got %s", rec.Body.String())
	}
}

func TestRateLimiter(t *testing.T) {
	limiter := NewRateLimiter(2.0, 2, true)
	defer limiter.Stop()

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected request %d to be allowed, got %d", i, rec.Code)
		}
	}

	// Third request exceeds burst
	reqExceeded := httptest.NewRequest(http.MethodGet, "/test", nil)
	recExceeded := httptest.NewRecorder()
	handler.ServeHTTP(recExceeded, reqExceeded)

	if recExceeded.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 Too Many Requests, got %d", recExceeded.Code)
	}
}

func TestLogger(t *testing.T) {
	handler := Logger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		SetUpstreamTarget(r.Context(), "cognito-backend")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test/path", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}
}
