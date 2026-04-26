// Package server — tests for trading strategy HTTP handlers.
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

// newStrategyTestServer creates a test server wired with a real StrategyStore.
func newStrategyTestServer(t *testing.T) (*httptest.Server, *store.InMemoryStrategyStore) {
	t.Helper()
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)
	stratStore := store.NewInMemoryStrategyStore()

	cfg := DefaultConfig()
	srv := New(
		instrStore, orderStore, tradeStore, positionStore,
		nil, // settlementStore
		store.NewInMemoryCorporateActionStore(),
		store.NewInMemoryEntitlementStore(),
		store.NewInMemoryMarketStore(),
		store.NewInMemorySegmentStore(),
		store.NewInMemoryCircuitBreakerStore(),
		store.NewInMemoryFirmStore(),
		store.NewInMemoryParticipantStore(),
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
		nil, // locateStore
		nil, // rfqStore
		nil, // giveUpStore
		nil, // investigationStore
		nil, // replayStore
		nil, // bondStore
		stratStore,
		nil, // custodyAccountStore
		nil, // custodyBalanceStore
		nil, // csdTransferStore
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
	ts := httptest.NewServer(tenantMW(mux))
	t.Cleanup(ts.Close)
	return ts, stratStore
}

// validStrategyPayload returns a minimal valid strategy creation payload with 2 legs.
func validStrategyPayload(id string) map[string]interface{} {
	return map[string]interface{}{
		"id":            id,
		"name":          "Wheat Spread",
		"strategy_type": "SPREAD",
		"tenant_id":     testTenant,
		"legs": []map[string]interface{}{
			{"instrument_id": "WHEAT-JAN", "side": "BUY", "ratio_qty": 1},
			{"instrument_id": "WHEAT-MAR", "side": "SELL", "ratio_qty": 1},
		},
	}
}

// seedStrategy inserts a strategy directly into the store.
func seedStrategy(t *testing.T, s *store.InMemoryStrategyStore, strat *types.TradingStrategy) {
	t.Helper()
	if err := s.Create(strat); err != nil {
		t.Fatalf("seedStrategy %s: %v", strat.ID, err)
	}
}

// ============================================================
// TestCreateStrategy_Success
// ============================================================

func TestCreateStrategy_Success(t *testing.T) {
	ts, _ := newStrategyTestServer(t)

	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/strategies", validStrategyPayload("STRAT-HTTP-1"))
	assertStatus(t, resp, http.StatusCreated)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	if result["id"] != "STRAT-HTTP-1" {
		t.Errorf("id: want STRAT-HTTP-1, got %v", result["id"])
	}
	if result["name"] != "Wheat Spread" {
		t.Errorf("name: want 'Wheat Spread', got %v", result["name"])
	}
	if result["strategy_type"] != "SPREAD" {
		t.Errorf("strategy_type: want SPREAD, got %v", result["strategy_type"])
	}
	if result["status"] != string(types.StrategyStatusActive) {
		t.Errorf("status: want %s, got %v", types.StrategyStatusActive, result["status"])
	}
	legs, _ := result["legs"].([]interface{})
	if len(legs) != 2 {
		t.Errorf("legs count: want 2, got %d", len(legs))
	}
	if result["created_at"] == nil || result["created_at"] == "" {
		t.Error("created_at must be set on create")
	}
}

func TestCreateStrategy_AutoID(t *testing.T) {
	ts, _ := newStrategyTestServer(t)

	// No id provided — should auto-generate.
	payload := validStrategyPayload("")
	delete(payload, "id")
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/strategies", payload)
	assertStatus(t, resp, http.StatusCreated)

	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["id"] == nil || result["id"] == "" {
		t.Error("auto-generated id must not be empty")
	}
}

func TestCreateStrategy_DefaultType(t *testing.T) {
	ts, _ := newStrategyTestServer(t)

	payload := validStrategyPayload("STRAT-NOTP")
	delete(payload, "strategy_type")
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/strategies", payload)
	assertStatus(t, resp, http.StatusCreated)

	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["strategy_type"] != string(types.StrategyTypeCustom) {
		t.Errorf("default strategy_type: want CUSTOM, got %v", result["strategy_type"])
	}
}

// ============================================================
// TestCreateStrategy_TooFewLegs — 400 when < 2 legs
// ============================================================

func TestCreateStrategy_TooFewLegs(t *testing.T) {
	ts, _ := newStrategyTestServer(t)

	t.Run("zero legs", func(t *testing.T) {
		payload := map[string]interface{}{
			"id":            "STRAT-0LEG",
			"name":          "Zero Leg Strategy",
			"strategy_type": "SPREAD",
			"legs":          []interface{}{},
		}
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/strategies", payload)
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})

	t.Run("one leg", func(t *testing.T) {
		payload := map[string]interface{}{
			"id":   "STRAT-1LEG",
			"name": "One Leg Strategy",
			"legs": []map[string]interface{}{
				{"instrument_id": "INST-A", "side": "BUY", "ratio_qty": 1},
			},
		}
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/strategies", payload)
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})

	t.Run("missing legs field", func(t *testing.T) {
		payload := map[string]interface{}{
			"id":   "STRAT-NOLEGS",
			"name": "No Legs Field",
		}
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/strategies", payload)
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})
}

