package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/newrelic/go-agent/v3/newrelic"
)

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
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

// Logger logs each incoming HTTP request and response status in structured JSON.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &responseRecorder{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(rec, r)

		duration := time.Since(start)
		reqID := GetRequestID(r.Context())
		clientIP := GetClientIP(r)

		// Record log correlated with New Relic transaction if active
		txn := newrelic.FromContext(r.Context())
		if txn != nil {
			logData := newrelic.LogData{
				Timestamp: start.UnixMilli(),
				Message:   "HTTP request handled",
				Severity:  "INFO",
				Attributes: map[string]any{
					"http.method":      r.Method,
					"http.path":        r.URL.Path,
					"http.status_code": rec.statusCode,
					"http.latency_ms":  duration.Seconds() * 1000,
					"client.ip":        clientIP,
					"request.id":       reqID,
				},
			}
			if rec.statusCode >= 500 {
				logData.Severity = "ERROR"
			} else if rec.statusCode >= 400 {
				logData.Severity = "WARN"
			}
			txn.RecordLog(logData)
		}

		// Avoid logging noisy health checks in normal logs
		if r.URL.Path == "/health" || r.URL.Path == "/healthz" {
			return
		}

		level := slog.LevelInfo
		if rec.statusCode >= 500 {
			level = slog.LevelError
		} else if rec.statusCode >= 400 {
			level = slog.LevelWarn
		}

		slog.Log(r.Context(), level, "HTTP request handled",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.statusCode,
			"latency_ms", float64(duration.Microseconds())/1000.0,
			"bytes", rec.bytesWritten,
			"client_ip", clientIP,
			"request_id", reqID,
		)
	})
}
