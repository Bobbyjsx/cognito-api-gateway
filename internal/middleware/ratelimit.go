package middleware

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter manages per-client token bucket limiters.
type RateLimiter struct {
	mu       sync.Mutex
	clients  map[string]*clientLimiter
	rps      rate.Limit
	burst    int
	enabled  bool
	stopChan chan struct{}
}

// NewRateLimiter creates an active rate limiter with background garbage collection.
func NewRateLimiter(rps float64, burst int, enabled bool) *RateLimiter {
	rl := &RateLimiter{
		clients:  make(map[string]*clientLimiter),
		rps:      rate.Limit(rps),
		burst:    burst,
		enabled:  enabled,
		stopChan: make(chan struct{}),
	}

	if enabled {
		go rl.cleanupLoop()
	}

	return rl
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(3 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			for ip, cl := range rl.clients {
				if now.Sub(cl.lastSeen) > 5*time.Minute {
					delete(rl.clients, ip)
				}
			}
			rl.mu.Unlock()
		case <-rl.stopChan:
			return
		}
	}
}

// Stop terminates background cleanup.
func (rl *RateLimiter) Stop() {
	if rl.enabled && rl.stopChan != nil {
		select {
		case <-rl.stopChan:
		default:
			close(rl.stopChan)
		}
	}
}

// Middleware enforces rate limits per client IP.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.enabled {
			next.ServeHTTP(w, r)
			return
		}

		ip := GetClientIP(r)

		rl.mu.Lock()
		cl, exists := rl.clients[ip]
		if !exists {
			cl = &clientLimiter{
				limiter: rate.NewLimiter(rl.rps, rl.burst),
			}
			rl.clients[ip] = cl
		}
		cl.lastSeen = time.Now()
		allowed := cl.limiter.Allow()
		rl.mu.Unlock()

		if !allowed {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":       "rate_limit_exceeded",
				"message":     "Too many requests, please slow down.",
				"status_code": http.StatusTooManyRequests,
				"request_id":  GetRequestID(r.Context()),
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}
