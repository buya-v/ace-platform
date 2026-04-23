package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/garudax-platform/platform-service/internal/server"
	"github.com/garudax-platform/platform-service/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With(
		slog.String("service", "platform-service"),
	)

	logger.Info("GarudaX Platform Service starting...")

	cfg := server.DefaultConfig()

	// Override ports from environment if provided.
	if port := os.Getenv("API_PORT"); port != "" {
		var p int
		if _, err := parsePort(port, &p); err == nil {
			cfg.APIPort = p
		}
	}
	if port := os.Getenv("HEALTH_PORT"); port != "" {
		var p int
		if _, err := parsePort(port, &p); err == nil {
			cfg.HealthPort = p
		}
	}
	if addr := os.Getenv("BIND_ADDRESS"); addr != "" {
		cfg.BindAddress = addr
	}

	// Init stores — in-memory by default; swap for DB-backed store when DATABASE_URL is set.
	tenantStore := store.NewInMemoryTenantStore()
	logger.Info("using in-memory tenant store, seeded with ace-commodities and mse-equities")

	srv := server.New(tenantStore, cfg)

	// Start health server on port 9090.
	go func() {
		logger.Info("health server starting",
			slog.String("bind", cfg.BindAddress),
			slog.Int("port", cfg.HealthPort),
		)
		if err := srv.StartHealthServer(); err != nil {
			logger.Error("health server failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	// Start API server on port 8090.
	go func() {
		logger.Info("API server starting",
			slog.String("bind", cfg.BindAddress),
			slog.Int("port", cfg.APIPort),
		)
		if err := srv.StartAPIServer(); err != nil {
			logger.Error("API server failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	// Mark ready after all stores and routes are initialised.
	srv.SetReady()
	logger.Info("GarudaX Platform Service ready",
		slog.Int("api_port", cfg.APIPort),
		slog.Int("health_port", cfg.HealthPort),
	)

	// Wait for shutdown signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	logger.Info("received signal, shutting down", slog.String("signal", sig.String()))
}

// parsePort parses a string port number into an int.
// Returns an error if the string is not a valid port number.
func parsePort(s string, out *int) (string, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return s, &portError{s}
		}
		n = n*10 + int(c-'0')
	}
	if n < 1 || n > 65535 {
		return s, &portError{s}
	}
	*out = n
	return s, nil
}

type portError struct{ s string }

func (e *portError) Error() string { return "invalid port: " + e.s }
