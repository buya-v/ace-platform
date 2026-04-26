// Package server — tests for firm-view surveillance aggregation handler.
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

// newFirmViewTestServer creates a test server with firm, participant, order, and
// trade stores all wired so firm-view can aggregate real data.
func newFirmViewTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	firmStore := store.NewInMemoryFirmStore()
	participantStore := store.NewInMemoryParticipantStore()

	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)
	cfg := DefaultConfig()
	srv := New(
		instrStore, orderStore, tradeStore, positionStore,
		nil, // settlementStore
		store.NewInMemoryCorporateActionStore(),
		store.NewInMemoryEntitlementStore(),
		store.NewInMemoryMarketStore(),
		store.NewInMemorySegmentStore(),
		store.NewInMemoryCircuitBreakerStore(),
		firmStore,
		participantStore,
		nil, // tickTableStore
		nil, // tradeCorrectionStore
		nil, // throttleStore
		nil, // throttleConfigStore
		nil, // announcementStore
		nil, // auditStore
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
		nil, // tradingCycleStore
		nil, // dayManager
		me,
		nil, // sessionManager
		nil, // settlementEngine
		nil, // producer
		nil, // privilegeEngine
		nil, // roleStore
		nil, // tradingParamSetStore
		cfg,
	)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	ts := httptest.NewServer(tenantMW(mux))
	t.Cleanup(ts.Close)
	return ts
}

// seedFirmWithData seeds a firm, a participant, and some orders into the server,
// then returns (firmID, participantID, instrumentID).
func seedFirmWithData(t *testing.T, ts *httptest.Server) (firmID, participantID, instrumentID string) {
	t.Helper()
	firmID = "firm-fv-001"
	participantID = "part-fv-001"

	// Create firm.
	firmResp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/firms",
		map[string]interface{}{
			"id":   firmID,
			"name": "FirmView Test Firm",
		})
	assertStatus(t, firmResp, http.StatusCreated)
	firmResp.Body.Close()

	// Create participant belonging to firm.
	pResp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/participants",
		map[string]interface{}{
			"id":          participantID,
			"firm_id":     firmID,
			"name":        "FV Trader",
			"permissions": []string{"ORDER_CREATE"},
		})
	assertStatus(t, pResp, http.StatusCreated)
	pResp.Body.Close()

	// Create an instrument.
	iResp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/instruments",
		map[string]interface{}{
			"ticker":      "FVTST",
			"name":        "FirmView Test Instrument",
			"asset_class": "EQUITY",
			"lot_size":    1,
			"tick_size":   0.01,
		})
	assertStatus(t, iResp, http.StatusCreated)
	var iBody map[string]interface{}
	decodeBody(t, iResp, &iBody)
	instrumentID = iBody["id"].(string)

	// Submit orders for the participant.
	for i := 0; i < 3; i++ {
		oResp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/orders",
			map[string]interface{}{
				"instrument_id":  instrumentID,
				"participant_id": participantID,
				"side":           "BUY",
				"order_type":     "LIMIT",
				"quantity":       100,
				"price":          50.00,
			})
		assertStatus(t, oResp, http.StatusCreated)
		oResp.Body.Close()
	}

	return firmID, participantID, instrumentID
}

// ============================================================
// TestFirmView_WithData
// ============================================================

