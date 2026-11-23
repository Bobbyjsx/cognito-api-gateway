package probe

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"cognito-api-gateway/internal/config"
)

// UpstreamStatus represents the health status of a single upstream service.
type UpstreamStatus struct {
	Name      string    `json:"name"`
	Target    string    `json:"target"`
	Status    string    `json:"status"`
	LatencyMs float64   `json:"latency_ms"`
	CheckedAt time.Time `json:"checked_at"`
	Error     string    `json:"error,omitempty"`
}

// ProbeManager checks health of dependent upstreams with caching to prevent probe storms.
type ProbeManager struct {
	client       *http.Client
	backendURL   *url.URL
	identityURL  *url.URL
	cacheTTL     time.Duration
	mu           sync.RWMutex
	lastCheck    time.Time
	cachedStatus map[string]UpstreamStatus
}

// NewProbeManager creates a cached probe manager for upstreams.
func NewProbeManager(cfg *config.Config, backendURL, identityURL *url.URL) *ProbeManager {
	return &ProbeManager{
		client: &http.Client{
			Timeout: 2 * time.Second,
		},
		backendURL:   backendURL,
		identityURL:  identityURL,
		cacheTTL:     3 * time.Second,
		cachedStatus: make(map[string]UpstreamStatus),
	}
}

// CheckBackend probes the cognito backend health endpoint.
func (pm *ProbeManager) CheckBackend(ctx context.Context) UpstreamStatus {
	return pm.checkSingle(ctx, "cognito-backend", pm.backendURL, "/health")
}

// CheckIdentity probes the identity service JWKS or health endpoint.
func (pm *ProbeManager) CheckIdentity(ctx context.Context) UpstreamStatus {
	return pm.checkSingle(ctx, "identity-service", pm.identityURL, "/.well-known/jwks.json")
}

// CheckAll executes concurrent probes against all upstreams.
func (pm *ProbeManager) CheckAll(ctx context.Context) (map[string]UpstreamStatus, bool) {
	pm.mu.RLock()
	if time.Since(pm.lastCheck) < pm.cacheTTL && len(pm.cachedStatus) > 0 {
		cached := make(map[string]UpstreamStatus, len(pm.cachedStatus))
		for k, v := range pm.cachedStatus {
			cached[k] = v
		}
		pm.mu.RUnlock()

		allHealthy := true
		for _, s := range cached {
			if s.Status != "healthy" {
				allHealthy = false
				break
			}
		}
		return cached, allHealthy
	}
	pm.mu.RUnlock()

	var wg sync.WaitGroup
	var mu sync.Mutex
	results := make(map[string]UpstreamStatus)
	allHealthy := true

	wg.Add(2)
	go func() {
		defer wg.Done()
		bStatus := pm.CheckBackend(ctx)
		mu.Lock()
		results["backend"] = bStatus
		if bStatus.Status != "healthy" {
			allHealthy = false
		}
		mu.Unlock()
	}()

	go func() {
		defer wg.Done()
		iStatus := pm.CheckIdentity(ctx)
		mu.Lock()
		results["identity"] = iStatus
		if iStatus.Status != "healthy" {
			allHealthy = false
		}
		mu.Unlock()
	}()

	wg.Wait()

	pm.mu.Lock()
	pm.lastCheck = time.Now()
	pm.cachedStatus = results
	pm.mu.Unlock()

	return results, allHealthy
}

func (pm *ProbeManager) checkSingle(ctx context.Context, name string, base *url.URL, probePath string) UpstreamStatus {
	if base == nil {
		return UpstreamStatus{
			Name:      name,
			Status:    "unconfigured",
			CheckedAt: time.Now(),
		}
	}

	target := strings.TrimSuffix(base.String(), "/") + probePath
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return UpstreamStatus{
			Name:      name,
			Target:    target,
			Status:    "unhealthy",
			LatencyMs: float64(time.Since(start).Microseconds()) / 1000.0,
			CheckedAt: time.Now(),
			Error:     err.Error(),
		}
	}

	resp, err := pm.client.Do(req)
	latency := float64(time.Since(start).Microseconds()) / 1000.0

	if err != nil {
		return UpstreamStatus{
			Name:      name,
			Target:    target,
			Status:    "unhealthy",
			LatencyMs: latency,
			CheckedAt: time.Now(),
			Error:     err.Error(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return UpstreamStatus{
			Name:      name,
			Target:    target,
			Status:    "unhealthy",
			LatencyMs: latency,
			CheckedAt: time.Now(),
			Error:     fmt.Sprintf("status code %d", resp.StatusCode),
		}
	}

	return UpstreamStatus{
		Name:      name,
		Target:    target,
		Status:    "healthy",
		LatencyMs: latency,
		CheckedAt: time.Now(),
	}
}
