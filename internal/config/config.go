package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration for cognito-api-gateway.
type Config struct {
	Environment         string
	Port                int
	BackendServiceURL   *url.URL
	IdentityServiceURL  *url.URL
	CORSAllowedOrigins  []string
	RateLimitEnabled    bool
	RateLimitRPS        float64
	RateLimitBurst      int
	ReadTimeout         time.Duration
	WriteTimeout        time.Duration
	IdleTimeout         time.Duration
	UpstreamTimeout     time.Duration
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	IdleConnTimeout     time.Duration
	NewRelicEnabled     bool
	NewRelicLicenseKey  string
	NewRelicAppName     string
}

// Load reads configuration from environment variables with production-ready defaults.
func Load() (*Config, error) {
	backendURLStr := getEnv("BACKEND_SERVICE_URL", "http://localhost:8002")
	parsedBackend, err := url.Parse(backendURLStr)
	if err != nil || parsedBackend.Scheme == "" || parsedBackend.Host == "" {
		return nil, fmt.Errorf("invalid BACKEND_SERVICE_URL '%s': %w", backendURLStr, err)
	}

	identityURLStr := getEnv("IDENTITY_SERVICE_URL", "https://vm-id.bobslab.xyz")
	parsedIdentity, err := url.Parse(identityURLStr)
	if err != nil || parsedIdentity.Scheme == "" || parsedIdentity.Host == "" {
		return nil, fmt.Errorf("invalid IDENTITY_SERVICE_URL '%s': %w", identityURLStr, err)
	}

	port := getEnvInt("PORT", 8000)
	corsOrigins := strings.Split(getEnv("CORS_ALLOWED_ORIGINS", "*"), ",")
	for i := range corsOrigins {
		corsOrigins[i] = strings.TrimSpace(corsOrigins[i])
	}

	return &Config{
		Environment:         getEnv("ENVIRONMENT", "development"),
		Port:                port,
		BackendServiceURL:   parsedBackend,
		IdentityServiceURL:  parsedIdentity,
		CORSAllowedOrigins:  corsOrigins,
		RateLimitEnabled:    getEnvBool("RATE_LIMIT_ENABLED", true),
		RateLimitRPS:        getEnvFloat("RATE_LIMIT_RPS", 100.0),
		RateLimitBurst:      getEnvInt("RATE_LIMIT_BURST", 200),
		ReadTimeout:         time.Duration(getEnvInt("READ_TIMEOUT_SECONDS", 15)) * time.Second,
		WriteTimeout:        time.Duration(getEnvInt("WRITE_TIMEOUT_SECONDS", 30)) * time.Second,
		IdleTimeout:         time.Duration(getEnvInt("IDLE_TIMEOUT_SECONDS", 120)) * time.Second,
		UpstreamTimeout:     time.Duration(getEnvInt("UPSTREAM_TIMEOUT_SECONDS", 30)) * time.Second,
		MaxIdleConns:        getEnvInt("MAX_IDLE_CONNS", 1000),
		MaxIdleConnsPerHost: getEnvInt("MAX_IDLE_CONNS_PER_HOST", 250),
		IdleConnTimeout:     time.Duration(getEnvInt("IDLE_CONN_TIMEOUT_SECONDS", 90)) * time.Second,
		NewRelicEnabled:     getEnvBool("NEW_RELIC_ENABLED", false),
		NewRelicLicenseKey:  getEnv("NEW_RELIC_LICENSE_KEY", ""),
		NewRelicAppName:     getEnv("NEW_RELIC_APP_NAME", "cognito-gateway"),
	}, nil
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val, ok := os.LookupEnv(key); ok {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
	}
	return defaultVal
}

func getEnvFloat(key string, defaultVal float64) float64 {
	if val, ok := os.LookupEnv(key); ok {
		if floatVal, err := strconv.ParseFloat(val, 64); err == nil {
			return floatVal
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val, ok := os.LookupEnv(key); ok {
		lower := strings.ToLower(strings.TrimSpace(val))
		return lower == "true" || lower == "1" || lower == "yes"
	}
	return defaultVal
}
