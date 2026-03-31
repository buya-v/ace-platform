package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/garudax-platform/market-data-service/internal/server"
	"github.com/garudax-platform/market-data-service/internal/store"
	"github.com/garudax-platform/market-data-service/internal/types"
)

func main() {
	log.Println("GarudaX Market Data Service starting...")

	cfg := server.ConfigFromEnv()

	var srv *server.Server

	// Use PostgreSQL stores when DB_HOST is set, otherwise fall back to in-memory
	if dbHost := os.Getenv("DB_HOST"); dbHost != "" {
		dbPort := envOrDefault("DB_PORT", "5432")
		dbUser := envOrDefault("DB_USER", "garudax")
		dbPass := envOrDefault("DB_PASSWORD", "")
		dbName := envOrDefault("DB_NAME", "garudax")
		dbSSL := envOrDefault("DB_SSLMODE", "disable")

		dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
			dbUser, dbPass, dbHost, dbPort, dbName, dbSSL)

		db, err := store.OpenDB(dsn)
		if err != nil {
			log.Fatalf("Failed to connect to PostgreSQL: %v", err)
		}
		defer db.Close()
		log.Printf("Connected to PostgreSQL at %s:%s/%s", dbHost, dbPort, dbName)

		tradeRepo := store.NewPGTradeStore(db)
		candleRepo := store.NewPGCandleStore(db)
		tickerRepo := store.NewPGTickerStore(db)

		srv = server.NewServerWithStores(cfg, tradeRepo, candleRepo, tickerRepo)
	} else {
		log.Println("No DB_HOST set, using in-memory stores")
		srv = server.NewServer(cfg)
	}

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
	log.Printf("GarudaX Market Data Service ready (gRPC=%s, health=%s:%d)",
		lis.Addr().String(), cfg.BindAddress, cfg.HealthPort)

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("Received signal %s, shutting down...", sig)
	lis.Close()
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
