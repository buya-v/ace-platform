// Package server — tests for market/segment halt cascade handlers.
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/middleware"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// newHaltTestServer creates a test server wired with real market, segment,
// instrument, and audit stores so that halt-cascade tests can exercise the
// full handler chain.
func newHaltTestServer(t *testing.T) (
	ts *httptest.Server,
	instrStore *store.InMemoryInstrumentStore,
	segStore *store.InMemorySegmentStore,
	mktStore *store.InMemoryMarketStore,
	auditStore *store.InMemoryAuditStore,
) {
	t.Helper()
	instrStore = store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)
	mktStore = store.NewInMemoryMarketStore()
	segStore = store.NewInMemorySegmentStore()
	auditStore = store.NewInMemoryAuditStore()

	cfg := DefaultConfig()
	srv := New(
		instrStore, orderStore, tradeStore, positionStore,
		nil, // settlementStore
		store.NewInMemoryCorporateActionStore(),
		store.NewInMemoryEntitlementStore(),
		mktStore,
		segStore,
		store.NewInMemoryCircuitBreakerStore(),
		store.NewInMemoryFirmStore(),
		store.NewInMemoryParticipantStore(),
		nil, // tickTableStore
		nil, // tradeCorrectionStore
		nil, // throttleStore
		nil, // throttleConfigStore
		nil, // announcementStore
		auditStore,
		nil, // pendingChangeStore
		nil, // referencePriceStore
		nil, // surveillanceStore
		nil, // instrumentGroupStore
		nil, // offBookTradeStore
		nil, // nodeStore
		nil, // locateStore
		nil, // rfqStore
		nil, // giveUpStore
		nil, // investigationStore
		nil, // replayStore
		nil, // bondStore
		nil, // strategyStore
		nil, // custodyAccountStore
		nil, // custodyBalanceStore
		nil, // csdTransferStore
		nil, // watchListStore
		nil, // ipRestrictionStore
		nil, // passwordPolicyStore
		nil, // dayManager
		me,
		nil, // sessionManager
		nil, // settlementEngine
		nil, // producer
		cfg,
	)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	handler := tenantMW(mux)

	ts = httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts, instrStore, segStore, mktStore, auditStore
}

// createInstrumentWithSegment creates an instrument with a specific segment_id
// directly in the store, bypassing the HTTP layer for setup convenience.
func createInstrumentWithSegment(t *testing.T, s *store.InMemoryInstrumentStore, id, segmentID string) {
	t.Helper()
	instr := &types.Instrument{
		ID:            id,
		Ticker:        id,
		Name:          id + " Corp",
		AssetClass:    types.AssetClassEquity,
		TradingStatus: types.TradingStatusActive,
		LotSize:       100,
		TickSize:      0.01,
		SegmentID:     segmentID,
	}
	if err := s.Create(instr); err != nil {
		t.Fatalf("create instrument %s: %v", id, err)
	}
}

// ============================================================
// TestMarketHaltCascade — market status → instrument cascade
// ============================================================

// TestMarketHalt_CascadesInstrumentsToHalted verifies that setting a market to
// MARKET_HALTED cascades HALTED to all instruments in segments of that market.
func TestMarketHalt_CascadesInstrumentsToHalted(t *testing.T) {
	ts, instrStore, segStore, _, _ := newHaltTestServer(t)

	// The default InMemorySegmentStore has one segment "EQUITY" in market "MSE".
	// Create instruments in that segment.
	createInstrumentWithSegment(t, instrStore, "INSTR-1", "EQUITY")
	createInstrumentWithSegment(t, instrStore, "INSTR-2", "EQUITY")

	// Also create a segment in a different market to confirm it is NOT cascaded.
	err := segStore.Create(&types.Segment{
		ID:       "OTHER-SEG",
		MarketID: "OTHER-MKT",
		Name:     "Other Segment",
		Status:   types.SegActive,
	})
	if err != nil {
		t.Fatalf("create other segment: %v", err)
	}
	createInstrumentWithSegment(t, instrStore, "INSTR-OTHER", "OTHER-SEG")

	// PUT /api/v1/securities/markets/MSE/status → MARKET_HALTED
	resp := doJSON(t, ts, http.MethodPut, "/api/v1/securities/markets/MSE/status",
		map[string]string{"status": types.MarketHalted})
	assertStatus(t, resp, http.StatusOK)

	var mkt map[string]interface{}
	decodeBody(t, resp, &mkt)
	if mkt["status"] != types.MarketHalted {
		t.Errorf("market status: want %q, got %v", types.MarketHalted, mkt["status"])
	}

	// Instruments in MSE's segments should be HALTED.
	for _, id := range []string{"INSTR-1", "INSTR-2"} {
		got, err := instrStore.Get(id)
		if err != nil {
			t.Fatalf("get instrument %s: %v", id, err)
		}
		if got.TradingStatus != types.TradingStatusHalted {
			t.Errorf("instrument %s: want HALTED, got %q", id, got.TradingStatus)
		}
	}

	// Instrument in a different market's segment should remain ACTIVE.
	other, _ := instrStore.Get("INSTR-OTHER")
	if other.TradingStatus != types.TradingStatusActive {
		t.Errorf("instrument INSTR-OTHER: want ACTIVE (no cascade), got %q", other.TradingStatus)
	}
}

