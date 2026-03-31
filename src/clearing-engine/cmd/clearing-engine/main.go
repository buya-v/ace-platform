package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
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

	var oblStore store.ObligationStore
	if dbHost := os.Getenv("DB_HOST"); dbHost != "" {
		dbPort := 5432
		if v := os.Getenv("DB_PORT"); v != "" {
			if p, err := strconv.Atoi(v); err == nil {
				dbPort = p
			}
		}
		dbUser := envOrDefault("DB_USER", "clearing")
		dbPass := envOrDefault("DB_PASSWORD", "")
		dbName := envOrDefault("DB_NAME", "garudax")
		dbSSL := envOrDefault("DB_SSLMODE", "disable")

		db, err := store.OpenDB(dbHost, dbPort, dbUser, dbPass, dbName, dbSSL)
		if err != nil {
			log.Fatalf("Failed to connect to PostgreSQL: %v", err)
		}
		defer db.Close()
		oblStore = store.NewPostgresObligationStore(db)
		log.Printf("Using PostgreSQL store at %s:%d/%s", dbHost, dbPort, dbName)
	} else {
		oblStore = store.NewInMemoryObligationStore()
		log.Println("Using in-memory store (set DB_HOST for PostgreSQL)")
	}

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

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
