// Package server — internal tests for reference price HTTP handlers.
package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/middleware"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// newReferencePriceTestServer creates a test server with wired ReferencePriceStore
// and CircuitBreakerStore (needed for the side-effect path in SetReferencePrice).
func newReferencePriceTestServer(t *testing.T) (*httptest.Server, *store.InMemoryReferencePriceStore, *store.InMemoryCircuitBreakerStore) {
	t.Helper()
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	rpStore := store.NewInMemoryReferencePriceStore()
	cbStore := store.NewInMemoryCircuitBreakerStore()

	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)
	cfg := DefaultConfig()

	srv := New(
		instrStore, orderStore, tradeStore, positionStore,
		nil,
		store.NewInMemoryCorporateActionStore(),
		store.NewInMemoryEntitlementStore(),
		store.NewInMemoryMarketStore(),
		store.NewInMemorySegmentStore(),
		cbStore,
		store.NewInMemoryFirmStore(),
		store.NewInMemoryParticipantStore(),
		nil, nil, nil, nil, nil,
		nil,
		rpStore,
		nil, me, nil, nil, nil,
		cfg,
	)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	ts := httptest.NewServer(tenantMW(mux))
	t.Cleanup(ts.Close)
	return ts, rpStore, cbStore
}

func refPricePath(instrumentID string) string {
	return fmt.Sprintf("/api/v1/securities/instruments/%s/reference-price", instrumentID)
}

// ============================================================
// TestSetReferencePrice
// ============================================================

func TestSetReferencePrice_Success(t *testing.T) {
	ts, _, _ := newReferencePriceTestServer(t)

	body := map[string]interface{}{
		"price":                   125.50,
		"set_by":                  "operator-1",
		"stale_threshold_minutes": 30,
	}
	resp := doJSON(t, ts, http.MethodPost, refPricePath("INST-A"), body)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	if result["instrument_id"] != "INST-A" {
		t.Errorf("expected instrument_id INST-A, got %v", result["instrument_id"])
	}
	if result["price"] != 125.50 {
		t.Errorf("expected price 125.50, got %v", result["price"])
	}
	if result["set_by"] != "operator-1" {
		t.Errorf("expected set_by operator-1, got %v", result["set_by"])
	}
	if result["set_at"] == nil || result["set_at"].(string) == "" {
		t.Error("expected set_at to be populated")
	}
}

func TestSetReferencePrice_InvalidPrice_Zero(t *testing.T) {
	ts, _, _ := newReferencePriceTestServer(t)

	body := map[string]interface{}{
		"price":  0,
		"set_by": "operator-1",
	}
	resp := doJSON(t, ts, http.MethodPost, refPricePath("INST-B"), body)
	assertStatus(t, resp, http.StatusBadRequest)

	var errResp map[string]interface{}
	decodeBody(t, resp, &errResp)
	errBlock := errResp["error"].(map[string]interface{})
	if errBlock["code"] != "INVALID_FIELD" {
		t.Errorf("expected code INVALID_FIELD, got %v", errBlock["code"])
	}
}

func TestSetReferencePrice_InvalidPrice_Negative(t *testing.T) {
	ts, _, _ := newReferencePriceTestServer(t)

	body := map[string]interface{}{
		"price":  -10.0,
		"set_by": "operator-1",
	}
	resp := doJSON(t, ts, http.MethodPost, refPricePath("INST-C"), body)
	assertStatus(t, resp, http.StatusBadRequest)

	var errResp map[string]interface{}
	decodeBody(t, resp, &errResp)
	errBlock := errResp["error"].(map[string]interface{})
	if errBlock["code"] != "INVALID_FIELD" {
		t.Errorf("expected code INVALID_FIELD for negative price, got %v", errBlock["code"])
	}
}

