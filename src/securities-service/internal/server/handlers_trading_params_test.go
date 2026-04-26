// Package server — internal tests for trading parameter set HTTP handlers.
package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/middleware"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// ============================================================
// Trading-params test infrastructure
// ============================================================

// newTradingParamsServer builds a test server with a live TradingParamSetStore
// wired in. Instrument and order stores are also fresh so tests can seed
// instruments for use in order-validation tests.
func newTradingParamsServer(t *testing.T) (*httptest.Server, store.InstrumentStore, store.TradingParamSetStore) {
	t.Helper()
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	paramStore := store.NewInMemoryTradingParamSetStore()

	cfg := DefaultConfig()
	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)
	srv := New(
		instrStore, orderStore, tradeStore, positionStore,
		nil,                                        // settlementStore
		store.NewInMemoryCorporateActionStore(),
		store.NewInMemoryEntitlementStore(),
		store.NewInMemoryMarketStore(),
		store.NewInMemorySegmentStore(),
		store.NewInMemoryCircuitBreakerStore(),
		store.NewInMemoryFirmStore(),
		store.NewInMemoryParticipantStore(),
		nil, nil, nil, nil, // tickTableStore, tradeCorrectionStore, throttleStore, throttleConfigStore
		nil, nil, nil, nil, // announcementStore, auditStore, pendingChangeStore, referencePriceStore
		nil, nil, nil,      // surveillanceStore, instrumentGroupStore, offBookTradeStore
		nil,                // nodeStore
		nil,                // locateStore
		nil,                // rfqStore
		nil,                // giveUpStore
		nil, nil, nil,      // investigationStore, replayStore, bondStore
		nil, nil, nil, nil, // strategyStore, custodyAccountStore, custodyBalanceStore, csdTransferStore
		nil, nil, nil,      // watchListStore, ipRestrictionStore, passwordPolicyStore
		nil, me, nil, nil,  // dayManager, matchingEngine, sessionManager, settlementEngine
		nil, nil, nil,      // producer, privilegeEngine, roleStore
		paramStore,
		cfg,
	)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	handler := tenantMW(mux)

	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts, instrStore, paramStore
}

// validParamSetPayload returns a minimal valid request body for creating a
// TradingParameterSet.
func validParamSetPayload(instrumentID string) map[string]interface{} {
	return map[string]interface{}{
		"instrument_id":        instrumentID,
		"name":                 "Standard",
		"allowed_order_types":  []string{"LIMIT", "MARKET"},
		"allowed_time_in_force": []string{"GTC", "DAY"},
		"min_order_size":       10,
		"max_order_size":       5000,
		"max_order_value":      250000.0,
		"short_selling_allowed": false,
	}
}

// createParamSet creates a TradingParameterSet via POST and returns its ID.
func createParamSet(t *testing.T, ts *httptest.Server, payload map[string]interface{}) string {
	t.Helper()
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/trading-params", payload)
	assertStatus(t, resp, http.StatusCreated)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	id, ok := result["id"].(string)
	if !ok || id == "" {
		t.Fatalf("expected non-empty id in create response")
	}
	return id
}

// ============================================================
// TestCreateTradingParamSet
// ============================================================

func TestCreateTradingParamSet(t *testing.T) {
	ts, instrStore, _ := newTradingParamsServer(t)

	// Seed instrument.
	inst := &types.Instrument{
		ID: "inst-tp-create", Ticker: "TPC", Name: "TP Create Corp",
		AssetClass: types.AssetClassEquity, TradingStatus: types.TradingStatusActive,
		LotSize: 100, TickSize: 0.01, CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z",
	}
	if err := instrStore.Create(inst); err != nil {
		t.Fatalf("seed instrument: %v", err)
	}

	payload := validParamSetPayload("inst-tp-create")
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/trading-params", payload)
	assertStatus(t, resp, http.StatusCreated)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	if id, ok := result["id"].(string); !ok || id == "" {
		t.Error("expected non-empty id in response")
	}
	if result["instrument_id"] != "inst-tp-create" {
		t.Errorf("expected instrument_id inst-tp-create, got %v", result["instrument_id"])
	}
	if result["name"] != "Standard" {
		t.Errorf("expected name Standard, got %v", result["name"])
	}
	if ca, ok := result["created_at"].(string); !ok || ca == "" {
		t.Error("expected non-empty created_at")
	}

	t.Run("missing instrument_id returns 422", func(t *testing.T) {
		p := validParamSetPayload("inst-tp-create")
		delete(p, "instrument_id")
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/trading-params", p)
		assertStatus(t, resp, http.StatusUnprocessableEntity)
		var errResp types.ErrorResponse
		decodeBody(t, resp, &errResp)
		if errResp.Error.Code != "MISSING_FIELD" {
			t.Errorf("expected MISSING_FIELD, got %q", errResp.Error.Code)
		}
	})
}

// ============================================================
// TestGetByInstrument
// ============================================================

