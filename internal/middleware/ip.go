package middleware

import (
	"net"
	"net/http"
	"strings"
)

// GetClientIP extracts the real client IP address, prioritizing proxy headers
// provided by edge reverse proxies (Cloudflare, Caddy, etc.) before falling back
// to RemoteAddr.
func GetClientIP(r *http.Request) string {
	// 1. Cloudflare Connecting IP header (highest trust when routed through Cloudflare)
	if cfIP := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); cfIP != "" {
		if net.ParseIP(cfIP) != nil {
			return cfIP
		}
	}

	// 2. Standard X-Real-IP header set by Caddy / Nginx
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		if net.ParseIP(realIP) != nil {
			return realIP
		}
	}

	// 3. X-Forwarded-For header (comma-separated list of IPs, leftmost is original client)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if ip := net.ParseIP(trimmed); ip != nil {
				return trimmed
			}
		}
	}

	// 4. Fallback to direct TCP connection remote address
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}

	return r.RemoteAddr
}
