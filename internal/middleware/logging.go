package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/newrelic/go-agent/v3/newrelic"
)

type upstreamKeyType struct{}

var upstreamKey = upstreamKeyType{}

// UpstreamInfo tracks which upstream service handled the request.
type UpstreamInfo struct {
	Target string
}

// GetUpstreamInfo extracts the UpstreamInfo pointer from context if available.
func GetUpstreamInfo(ctx context.Context) *UpstreamInfo {
	if info, ok := ctx.Value(upstreamKey).(*UpstreamInfo); ok {
		return info
	}
	return nil
}

// SetUpstreamTarget marks the upstream target name on the request context.
func SetUpstreamTarget(ctx context.Context, target string) {
	if info := GetUpstreamInfo(ctx); info != nil {
		info.Target = target
	}
}

type responseRecorder struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
}

func (rec *responseRecorder) WriteHeader(code int) {
	rec.statusCode = code
	rec.ResponseWriter.WriteHeader(code)
}

func (rec *responseRecorder) Write(b []byte) (int, error) {
	n, err := rec.ResponseWriter.Write(b)
	rec.bytesWritten += int64(n)
	return n, err
}

// Flush implements http.Flusher for SSE streaming passthrough.
func (rec *responseRecorder) Flush() {
	if flusher, ok := rec.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%.0fµs", float64(d.Microseconds()))
	}
	if d < time.Second {
		return fmt.Sprintf("%.1fms", float64(d.Microseconds())/1000.0)
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

// Logger logs each incoming HTTP request and response status in structured JSON
// with an expressive, high-signal message property: "[upstream] status status_text - method path (duration)".
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &responseRecorder{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// Initialize upstream tracker defaulting to "gateway"
		upstream := &UpstreamInfo{Target: "gateway"}
		ctx := context.WithValue(r.Context(), upstreamKey, upstream)
		r = r.WithContext(ctx)

		next.ServeHTTP(rec, r)

		duration := time.Since(start)
		reqID := GetRequestID(r.Context())
		clientIP := GetClientIP(r)

		target := upstream.Target
		statusText := http.StatusText(rec.statusCode)
		if statusText == "" {
			statusText = "Unknown"
		}
		durationStr := formatDuration(duration)

		// Formatted message: [cognito-backend] 200 OK - GET /docs (52.9ms)
		msg := fmt.Sprintf("[%s] %d %s - %s %s (%s)",
			target, rec.statusCode, statusText, r.Method, r.URL.Path, durationStr)

		// Assign severity: Error for 5xx, Warn for 4xx, Info for 2xx/3xx
		severity := "INFO"
		level := slog.LevelInfo
		if rec.statusCode >= 500 {
			severity = "ERROR"
			level = slog.LevelError
		} else if rec.statusCode >= 400 {
			severity = "WARN"
			level = slog.LevelWarn
		}

		// Record log correlated with New Relic transaction if active
		txn := newrelic.FromContext(r.Context())
		if txn != nil {
			logData := newrelic.LogData{
				Timestamp: start.UnixMilli(),
				Message:   msg,
				Severity:  severity,
				Attributes: map[string]any{
					"upstream":         target,
					"http.method":      r.Method,
					"http.path":        r.URL.Path,
					"http.status_code": rec.statusCode,
					"http.status_text": statusText,
					"http.latency_ms":  float64(duration.Microseconds()) / 1000.0,
					"client.ip":        clientIP,
					"request.id":       reqID,
				},
			}
			txn.RecordLog(logData)
		}

		// Avoid logging noisy health checks in normal stdout logs
		if r.URL.Path == "/health" || r.URL.Path == "/healthz" {
			return
		}

		slog.Log(r.Context(), level, msg,
			"upstream", target,
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.statusCode,
			"status_text", statusText,
			"latency_ms", float64(duration.Microseconds())/1000.0,
			"bytes", rec.bytesWritten,
			"client_ip", clientIP,
			"request_id", reqID,
		)
	})
}
