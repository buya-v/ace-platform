package main

import (
	"database/sql"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/kafka"
	"github.com/garudax-platform/securities-service/internal/middleware"
	"github.com/garudax-platform/securities-service/internal/server"
	"github.com/garudax-platform/securities-service/internal/settlement"
	"github.com/garudax-platform/securities-service/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With(
		slog.String("service", "securities-service"),
	)

	logger.Info("GarudaX Securities Service starting...")

	cfg := server.DefaultConfig()

	// Override ports from environment if provided.
	if port := os.Getenv("API_PORT"); port != "" {
		var p int
		if _, err := parsePort(port, &p); err == nil {
			cfg.APIPort = p
		}
	}
	if port := os.Getenv("HEALTH_PORT"); port != "" {
		var p int
		if _, err := parsePort(port, &p); err == nil {
			cfg.HealthPort = p
		}
	}
	if addr := os.Getenv("BIND_ADDRESS"); addr != "" {
		cfg.BindAddress = addr
	}

	var instrumentStore store.InstrumentStore
	var orderStore store.OrderStore

	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		db, err := sql.Open("pgx", databaseURL)
		if err != nil {
			logger.Error("failed to open database", slog.String("error", err.Error()))
			os.Exit(1)
		}
		if err := db.Ping(); err != nil {
			logger.Error("database ping failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		logger.Info("connected to PostgreSQL database")
		instrumentStore = store.NewPgInstrumentStore(db)
		orderStore = store.NewPgOrderStore(db)
	} else {
		logger.Info("DATABASE_URL not set, using in-memory stores")
		instrumentStore = store.NewInMemoryInstrumentStore()
		orderStore = store.NewInMemoryOrderStore()
	}

	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	settlementStore := store.NewInMemorySettlementStore()
	corporateActionStore := store.NewInMemoryCorporateActionStore()
	entitlementStore := store.NewInMemoryEntitlementStore()

	// Market/Segment/CircuitBreaker stores (MillenniumIT P1).
	marketStore := store.NewInMemoryMarketStore()
	segmentStore := store.NewInMemorySegmentStore()
	circuitBreakerStore := store.NewInMemoryCircuitBreakerStore()

	// Firm and participant stores.
	firmStore := store.NewInMemoryFirmStore()
	participantStore := store.NewInMemoryParticipantStore()

	// Tick table, trade correction, throttle, and throttle-config stores.
	tickTableStore := store.NewInMemoryTickTableStore()
	tradeCorrectionStore := store.NewInMemoryTradeCorrectionStore()
	throttleStore := store.NewInMemoryThrottleStore()
	throttleConfigStore := store.NewInMemoryThrottleConfigStore()

	// Announcement and audit stores.
	announcementStore := store.NewInMemoryAnnouncementStore()
	auditStore := store.NewInMemoryAuditStore()

	// Pending change and reference price stores (P2c).
	pendingChangeStore := store.NewInMemoryPendingChangeStore()
	referencePriceStore := store.NewInMemoryReferencePriceStore()

	// Create a channel-based producer for local/dev. In production, swap for
	// a real Kafka wire-protocol producer behind the kafka.Producer interface.
	producer := kafka.NewChannelProducer(kafka.DefaultProducerConfig())

	// Register per-tenant topics for all configured tenants.
	validTenants := middleware.ValidTenantsFromEnv()
	logger.Info("registering tenant Kafka topics", slog.String("tenants", strings.Join(validTenants, ",")))
	for _, tenantID := range validTenants {
		producer.RegisterTopic(kafka.TopicTradeExecuted(tenantID), 256)
		producer.RegisterTopic(kafka.TopicOrderCreated(tenantID), 256)
		producer.RegisterTopic(kafka.TopicOrderCancelled(tenantID), 256)
	}

	// Settlement engine processes T+2 obligations from trades.
	settlementEngine := settlement.NewSettlementEngine(orderStore, settlementStore)

	// Circuit breaker engine shares the same store used by the HTTP handlers.
	cbEngine := engine.NewCircuitBreakerEngine(circuitBreakerStore)
	matchingEngine := engine.NewMatchingEngine(instrumentStore, orderStore, tradeStore, positionStore, producer, settlementEngine, cbEngine)

	// Auction engine collects orders during pre-open and closing auction phases.
	auctionEngine := engine.NewAuctionEngine(orderStore, tradeStore, positionStore, settlementEngine)

	// Session manager routes orders to the correct engine based on market phase.
	sessionManager := engine.NewSessionManager(auctionEngine, matchingEngine)

	// Day manager controls the overall trading day lifecycle.
	dayManager := engine.NewDayManager(sessionManager, instrumentStore)

	srv := server.New(
		instrumentStore,
		orderStore,
		tradeStore,
		positionStore,
		settlementStore,
		corporateActionStore,
		entitlementStore,
		marketStore,
		segmentStore,
		circuitBreakerStore,
		firmStore,
		participantStore,
		tickTableStore,
		tradeCorrectionStore,
		throttleStore,
		throttleConfigStore,
		announcementStore,
		auditStore,
		pendingChangeStore,
		referencePriceStore,
		store.NewInMemorySurveillanceStore(),
		store.NewInMemoryInstrumentGroupStore(),
		store.NewInMemoryOffBookTradeStore(),
		// P4a stores: locate, RFQ, give-up
		store.NewInMemoryLocateStore(),
		store.NewInMemoryRFQStore(),
		store.NewInMemoryGiveUpStore(),
		// Investigation, replay, and bond stores.
		store.NewInMemoryInvestigationStore(),
		store.NewInMemoryReplayStore(),
		store.NewInMemoryBondStore(),
		// Strategy and CSD stores.
		store.NewInMemoryStrategyStore(),
		store.NewInMemoryCustodyAccountStore(),
		store.NewInMemoryCustodyBalanceStore(),
		store.NewInMemoryCSDTransferStore(),
		dayManager,
		matchingEngine,
		sessionManager,
		settlementEngine,
		producer,
		cfg,
	)

	// Start health server on port 9089.
	go func() {
		logger.Info("health server starting",
			slog.String("bind", cfg.BindAddress),
			slog.Int("port", cfg.HealthPort),
		)
		if err := srv.StartHealthServer(); err != nil {
			logger.Error("health server failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	// Start API server on port 8089.
	go func() {
		logger.Info("API server starting",
			slog.String("bind", cfg.BindAddress),
			slog.Int("port", cfg.APIPort),
		)
		if err := srv.StartAPIServer(); err != nil {
			logger.Error("API server failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	// Mark ready after init is complete.
	srv.SetReady()
	logger.Info("GarudaX Securities Service ready",
		slog.Int("api_port", cfg.APIPort),
		slog.Int("health_port", cfg.HealthPort),
	)

	// Wait for shutdown signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	logger.Info("received signal, shutting down", slog.String("signal", sig.String()))
}

// parsePort parses a string port number into an int.
// Returns an error if the string is not a valid port.
func parsePort(s string, out *int) (string, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return s, &portError{s}
		}
		n = n*10 + int(c-'0')
	}
	if n < 1 || n > 65535 {
		return s, &portError{s}
	}
	*out = n
	return s, nil
}

type portError struct{ s string }

func (e *portError) Error() string { return "invalid port: " + e.s }
