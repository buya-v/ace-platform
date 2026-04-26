// Package server — internal tests for throttle-config HTTP handlers.
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

// ── Throttle-config test helpers ──────────────────────────────────────────────

// newThrottleTestServer creates an httptest.Server fully wired with
// throttleConfigStore and throttleStore so that throttle-config endpoints and
// per-firm rate-limiting are exercised.
func newThrottleTestServer(t *testing.T) (*httptest.Server, *store.InMemoryInstrumentStore, *store.InMemoryThrottleConfigStore, *store.InMemoryParticipantStore) {
	t.Helper()
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	throttleCfgStore := store.NewInMemoryThrottleConfigStore()
	throttleStore := store.NewInMemoryThrottleStore()
	participantStore := store.NewInMemoryParticipantStore()

	cfg := DefaultConfig()
	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)

	srv := New(
		instrStore,
		orderStore,
		tradeStore,
		positionStore,
		nil,                                      // settlementStore
		store.NewInMemoryCorporateActionStore(),
		store.NewInMemoryEntitlementStore(),
		store.NewInMemoryMarketStore(),
		store.NewInMemorySegmentStore(),
		store.NewInMemoryCircuitBreakerStore(),
		store.NewInMemoryFirmStore(),
		participantStore,
		nil,          // tickTableStore
		nil,          // tradeCorrectionStore
		throttleStore,
		throttleCfgStore,
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
		nil, nil, nil, // investigationStore, replayStore, bondStore
		nil, nil, nil, nil, // strategyStore, custodyAccountStore, custodyBalanceStore, csdTransferStore
		nil, nil, nil, // watchListStore, ipRestrictionStore, passwordPolicyStore
		nil, nil, me, nil, nil,
		nil, nil, // privilegeEngine, roleStore
		nil, nil, // roleStore, tradingParamSetStore
		cfg,
	)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)
	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	ts := httptest.NewServer(tenantMW(mux))
	t.Cleanup(ts.Close)
	return ts, instrStore, throttleCfgStore, participantStore
}

// ── ThrottleConfig CRUD tests ─────────────────────────────────────────────────

// TestThrottleConfig_Set verifies that PUT /api/v1/securities/throttle-config/{firm_id}
// creates or updates a per-firm throttle config and returns HTTP 200.
func TestThrottleConfig_Set(t *testing.T) {
	ts, _, _, _ := newThrottleTestServer(t)

	payload := map[string]interface{}{
		"max_orders_per_second": 50,
		"enabled":               true,
	}
	resp := doJSON(t, ts, http.MethodPut, "/api/v1/securities/throttle-config/FIRM-A", payload)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	if result["firm_id"] != "FIRM-A" {
		t.Errorf("firm_id: want FIRM-A, got %v", result["firm_id"])
	}
	if result["max_orders_per_second"] != float64(50) {
		t.Errorf("max_orders_per_second: want 50, got %v", result["max_orders_per_second"])
	}
	if result["enabled"] != true {
		t.Errorf("enabled: want true, got %v", result["enabled"])
	}
}

// TestThrottleConfig_Get verifies that GET /api/v1/securities/throttle-config/{firm_id}
// returns the stored config with HTTP 200.
func TestThrottleConfig_Get(t *testing.T) {
	ts, _, cfgStore, _ := newThrottleTestServer(t)

	// Pre-seed via store.
	if err := cfgStore.Set(&types.ThrottleConfig{
		FirmID:             "FIRM-B",
		MaxOrdersPerSecond: 75,
		Enabled:            true,
	}); err != nil {
		t.Fatalf("seed throttle config: %v", err)
	}

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/throttle-config/FIRM-B", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	if result["firm_id"] != "FIRM-B" {
		t.Errorf("firm_id: want FIRM-B, got %v", result["firm_id"])
	}
	if result["max_orders_per_second"] != float64(75) {
		t.Errorf("max_orders_per_second: want 75, got %v", result["max_orders_per_second"])
	}
}