// TestMarketActive_CascadesInstrumentsToActive verifies that setting a market
// to MARKET_ACTIVE restores all instruments in that market's segments to ACTIVE.
func TestMarketActive_CascadesInstrumentsToActive(t *testing.T) {
	ts, instrStore, _, _, _ := newHaltTestServer(t)

	// Create instruments and pre-halt them.
	createInstrumentWithSegment(t, instrStore, "INSTR-A", "EQUITY")
	createInstrumentWithSegment(t, instrStore, "INSTR-B", "EQUITY")
	instrStore.UpdateStatus("INSTR-A", types.TradingStatusHalted) //nolint:errcheck
	instrStore.UpdateStatus("INSTR-B", types.TradingStatusHalted) //nolint:errcheck

	// First halt the market.
	resp := doJSON(t, ts, http.MethodPut, "/api/v1/securities/markets/MSE/status",
		map[string]string{"status": types.MarketHalted})
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// Now restore the market to ACTIVE.
	resp = doJSON(t, ts, http.MethodPut, "/api/v1/securities/markets/MSE/status",
		map[string]string{"status": types.MarketActive})
	assertStatus(t, resp, http.StatusOK)

	var mkt map[string]interface{}
	decodeBody(t, resp, &mkt)
	if mkt["status"] != types.MarketActive {
		t.Errorf("market status: want %q, got %v", types.MarketActive, mkt["status"])
	}

	// Instruments should now be ACTIVE.
	for _, id := range []string{"INSTR-A", "INSTR-B"} {
		got, _ := instrStore.Get(id)
		if got.TradingStatus != types.TradingStatusActive {
			t.Errorf("instrument %s: want ACTIVE after market restore, got %q", id, got.TradingStatus)
		}
	}
}