func TestFirmView_WithData(t *testing.T) {
	ts := newFirmViewTestServer(t)
	firmID, _, _ := seedFirmWithData(t, ts)

	resp := doJSON(t, ts, http.MethodGet,
		"/api/v1/securities/surveillance/firm-view/"+firmID, nil)
	assertStatus(t, resp, http.StatusOK)

	var body map[string]interface{}
	decodeBody(t, resp, &body)

	// firm_id echoed back.
	if body["firm_id"] != firmID {
		t.Errorf("firm_id: want %q, got %v", firmID, body["firm_id"])
	}

	// orders summary — 3 PENDING orders seeded.
	orders, ok := body["orders"].(map[string]interface{})
	if !ok {
		t.Fatalf("orders field missing or not an object: %T", body["orders"])
	}
	if orders["total"].(float64) < 3 {
		t.Errorf("orders.total: want >= 3, got %v", orders["total"])
	}
	if orders["pending"].(float64) < 3 {
		t.Errorf("orders.pending: want >= 3, got %v", orders["pending"])
	}

	// trades summary — present even with 0 trades.
	_, ok = body["trades"].(map[string]interface{})
	if !ok {
		t.Fatalf("trades field missing or not an object: %T", body["trades"])
	}

	// positions — present (may be empty slice).
	_, ok = body["positions"].([]interface{})
	if !ok {
		t.Fatalf("positions field missing or not an array: %T", body["positions"])
	}

	// recent_orders — up to last 10 orders.
	recentOrders, ok := body["recent_orders"].([]interface{})
	if !ok {
		t.Fatalf("recent_orders field missing or not an array: %T", body["recent_orders"])
	}
	if len(recentOrders) == 0 {
		t.Error("expected at least one recent order")
	}

	// recent_trades — present (empty when no trades).
	_, ok = body["recent_trades"].([]interface{})
	if !ok {
		t.Fatalf("recent_trades field missing or not an array: %T", body["recent_trades"])
	}
}

// ============================================================
// TestFirmView_EmptyFirm
// ============================================================

func TestFirmView_EmptyFirm(t *testing.T) {
	ts := newFirmViewTestServer(t)

	// Create a firm with no participants or orders.
	firmID := "firm-empty"
	firmResp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/firms",
		map[string]interface{}{
			"id":   firmID,
			"name": "Empty Firm",
		})
	assertStatus(t, firmResp, http.StatusCreated)
	firmResp.Body.Close()

	resp := doJSON(t, ts, http.MethodGet,
		"/api/v1/securities/surveillance/firm-view/"+firmID, nil)
	assertStatus(t, resp, http.StatusOK)

	var body map[string]interface{}
	decodeBody(t, resp, &body)

	if body["firm_id"] != firmID {
		t.Errorf("firm_id: want %q, got %v", firmID, body["firm_id"])
	}

	orders := body["orders"].(map[string]interface{})
	if orders["total"].(float64) != 0 {
		t.Errorf("orders.total: want 0 for empty firm, got %v", orders["total"])
	}
	if orders["pending"].(float64) != 0 {
		t.Errorf("orders.pending: want 0, got %v", orders["pending"])
	}
	if orders["filled"].(float64) != 0 {
		t.Errorf("orders.filled: want 0, got %v", orders["filled"])
	}
	if orders["cancelled"].(float64) != 0 {
		t.Errorf("orders.cancelled: want 0, got %v", orders["cancelled"])
	}

	trades := body["trades"].(map[string]interface{})
	if trades["total"].(float64) != 0 {
		t.Errorf("trades.total: want 0, got %v", trades["total"])
	}

	positions := body["positions"].([]interface{})
	if len(positions) != 0 {
		t.Errorf("positions: want empty for empty firm, got %d", len(positions))
	}

	recentOrders := body["recent_orders"].([]interface{})
	if len(recentOrders) != 0 {
		t.Errorf("recent_orders: want empty for empty firm, got %d", len(recentOrders))
	}

	recentTrades := body["recent_trades"].([]interface{})
	if len(recentTrades) != 0 {
		t.Errorf("recent_trades: want empty for empty firm, got %d", len(recentTrades))
	}
}

// ============================================================
// TestFirmView_MissingFirmID
// ============================================================