// TestThrottleConfig_Get_NotFound verifies that GET for an unknown firm returns HTTP 404.
func TestThrottleConfig_Get_NotFound(t *testing.T) {
	ts, _, _, _ := newThrottleTestServer(t)

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/throttle-config/FIRM-UNKNOWN", nil)
	assertStatus(t, resp, http.StatusNotFound)

	var errResp types.ErrorResponse
	decodeBody(t, resp, &errResp)
	if errResp.Error.Code != "NOT_FOUND" {
		t.Errorf("error code: want NOT_FOUND, got %q", errResp.Error.Code)
	}
}

// TestThrottleConfig_List verifies that GET /api/v1/securities/throttle-config
// returns a JSON array (possibly empty) with HTTP 200.
func TestThrottleConfig_List(t *testing.T) {
	ts, _, cfgStore, _ := newThrottleTestServer(t)

	// Seed two configs.
	for _, firmID := range []string{"FIRM-C", "FIRM-D"} {
		if err := cfgStore.Set(&types.ThrottleConfig{
			FirmID:             firmID,
			MaxOrdersPerSecond: 100,
			Enabled:            true,
		}); err != nil {
			t.Fatalf("seed throttle config %s: %v", firmID, err)
		}
	}

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/throttle-config", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	data, ok := result["data"].([]interface{})
	if !ok {
		t.Fatalf("expected 'data' to be a JSON array, got %T", result["data"])
	}
	if len(data) < 2 {
		t.Errorf("expected at least 2 configs in data array, got %d", len(data))
	}
	total, ok := result["total"].(float64)
	if !ok {
		t.Fatalf("expected 'total' field in response, got %T", result["total"])
	}
	if int(total) != len(data) {
		t.Errorf("total %d != len(data) %d", int(total), len(data))
	}
}

// TestThrottleConfig_List_Empty verifies that the list endpoint returns an
// empty array (not null) when no configs exist.
func TestThrottleConfig_List_Empty(t *testing.T) {
	ts, _, _, _ := newThrottleTestServer(t)

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/throttle-config", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	data, ok := result["data"].([]interface{})
	if !ok {
		t.Fatalf("expected 'data' to be a JSON array, got %T", result["data"])
	}
	if len(data) != 0 {
		t.Errorf("expected empty data array, got %d items", len(data))
	}
}

// TestThrottleConfig_Delete verifies that DELETE /api/v1/securities/throttle-config/{firm_id}
// removes the config and returns HTTP 204 (No Content).
func TestThrottleConfig_Delete(t *testing.T) {
	ts, _, cfgStore, _ := newThrottleTestServer(t)

	// Pre-seed config.
	if err := cfgStore.Set(&types.ThrottleConfig{
		FirmID:             "FIRM-E",
		MaxOrdersPerSecond: 30,
		Enabled:            true,
	}); err != nil {
		t.Fatalf("seed throttle config: %v", err)
	}

	// Delete it.
	resp := doJSON(t, ts, http.MethodDelete, "/api/v1/securities/throttle-config/FIRM-E", nil)
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()

	// Subsequent GET must return 404.
	resp2 := doJSON(t, ts, http.MethodGet, "/api/v1/securities/throttle-config/FIRM-E", nil)
	assertStatus(t, resp2, http.StatusNotFound)
	resp2.Body.Close()
}

// TestThrottleConfig_Delete_NotFound verifies that DELETE for an unknown firm
// returns HTTP 404.
func TestThrottleConfig_Delete_NotFound(t *testing.T) {
	ts, _, _, _ := newThrottleTestServer(t)

	resp := doJSON(t, ts, http.MethodDelete, "/api/v1/securities/throttle-config/NO-SUCH-FIRM", nil)
	assertStatus(t, resp, http.StatusNotFound)

	var errResp types.ErrorResponse
	decodeBody(t, resp, &errResp)
	if errResp.Error.Code != "NOT_FOUND" {
		t.Errorf("error code: want NOT_FOUND, got %q", errResp.Error.Code)
	}
}

