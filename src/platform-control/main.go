package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

// Default port allocation for platform-control. The supporting/control-plane
// services use the 80xx range; platform-service already claims 8095/9095, so the
// control plane takes the next free pair.
const (
	defaultAPIPort    = 8096
	defaultHealthPort = 9096
	defaultBindAddr   = "0.0.0.0"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With(
		slog.String("service", "platform-control"),
	)
	logger.Info("GarudaX Platform Control Plane starting...")

	apiPort := envPort("API_PORT", defaultAPIPort)
	healthPort := envPort("HEALTH_PORT", defaultHealthPort)
	bindAddr := envStr("BIND_ADDRESS", defaultBindAddr)

	reg := NewSeededRegistry()
	logger.Info("tenant registry seeded", slog.Int("tenants", len(reg.List(""))))

	api := NewAPI(reg)

	apiSrv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", bindAddr, apiPort),
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		logger.Info("API server listening", slog.String("addr", apiSrv.Addr))
		if err := apiSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("API server failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	// Separate health listener so liveness is independent of the API port.
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", api.handleHealthz)
	healthMux.HandleFunc("/readyz", api.handleReadyz)
	healthSrv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", bindAddr, healthPort),
		Handler:           healthMux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		logger.Info("health server listening", slog.String("addr", healthSrv.Addr))
		if err := healthSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("health server failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	api.SetReady()
	logger.Info("GarudaX Platform Control Plane ready",
		slog.Int("api_port", apiPort), slog.Int("health_port", healthPort))

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	logger.Info("shutting down", slog.String("signal", sig.String()))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = apiSrv.Shutdown(ctx)
	_ = healthSrv.Shutdown(ctx)
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envPort(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 65535 {
		return def
	}
	return n
}
