// Package server provides the HTTP server for the securities-service.
package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/kafka"
	"github.com/garudax-platform/securities-service/internal/middleware"
	"github.com/garudax-platform/securities-service/internal/settlement"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// Config holds the server configuration.
type Config struct {
	// APIPort is the port for the main API HTTP server.
	APIPort int
	// HealthPort is the port for the health/readiness HTTP server.
	HealthPort int
	// BindAddress is the interface address to bind to (default "0.0.0.0").
	BindAddress string
}

// DefaultConfig returns a Config with the standard port allocation for securities-service.
func DefaultConfig() Config {
	return Config{
		APIPort:     8089,
		HealthPort:  9089,
		BindAddress: "0.0.0.0",
	}
}

// Server is the HTTP server for the securities-service.
type Server struct {
	cfg                  Config
	instrumentStore      store.InstrumentStore
	orderStore           store.OrderStore
	tradeStore           store.TradeStore
	positionStore        store.PositionStore
	settlementStore      store.SettlementStore
	corporateActionStore store.CorporateActionStore
	entitlementStore     store.EntitlementStore
	marketStore          store.MarketStore
	segmentStore         store.SegmentStore
	circuitBreakerStore  store.CircuitBreakerStore
	firmStore            store.FirmStore
	participantStore     store.ParticipantStore
	tickTableStore       store.TickTableStore
	tradeCorrectionStore store.TradeCorrectionStore
	throttleStore        store.ThrottleStore
	throttleConfigStore  store.ThrottleConfigStore
	announcementStore    store.AnnouncementStore
	auditStore           store.AuditStore
	pendingChangeStore   store.PendingChangeStore
	referencePriceStore  store.ReferencePriceStore
	surveillanceStore    store.SurveillanceStore
	instrumentGroupStore store.InstrumentGroupStore
	offBookTradeStore    store.OffBookTradeStore
	nodeStore            store.NodeStore
	locateStore          store.LocateStore
	rfqStore             store.RFQStore
	giveUpStore          store.GiveUpStore
	investigationStore   store.InvestigationStore
	replayStore          store.ReplayStore
	bondStore            store.BondStore
	strategyStore        store.StrategyStore
	custodyAccountStore  store.CustodyAccountStore
	custodyBalanceStore  store.CustodyBalanceStore
	csdTransferStore     store.CSDTransferStore
	watchListStore        store.WatchListStore
	ipRestrictionStore    store.IPRestrictionStore
	passwordPolicyStore   store.PasswordPolicyStore
	tradingParamSetStore  store.TradingParamSetStore
	tradingCycleStore     store.TradingCycleStore // T1: trading cycle management
	historyStore          store.HistoryStore        // Part A: history archive
	postTradeParamsStore  store.PostTradeParamsStore // Part B: post-trade parameters
	configTableStore      store.ConfigTableStore    // Part C: tabular structures
	clientStore           store.ClientStore         // Part D: client entities
	indexStore            store.IndexStore            // Sprint 8 Part A: indices
	entityPermissionStore store.EntityPermissionStore // Sprint 8 Part B: entity permissions
	folderStore           store.FolderStore           // Sprint 8 Part C: folders
	warningStore          store.WarningStore          // Sprint 8 Part D: warnings
	dayManager            *engine.DayManager
	engine               *engine.MatchingEngine
	sessionManager       *engine.SessionManager
	settlementEngine     *settlement.SettlementEngine
	producer             kafka.Producer
	db                   *sql.DB
	ready                atomic.Int32
	privilegeEngine      *engine.PrivilegeEngine
	roleStore            store.RoleStore
}

