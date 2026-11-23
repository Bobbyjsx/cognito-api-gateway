package middleware

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/newrelic/go-agent/v3/newrelic"
)

// Recoverer recovers from panics, logs the stack trace, captures error in New Relic,
// and returns a safe 500 Internal Server Error.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rvr := recover(); rvr != nil {
				stack := string(debug.Stack())
				reqID := GetRequestID(r.Context())

				slog.Error("panic recovered in HTTP handler",
					"error", fmt.Sprintf("%v", rvr),
					"request_id", reqID,
					"path", r.URL.Path,
					"stack", stack,
				)

				// Report panic to active New Relic transaction if present
				txn := newrelic.FromContext(r.Context())
				if txn != nil {
					txn.NoticeError(fmt.Errorf("panic recovered: %v", rvr))
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error":       "internal_server_error",
					"message":     "An unexpected internal server error occurred",
					"status_code": http.StatusInternalServerError,
					"request_id":  reqID,
				})
			}
		}()

		next.ServeHTTP(w, r)
	})
}
