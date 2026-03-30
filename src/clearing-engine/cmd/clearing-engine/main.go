package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"github.com/garudax-platform/clearing-engine/internal/engine"
	"github.com/garudax-platform/clearing-engine/internal/novation"
	"github.com/garudax-platform/clearing-engine/internal/server"
	"github.com/garudax-platform/clearing-engine/internal/store"
	"github.com/garudax-platform/clearing-engine/internal/types"
)

type seqIDGen struct {
	counter uint64
}

func (g *seqIDGen) NewID() string {
	n := atomic.AddUint64(&g.counter, 1)
	return fmt.Sprintf("clr-%d", n)
}

func main() {
	log.Println("GarudaX Clearing Engine starting...")

	cfg := server.ConfigFromEnv()
	idGen := &seqIDGen{}
	oblStore := store.NewInMemoryObligationStore()
	eng := engine.NewEngine(idGen, oblStore)

	eng.SetTradeHandler(func(trade types.Trade, result novation.NovationResult) {
		log.Printf("CLEARED: trade=%s instrument=%s qty=%d price=%s buyer=%s seller=%s",
			trade.TradeID, trade.InstrumentID,
			trade.Quantity, trade.Price.String(),
			trade.BuyerParticipantID, trade.SellerParticipantID)
	})

	srv := server.NewServer(eng, cfg)

	go func() {
		if err := srv.StartHealthServer(); err != nil {
			log.Fatalf("Health server error: %v", err)
		}
	}()

	lis, err := srv.ListenGRPC()
	if err != nil {
		log.Fatalf("Failed to create gRPC listener: %v", err)
	}

	srv.SetReady()
	log.Printf("GarudaX Clearing Engine ready (gRPC=%s, health=%s:%d, direct_pod_comms=%v)",
		lis.Addr().String(), cfg.BindAddress, cfg.HealthPort, cfg.DirectPodComms)

	_ = os.Getenv("MATCHING_ENGINE_ADDR") // Will be used for trade subscription

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("Received signal %s, shutting down...", sig)
	lis.Close()
}