// New creates a new Server with the given stores, matching engine, and configuration.
// producer may be nil; if so, order events are not published.
// settlementEngine and settlementStore may be nil; if so, settlement endpoints return 503.
// marketStore, segmentStore, and circuitBreakerStore may be nil; if so, those endpoints
// return 503.
// firmStore, participantStore, and dayManager may be nil; if so, those endpoints return 503.
// tradeCorrectionStore may be nil; if so, trade correction endpoints return 503.
// announcementStore and auditStore may be nil; if so, those endpoints return 503.
// pendingChangeStore and referencePriceStore may be nil; if so, those endpoints return 503.
// surveillanceStore, instrumentGroupStore, offBookTradeStore, and nodeStore may be nil; if so, those
// endpoints return 503.
// locateStore, rfqStore, and giveUpStore may be nil; if so, those P4a endpoints return 503.
// investigationStore, replayStore, and bondStore may be nil; if so, those endpoints return 503.
// throttleConfigStore may be nil; if so, order throttle falls back to the default 100 orders/sec.
// watchListStore, ipRestrictionStore, and passwordPolicyStore may be nil; if so, those endpoints return 503.
// privilegeEngine and roleStore may be nil; if so, permission checks are skipped (backwards compat).
// tradingParamSetStore may be nil; if so, trading-params endpoints return 503.
// historyStore, postTradeParamsStore, configTableStore, and clientStore may be nil;
// if so, those endpoints return 503.
func New(
	instrumentStore store.InstrumentStore,
	orderStore store.OrderStore,
	tradeStore store.TradeStore,
	positionStore store.PositionStore,
	settlementStore store.SettlementStore,
	corporateActionStore store.CorporateActionStore,
	entitlementStore store.EntitlementStore,
	marketStore store.MarketStore,
	segmentStore store.SegmentStore,
	circuitBreakerStore store.CircuitBreakerStore,
	firmStore store.FirmStore,
	participantStore store.ParticipantStore,
	tickTableStore store.TickTableStore,
	tradeCorrectionStore store.TradeCorrectionStore,
	throttleStore store.ThrottleStore,
	throttleConfigStore store.ThrottleConfigStore,
	announcementStore store.AnnouncementStore,
	auditStore store.AuditStore,
	pendingChangeStore store.PendingChangeStore,
	referencePriceStore store.ReferencePriceStore,
	surveillanceStore store.SurveillanceStore,
	instrumentGroupStore store.InstrumentGroupStore,
	offBookTradeStore store.OffBookTradeStore,
	nodeStore store.NodeStore,
	locateStore store.LocateStore,
	rfqStore store.RFQStore,
	giveUpStore store.GiveUpStore,
	investigationStore store.InvestigationStore,
	replayStore store.ReplayStore,
	bondStore store.BondStore,
	strategyStore store.StrategyStore,
	custodyAccountStore store.CustodyAccountStore,
	custodyBalanceStore store.CustodyBalanceStore,
	csdTransferStore store.CSDTransferStore,
	watchListStore store.WatchListStore,
	ipRestrictionStore store.IPRestrictionStore,
	passwordPolicyStore store.PasswordPolicyStore,
	tradingCycleStore store.TradingCycleStore,
	dayManager *engine.DayManager,
	matchingEngine *engine.MatchingEngine,
	sessionManager *engine.SessionManager,
	settlementEngine *settlement.SettlementEngine,
	producer kafka.Producer,
	privilegeEngine *engine.PrivilegeEngine,
	roleStore store.RoleStore,
	tradingParamSetStore store.TradingParamSetStore,
	cfg Config,
) *Server {
	return &Server{
		cfg:                  cfg,
		instrumentStore:      instrumentStore,
		orderStore:           orderStore,
		tradeStore:           tradeStore,
		positionStore:        positionStore,
		settlementStore:      settlementStore,
		corporateActionStore: corporateActionStore,
		entitlementStore:     entitlementStore,
		marketStore:          marketStore,
		segmentStore:         segmentStore,
		circuitBreakerStore:  circuitBreakerStore,
		firmStore:            firmStore,
		participantStore:     participantStore,
		tickTableStore:       tickTableStore,
		tradeCorrectionStore: tradeCorrectionStore,
		throttleStore:        throttleStore,
		throttleConfigStore:  throttleConfigStore,
		announcementStore:    announcementStore,
		auditStore:           auditStore,
		pendingChangeStore:   pendingChangeStore,
		referencePriceStore:  referencePriceStore,
		surveillanceStore:    surveillanceStore,
		instrumentGroupStore: instrumentGroupStore,
		offBookTradeStore:    offBookTradeStore,
		nodeStore:            nodeStore,
		locateStore:          locateStore,
		rfqStore:             rfqStore,
		giveUpStore:          giveUpStore,
		investigationStore:   investigationStore,
		replayStore:          replayStore,
		bondStore:            bondStore,
		strategyStore:        strategyStore,
		custodyAccountStore:  custodyAccountStore,
		custodyBalanceStore:  custodyBalanceStore,
		csdTransferStore:     csdTransferStore,
		watchListStore:      watchListStore,
		ipRestrictionStore:  ipRestrictionStore,
		passwordPolicyStore: passwordPolicyStore,
		tradingCycleStore:   tradingCycleStore,
		dayManager:          dayManager,
		engine:               matchingEngine,
		sessionManager:       sessionManager,
		settlementEngine:     settlementEngine,
		producer:             producer,
		privilegeEngine:      privilegeEngine,
		roleStore:            roleStore,
		tradingParamSetStore: tradingParamSetStore,
	}
}