// TestThrottleConfig_Set_InvalidLimit verifies that PUT with max_orders_per_second <= 0
// returns HTTP 422 UNPROCESSABLE_ENTITY.
func TestThrottleConfig_Set_InvalidLimit(t *testing.T) {
	ts, _, _, _ := newThrottleTestServer(t)

	payload := map[string]interface{}{
		"max_orders_per_second": 0,
		"enabled":               true,
	}
	resp := doJSON(t, ts, http.MethodPut, "/api/v1/securities/throttle-config/FIRM-F", payload)
	assertStatus(t, resp, http.StatusUnprocessableEntity)

	var errResp types.ErrorResponse
	decodeBody(t, resp, &errResp)
	if errResp.Error.Code != "INVALID_FIELD" {
		t.Errorf("error code: want INVALID_FIELD, got %q", errResp.Error.Code)
	}
}

// ── Per-firm throttle enforcement test ────────────────────────────────────────

// TestThrottle_UsesPerFirmConfig verifies the end-to-end per-firm throttle path:
//
//  1. Create a firm-specific ThrottleConfig with MaxOrdersPerSecond = 50.
//  2. Create a participant linked to that firm.
//  3. Create an active instrument.
//  4. Submit 51 orders for that participant within the same second.
//  5. The first 50 orders must succeed (HTTP 201).
//  6. The 51st order must be rejected with HTTP 429 THROTTLED.
func TestThrottle_UsesPerFirmConfig(t *testing.T) {
	ts, instrStore, cfgStore, participantStore := newThrottleTestServer(t)

	const firmID = "FIRM-THROTTLED"
	const participantID = "PART-THROTTLED"
	const maxPerSec = 50

	// 1. Set per-firm throttle config: 50 orders/sec.
	if err := cfgStore.Set(&types.ThrottleConfig{
		FirmID:             firmID,
		MaxOrdersPerSecond: maxPerSec,
		Enabled:            true,
	}); err != nil {
		t.Fatalf("set throttle config: %v", err)
	}

	// 2. Create a participant linked to the firm.
	participant := &types.ExchangeParticipant{
		ID:     participantID,
		FirmID: firmID,
		Status: types.ParticipantActive,
	}
	if err := participantStore.Create(participant); err != nil {
		t.Fatalf("create participant: %v", err)
	}

	// 3. Create an active instrument.
	inst := &types.Instrument{
		ID:            "THROTTLE-INST",
		Ticker:        "TTHROT",
		Name:          "Throttle Test Corp",
		AssetClass:    types.AssetClassEquity,
		LotSize:       1,
		TickSize:      0.01,
		TradingStatus: types.TradingStatusActive,
		ExchangeCode:  "MSE",
	}
	if err := instrStore.Create(inst); err != nil {
		t.Fatalf("create instrument: %v", err)
	}

	// 4. Submit 51 orders and track results.
	allowed := 0
	throttled := 0
	for i := 0; i < maxPerSec+1; i++ {
		payload := map[string]interface{}{
			"instrument_id":  inst.ID,
			"participant_id": participantID,
			"side":           "BUY",
			"order_type":     "LIMIT",
			"quantity":       1,
			"price":          10.0 + float64(i)*0.01,
		}
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/orders", payload)
		switch resp.StatusCode {
		case http.StatusCreated:
			allowed++
		case http.StatusTooManyRequests:
			throttled++
			// Verify error code.
			var errResp types.ErrorResponse
			decodeBody(t, resp, &errResp)
			if errResp.Error.Code != "THROTTLED" {
				t.Errorf("order %d: expected THROTTLED error code, got %q", i+1, errResp.Error.Code)
			}
		default:
			t.Errorf("order %d: unexpected HTTP %d", i+1, resp.StatusCode)
			resp.Body.Close()
		}
	}

	// 5-6. First 50 allowed, 51st rejected.
	if allowed != maxPerSec {
		t.Errorf("expected %d allowed orders, got %d", maxPerSec, allowed)
	}
	if throttled != 1 {
		t.Errorf("expected exactly 1 throttled order (the 51st), got %d", throttled)
	}
}