// TestMarketHalt_NoInstruments verifies that halting a market with no
// instruments in its segments succeeds without error.
func TestMarketHalt_NoInstruments(t *testing.T) {
	ts, _, _, _, _ := newHaltTestServer(t)

	// MSE has "EQUITY" segment but no instruments — should return 200 cleanly.
	resp := doJSON(t, ts, http.MethodPut, "/api/v1/securities/markets/MSE/status",
		map[string]string{"status": types.MarketHalted})
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

// TestMarketStatus_NonHaltedStatus verifies that non-halt statuses (e.g.
// MARKET_SUSPENDED) do not trigger cascade (instruments stay ACTIVE).
func TestMarketStatus_NonHaltedStatus(t *testing.T) {
	ts, instrStore, _, _, _ := newHaltTestServer(t)

	createInstrumentWithSegment(t, instrStore, "INSTR-S", "EQUITY")

	resp := doJSON(t, ts, http.MethodPut, "/api/v1/securities/markets/MSE/status",
		map[string]string{"status": types.MarketSuspended})
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// Instrument should remain ACTIVE — MARKET_SUSPENDED does not cascade.
	got, _ := instrStore.Get("INSTR-S")
	if got.TradingStatus != types.TradingStatusActive {
		t.Errorf("instrument INSTR-S: want ACTIVE (no cascade for SUSPENDED), got %q", got.TradingStatus)
	}
}

// TestMarketHalt_AuditLog verifies that an audit entry is logged for each
// cascaded instrument halt.
func TestMarketHalt_AuditLog(t *testing.T) {
	ts, instrStore, _, _, auditStore := newHaltTestServer(t)

	createInstrumentWithSegment(t, instrStore, "AUDIT-INSTR-1", "EQUITY")
	createInstrumentWithSegment(t, instrStore, "AUDIT-INSTR-2", "EQUITY")

	resp := doJSON(t, ts, http.MethodPut, "/api/v1/securities/markets/MSE/status",
		map[string]string{"status": types.MarketHalted})
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	entries, err := auditStore.List(types.AuditFilters{EntityType: "INSTRUMENT"})
	if err != nil {
		t.Fatalf("list audit entries: %v", err)
	}
	if len(entries) < 2 {
		t.Errorf("expected at least 2 audit entries for cascaded halt, got %d", len(entries))
	}
	for _, e := range entries {
		if e.Action != "HALT" {
			t.Errorf("audit entry action: want HALT, got %q", e.Action)
		}
		if e.ActorID != "system" {
			t.Errorf("audit entry actor: want system, got %q", e.ActorID)
		}
	}
}

// TestMarketStatus_NotFound verifies that updating status for a non-existent market
// returns 404.
func TestMarketStatus_NotFound(t *testing.T) {
	ts, _, _, _, _ := newHaltTestServer(t)

	resp := doJSON(t, ts, http.MethodPut, "/api/v1/securities/markets/NONEXISTENT/status",
		map[string]string{"status": types.MarketHalted})
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// ============================================================
// TestSegmentHaltCascade — segment status → instrument cascade
// ============================================================

// TestSegmentHalt_CascadesInstrumentsToHalted verifies that setting a segment
// to SEG_HALTED cascades HALTED to all instruments in that segment.
func TestSegmentHalt_CascadesInstrumentsToHalted(t *testing.T) {
	ts, instrStore, segStore, _, _ := newHaltTestServer(t)

	// Create a second segment to confirm isolation.
	err := segStore.Create(&types.Segment{
		ID:       "BONDS",
		MarketID: "MSE",
		Name:     "Bonds",
		Status:   types.SegActive,
	})
	if err != nil {
		t.Fatalf("create BONDS segment: %v", err)
	}

	createInstrumentWithSegment(t, instrStore, "EQ-1", "EQUITY")
	createInstrumentWithSegment(t, instrStore, "EQ-2", "EQUITY")
	createInstrumentWithSegment(t, instrStore, "BD-1", "BONDS")

	// PUT /api/v1/securities/segments/EQUITY/status → SEG_HALTED
	resp := doJSON(t, ts, http.MethodPut, "/api/v1/securities/segments/EQUITY/status",
		map[string]string{"status": types.SegHalted})
	assertStatus(t, resp, http.StatusOK)

	var seg map[string]interface{}
	decodeBody(t, resp, &seg)
	if seg["status"] != types.SegHalted {
		t.Errorf("segment status: want %q, got %v", types.SegHalted, seg["status"])
	}

	// EQUITY instruments should be HALTED.
	for _, id := range []string{"EQ-1", "EQ-2"} {
		got, _ := instrStore.Get(id)
		if got.TradingStatus != types.TradingStatusHalted {
			t.Errorf("instrument %s: want HALTED, got %q", id, got.TradingStatus)
		}
	}

	// BONDS instrument should remain ACTIVE.
	bd1, _ := instrStore.Get("BD-1")
	if bd1.TradingStatus != types.TradingStatusActive {
		t.Errorf("instrument BD-1: want ACTIVE (different segment), got %q", bd1.TradingStatus)
	}
}

// TestSegmentActive_CascadesInstrumentsToActive verifies that setting a
// segment to SEG_ACTIVE restores its instruments to ACTIVE.
func TestSegmentActive_CascadesInstrumentsToActive(t *testing.T) {
	ts, instrStore, _, _, _ := newHaltTestServer(t)

	createInstrumentWithSegment(t, instrStore, "EQ-RES-1", "EQUITY")
	instrStore.UpdateStatus("EQ-RES-1", types.TradingStatusHalted) //nolint:errcheck

	// Halt the segment first.
	resp := doJSON(t, ts, http.MethodPut, "/api/v1/securities/segments/EQUITY/status",
		map[string]string{"status": types.SegHalted})
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// Restore segment to active.
	resp = doJSON(t, ts, http.MethodPut, "/api/v1/securities/segments/EQUITY/status",
		map[string]string{"status": types.SegActive})
	assertStatus(t, resp, http.StatusOK)

	var seg map[string]interface{}
	decodeBody(t, resp, &seg)
	if seg["status"] != types.SegActive {
		t.Errorf("segment status: want %q, got %v", types.SegActive, seg["status"])
	}

	got, _ := instrStore.Get("EQ-RES-1")
	if got.TradingStatus != types.TradingStatusActive {
		t.Errorf("instrument EQ-RES-1: want ACTIVE after segment restore, got %q", got.TradingStatus)
	}
}

// TestSegmentStatus_NotFound verifies that updating status for a non-existent
// segment returns 404.
func TestSegmentStatus_NotFound(t *testing.T) {
	ts, _, _, _, _ := newHaltTestServer(t)

	resp := doJSON(t, ts, http.MethodPut, "/api/v1/securities/segments/NONEXISTENT/status",
		map[string]string{"status": types.SegHalted})
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// TestSegmentStatus_MissingField verifies that a missing status field returns 400.
func TestSegmentStatus_MissingField(t *testing.T) {
	ts, _, _, _, _ := newHaltTestServer(t)

	resp := doJSON(t, ts, http.MethodPut, "/api/v1/securities/segments/EQUITY/status",
		map[string]string{})
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

// TestSegmentGet_OK verifies that GET /segments/{id} returns the segment.
func TestSegmentGet_OK(t *testing.T) {
	ts, _, _, _, _ := newHaltTestServer(t)

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/segments/EQUITY", nil)
	assertStatus(t, resp, http.StatusOK)

	var seg map[string]interface{}
	decodeBody(t, resp, &seg)
	if seg["id"] != "EQUITY" {
		t.Errorf("segment id: want EQUITY, got %v", seg["id"])
	}
}

// TestSegmentGet_NotFound verifies that GET /segments/{id} for missing id returns 404.
func TestSegmentGet_NotFound(t *testing.T) {
	ts, _, _, _, _ := newHaltTestServer(t)

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/segments/MISSING", nil)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// TestSegmentStatus_MethodNotAllowed verifies that non-PUT on /status returns 405.
func TestSegmentStatus_MethodNotAllowed(t *testing.T) {
	ts, _, _, _, _ := newHaltTestServer(t)

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/segments/EQUITY/status", nil)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	resp.Body.Close()
}
