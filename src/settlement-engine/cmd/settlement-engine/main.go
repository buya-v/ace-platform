package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"

	"github.com/garudax-platform/settlement-engine/internal/engine"
	"github.com/garudax-platform/settlement-engine/internal/eventbus"
	"github.com/garudax-platform/settlement-engine/internal/payment"
	"github.com/garudax-platform/settlement-engine/internal/server"
	"github.com/garudax-platform/settlement-engine/internal/store"
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
	idGen := &seqIDGen{}
	gateway := payment.NewInMemoryGateway()

	var priceStore valuation.PriceStore

	if dbHost := os.Getenv("DB_HOST"); dbHost != "" {
		dbPort := 5432
		if v := os.Getenv("DB_PORT"); v != "" {
			if p, err := strconv.Atoi(v); err == nil {
				dbPort = p
			}
		}
		dbUser := envOrDefault("DB_USER", "settlement")
		dbPass := envOrDefault("DB_PASSWORD", "")
		dbName := envOrDefault("DB_NAME", "garudax")
		dbSSL := envOrDefault("DB_SSLMODE", "disable")

		db, err := store.OpenDB(dbHost, dbPort, dbUser, dbPass, dbName, dbSSL)
		if err != nil {
			log.Fatalf("Failed to connect to PostgreSQL: %v", err)
		}
		defer db.Close()

		priceStore = store.NewPostgresPriceStore(db)
		log.Printf("Using PostgreSQL store at %s:%d/%s", dbHost, dbPort, dbName)
	} else {
		priceStore = valuation.NewStore()
		log.Println("Using in-memory store (set DB_HOST for PostgreSQL)")
	}

	eng := engine.NewEngine(priceStore, idGen, gateway)

	// Cross-service Kafka wiring (R024): consume {tenant}.clearing.novated from
	// the clearing engine, run a settlement cycle, and publish
	// {tenant}.settlement.completed — the terminal event of the trading chain.
	// Created before the cycle handler so the handler composes logging with
	// cross-service publication. Skipped when KAFKA_BROKERS is unset.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var rt *eventbus.Runtime
	if eventbus.Enabled() {
		rt = eventbus.New(eng)
		defer func() { _ = rt.Close() }()
		go func() {
			if err := rt.Start(ctx); err != nil {
				log.Printf("settlement-engine event bus stopped: %v", err)
			}
		}()
		log.Println("Kafka cross-service consumer enabled (clearing.novated -> settlement.completed)")
	} else {
		log.Println("Kafka cross-service consumer disabled (KAFKA_BROKERS not set)")
	}

	eng.SetCycleHandler(func(cycle types.SettlementCycle) {
		log.Printf("SETTLEMENT: cycle=%s date=%s status=%s pay_in=%s pay_out=%s instructions=%d",
			cycle.CycleID, cycle.SettleDate.Format("2006-01-02"),
			cycle.Status.String(), cycle.TotalPayIn.String(),
			cycle.TotalPayOut.String(), len(cycle.Instructions))
		if rt != nil {
			rt.PublishCycle(cycle)
		}
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

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