func TestFirmView_MissingFirmID(t *testing.T) {
	ts := newFirmViewTestServer(t)

	resp := doJSON(t, ts, http.MethodGet,
		"/api/v1/securities/surveillance/firm-view/", nil)
	// Either 400 (missing firm_id) or 404 (routing) — both are acceptable.
	if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 400 or 404 for missing firm_id, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// ============================================================
// TestFirmView_NonExistentFirm (aggregates zero data)
// ============================================================

func TestFirmView_NonExistentFirm(t *testing.T) {
	ts := newFirmViewTestServer(t)

	// Firm doesn't exist but the handler aggregates data for it (returns zero counts).
	resp := doJSON(t, ts, http.MethodGet,
		"/api/v1/securities/surveillance/firm-view/firm-does-not-exist", nil)
	assertStatus(t, resp, http.StatusOK)

	var body map[string]interface{}
	decodeBody(t, resp, &body)

	orders := body["orders"].(map[string]interface{})
	if orders["total"].(float64) != 0 {
		t.Errorf("orders.total: want 0 for unknown firm, got %v", orders["total"])
	}
}

// ============================================================
// TestFirmView_MethodNotAllowed
// ============================================================

func TestFirmView_MethodNotAllowed(t *testing.T) {
	ts := newFirmViewTestServer(t)

	resp := doJSON(t, ts, http.MethodPost,
		"/api/v1/securities/surveillance/firm-view/firm-fv-001", nil)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	resp.Body.Close()
}

// ============================================================
// TestFirmView_RecentOrdersLimit
// ============================================================

func TestFirmView_RecentOrdersLimit(t *testing.T) {
	ts := newFirmViewTestServer(t)

	// Seed firm with 12 orders — recent_orders should be capped at 10.
	firmID := "firm-limit-test"
	doJSON(t, ts, http.MethodPost, "/api/v1/securities/firms",
		map[string]interface{}{"id": firmID, "name": "Limit Test Firm"}).Body.Close()

	doJSON(t, ts, http.MethodPost, "/api/v1/securities/participants",
		map[string]interface{}{
			"id":          "part-limit-001",
			"firm_id":     firmID,
			"name":        "Limit Trader",
			"permissions": []string{},
		}).Body.Close()

	iResp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/instruments",
		map[string]interface{}{
			"ticker":      "LMTTST",
			"name":        "Limit Test Instr",
			"asset_class": "EQUITY",
			"lot_size":    1,
			"tick_size":   0.01,
		})
	var iBody map[string]interface{}
	decodeBody(t, iResp, &iBody)
	instrID := iBody["id"].(string)

	// Submit 12 orders.
	for i := 0; i < 12; i++ {
		r := doJSON(t, ts, http.MethodPost, "/api/v1/securities/orders",
			map[string]interface{}{
				"instrument_id":  instrID,
				"participant_id": "part-limit-001",
				"side":           "BUY",
				"order_type":     "LIMIT",
				"quantity":       10,
				"price":          float64(50 + i),
			})
		assertStatus(t, r, http.StatusCreated)
		r.Body.Close()
	}

	resp := doJSON(t, ts, http.MethodGet,
		"/api/v1/securities/surveillance/firm-view/"+firmID, nil)
	assertStatus(t, resp, http.StatusOK)

	var body map[string]interface{}
	decodeBody(t, resp, &body)

	orders := body["orders"].(map[string]interface{})
	if int(orders["total"].(float64)) != 12 {
		t.Errorf("orders.total: want 12, got %v", orders["total"])
	}

	recentOrders := body["recent_orders"].([]interface{})
	if len(recentOrders) > 10 {
		t.Errorf("recent_orders capped at 10, got %d", len(recentOrders))
	}
	if len(recentOrders) == 0 {
		t.Error("recent_orders should not be empty")
	}

	// Verify recent_orders contains types.SecurityOrder-compatible fields.
	first := recentOrders[0].(map[string]interface{})
	if first["id"] == nil || first["id"] == "" {
		t.Error("first recent_order missing id field")
	}
}

// ============================================================
// Compile-time guard: ensure types package is used
// ============================================================

var _ = types.ParticipantActive // suppress unused import if participant fields aren't checked