// SetTradingCycleStore wires the trading cycle store into the server after construction.
// This is separate from New() to avoid breaking existing test call sites.
func (s *Server) SetTradingCycleStore(tcs store.TradingCycleStore) {
	s.tradingCycleStore = tcs
}

// SetHistoryStore wires the history store into the server after construction (Part A).
func (s *Server) SetHistoryStore(hs store.HistoryStore) {
	s.historyStore = hs
}

// SetPostTradeParamsStore wires the post-trade params store into the server after construction (Part B).
func (s *Server) SetPostTradeParamsStore(ps store.PostTradeParamsStore) {
	s.postTradeParamsStore = ps
}

// SetConfigTableStore wires the config table store into the server after construction (Part C).
func (s *Server) SetConfigTableStore(cs store.ConfigTableStore) {
	s.configTableStore = cs
}

// SetClientStore wires the client store into the server after construction (Part D).
func (s *Server) SetClientStore(cs store.ClientStore) {
	s.clientStore = cs
}

// SetIndexStore wires the index store into the server (Sprint 8 Part A).
func (s *Server) SetIndexStore(is store.IndexStore) {
	s.indexStore = is
}

// SetEntityPermissionStore wires the entity permission store into the server (Sprint 8 Part B).
func (s *Server) SetEntityPermissionStore(eps store.EntityPermissionStore) {
	s.entityPermissionStore = eps
}

// SetFolderStore wires the folder store into the server (Sprint 8 Part C).
func (s *Server) SetFolderStore(fs store.FolderStore) {
	s.folderStore = fs
}

// SetWarningStore wires the warning store into the server (Sprint 8 Part D).
func (s *Server) SetWarningStore(ws store.WarningStore) {
	s.warningStore = ws
}

// checkPermission validates that the caller identified by X-Participant-ID holds
// requiredPerm.  Returns nil when:
//   - X-Participant-ID header is absent (backwards compat — public/system endpoint)
//   - privilegeEngine is nil (RBAC not configured)
//   - The participant holds the permission
//
// Returns an error when the participant exists but lacks the permission.
func (s *Server) checkPermission(r *http.Request, requiredPerm string) error {
	if s.privilegeEngine == nil {
		return nil
	}
	participantID := r.Header.Get("X-Participant-ID")
	if participantID == "" {
		return nil
	}
	return s.privilegeEngine.HasPermission(participantID, requiredPerm)
}

// SetReady marks the server as ready to serve traffic.
func (s *Server) SetReady() {
	s.ready.Store(1)
}

// isReady reports whether the server has been marked ready.
func (s *Server) isReady() bool {
	return s.ready.Load() == 1
}

// SetDB wires an optional *sql.DB into the server so the health endpoint
// can include a live database connectivity check.
func (s *Server) SetDB(db *sql.DB) {
	s.db = db
}

