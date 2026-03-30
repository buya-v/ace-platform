package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"github.com/garudax-platform/settlement-engine/internal/engine"
	"github.com/garudax-platform/settlement-engine/internal/payment"
	"github.com/garudax-platform/settlement-engine/internal/server"
	"github.com/garudax-platform/settlement-engine/internal/types"
	"github.com/garudax-platform/settlement-engine/internal/valuation"
)

type seqIDGen struct {
	counter uint64
}

func (g *seqIDGen) NewID() string {
	n := atomic.AddUint64(&g.counter, 1)
	return fmt.Sprintf("si-%d", n)
}

func main() {
	log.Println("GarudaX Settlement Engine starting...")

	cfg := server.ConfigFromEnv()
	priceStore := valuation.NewStore()
	idGen := &seqIDGen{}
	gateway := payment.NewInMemoryGateway()

	eng := engine.NewEngine(priceStore, idGen, gateway)

	eng.SetCycleHandler(func(cycle types.SettlementCycle) {
		log.Printf("SETTLEMENT: cycle=%s date=%s status=%s pay_in=%s pay_out=%s instructions=%d",
			cycle.CycleID, cycle.SettleDate.Format("2006-01-02"),
			cycle.Status.String(), cycle.TotalPayIn.String(),
			cycle.TotalPayOut.String(), len(cycle.Instructions))
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
	log.Printf("GarudaX Settlement Engine ready (gRPC=%s, health=%s:%d, direct_pod_comms=%v)",
		lis.Addr().String(), cfg.BindAddress, cfg.HealthPort, cfg.DirectPodComms)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("Received signal %s, shutting down...", sig)
	lis.Close()
}
