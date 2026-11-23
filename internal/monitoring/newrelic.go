package monitoring

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/newrelic/go-agent/v3/newrelic"

	"cognito-api-gateway/internal/config"
	"cognito-api-gateway/internal/middleware"
)

// LogWriter forwards application log records to New Relic Logs while echoing to stdout.
type LogWriter struct {
	Output io.Writer
	App    *newrelic.Application
}

func (w *LogWriter) Write(p []byte) (n int, err error) {
	if w.Output != nil {
		n, err = w.Output.Write(p)
	} else {
		n = len(p)
	}

	if w.App != nil {
		w.App.RecordLog(newrelic.LogData{
			Timestamp: time.Now().UnixMilli(),
			Message:   strings.TrimSpace(string(p)),
			Severity:  "INFO",
		})
	}

	return n, err
}

// InitNewRelic initializes the official New Relic Go Agent when configured.
func InitNewRelic(cfg *config.Config) (*newrelic.Application, error) {
	if !cfg.NewRelicEnabled || cfg.NewRelicLicenseKey == "" {
		slog.Info("New Relic monitoring is disabled or license key not provided")
		return nil, nil
	}

	appName := cfg.NewRelicAppName
	if appName == "" {
		appName = fmt.Sprintf("cognito-gateway-%s", cfg.Environment)
	}

	app, err := newrelic.NewApplication(
		newrelic.ConfigAppName(appName),
		newrelic.ConfigLicense(cfg.NewRelicLicenseKey),
		newrelic.ConfigDistributedTracerEnabled(true),
		newrelic.ConfigAppLogForwardingEnabled(true),
		func(c *newrelic.Config) {
			if strings.HasPrefix(cfg.NewRelicLicenseKey, "eu01") {
				c.Host = "staging-collector.eu01.nr-data.net"
			}
			c.Labels = map[string]string{
				"Environment": cfg.Environment,
				"Service":     "cognito-gateway",
			}
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create New Relic application: %w", err)
	}

	slog.Info("New Relic APM initialized successfully", "app_name", appName, "environment", cfg.Environment)
	return app, nil
}

// Middleware creates a Chi middleware that captures APM transaction metrics.
func Middleware(app *newrelic.Application, env string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip high-frequency health probes from APM metrics
			if isProbeEndpoint(r.URL.Path) || app == nil {
				next.ServeHTTP(w, r)
				return
			}

			txnName := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
			txn := app.StartTransaction(txnName)
			defer txn.End()

			txn.SetWebRequestHTTP(r)
			rw := txn.SetWebResponse(w)

			reqID := middleware.GetRequestID(r.Context())
			clientIP := middleware.GetClientIP(r)

			txn.AddAttribute("request.id", reqID)
			txn.AddAttribute("client.ip", clientIP)
			txn.AddAttribute("environment", env)

			ctx := newrelic.NewContext(r.Context(), txn)

			next.ServeHTTP(rw, r.WithContext(ctx))

			// Update transaction name to parameterized Chi route pattern
			rctx := chi.RouteContext(r.Context())
			if rctx != nil && rctx.RoutePattern() != "" {
				txn.SetName(fmt.Sprintf("%s %s", r.Method, rctx.RoutePattern()))
			}
		})
	}
}

// InjectTraceHeaders injects W3C distributed trace headers into outbound requests.
func InjectTraceHeaders(ctx context.Context, req *http.Request) {
	txn := newrelic.FromContext(ctx)
	if txn != nil {
		txn.InsertDistributedTraceHeaders(req.Header)
	}
}

func isProbeEndpoint(path string) bool {
	return path == "/health" ||
		path == "/healthz" ||
		path == "/ready" ||
		strings.HasPrefix(path, "/health/") ||
		strings.HasPrefix(path, "/ready/")
}