// StartHealthServer starts the health/readiness HTTP server on HealthPort.
// It blocks until the server fails; call it in a goroutine.
func (s *Server) StartHealthServer() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/readyz", s.readyz)

	addr := fmt.Sprintf("%s:%d", s.cfg.BindAddress, s.cfg.HealthPort)
	return http.ListenAndServe(addr, mux)
}

// StartAPIServer starts the main API HTTP server on APIPort.
// It blocks until the server fails; call it in a goroutine.
// TenantMiddleware is applied to the API handler chain using VALID_TENANTS env var.
func (s *Server) StartAPIServer() error {
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	// Wrap the mux with tenant middleware.
	tenantMW := middleware.TenantMiddleware(middleware.ValidTenantsFromEnv())
	handler := tenantMW(mux)

	addr := fmt.Sprintf("%s:%d", s.cfg.BindAddress, s.cfg.APIPort)
	return http.ListenAndServe(addr, handler)
}

// registerRoutes wires all API routes onto the given ServeMux.
// Placeholder handlers are used for routes not yet implemented.
func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Instruments — the wildcard handler also dispatches the reference-price sub-resource.
	mux.HandleFunc("/api/v1/securities/instruments", s.handleInstruments)
	mux.HandleFunc("/api/v1/securities/instruments/", s.handleInstrumentOrReferencePrice)

	// Orders — mass-cancel must be registered before the wildcard orders/ route.
	mux.HandleFunc("/api/v1/securities/orders/mass-cancel", s.handleMassCancel)
	mux.HandleFunc("/api/v1/securities/orders", s.handleOrders)
	mux.HandleFunc("/api/v1/securities/orders/", s.handleOrder)

	// Settlements
	mux.HandleFunc("/api/v1/securities/settlements", s.handleSettlements)
	mux.HandleFunc("/api/v1/securities/settlements/cycle", s.handleSettlementCycle)

	// Corporate Actions
	mux.HandleFunc("/api/v1/securities/corporate-actions", s.handleCorporateActions)
	mux.HandleFunc("/api/v1/securities/corporate-actions/", s.handleCorporateAction)

	// Sessions
	mux.HandleFunc("/api/v1/securities/sessions", s.handleSessions)
	mux.HandleFunc("/api/v1/securities/sessions/", s.handleSessionOrAdjustment)

	// FRC Reports
	mux.HandleFunc("/api/v1/securities/reports/frc", s.handleFRCReport)

	// Markets and Segments (MillenniumIT P1)
	mux.HandleFunc("/api/v1/securities/markets", s.handleMarkets)
	mux.HandleFunc("/api/v1/securities/markets/", s.handleMarket)
	mux.HandleFunc("/api/v1/securities/segments", s.handleSegments)
	mux.HandleFunc("/api/v1/securities/segments/", s.handleSegment)

	// Circuit Breakers (MillenniumIT P1)
	mux.HandleFunc("/api/v1/securities/circuit-breakers", s.handleCircuitBreakers)
	mux.HandleFunc("/api/v1/securities/circuit-breakers/", s.handleCircuitBreaker)

	// Firms
	mux.HandleFunc("/api/v1/securities/firms", s.handleFirms)
	mux.HandleFunc("/api/v1/securities/firms/", s.handleFirm)

	// Participants
	mux.HandleFunc("/api/v1/securities/participants", s.handleParticipants)
	mux.HandleFunc("/api/v1/securities/participants/", s.handleParticipant)

	// Announcements
	mux.HandleFunc("/api/v1/securities/announcements", s.handleAnnouncements)

	// Audit trail
	mux.HandleFunc("/api/v1/securities/audit-trail", s.handleAuditTrail)

	// Day lifecycle
	mux.HandleFunc("/api/v1/securities/day/status", s.handleDayStatus)
	mux.HandleFunc("/api/v1/securities/day/start", s.handleDayStart)
	mux.HandleFunc("/api/v1/securities/day/trading", s.handleDayTrading)
	mux.HandleFunc("/api/v1/securities/day/end-trading", s.handleDayEndTrading)
	mux.HandleFunc("/api/v1/securities/day/end", s.handleDayEnd)

	// Trades and trade corrections (Part A)
	mux.HandleFunc("/api/v1/securities/trades", s.handleTrades)
	mux.HandleFunc("/api/v1/securities/trades/", s.handleTrade)

	// Tick tables (Part B)
	mux.HandleFunc("/api/v1/securities/tick-tables", s.handleTickTables)
	mux.HandleFunc("/api/v1/securities/tick-tables/", s.handleTickTable)

	// Positions (P2c Part E) — must be registered before the instruments/ wildcard above,
	// but positions is its own top-level path so order does not conflict.
	mux.HandleFunc("/api/v1/securities/positions", s.handlePositions)

	// Pending changes (P2c Part C)
	mux.HandleFunc("/api/v1/securities/pending-changes", s.handlePendingChanges)
	mux.HandleFunc("/api/v1/securities/pending-changes/", s.handlePendingChange)

	// Surveillance
	mux.HandleFunc("/api/v1/securities/surveillance/dashboard", s.handleSurveillanceDashboard)
	mux.HandleFunc("/api/v1/securities/surveillance/alerts", s.handleSurveillanceAlerts)
	mux.HandleFunc("/api/v1/securities/surveillance/alerts/", s.handleSurveillanceAlert)
	mux.HandleFunc("/api/v1/securities/surveillance/thresholds/", s.handleSurveillanceThresholds)
	// Firm view — must be registered before the wildcard surveillance/ route would conflict.
	mux.HandleFunc("/api/v1/securities/surveillance/firm-view/", s.handleFirmView)

	// Instrument groups
	mux.HandleFunc("/api/v1/securities/instrument-groups", s.handleInstrumentGroups)
	mux.HandleFunc("/api/v1/securities/instrument-groups/", s.handleInstrumentGroup)

	// Off-book trades
	mux.HandleFunc("/api/v1/securities/off-book-trades", s.handleOffBookTrades)
	mux.HandleFunc("/api/v1/securities/off-book-trades/", s.handleOffBookTrade)

	// Node hierarchy
	mux.HandleFunc("/api/v1/securities/nodes", s.handleNodes)
	mux.HandleFunc("/api/v1/securities/nodes/", s.handleNodeItem)

	// Market data (P3b Part B)
	mux.HandleFunc("/api/v1/securities/market-data/book/", s.handleMarketDataBook)
	mux.HandleFunc("/api/v1/securities/market-data/ticker/", s.handleMarketDataTicker)
	mux.HandleFunc("/api/v1/securities/market-data/trades/", s.handleMarketDataTrades)

	// Service desk (P3b Part C)
	mux.HandleFunc("/api/v1/securities/service-desk/orders", s.handleServiceDeskSubmitOrder)
	mux.HandleFunc("/api/v1/securities/service-desk/cancel-order", s.handleServiceDeskCancelOrder)

	// Bulk upload (P3b Part D)
	mux.HandleFunc("/api/v1/securities/bulk/instruments", s.handleBulkInstruments)
	mux.HandleFunc("/api/v1/securities/bulk/instruments/csv", s.handleBulkInstrumentsCSV)
	mux.HandleFunc("/api/v1/securities/bulk/instruments/amend", s.handleBulkInstrumentsAmend)

	// P4a — Locates (short-sell locate requests)
	mux.HandleFunc("/api/v1/securities/locates", s.handleLocates)
	mux.HandleFunc("/api/v1/securities/locates/", s.handleLocateAction)

	// P4a — RFQ (requests for quote)
	mux.HandleFunc("/api/v1/securities/rfq", s.handleRFQs)
	mux.HandleFunc("/api/v1/securities/rfq/", s.handleRFQAction)

	// P4a — Give-ups (trade give-up instructions)
	// Note: give-up initiation is under /trades/{id}/give-up, wired via the handleTrade wildcard.
	mux.HandleFunc("/api/v1/securities/give-ups", s.handleGiveUps)
	mux.HandleFunc("/api/v1/securities/give-ups/", s.handleGiveUpAction)

	// Surveillance investigations
	mux.HandleFunc("/api/v1/securities/investigations", s.handleInvestigations)
	mux.HandleFunc("/api/v1/securities/investigations/", s.handleInvestigation)

	// Market replay
	mux.HandleFunc("/api/v1/securities/replay/sessions", s.handleReplaySessions)
	mux.HandleFunc("/api/v1/securities/replay/sessions/", s.handleReplaySession)

	// Fixed-income bonds
	mux.HandleFunc("/api/v1/securities/bonds", s.handleBonds)
	mux.HandleFunc("/api/v1/securities/bonds/", s.handleBond)

	// Trading strategies
	mux.HandleFunc("/api/v1/securities/strategies", s.handleStrategies)
	mux.HandleFunc("/api/v1/securities/strategies/", s.handleStrategy)

	// CSD — custody accounts, balances, transfers
	mux.HandleFunc("/api/v1/securities/csd/accounts", s.handleCustodyAccounts)
	mux.HandleFunc("/api/v1/securities/csd/accounts/", s.handleCustodyAccount)
	mux.HandleFunc("/api/v1/securities/csd/transfers", s.handleCSDTransfers)
	mux.HandleFunc("/api/v1/securities/csd/transfers/", s.handleCSDTransfer)

	// Throttle config — per-firm rate limit configuration
	// The collection route must be registered before the wildcard item route.
	mux.HandleFunc("/api/v1/securities/throttle-config", s.handleThrottleConfigs)
	mux.HandleFunc("/api/v1/securities/throttle-config/", s.handleThrottleConfig)

	// Server-Sent Events (SSE) — real-time market event stream.
	mux.HandleFunc("/api/v1/securities/events", s.handleSSE)

	// Watch lists — named collections of instruments/clients/firms for monitoring.
	mux.HandleFunc("/watchlists", s.handleWatchLists)
	mux.HandleFunc("/watchlists/", s.handleWatchList)

	// Trade capture reports — firm-level aggregated trade reports.
	mux.HandleFunc("/api/v1/securities/trade-capture-reports", s.handleTradeCaptureReports)

	// IP restrictions — per-participant IP allow-list management.
	mux.HandleFunc("/ip-restrictions/", s.handleIPRestriction)

	// Password policy — per-tenant password complexity and expiry rules.
	mux.HandleFunc("/password-policy", s.handlePasswordPolicy)

	// RBAC Roles — create, list, get, update, delete.
	mux.HandleFunc("/api/v1/securities/roles", s.handleRoles)
	mux.HandleFunc("/api/v1/securities/roles/", s.handleRole)

	// Trading parameter sets — unified per-instrument trading controls.
	// The instrument sub-route must be registered before the wildcard item route.
	mux.HandleFunc("/api/v1/securities/trading-params/instrument/", s.handleTradingParamByInstrument)
	mux.HandleFunc("/api/v1/securities/trading-params", s.handleTradingParams)
	mux.HandleFunc("/api/v1/securities/trading-params/", s.handleTradingParamItem)

	// Trading cycles (T1) — named session-sequence definitions per market.
	mux.HandleFunc("/api/v1/securities/trading-cycles", s.handleTradingCycles)
	mux.HandleFunc("/api/v1/securities/trading-cycles/", s.handleTradingCycle)

	// Part A — History archive.
	mux.HandleFunc("/api/v1/securities/history/", s.handleHistory)

	// Part B — Post-trade parameters.
	// Instrument sub-route must be registered before the wildcard item route.
	mux.HandleFunc("/api/v1/securities/post-trade-params/instrument/", s.handlePostTradeParamsByInstrument)
	mux.HandleFunc("/api/v1/securities/post-trade-params", s.handlePostTradeParams)
	mux.HandleFunc("/api/v1/securities/post-trade-params/", s.handlePostTradeParamItem)

	// Part C — Config tables.
	mux.HandleFunc("/api/v1/securities/config-tables", s.handleConfigTables)
	mux.HandleFunc("/api/v1/securities/config-tables/", s.handleConfigTableItem)

	// Part D — Client entities.
	mux.HandleFunc("/api/v1/securities/clients", s.handleClients)
	mux.HandleFunc("/api/v1/securities/clients/", s.handleClientItem)

	// Sprint 8 Part A — Indices.
	mux.HandleFunc("/api/v1/securities/indices", s.handleIndices)
	mux.HandleFunc("/api/v1/securities/indices/", s.handleIndexItem)

	// Sprint 8 Part B — Entity permissions.
	mux.HandleFunc("/api/v1/securities/entity-permissions", s.handleEntityPermissions)
	mux.HandleFunc("/api/v1/securities/entity-permissions/", s.handleEntityPermissionItem)

	// Sprint 8 Part C — Instrument folders.
	mux.HandleFunc("/api/v1/securities/folders", s.handleFolders)
	mux.HandleFunc("/api/v1/securities/folders/", s.handleFolderItem)

	// Sprint 8 Part D — Warnings.
	mux.HandleFunc("/api/v1/securities/warnings", s.handleWarnings)
	mux.HandleFunc("/api/v1/securities/warnings/", s.handleWarningItem)

	// Demo reset — clears all in-memory state (no auth required in demo mode).
	mux.HandleFunc("/api/v1/securities/demo/reset", s.handleDemoReset)
}

