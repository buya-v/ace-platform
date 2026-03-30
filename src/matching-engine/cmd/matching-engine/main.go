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
	tradeStore := store.NewInMemoryTradeStore()

	// Set up trade handler for logging
	eng.SetTradeHandler(func(trade types.Trade) {
		log.Printf("TRADE: %s %s qty=%d price=%s aggressor=%s",
			trade.TradeID, trade.InstrumentID,
			trade.Quantity, trade.Price.String(), trade.AggressorSide)
	})

	eng.SetExecReportHandler(func(report types.ExecutionReport) {
		log.Printf("EXEC: %s order=%s type=%d status=%d",
			report.ExecID, report.OrderID, report.ExecType, report.OrderStatus)
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
	lis.Close()
}
