// Package server — internal tests for position HTTP handlers.
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

// newPositionTestServer creates a test server with a wired PositionStore
// that callers can pre-populate.
func newPositionTestServer(t *testing.T) (*httptest.Server, *store.InMemoryPositionStore) {
	t.Helper()
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()

	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)
	cfg := DefaultConfig()

	srv := New(
		instrStore, orderStore, tradeStore, positionStore,
		nil,
		store.NewInMemoryCorporateActionStore(),
		store.NewInMemoryEntitlementStore(),
		store.NewInMemoryMarketStore(),
		store.NewInMemorySegmentStore(),
		store.NewInMemoryCircuitBreakerStore(),
		store.NewInMemoryFirmStore(),
		store.NewInMemoryParticipantStore(),
		nil, nil, nil, nil, nil,
		nil, nil,
		nil, nil, nil,
		nil, nil, nil, // locateStore, rfqStore, giveUpStore
		nil, nil, nil, // investigationStore, replayStore, bondStore
		nil, nil, nil, nil, // strategyStore, custodyAccountStore, custodyBalanceStore, csdTransferStore
		nil, me, nil, nil, nil,
		cfg,
	)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	ts := httptest.NewServer(tenantMW(mux))
	t.Cleanup(ts.Close)
	return ts, positionStore
}

// ============================================================
// TestListPositions
// ============================================================

func TestListPositions_Empty(t *testing.T) {
	ts, _ := newPositionTestServer(t)

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/positions", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	total, ok := result["total"].(float64)
	if !ok {
		t.Fatalf("expected total field as number, got %T", result["total"])
	}
	if total != 0 {
		t.Errorf("expected total 0, got %v", total)
	}

	data, ok := result["data"].([]interface{})
	if !ok {
		t.Fatalf("expected data as array, got %T", result["data"])
	}
	if len(data) != 0 {
		t.Errorf("expected empty data array, got %d elements", len(data))
	}
}

func TestListPositions_WithData(t *testing.T) {
	ts, posStore := newPositionTestServer(t)

	// Seed positions directly into the store.
	positions := []types.Position{
		{
			ID:            "P1:INST-1",
			ParticipantID: "P1",
			InstrumentID:  "INST-1",
			Quantity:      500,
			AvgCost:       10.50,
			MarketValue:   5250.0,
			UnrealizedPnl: 250.0,
		},
		{
			ID:            "P2:INST-1",
			ParticipantID: "P2",
			InstrumentID:  "INST-1",
			Quantity:      200,
			AvgCost:       11.00,
			MarketValue:   2200.0,
			UnrealizedPnl: -100.0,
		},
		{
			ID:            "P1:INST-2",
			ParticipantID: "P1",
			InstrumentID:  "INST-2",
			Quantity:      1000,
			AvgCost:       5.00,
			MarketValue:   5000.0,
			UnrealizedPnl: 0.0,
		},
	}
	for i := range positions {
		if err := posStore.Update(&positions[i]); err != nil {
			t.Fatalf("posStore.Update[%d]: %v", i, err)
		}
	}

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/positions", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	total, ok := result["total"].(float64)
	if !ok {
		t.Fatalf("expected total as number, got %T", result["total"])
	}
	if int(total) != len(positions) {
		t.Errorf("expected total %d, got %v", len(positions), total)
	}

	data, ok := result["data"].([]interface{})
	if !ok {
		t.Fatalf("expected data as array, got %T", result["data"])
	}
	if len(data) != len(positions) {
		t.Errorf("expected %d positions in data, got %d", len(positions), len(data))
	}

	// Verify each returned position has the required fields.
	for i, item := range data {
		pos, ok := item.(map[string]interface{})
		if !ok {
			t.Errorf("data[%d]: expected object, got %T", i, item)
			continue
		}
		if pos["participant_id"] == nil {
			t.Errorf("data[%d]: missing participant_id", i)
		}
		if pos["instrument_id"] == nil {
			t.Errorf("data[%d]: missing instrument_id", i)
		}
		if pos["quantity"] == nil {
			t.Errorf("data[%d]: missing quantity", i)
		}
	}
}

func TestListPositions_MethodNotAllowed(t *testing.T) {
	ts, _ := newPositionTestServer(t)

	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/positions", nil)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	resp.Body.Close()
}

func TestListPositions_NilStore_Returns503(t *testing.T) {
	// Build a server with a nil positionStore to verify the guard.
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	// positionStore is nil — matching engine also needs one, use a real one for engine but nil for server.
	posStoreForEngine := store.NewInMemoryPositionStore()
	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, posStoreForEngine, nil, nil, nil)
	cfg := DefaultConfig()

	srv := New(
		instrStore, orderStore, tradeStore,
		nil, // positionStore nil → guard triggers
		nil,
		store.NewInMemoryCorporateActionStore(),
		store.NewInMemoryEntitlementStore(),
		store.NewInMemoryMarketStore(),
		store.NewInMemorySegmentStore(),
		store.NewInMemoryCircuitBreakerStore(),
		store.NewInMemoryFirmStore(),
		store.NewInMemoryParticipantStore(),
		nil, nil, nil, nil, nil,
		nil, nil,
		nil, nil, nil,
		nil, nil, nil, // locateStore, rfqStore, giveUpStore
		nil, nil, nil, // investigationStore, replayStore, bondStore
		nil, nil, nil, nil, // strategyStore, custodyAccountStore, custodyBalanceStore, csdTransferStore
		nil, me, nil, nil, nil,
		cfg,
	)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	ts := httptest.NewServer(tenantMW(mux))
	defer ts.Close()

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/positions", nil)
	assertStatus(t, resp, http.StatusServiceUnavailable)
	resp.Body.Close()
}
