package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cognito-api-gateway/internal/config"
	"cognito-api-gateway/internal/monitoring"
	"cognito-api-gateway/internal/server"
)

func main() {
	// Configure high-performance JSON structured logging
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	slog.SetDefault(slog.New(handler))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	if cfg.Environment == "development" {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})))
	}

	nrApp, err := monitoring.InitNewRelic(cfg)
	if err != nil {
		slog.Error("failed to initialize New Relic", "error", err)
	}

	if nrApp != nil {
		// Forward application logs to New Relic with entity correlation
		slog.SetDefault(slog.New(slog.NewJSONHandler(&monitoring.LogWriter{Output: os.Stdout, App: nrApp}, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})))
	}

	gatewayServer := server.New(cfg, nrApp)

	addr := fmt.Sprintf(":%d", cfg.Port)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           gatewayServer.Handler(),
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		MaxHeaderBytes:    1 << 20, // 1MB
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Server shutdown channel
	shutdownChan := make(chan struct{})

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
		sig := <-sigChan

		slog.Info("received termination signal, initiating graceful shutdown", "signal", sig.String())

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(ctx); err != nil {
			slog.Error("forced shutdown error", "error", err)
		}

		if nrApp != nil {
			nrApp.Shutdown(5 * time.Second)
		}

		gatewayServer.CloseIdleConnections()
		slog.Info("gateway server shutdown completed cleanly")
		close(shutdownChan)
	}()

	slog.Info("starting Cognito API Gateway",
		"port", cfg.Port,
		"environment", cfg.Environment,
		"backend_upstream", cfg.BackendServiceURL.String(),
		"identity_upstream", cfg.IdentityServiceURL.String(),
		"rate_limit_enabled", cfg.RateLimitEnabled,
	)

	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server fatal error", "error", err)
		os.Exit(1)
	}

	<-shutdownChan
}