func TestGetByInstrument(t *testing.T) {
	ts, instrStore, _ := newTradingParamsServer(t)

	inst := &types.Instrument{
		ID: "inst-tp-gbi", Ticker: "GBI", Name: "GBI Corp",
		AssetClass: types.AssetClassEquity, TradingStatus: types.TradingStatusActive,
		LotSize: 10, TickSize: 0.05, CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z",
	}
	if err := instrStore.Create(inst); err != nil {
		t.Fatalf("seed instrument: %v", err)
	}

	// Create a param set linked to the instrument.
	createParamSet(t, ts, validParamSetPayload("inst-tp-gbi"))

	t.Run("found", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/trading-params/instrument/inst-tp-gbi", nil)
		assertStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		if result["instrument_id"] != "inst-tp-gbi" {
			t.Errorf("expected instrument_id inst-tp-gbi, got %v", result["instrument_id"])
		}
	})

	t.Run("not found", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/trading-params/instrument/NO-SUCH-INST", nil)
		assertStatus(t, resp, http.StatusNotFound)
		var errResp types.ErrorResponse
		decodeBody(t, resp, &errResp)
		if errResp.Error.Code != "NOT_FOUND" {
			t.Errorf("expected NOT_FOUND, got %q", errResp.Error.Code)
		}
	})
}

// ============================================================
// TestUpdateTradingParamSet
// ============================================================

func TestUpdateTradingParamSet(t *testing.T) {
	ts, instrStore, _ := newTradingParamsServer(t)

	inst := &types.Instrument{
		ID: "inst-tp-upd", Ticker: "TPU", Name: "TPU Corp",
		AssetClass: types.AssetClassEquity, TradingStatus: types.TradingStatusActive,
		LotSize: 100, TickSize: 0.01, CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z",
	}
	if err := instrStore.Create(inst); err != nil {
		t.Fatalf("seed instrument: %v", err)
	}

	id := createParamSet(t, ts, validParamSetPayload("inst-tp-upd"))

	updatePayload := map[string]interface{}{
		"instrument_id":       "inst-tp-upd",
		"name":                "Updated Params",
		"allowed_order_types": []string{"LIMIT"},
		"min_order_size":      50,
		"max_order_size":      1000,
		"max_order_value":     100000.0,
	}
	resp := doJSON(t, ts, http.MethodPut, fmt.Sprintf("/api/v1/securities/trading-params/%s", id), updatePayload)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["name"] != "Updated Params" {
		t.Errorf("expected name 'Updated Params', got %v", result["name"])
	}
	if result["id"] != id {
		t.Errorf("expected id %q, got %v", id, result["id"])
	}

	t.Run("not found", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodPut, "/api/v1/securities/trading-params/no-such-id", updatePayload)
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})
}

// ============================================================
// TestListTradingParamSets
// ============================================================

func TestListTradingParamSets(t *testing.T) {
	ts, instrStore, _ := newTradingParamsServer(t)

	t.Run("empty", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/trading-params", nil)
		assertStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		data, ok := result["data"].([]interface{})
		if !ok {
			t.Fatalf("expected data array, got %T", result["data"])
		}
		if len(data) != 0 {
			t.Errorf("expected 0 items, got %d", len(data))
		}
		if result["total"] != float64(0) {
			t.Errorf("expected total=0, got %v", result["total"])
		}
	})

	// Seed two instruments and param sets.
	for i, ticker := range []string{"LS1", "LS2"} {
		instID := fmt.Sprintf("inst-tp-list-%d", i)
		inst := &types.Instrument{
			ID: instID, Ticker: ticker, Name: ticker + " Corp",
			AssetClass: types.AssetClassEquity, TradingStatus: types.TradingStatusActive,
			LotSize: 100, TickSize: 0.01, CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z",
		}
		if err := instrStore.Create(inst); err != nil {
			t.Fatalf("seed instrument %s: %v", ticker, err)
		}
		createParamSet(t, ts, validParamSetPayload(instID))
	}

	t.Run("two items", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/trading-params", nil)
		assertStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		data := result["data"].([]interface{})
		if len(data) != 2 {
			t.Errorf("expected 2 items, got %d", len(data))
		}
		if result["total"] != float64(2) {
			t.Errorf("expected total=2, got %v", result["total"])
		}
	})
}

// ============================================================
// TestDeleteTradingParamSet
// ============================================================

func TestDeleteTradingParamSet(t *testing.T) {
	ts, instrStore, _ := newTradingParamsServer(t)

	inst := &types.Instrument{
		ID: "inst-tp-del", Ticker: "TPD", Name: "TPD Corp",
		AssetClass: types.AssetClassEquity, TradingStatus: types.TradingStatusActive,
		LotSize: 100, TickSize: 0.01, CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z",
	}
	if err := instrStore.Create(inst); err != nil {
		t.Fatalf("seed instrument: %v", err)
	}

	id := createParamSet(t, ts, validParamSetPayload("inst-tp-del"))

	// Delete returns 204 No Content.
	resp := doJSON(t, ts, http.MethodDelete, fmt.Sprintf("/api/v1/securities/trading-params/%s", id), nil)
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()

	// Subsequent GET returns 404.
	resp2 := doJSON(t, ts, http.MethodGet, fmt.Sprintf("/api/v1/securities/trading-params/%s", id), nil)
	assertStatus(t, resp2, http.StatusNotFound)
	resp2.Body.Close()

	t.Run("delete non-existent returns 404", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodDelete, "/api/v1/securities/trading-params/no-such-id", nil)
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})

	t.Run("store not configured returns 503", func(t *testing.T) {
		// Build a server with tradingParamSetStore == nil.
		ts2 := newTestServer(t) // newTestServer wires nil for tradingParamSetStore
		resp := doJSON(t, ts2, http.MethodPost, "/api/v1/securities/trading-params", validParamSetPayload("any"))
		assertStatus(t, resp, http.StatusServiceUnavailable)
		resp.Body.Close()
	})
}
