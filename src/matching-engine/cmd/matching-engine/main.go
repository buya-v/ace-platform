package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"

	"github.com/garudax-platform/matching-engine/internal/engine"
	"github.com/garudax-platform/matching-engine/internal/server"
	"github.com/garudax-platform/matching-engine/internal/store"
	"github.com/garudax-platform/matching-engine/internal/types"
)

// seqIDGen is a simple sequential ID generator for development.
// In production, replace with UUID v7 generator.
type seqIDGen struct {
	counter uint64
}

func (g *seqIDGen) NewID() string {
	n := atomic.AddUint64(&g.counter, 1)
	return fmt.Sprintf("id-%d", n)
}

func main() {
	log.Println("GarudaX Matching Engine starting...")

	cfg := server.ConfigFromEnv()
	idGen := &seqIDGen{}
	eng := engine.NewEngine(idGen)

	// Choose trade store based on DB_HOST environment variable.
	// If DB_HOST is set, use PostgreSQL async trade writer.
	// Otherwise, use in-memory store (development/testing).
	var tradeStore store.TradeStore
	var pgStore *store.PostgresTradeStore

	if dbHost := os.Getenv("DB_HOST"); dbHost != "" {
		dsn := store.BuildDSN(
			dbHost,
			os.Getenv("DB_PORT"),
			os.Getenv("DB_USER"),
			os.Getenv("DB_PASSWORD"),
			os.Getenv("DB_NAME"),
			os.Getenv("DB_SSLMODE"),
		)

		db, err := store.ConnectPostgres(dsn)
		if err != nil {
			log.Fatalf("Failed to connect to PostgreSQL: %v", err)
		}
		log.Printf("Connected to PostgreSQL at %s", dbHost)

		pgStore = store.NewPostgresTradeStore(store.PostgresConfig{DB: db})
		tradeStore = pgStore
	} else {
		log.Println("No DB_HOST set — using in-memory trade store")
		tradeStore = store.NewInMemoryTradeStore()
	}

	// Set up trade handler: log + persist to trade store
	eng.SetTradeHandler(func(trade types.Trade) {
		log.Printf("TRADE: %s %s qty=%d price=%s aggressor=%s",
			trade.TradeID, trade.InstrumentID,
			trade.Quantity, trade.Price.String(), trade.AggressorSide)
	})

	// Set up execution report handler: log + persist if PostgreSQL is configured
	eng.SetExecReportHandler(func(report types.ExecutionReport) {
		log.Printf("EXEC: %s order=%s type=%d status=%d",
			report.ExecID, report.OrderID, report.ExecType, report.OrderStatus)
		if pgStore != nil {
			pgStore.AppendExecutionReport(report)
		}
	})

	srv := server.NewServer(eng, tradeStore, cfg)

	// Register instruments from environment or defaults
	instruments := os.Getenv("INSTRUMENTS")
	if instruments == "" {
		instruments = "WHT-HRW-2026M07-UB"
	}
	for _, inst := range strings.Split(instruments, ",") {
		inst = strings.TrimSpace(inst)
		if inst == "" {
			continue
		}
		if err := srv.RegisterInstrument(inst); err != nil {
			log.Fatalf("Failed to register instrument %s: %v", inst, err)
		}
		log.Printf("Registered instrument: %s", inst)
	}

	// Start health server in background
	go func() {
		if err := srv.StartHealthServer(); err != nil {
			log.Fatalf("Health server error: %v", err)
		}
	}()

	// Create gRPC listener
	lis, err := srv.ListenGRPC()
	if err != nil {
		log.Fatalf("Failed to create gRPC listener: %v", err)
	}

	srv.SetReady()
	log.Printf("GarudaX Matching Engine ready (gRPC=%s, health=%s:%d, direct_pod_comms=%v)",
		lis.Addr().String(), cfg.BindAddress, cfg.HealthPort, cfg.DirectPodComms)

	// Wait for shutdown signal
	// In production, the gRPC server (grpc.NewServer()) would serve on this listener.
	// For now, we hold the listener open and wait for shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("Received signal %s, shutting down...", sig)

	// Graceful shutdown: flush pending writes
	if pgStore != nil {
		log.Println("Flushing pending PostgreSQL writes...")
		if err := pgStore.Close(); err != nil {
			log.Printf("WARN: error closing PostgreSQL store: %v", err)
		}
	}

	lis.Close()
}