// --- Health endpoints ---

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	dbStatus := "ok"
	if s.db != nil {
		if err := s.db.Ping(); err != nil {
			dbStatus = "unreachable"
		}
	}

	if dbStatus != "ok" && s.db != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"status":   "degraded",
			"service":  "securities-service",
			"database": dbStatus,
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":   "ok",
		"service":  "securities-service",
		"database": dbStatus,
	})
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !s.isReady() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "not_ready",
		})
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ready",
	})
}

// --- Instrument route handlers ---

// handleInstruments dispatches GET /api/v1/securities/instruments (list)
// and POST /api/v1/securities/instruments (create).
func (s *Server) handleInstruments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListInstruments(w, r)
	case http.MethodPost:
		s.handleCreateInstrument(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleInstrumentOrReferencePrice is the wildcard handler for /api/v1/securities/instruments/.
// It dispatches to the reference-price sub-resource handler or the standard instrument handler.
func (s *Server) handleInstrumentOrReferencePrice(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(strings.TrimSuffix(r.URL.Path, "/"), "/reference-price") {
		s.handleReferencePrice(w, r)
		return
	}
	s.handleInstrument(w, r)
}

// handleInstrument dispatches GET/PATCH /api/v1/securities/instruments/{id}
// and PUT /api/v1/securities/instruments/{id}/status.
func (s *Server) handleInstrument(w http.ResponseWriter, r *http.Request) {
	// Detect the /status sub-resource.
	if strings.HasSuffix(strings.TrimSuffix(r.URL.Path, "/"), "/status") {
		if r.Method == http.MethodPut {
			s.handleUpdateInstrumentStatus(w, r)
		} else {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetInstrument(w, r)
	case http.MethodPatch:
		s.handleUpdateInstrument(w, r)
	case http.MethodDelete:
		s.handleDeleteInstrument(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// --- Order route handlers ---

// handleOrders dispatches GET /api/v1/securities/orders (list)
// and POST /api/v1/securities/orders (submit).
func (s *Server) handleOrders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListOrders(w, r)
	case http.MethodPost:
		s.handleSubmitOrder(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleOrder dispatches GET/DELETE /api/v1/securities/orders/{id}.
func (s *Server) handleOrder(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetOrder(w, r)
	case http.MethodDelete:
		s.handleCancelOrder(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// --- JSON helpers ---

func (s *Server) writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func (s *Server) writeError(w http.ResponseWriter, status int, code, message string, details []string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(types.ErrorResponse{
		Error: types.ErrorDetail{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}
