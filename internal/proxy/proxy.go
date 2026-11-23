package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/newrelic/go-agent/v3/newrelic"

	"cognito-api-gateway/internal/config"
	"cognito-api-gateway/internal/middleware"
)

// UpstreamProxy wraps a configured reverse proxy for an upstream target.
type UpstreamProxy struct {
	target    *url.URL
	name      string
	proxy     *httputil.ReverseProxy
	transport *http.Transport
}

// ErrorResponse represents standardized JSON error output for gateway proxy failures.
type ErrorResponse struct {
	Error      string `json:"error"`
	Message    string `json:"message"`
	StatusCode int    `json:"status_code"`
	RequestID  string `json:"request_id,omitempty"`
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}

func osIsTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// NewUpstreamProxy creates a connection-pooled, low-latency streaming reverse proxy with optional APM instrumentation.
func NewUpstreamProxy(name string, target *url.URL, cfg *config.Config, bufPool *BufferPool, nrApp ...*newrelic.Application) *UpstreamProxy {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          cfg.MaxIdleConns,
		MaxIdleConnsPerHost:   cfg.MaxIdleConnsPerHost,
		IdleConnTimeout:       cfg.IdleConnTimeout,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: cfg.UpstreamTimeout,
		DisableKeepAlives:     false,
	}

	// Instrument round-tripper with New Relic external segments when APM is active
	var rt http.RoundTripper = transport
	if len(nrApp) > 0 && nrApp[0] != nil {
		rt = newrelic.NewRoundTripper(transport)
	}

	revProxy := &httputil.ReverseProxy{
		Transport:     rt,
		BufferPool:    bufPool,
		FlushInterval: 10 * time.Millisecond, // Flushes SSE chunks immediately
		ModifyResponse: func(resp *http.Response) error {
			// Strip CORS headers from upstream responses to avoid duplicate or conflicting
			// headers on the client response. The API Gateway authoritatively handles CORS edge termination.
			resp.Header.Del("Access-Control-Allow-Origin")
			resp.Header.Del("Access-Control-Allow-Credentials")
			resp.Header.Del("Access-Control-Allow-Methods")
			resp.Header.Del("Access-Control-Allow-Headers")
			resp.Header.Del("Access-Control-Expose-Headers")
			resp.Header.Del("Access-Control-Max-Age")

			// Strip security headers that are already applied by Gateway's SecurityHeaders middleware
			resp.Header.Del("X-Content-Type-Options")
			resp.Header.Del("X-Frame-Options")
			resp.Header.Del("X-XSS-Protection")

			// If streaming SSE, ensure no downstream buffering
			if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
				resp.Header.Set("X-Accel-Buffering", "no")
				resp.Header.Set("Cache-Control", "no-cache, no-transform")
			}

			return nil
		},
	}

	targetQuery := target.RawQuery

	revProxy.Director = func(req *http.Request) {
		originalHost := req.Host
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host

		// Retain target path prefix if configured, otherwise forward clean path
		if target.Path != "" && target.Path != "/" {
			req.URL.Path = singleJoiningSlash(target.Path, req.URL.Path)
		}

		if targetQuery == "" || req.URL.RawQuery == "" {
			req.URL.RawQuery = targetQuery + req.URL.RawQuery
		} else {
			req.URL.RawQuery = targetQuery + "&" + req.URL.RawQuery
		}

		// Set upstream host header
		req.Host = target.Host

		// Client IP tracking
		clientIP := middleware.GetClientIP(req)
		if prior := req.Header.Get("X-Forwarded-For"); prior != "" {
			req.Header.Set("X-Forwarded-For", prior+", "+clientIP)
		} else {
			req.Header.Set("X-Forwarded-For", clientIP)
		}
		req.Header.Set("X-Real-IP", clientIP)

		// Scheme tracking
		if proto := req.Header.Get("X-Forwarded-Proto"); proto == "" {
			if req.TLS != nil {
				req.Header.Set("X-Forwarded-Proto", "https")
			} else {
				req.Header.Set("X-Forwarded-Proto", "http")
			}
		}

		// Forward original client host for OAuth and callback redirects
		if fwdHost := req.Header.Get("X-Forwarded-Host"); fwdHost == "" {
			req.Header.Set("X-Forwarded-Host", originalHost)
		}

		reqID := middleware.GetRequestID(req.Context())
		if reqID != "" {
			req.Header.Set(middleware.HeaderRequestID, reqID)
		}

		// Inject New Relic distributed tracing headers to propagate traces downstream
		if txn := newrelic.FromContext(req.Context()); txn != nil {
			txn.InsertDistributedTraceHeaders(req.Header)
		}
	}

	revProxy.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err error) {
		reqID := middleware.GetRequestID(req.Context())
		if errors.Is(err, context.Canceled) {
			slog.Debug("client canceled proxy request", "upstream", name, "request_id", reqID)
			return
		}

		if txn := newrelic.FromContext(req.Context()); txn != nil {
			txn.NoticeError(err)
		}

		statusCode := http.StatusBadGateway
		msg := "Upstream service temporarily unavailable"

		if errors.Is(err, context.DeadlineExceeded) || osIsTimeout(err) {
			statusCode = http.StatusGatewayTimeout
			msg = "Upstream service request timed out"
		}

		slog.Warn("upstream proxy error",
			"upstream", name,
			"target", target.String(),
			"path", req.URL.Path,
			"status_code", statusCode,
			"request_id", reqID,
			"error", err.Error(),
		)

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		w.WriteHeader(statusCode)

		_ = json.NewEncoder(w).Encode(ErrorResponse{
			Error:      http.StatusText(statusCode),
			Message:    msg,
			StatusCode: statusCode,
			RequestID:  reqID,
		})
	}

	return &UpstreamProxy{
		target:    target,
		name:      name,
		proxy:     revProxy,
		transport: transport,
	}
}

// ServeHTTP forwards the request to the upstream target.
func (u *UpstreamProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	middleware.SetUpstreamTarget(r.Context(), u.name)
	u.proxy.ServeHTTP(w, r)
}

// CloseIdleConnections closes all idle keepalive connections in the upstream connection pool.
func (u *UpstreamProxy) CloseIdleConnections() {
	if u.transport != nil {
		u.transport.CloseIdleConnections()
	}
}
