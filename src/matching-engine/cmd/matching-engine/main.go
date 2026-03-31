package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"

	"github.com/garudax-platform/matching-engine/internal/engine"
	"github.com/garudax-platform/matching-engine/internal/observability"
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
	logger := observability.NewLogger("matching-engine")

	logger.Info("GarudaX Matching Engine starting...")

	cfg := server.ConfigFromEnv()
	idGen := &seqIDGen{}
	eng := engine.NewEngine(idGen)
	tradeStore := store.NewInMemoryTradeStore()

	// Set up trade handler for structured logging of business events
	eng.SetTradeHandler(func(trade types.Trade) {
		logger.Info("trade_matched",
			slog.String("trade_id", trade.TradeID),
			slog.String("instrument_id", trade.InstrumentID),
			slog.Uint64("quantity", trade.Quantity),
			slog.String("price", trade.Price.String()),
			slog.String("aggressor_side", trade.AggressorSide.String()),
		)
	})

	eng.SetExecReportHandler(func(report types.ExecutionReport) {
		logger.Info("execution_report",
			slog.String("exec_id", report.ExecID),
			slog.String("order_id", report.OrderID),
			slog.Int("exec_type", int(report.ExecType)),
			slog.Int("order_status", int(report.OrderStatus)),
		)
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
			logger.Error("failed to register instrument",
				slog.String("instrument", inst),
				slog.String("error", err.Error()),
			)
			os.Exit(1)
		}
		logger.Info("instrument_registered", slog.String("instrument", inst))
	}

	// Start health server in background
	go func() {
		if err := srv.StartHealthServer(); err != nil {
			logger.Error("health server error", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	// Create gRPC listener
	lis, err := srv.ListenGRPC()
	if err != nil {
		logger.Error("failed to create gRPC listener", slog.String("error", err.Error()))
		os.Exit(1)
	}

	srv.SetReady()
	logger.Info("GarudaX Matching Engine ready",
		slog.String("grpc_addr", lis.Addr().String()),
		slog.String("bind_address", cfg.BindAddress),
		slog.Int("health_port", cfg.HealthPort),
		slog.Bool("direct_pod_comms", cfg.DirectPodComms),
	)

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	logger.Info("received signal, shutting down...", slog.String("signal", sig.String()))
	lis.Close()
}