func TestSetReferencePrice_UpdatesCircuitBreaker(t *testing.T) {
	ts, _, cbStore := newReferencePriceTestServer(t)

	// Pre-populate a circuit breaker for the instrument.
	cb := &types.CircuitBreaker{
		InstrumentID:   "INST-CB",
		ReferencePrice: 100.0,
		Status:         "ACTIVE",
	}
	if err := cbStore.Set(cb); err != nil {
		t.Fatalf("cbStore.Set: %v", err)
	}

	body := map[string]interface{}{
		"price":  200.0,
		"set_by": "operator-1",
	}
	resp := doJSON(t, ts, http.MethodPost, refPricePath("INST-CB"), body)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// Verify circuit breaker reference price was updated.
	updatedCB, err := cbStore.Get("INST-CB")
	if err != nil {
		t.Fatalf("cbStore.Get: %v", err)
	}
	if updatedCB.ReferencePrice != 200.0 {
		t.Errorf("expected circuit breaker reference_price 200.0, got %f", updatedCB.ReferencePrice)
	}
}

// ============================================================
// TestGetReferencePrice
// ============================================================

func TestGetReferencePrice_Found(t *testing.T) {
	ts, rpStore, _ := newReferencePriceTestServer(t)

	// Seed directly into the store so we control SetAt precisely.
	rp := &types.ReferencePrice{
		InstrumentID:          "INST-GET",
		Price:                 88.88,
		SetBy:                 "seed-admin",
		SetAt:                 time.Now().UTC().Format(time.RFC3339),
		StaleThresholdMinutes: 120,
	}
	if err := rpStore.Set(rp); err != nil {
		t.Fatalf("rpStore.Set: %v", err)
	}

	resp := doJSON(t, ts, http.MethodGet, refPricePath("INST-GET"), nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	// Response must include both reference_price and stale flag.
	rpData, ok := result["reference_price"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected reference_price object in response, got %T", result["reference_price"])
	}
	if rpData["instrument_id"] != "INST-GET" {
		t.Errorf("expected instrument_id INST-GET, got %v", rpData["instrument_id"])
	}
	if rpData["price"] != 88.88 {
		t.Errorf("expected price 88.88, got %v", rpData["price"])
	}

	// stale must be present and be a bool.
	if _, ok := result["stale"].(bool); !ok {
		t.Errorf("expected stale field as bool, got %T: %v", result["stale"], result["stale"])
	}
	// Price was just set, so it should not be stale.
	if result["stale"].(bool) {
		t.Error("expected stale=false for freshly-set price, got true")
	}
}

func TestGetReferencePrice_StaleFlag(t *testing.T) {
	ts, rpStore, _ := newReferencePriceTestServer(t)

	// Set a reference price with a past SetAt to trigger stale detection.
	pastTime := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	rp := &types.ReferencePrice{
		InstrumentID:          "INST-STALE",
		Price:                 50.0,
		SetBy:                 "admin",
		SetAt:                 pastTime,
		StaleThresholdMinutes: 30, // 30 min threshold — 2h old → stale
	}
	if err := rpStore.Set(rp); err != nil {
		t.Fatalf("rpStore.Set: %v", err)
	}

	resp := doJSON(t, ts, http.MethodGet, refPricePath("INST-STALE"), nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	if !result["stale"].(bool) {
		t.Error("expected stale=true for price older than threshold, got false")
	}
}

func TestGetReferencePrice_NotFound(t *testing.T) {
	ts, _, _ := newReferencePriceTestServer(t)

	resp := doJSON(t, ts, http.MethodGet, refPricePath("INST-MISSING"), nil)
	assertStatus(t, resp, http.StatusNotFound)

	var errResp map[string]interface{}
	decodeBody(t, resp, &errResp)
	errBlock := errResp["error"].(map[string]interface{})
	if errBlock["code"] != "NOT_FOUND" {
		t.Errorf("expected code NOT_FOUND, got %v", errBlock["code"])
	}
}

func TestGetReferencePrice_MethodNotAllowed(t *testing.T) {
	ts, _, _ := newReferencePriceTestServer(t)

	resp := doJSON(t, ts, http.MethodDelete, refPricePath("INST-ANY"), nil)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	resp.Body.Close()
}
