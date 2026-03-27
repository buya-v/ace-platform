package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ace-platform/margin-engine/internal/engine"
	"github.com/ace-platform/margin-engine/internal/params"
	"github.com/ace-platform/margin-engine/internal/server"
	"github.com/ace-platform/margin-engine/internal/types"
)

type seqIDGen struct {
	counter uint64
}

func (g *seqIDGen) NewID() string {
	n := atomic.AddUint64(&g.counter, 1)
	return fmt.Sprintf("mc-%d", n)
}

// inMemoryCollateral is a simple in-memory collateral source for dev/testing.
type inMemoryCollateral struct{}

func (c *inMemoryCollateral) GetCollateral(participantID string) types.Decimal {
	// Default zero collateral; production will use a real collateral service
	return types.DecimalZero()
}

func main() {
	log.Println("ACE Margin Engine starting...")

	cfg := server.ConfigFromEnv()
	paramStore := params.NewStore()
	idGen := &seqIDGen{}
	collateral := &inMemoryCollateral{}
	callDeadline := 1 * time.Hour

	eng := engine.NewEngine(paramStore, idGen, collateral, callDeadline)

	eng.SetMarginHandler(func(pm types.PortfolioMargin) {
		log.Printf("MARGIN: participant=%s required=%s collateral=%s excess_deficit=%s",
			pm.ParticipantID, pm.TotalRequired.String(),
			pm.CollateralOnHand.String(), pm.ExcessDeficit.String())
	})

	eng.SetMarginCallHandler(func(call types.MarginCall) {
		log.Printf("MARGIN CALL: participant=%s deficit=%s deadline=%s",
			call.ParticipantID, call.Deficit.String(), call.Deadline.Format(time.RFC3339))
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
	log.Printf("ACE Margin Engine ready (gRPC=%s, health=%s:%d, direct_pod_comms=%v)",
		lis.Addr().String(), cfg.BindAddress, cfg.HealthPort, cfg.DirectPodComms)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("Received signal %s, shutting down...", sig)
	lis.Close()
}
