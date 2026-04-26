// Package server — tests for trading cycle CRUD HTTP handlers.
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/middleware"
	"github.com/garudax-platform/securities-service/internal/store"
)

// newTradingCycleTestServer creates a test server wired with a real
// InMemoryTradingCycleStore (pre-seeded with STANDARD + OFF_BOOK cycles).
func newTradingCycleTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)
	cycleStore := store.NewInMemoryTradingCycleStore()

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
		cycleStore,
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

// ============================================================
// TestCreateTradingCycle
// ============================================================

func TestCreateTradingCycle(t *testing.T) {
	ts := newTradingCycleTestServer(t)

	t.Run("valid cycle returns 201", func(t *testing.T) {
		payload := map[string]interface{}{
			"id":               "cycle-new-001",
			"market_id":        "MSE",
			"name":             "CUSTOM",
			"session_sequence": []string{"PRE_OPEN", "CONTINUOUS", "CLOSED"},
			"is_default":       false,
		}
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/trading-cycles", payload)
		assertStatus(t, resp, http.StatusCreated)

		var body map[string]interface{}
		decodeBody(t, resp, &body)
		if body["id"] != "cycle-new-001" {
			t.Errorf("id: want cycle-new-001, got %v", body["id"])
		}
		if body["market_id"] != "MSE" {
			t.Errorf("market_id: want MSE, got %v", body["market_id"])
		}
		if body["name"] != "CUSTOM" {
			t.Errorf("name: want CUSTOM, got %v", body["name"])
		}
	})

	t.Run("missing id returns 400", func(t *testing.T) {
		payload := map[string]interface{}{
			"market_id":        "MSE",
			"name":             "NO_ID",
			"session_sequence": []string{"CLOSED"},
		}
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/trading-cycles", payload)
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})

	t.Run("missing market_id returns 400", func(t *testing.T) {
		payload := map[string]interface{}{
			"id":               "cycle-no-market",
			"name":             "NO_MARKET",
			"session_sequence": []string{"CLOSED"},
		}
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/trading-cycles", payload)
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})

	t.Run("missing name returns 400", func(t *testing.T) {
		payload := map[string]interface{}{
			"id":               "cycle-no-name",
			"market_id":        "MSE",
			"session_sequence": []string{"CLOSED"},
		}
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/trading-cycles", payload)
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})

	t.Run("empty session_sequence returns 400", func(t *testing.T) {
		payload := map[string]interface{}{
			"id":               "cycle-no-seq",
			"market_id":        "MSE",
			"name":             "NO_SEQ",
			"session_sequence": []string{},
		}
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/trading-cycles", payload)
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})

	t.Run("duplicate id returns 409", func(t *testing.T) {
		payload := map[string]interface{}{
			"id":               "cycle-mse-standard",
			"market_id":        "MSE",
			"name":             "DUP",
			"session_sequence": []string{"CLOSED"},
		}
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/trading-cycles", payload)
		assertStatus(t, resp, http.StatusConflict)
		resp.Body.Close()
	})
}

// ============================================================
// TestListTradingCycles
// ============================================================

func TestListTradingCycles(t *testing.T) {
	ts := newTradingCycleTestServer(t)

	t.Run("list all returns seeded cycles", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/trading-cycles", nil)
		assertStatus(t, resp, http.StatusOK)

		var body map[string]interface{}
		decodeBody(t, resp, &body)

		total, ok := body["total"].(float64)
		if !ok {
			t.Fatalf("total field missing or not a number: %v", body["total"])
		}
		if total < 2 {
			t.Errorf("expected at least 2 seeded cycles, got total=%.0f", total)
		}
	})

	t.Run("filter by market_id=MSE returns seeded cycles", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/trading-cycles?market_id=MSE", nil)
		assertStatus(t, resp, http.StatusOK)

		var body map[string]interface{}
		decodeBody(t, resp, &body)

		data, ok := body["data"].([]interface{})
		if !ok {
			t.Fatalf("data field missing or not an array")
		}
		if len(data) < 2 {
			t.Errorf("expected at least 2 MSE cycles, got %d", len(data))
		}
	})

	t.Run("filter by unknown market returns empty array", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/trading-cycles?market_id=UNKNOWN", nil)
		assertStatus(t, resp, http.StatusOK)

		var body map[string]interface{}
		decodeBody(t, resp, &body)

		data, ok := body["data"].([]interface{})
		if !ok {
			t.Fatalf("data field missing or not an array")
		}
		if len(data) != 0 {
			t.Errorf("expected 0 cycles for unknown market, got %d", len(data))
		}
	})
}

// ============================================================
// TestDeleteTradingCycle
// ============================================================

func TestDeleteTradingCycle(t *testing.T) {
	ts := newTradingCycleTestServer(t)

	// Create a cycle we can safely delete.
	createPayload := map[string]interface{}{
		"id":               "cycle-to-delete",
		"market_id":        "TST",
		"name":             "DELETE_TARGET",
		"session_sequence": []string{"PRE_OPEN", "CLOSED"},
	}
	createResp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/trading-cycles", createPayload)
	assertStatus(t, createResp, http.StatusCreated)
	createResp.Body.Close()

	t.Run("delete existing cycle returns 204", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodDelete, "/api/v1/securities/trading-cycles/cycle-to-delete", nil)
		assertStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()
	})

	t.Run("delete non-existent returns 404", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodDelete, "/api/v1/securities/trading-cycles/no-such-cycle", nil)
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})

	t.Run("delete seeded cycle removes it from list", func(t *testing.T) {
		// Create a fresh server to avoid shared state.
		ts2 := newTradingCycleTestServer(t)

		// Verify cycle-mse-off-book is present.
		listResp := doJSON(t, ts2, http.MethodGet, "/api/v1/securities/trading-cycles?market_id=MSE", nil)
		assertStatus(t, listResp, http.StatusOK)
		var before map[string]interface{}
		decodeBody(t, listResp, &before)
		beforeTotal := int(before["total"].(float64))

		// Delete it.
		delResp := doJSON(t, ts2, http.MethodDelete, "/api/v1/securities/trading-cycles/cycle-mse-off-book", nil)
		assertStatus(t, delResp, http.StatusNoContent)
		delResp.Body.Close()

		// Verify total decreased by 1.
		listResp2 := doJSON(t, ts2, http.MethodGet, "/api/v1/securities/trading-cycles?market_id=MSE", nil)
		assertStatus(t, listResp2, http.StatusOK)
		var after map[string]interface{}
		decodeBody(t, listResp2, &after)
		afterTotal := int(after["total"].(float64))
		if afterTotal != beforeTotal-1 {
			t.Errorf("expected total to decrease by 1: before=%d after=%d", beforeTotal, afterTotal)
		}
	})
}

// ============================================================
// TestTradingCycle_StoreNotConfigured
// ============================================================

func TestTradingCycle_StoreNotConfigured(t *testing.T) {
	// newTestServer sets tradingCycleStore=nil.
	ts := newTestServer(t)

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/trading-cycles", nil)
	assertStatus(t, resp, http.StatusServiceUnavailable)
	resp.Body.Close()
}
