package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/garudax-platform/fix-gateway/internal/broker"
	"github.com/garudax-platform/fix-gateway/internal/router"
	"github.com/garudax-platform/fix-gateway/internal/server"
	"github.com/garudax-platform/fix-gateway/internal/session"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With(
		slog.String("service", "fix-gateway"),
	)

	logger.Info("GarudaX FIX Protocol Gateway starting...")

	// Configuration from environment
	adminAddr := envStr("FIX_ADMIN_ADDR", ":8091")
	healthAddr := envStr("FIX_HEALTH_ADDR", ":9091")
	fixListenAddr := envStr("FIX_LISTEN_ADDR", ":9878")
	targetCompID := envStr("FIX_TARGET_COMP_ID", "GARUDAX")
	securitiesURL := envStr("SECURITIES_SERVICE_URL", "http://securities-service:8089")

	logger.Info("configuration loaded",
		slog.String("admin_addr", adminAddr),
		slog.String("health_addr", healthAddr),
		slog.String("fix_listen_addr", fixListenAddr),
		slog.String("target_comp_id", targetCompID),
		slog.String("securities_url", securitiesURL),
	)

	// Initialize session manager
	sessionMgr := session.NewSessionManager()

	// Initialize broker store with seed data
	brokerStore := broker.NewInMemoryStore()

	// Initialize order router
	orderRouter := router.NewOrderRouter(securitiesURL)

	// Create and start HTTP admin/health server
	srv := server.New(logger, adminAddr, healthAddr, sessionMgr, brokerStore)

	go func() {
		if err := srv.Start(); err != nil {
			logger.Error("server error", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	// Create and start FIX TCP server
	fixServer := server.NewFIXServer(logger, sessionMgr, brokerStore, orderRouter, targetCompID)
	if err := fixServer.Start(fixListenAddr); err != nil {
		logger.Error("FIX TCP server start error", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("GarudaX FIX Protocol Gateway ready",
		slog.String("admin", adminAddr),
		slog.String("health", healthAddr),
		slog.String("fix_tcp", fixListenAddr),
	)

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	logger.Info("received signal, shutting down...", slog.String("signal", sig.String()))

	if err := fixServer.Stop(); err != nil {
		logger.Error("FIX server shutdown error", slog.String("error", err.Error()))
	}

	if err := srv.Stop(); err != nil {
		logger.Error("HTTP server shutdown error", slog.String("error", err.Error()))
	}

	logger.Info("GarudaX FIX Protocol Gateway stopped")
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
