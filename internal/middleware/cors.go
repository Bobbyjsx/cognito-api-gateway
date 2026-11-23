package middleware

import (
	"net/http"
	"strings"
)

// CORS handles Cross-Origin Resource Sharing headers and preflight requests.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowAll := false
	originsMap := make(map[string]bool)

	for _, o := range allowedOrigins {
		trimmed := strings.TrimSpace(o)
		if trimmed == "*" {
			allowAll = true
		} else if trimmed != "" {
			originsMap[trimmed] = true
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if origin != "" {
				allowed := allowAll || originsMap[origin]
				if allowed {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS, HEAD")
					w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, Content-Length, X-CSRF-Token, X-Request-ID, CF-Connecting-IP, X-Real-IP, Cache-Control, Pragma, X-Requested-With")
					w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, X-New-Access-Token, X-New-Refresh-Token, Content-Length, Content-Type")
					w.Header().Set("Access-Control-Max-Age", "86400")
				}
			}

			// Preflight request terminates immediately with 204 No Content
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