func TestCreateStrategy_MissingName(t *testing.T) {
	ts, _ := newStrategyTestServer(t)

	payload := validStrategyPayload("STRAT-NONAME")
	delete(payload, "name")
	payload["name"] = ""
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/strategies", payload)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestCreateStrategy_InvalidLeg_MissingInstrument(t *testing.T) {
	ts, _ := newStrategyTestServer(t)

	payload := map[string]interface{}{
		"id":   "STRAT-BADLEG",
		"name": "Bad Leg Strategy",
		"legs": []map[string]interface{}{
			{"instrument_id": "", "side": "BUY", "ratio_qty": 1},
			{"instrument_id": "INST-B", "side": "SELL", "ratio_qty": 1},
		},
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/strategies", payload)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestCreateStrategy_Duplicate(t *testing.T) {
	ts, _ := newStrategyTestServer(t)

	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/strategies", validStrategyPayload("STRAT-DUP"))
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Second create with same ID should return 409.
	resp = doJSON(t, ts, http.MethodPost, "/api/v1/securities/strategies", validStrategyPayload("STRAT-DUP"))
	assertStatus(t, resp, http.StatusConflict)
	resp.Body.Close()
}

// ============================================================
// TestListStrategies
// ============================================================

func TestListStrategies(t *testing.T) {
	ts, _ := newStrategyTestServer(t)

	// Initially empty.
	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/strategies", nil)
	assertStatus(t, resp, http.StatusOK)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["total"] != float64(0) {
		t.Errorf("initial total: want 0, got %v", result["total"])
	}

	// Create two strategies.
	doJSON(t, ts, http.MethodPost, "/api/v1/securities/strategies", validStrategyPayload("STRAT-L1")).Body.Close()
	p2 := validStrategyPayload("STRAT-L2")
	p2["name"] = "Cotton Spread"
	p2["strategy_type"] = "STRADDLE"
	doJSON(t, ts, http.MethodPost, "/api/v1/securities/strategies", p2).Body.Close()

	resp = doJSON(t, ts, http.MethodGet, "/api/v1/securities/strategies", nil)
	assertStatus(t, resp, http.StatusOK)
	decodeBody(t, resp, &result)

	if result["total"] != float64(2) {
		t.Errorf("total after 2 creates: want 2, got %v", result["total"])
	}
	data, _ := result["data"].([]interface{})
	if len(data) != 2 {
		t.Errorf("data length: want 2, got %d", len(data))
	}
}

func TestListStrategies_FilterByTenant(t *testing.T) {
	ts, stratStore := newStrategyTestServer(t)

	// Seed two strategies for different tenants directly in the store.
	seedStrategy(t, stratStore, &types.TradingStrategy{
		ID: "T1", Name: "S1", StrategyType: types.StrategyTypeSpread,
		Status: types.StrategyStatusActive, TenantID: testTenant,
		Legs: []types.StrategyLeg{{InstrumentID: "A", Side: types.OrderSideBuy, RatioQty: 1}, {InstrumentID: "B", Side: types.OrderSideSell, RatioQty: 1}},
	})
	seedStrategy(t, stratStore, &types.TradingStrategy{
		ID: "T2", Name: "S2", StrategyType: types.StrategyTypeSpread,
		Status: types.StrategyStatusActive, TenantID: "other-tenant",
		Legs: []types.StrategyLeg{{InstrumentID: "C", Side: types.OrderSideBuy, RatioQty: 1}, {InstrumentID: "D", Side: types.OrderSideSell, RatioQty: 1}},
	})

	// Filter by testTenant — should return 1.
	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/strategies?tenant_id="+testTenant, nil)
	assertStatus(t, resp, http.StatusOK)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["total"] != float64(1) {
		t.Errorf("filtered total: want 1, got %v", result["total"])
	}
}

// ============================================================
// TestDeleteStrategy — 204 on success, 404 on missing
// ============================================================

func TestDeleteStrategy(t *testing.T) {
	ts, _ := newStrategyTestServer(t)

	// Create a strategy.
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/strategies", validStrategyPayload("STRAT-DEL"))
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Delete it — expect 204.
	resp = doJSON(t, ts, http.MethodDelete, "/api/v1/securities/strategies/STRAT-DEL", nil)
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		t.Errorf("Delete: expected 204 or 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Second delete returns 404.
	resp = doJSON(t, ts, http.MethodDelete, "/api/v1/securities/strategies/STRAT-DEL", nil)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestDeleteStrategy_NotFound(t *testing.T) {
	ts, _ := newStrategyTestServer(t)

	resp := doJSON(t, ts, http.MethodDelete, "/api/v1/securities/strategies/NO-STRAT", nil)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// ============================================================
// TestStrategyEndpoints_NotConfigured (503)
// ============================================================

func TestStrategyEndpoints_NotConfigured(t *testing.T) {
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)
	cfg := DefaultConfig()
	srv := New(
		instrStore, orderStore, tradeStore, positionStore,
		nil, store.NewInMemoryCorporateActionStore(), store.NewInMemoryEntitlementStore(),
		store.NewInMemoryMarketStore(), store.NewInMemorySegmentStore(),
		store.NewInMemoryCircuitBreakerStore(), store.NewInMemoryFirmStore(),
		store.NewInMemoryParticipantStore(),
		nil, nil, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil,
		nil, nil,
		nil,  // bondStore
		nil,  // strategyStore = nil
		nil,  // custodyAccountStore
		nil,  // custodyBalanceStore
		nil,  // csdTransferStore
		nil, me, nil, nil, nil, cfg,
	)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)
	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	httpTS := httptest.NewServer(tenantMW(mux))
	t.Cleanup(httpTS.Close)

	paths := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/securities/strategies"},
		{http.MethodPost, "/api/v1/securities/strategies"},
		{http.MethodGet, "/api/v1/securities/strategies/some-id"},
		{http.MethodDelete, "/api/v1/securities/strategies/some-id"},
	}

	for _, tc := range paths {
		resp := doJSON(t, httpTS, tc.method, tc.path, nil)
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("%s %s: expected 503, got %d", tc.method, tc.path, resp.StatusCode)
		}
		resp.Body.Close()
	}
}
