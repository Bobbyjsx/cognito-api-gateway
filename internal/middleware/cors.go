package middleware

import (
	"net/http"
	"net/url"
	"strings"
)

const defaultAllowedHeaders = "Accept, Authorization, Content-Type, Content-Length, X-CSRF-Token, X-Refresh-Token, X-Request-ID, X-Application-Id, CF-Connecting-IP, X-Real-IP, Cache-Control, Pragma, X-Requested-With, If-None-Match, Accept-Language, Range"
const defaultExposedHeaders = "X-Request-ID, X-New-Access-Token, X-New-Refresh-Token, Content-Length, Content-Type, X-Next-Cursor"

// CORS handles Cross-Origin Resource Sharing with preflight caching, header reflection, and wildcard subdomain support.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowAll := false
	originsMap := make(map[string]struct{}, len(allowedOrigins))
	var wildcardDomains []string

	for _, o := range allowedOrigins {
		trimmed := strings.TrimSpace(o)
		if trimmed == "*" {
			allowAll = true
			break
		}
		// Support wildcard subdomain patterns like "*.bobslab.xyz" or "https://*.bobslab.xyz"
		clean := trimmed
		clean = strings.TrimPrefix(clean, "https://")
		clean = strings.TrimPrefix(clean, "http://")
		if strings.HasPrefix(clean, "*.") {
			wildcardDomains = append(wildcardDomains, clean[2:])
		}
		originsMap[trimmed] = struct{}{}
	}

	isAllowed := func(origin string) bool {
		if allowAll {
			return true
		}
		if _, ok := originsMap[origin]; ok {
			return true
		}
		if len(wildcardDomains) > 0 {
			parsed, err := url.Parse(origin)
			if err == nil {
				host := parsed.Hostname()
				for _, domain := range wildcardDomains {
					if host == domain || strings.HasSuffix(host, "."+domain) {
						return true
					}
				}
			}
		}
		return false
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if origin != "" && isAllowed(origin) {
				// With Access-Control-Allow-Credentials: true, the Access-Control-Allow-Origin
				// header CANNOT be wildcard "*". Browsers require the specific requesting origin
				// to be reflected along with Vary: Origin.
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS, HEAD")

				// Include default allowed headers and reflect any client-requested headers
				allowedHeaders := defaultAllowedHeaders
				if reqHeaders := r.Header.Get("Access-Control-Request-Headers"); reqHeaders != "" {
					allowedHeaders = allowedHeaders + ", " + reqHeaders
				}
				w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)
				w.Header().Set("Access-Control-Expose-Headers", defaultExposedHeaders)
				w.Header().Set("Access-Control-Max-Age", "86400")
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
