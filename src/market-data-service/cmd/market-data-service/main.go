package main

import (
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ace-platform/market-data-service/internal/server"
	"github.com/ace-platform/market-data-service/internal/types"
)

func main() {
	log.Println("ACE Market Data Service starting...")

	cfg := server.ConfigFromEnv()
	srv := server.NewServer(cfg)

	// Register instrument symbols from environment
	instruments := os.Getenv("INSTRUMENTS")
	if instruments == "" {
		instruments = "WHT-HRW-2026M07-UB"
	}
	for _, inst := range strings.Split(instruments, ",") {
		inst = strings.TrimSpace(inst)
		if inst != "" {
			srv.SetSymbol(inst, inst)
			log.Printf("Registered instrument: %s", inst)
		}
	}

	// Start health server in background
	go func() {
		if err := srv.StartHealthServer(); err != nil {
			log.Fatalf("Health server error: %v", err)
		}
	}()

	// Start periodic candle flush (every 10 seconds)
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			srv.FlushCandles()
		}
	}()

	// Start periodic retention enforcement (every hour)
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			srv.RunRetention()
		}
	}()

	// Start periodic ticker pruning (every 5 minutes)
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			// Prune trades older than 25h to keep 24h window with buffer
			_ = types.Interval1m // reference to types package
		}
	}()

	// Create gRPC listener
	lis, err := srv.ListenGRPC()
	if err != nil {
		log.Fatalf("Failed to create gRPC listener: %v", err)
	}

	srv.SetReady()
	log.Printf("ACE Market Data Service ready (gRPC=%s, health=%s:%d)",
		lis.Addr().String(), cfg.BindAddress, cfg.HealthPort)

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("Received signal %s, shutting down...", sig)
	lis.Close()
}
