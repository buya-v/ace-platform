// Package server — tests for off-book trade HTTP handlers.
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

// newOffBookTestServer creates a test server wired with a real OffBookTradeStore.
func newOffBookTestServer(t *testing.T) (*httptest.Server, *store.InMemoryOffBookTradeStore) {
	t.Helper()
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)
	obStore := store.NewInMemoryOffBookTradeStore()

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
		nil, nil, nil, nil, nil, nil, nil,
		nil, // surveillanceStore
		nil, // instrumentGroupStore
		obStore,
		nil, // locateStore
		nil, // rfqStore
		nil, // giveUpStore
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
	return ts, obStore
}

// validOffBookPayload returns a complete valid off-book trade submission payload.
func validOffBookPayload() map[string]interface{} {
	return map[string]interface{}{
		"instrument_id":    "INST-OBT",
		"buy_participant":  "BUYER-1",
		"sell_participant": "SELLER-1",
		"price":            100.50,
		"quantity":         500,
	}
}

// submitOffBookViaHTTP submits an off-book trade via POST and returns its ID.
func submitOffBookViaHTTP(t *testing.T, ts *httptest.Server, payload map[string]interface{}) string {
	t.Helper()
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/off-book-trades", payload)
	assertStatus(t, resp, http.StatusCreated)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	id, ok := result["id"].(string)
	if !ok || id == "" {
		t.Fatal("expected non-empty id in submit response")
	}
	return id
}

// ============================================================
// TestSubmitOffBookTrade
// ============================================================

func TestSubmitOffBookTrade_Success(t *testing.T) {
	ts, _ := newOffBookTestServer(t)

	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/off-book-trades", validOffBookPayload())
	assertStatus(t, resp, http.StatusCreated)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	if id, ok := result["id"].(string); !ok || id == "" {
		t.Error("expected non-empty id in response")
	}
	if result["instrument_id"] != "INST-OBT" {
		t.Errorf("expected instrument_id INST-OBT, got %v", result["instrument_id"])
	}
	if result["status"] != string(types.OffBookReported) {
		t.Errorf("expected status REPORTED, got %v", result["status"])
	}
	if result["buy_participant"] != "BUYER-1" {
		t.Errorf("expected buy_participant BUYER-1, got %v", result["buy_participant"])
	}
	if result["quantity"] != float64(500) {
		t.Errorf("expected quantity 500, got %v", result["quantity"])
	}
}

func TestSubmitOffBookTrade_MissingInstrumentID(t *testing.T) {
	ts, _ := newOffBookTestServer(t)

	payload := map[string]interface{}{
		"buy_participant":  "BUYER-1",
		"sell_participant": "SELLER-1",
		"price":            100.0,
		"quantity":         100,
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/off-book-trades", payload)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestSubmitOffBookTrade_MissingBuyParticipant(t *testing.T) {
	ts, _ := newOffBookTestServer(t)

	payload := map[string]interface{}{
		"instrument_id":    "INST-1",
		"sell_participant": "SELLER-1",
		"price":            100.0,
		"quantity":         100,
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/off-book-trades", payload)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestSubmitOffBookTrade_MissingSellParticipant(t *testing.T) {
	ts, _ := newOffBookTestServer(t)

	payload := map[string]interface{}{
		"instrument_id":   "INST-1",
		"buy_participant": "BUYER-1",
		"price":           100.0,
		"quantity":        100,
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/off-book-trades", payload)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestSubmitOffBookTrade_ZeroQuantity(t *testing.T) {
	ts, _ := newOffBookTestServer(t)

	payload := map[string]interface{}{
		"instrument_id":    "INST-1",
		"buy_participant":  "BUYER-1",
		"sell_participant": "SELLER-1",
		"price":            100.0,
		"quantity":         0,
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/off-book-trades", payload)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

// ============================================================
// TestListOffBookTrades
// ============================================================

func TestListOffBookTrades_Empty(t *testing.T) {
	ts, _ := newOffBookTestServer(t)

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/off-book-trades", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["total"] != float64(0) {
		t.Errorf("expected total 0, got %v", result["total"])
	}
}

func TestListOffBookTrades_ReturnsTwoTrades(t *testing.T) {
	ts, _ := newOffBookTestServer(t)

	p1 := validOffBookPayload()
	p2 := map[string]interface{}{
		"instrument_id":    "INST-OBT-2",
		"buy_participant":  "BUYER-2",
		"sell_participant": "SELLER-2",
		"price":            200.0,
		"quantity":         1000,
	}
	submitOffBookViaHTTP(t, ts, p1)
	submitOffBookViaHTTP(t, ts, p2)

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/off-book-trades", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["total"] != float64(2) {
		t.Errorf("expected total 2, got %v", result["total"])
	}
	data, ok := result["data"].([]interface{})
	if !ok {
		t.Fatalf("expected data array, got %T", result["data"])
	}
	if len(data) != 2 {
		t.Errorf("expected 2 trades in data, got %d", len(data))
	}
}

// ============================================================
// TestUpdateOffBookStatus
// ============================================================

func TestUpdateOffBookStatus_ReportedToConfirmed(t *testing.T) {
	ts, obStore := newOffBookTestServer(t)

	id := submitOffBookViaHTTP(t, ts, validOffBookPayload())

	// Update to CONFIRMED.
	resp := doJSON(t, ts, http.MethodPut,
		fmt.Sprintf("/api/v1/securities/off-book-trades/%s/status", id),
		map[string]string{"status": "CONFIRMED"})
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["status"] != "CONFIRMED" {
		t.Errorf("expected status CONFIRMED, got %v", result["status"])
	}

	// Verify in store.
	trade, err := obStore.Get(id)
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if trade.Status != types.OffBookConfirmed {
		t.Errorf("expected CONFIRMED in store, got %q", trade.Status)
	}
}

func TestUpdateOffBookStatus_NotFound(t *testing.T) {
	ts, _ := newOffBookTestServer(t)

	resp := doJSON(t, ts, http.MethodPut,
		"/api/v1/securities/off-book-trades/no-such-id/status",
		map[string]string{"status": "CONFIRMED"})
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestUpdateOffBookStatus_MissingStatus(t *testing.T) {
	ts, _ := newOffBookTestServer(t)

	id := submitOffBookViaHTTP(t, ts, validOffBookPayload())

	resp := doJSON(t, ts, http.MethodPut,
		fmt.Sprintf("/api/v1/securities/off-book-trades/%s/status", id),
		map[string]string{})
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

// ============================================================
// Not configured (503) test
// ============================================================

func TestOffBookEndpoints_NotConfigured(t *testing.T) {
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
		nil, nil, nil, nil, nil, nil, nil,
		nil, nil,
		nil, // offBookTradeStore = nil
		nil, nil, nil, // locateStore, rfqStore, giveUpStore
		nil, me, nil, nil, nil, cfg,
	)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)
	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	httpTS := httptest.NewServer(tenantMW(mux))
	t.Cleanup(httpTS.Close)

	for _, path := range []string{
		"/api/v1/securities/off-book-trades",
		"/api/v1/securities/off-book-trades/some-id/status",
	} {
		resp := doJSON(t, httpTS, http.MethodGet, path, nil)
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("path %s: expected 503, got %d", path, resp.StatusCode)
		}
		resp.Body.Close()
	}
}
