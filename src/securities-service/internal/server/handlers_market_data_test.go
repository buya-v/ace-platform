// Package server — tests for market data HTTP handlers (P3b).
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

// newTestServerWithTradeStore creates a test server where the caller supplies
// the tradeStore directly so tests can seed trades before any HTTP request.
func newTestServerWithTradeStore(
	t *testing.T,
	instrStore store.InstrumentStore,
	orderStore store.OrderStore,
	tradeStore store.TradeStore,
) *httptest.Server {
	t.Helper()
	cfg := DefaultConfig()
	positionStore := store.NewInMemoryPositionStore()
	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)
	srv := New(instrStore, orderStore, tradeStore, positionStore, nil, store.NewInMemoryCorporateActionStore(), store.NewInMemoryEntitlementStore(), store.NewInMemoryMarketStore(), store.NewInMemorySegmentStore(), store.NewInMemoryCircuitBreakerStore(), store.NewInMemoryFirmStore(), store.NewInMemoryParticipantStore(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, me, nil, nil, nil, cfg)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	handler := tenantMW(mux)

	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts
}

// submitLimitOrder submits a PENDING limit order via the service-desk endpoint.
func submitLimitOrder(t *testing.T, ts *httptest.Server, instrID, participantID, side string, price float64, qty int) {
	t.Helper()
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/service-desk/orders", map[string]interface{}{
		"participant_id": participantID,
		"instrument_id":  instrID,
		"side":           side,
		"order_type":     "LIMIT",
		"quantity":       qty,
		"price":          price,
		"time_in_force":  "GTC",
	})
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
}

// ============================================================
// TestOrderBookSnapshot
// ============================================================

// TestOrderBookSnapshot creates an instrument, submits 2 buy + 2 sell PENDING
// orders at distinct price levels, then verifies the order-book snapshot
// returns non-empty bids and asks sorted correctly.
func TestOrderBookSnapshot(t *testing.T) {
	ts := newTestServer(t)

	instrID := createInstr(t, ts, map[string]interface{}{
		"ticker":      "BOOK1",
		"name":        "Book Instrument One",
		"asset_class": "EQUITY",
		"lot_size":    100,
		"tick_size":   0.01,
	})

	// 2 buy orders at different prices.
	submitLimitOrder(t, ts, instrID, "P1", "BUY", 10.00, 100)
	submitLimitOrder(t, ts, instrID, "P1", "BUY", 9.50, 200)
	// 2 sell orders at different prices.
	submitLimitOrder(t, ts, instrID, "P2", "SELL", 10.50, 150)
	submitLimitOrder(t, ts, instrID, "P2", "SELL", 11.00, 50)

	resp := doJSON(t, ts, http.MethodGet,
		"/api/v1/securities/market-data/book/"+instrID, nil)
	assertStatus(t, resp, http.StatusOK)

	var snapshot types.OrderBookSnapshot
	decodeBody(t, resp, &snapshot)

	if snapshot.InstrumentID != instrID {
		t.Errorf("expected instrument_id %q, got %q", instrID, snapshot.InstrumentID)
	}
	if len(snapshot.Bids) == 0 {
		t.Fatal("expected non-empty bids")
	}
	if len(snapshot.Asks) == 0 {
		t.Fatal("expected non-empty asks")
	}

	// Bids must be sorted descending by price.
	for i := 1; i < len(snapshot.Bids); i++ {
		if snapshot.Bids[i].Price > snapshot.Bids[i-1].Price {
			t.Errorf("bids not descending at index %d: %.2f > %.2f",
				i, snapshot.Bids[i].Price, snapshot.Bids[i-1].Price)
		}
	}
	// Asks must be sorted ascending by price.
	for i := 1; i < len(snapshot.Asks); i++ {
		if snapshot.Asks[i].Price < snapshot.Asks[i-1].Price {
			t.Errorf("asks not ascending at index %d: %.2f < %.2f",
				i, snapshot.Asks[i].Price, snapshot.Asks[i-1].Price)
		}
	}

	// Best bid = highest buy price.
	if snapshot.Bids[0].Price != 10.00 {
		t.Errorf("expected best bid 10.00, got %.2f", snapshot.Bids[0].Price)
	}
	// Best ask = lowest sell price.
	if snapshot.Asks[0].Price != 10.50 {
		t.Errorf("expected best ask 10.50, got %.2f", snapshot.Asks[0].Price)
	}
	if snapshot.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}
}

// ============================================================
// TestOrderBookEmpty
// ============================================================

// TestOrderBookEmpty verifies that an instrument with no orders returns
// empty (not nil) bids and asks arrays.
func TestOrderBookEmpty(t *testing.T) {
	ts := newTestServer(t)

	instrID := createInstr(t, ts, map[string]interface{}{
		"ticker":      "EMPTY1",
		"name":        "Empty Book Instrument",
		"asset_class": "EQUITY",
		"lot_size":    100,
		"tick_size":   0.01,
	})

	resp := doJSON(t, ts, http.MethodGet,
		"/api/v1/securities/market-data/book/"+instrID, nil)
	assertStatus(t, resp, http.StatusOK)

	var snapshot types.OrderBookSnapshot
	decodeBody(t, resp, &snapshot)

	if snapshot.Bids == nil {
		t.Error("expected bids to be an empty array, not nil")
	}
	if len(snapshot.Bids) != 0 {
		t.Errorf("expected 0 bids, got %d", len(snapshot.Bids))
	}
	if snapshot.Asks == nil {
		t.Error("expected asks to be an empty array, not nil")
	}
	if len(snapshot.Asks) != 0 {
		t.Errorf("expected 0 asks, got %d", len(snapshot.Asks))
	}
}

// ============================================================
// TestTicker
// ============================================================

// TestTicker seeds two trades directly in the store then verifies GET ticker
// returns populated last_price and volume.
func TestTicker(t *testing.T) {
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()

	ts := newTestServerWithTradeStore(t, instrStore, orderStore, tradeStore)

	instrID := createInstr(t, ts, map[string]interface{}{
		"ticker":      "TICK1",
		"name":        "Ticker Instrument One",
		"asset_class": "EQUITY",
		"lot_size":    100,
		"tick_size":   0.01,
	})

	today := time.Now().UTC().Format("2006-01-02")
	now := time.Now().UTC().Format(time.RFC3339)

	if err := tradeStore.Create(&types.SecurityTrade{
		ID:           "tick-trade-1",
		InstrumentID: instrID,
		Price:        15.50,
		Quantity:     100,
		TradeDate:    today,
		Status:       types.TradeStatusConfirmed,
		CreatedAt:    now,
	}); err != nil {
		t.Fatalf("seed trade 1: %v", err)
	}
	if err := tradeStore.Create(&types.SecurityTrade{
		ID:           "tick-trade-2",
		InstrumentID: instrID,
		Price:        16.00,
		Quantity:     200,
		TradeDate:    today,
		Status:       types.TradeStatusConfirmed,
		CreatedAt:    now,
	}); err != nil {
		t.Fatalf("seed trade 2: %v", err)
	}

	resp := doJSON(t, ts, http.MethodGet,
		"/api/v1/securities/market-data/ticker/"+instrID, nil)
	assertStatus(t, resp, http.StatusOK)

	var ticker types.TickerData
	decodeBody(t, resp, &ticker)

	if ticker.InstrumentID != instrID {
		t.Errorf("expected instrument_id %q, got %q", instrID, ticker.InstrumentID)
	}
	if ticker.LastPrice == 0 {
		t.Error("expected non-zero last_price")
	}
	if ticker.Volume == 0 {
		t.Error("expected non-zero volume")
	}
	if ticker.DayHigh == 0 {
		t.Error("expected non-zero day_high")
	}
	if ticker.DayLow == 0 {
		t.Error("expected non-zero day_low")
	}
	if ticker.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}
}

// ============================================================
// TestRecentTrades
// ============================================================

// TestRecentTrades seeds 3 trades for an instrument and verifies that
// GET /market-data/trades/{id} returns total=3 with a data array of length 3.
func TestRecentTrades(t *testing.T) {
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()

	ts := newTestServerWithTradeStore(t, instrStore, orderStore, tradeStore)

	instrID := createInstr(t, ts, map[string]interface{}{
		"ticker":      "RTRX1",
		"name":        "Recent Trades Instrument",
		"asset_class": "EQUITY",
		"lot_size":    100,
		"tick_size":   0.01,
	})

	today := time.Now().UTC().Format("2006-01-02")
	now := time.Now().UTC().Format(time.RFC3339)

	prices := []float64{10.00, 10.50, 11.00}
	for i, price := range prices {
		if err := tradeStore.Create(&types.SecurityTrade{
			ID:           fmt.Sprintf("rt-trade-%d", i+1),
			InstrumentID: instrID,
			Price:        price,
			Quantity:     100,
			TradeDate:    today,
			Status:       types.TradeStatusConfirmed,
			CreatedAt:    now,
		}); err != nil {
			t.Fatalf("seed trade %d: %v", i+1, err)
		}
	}

	resp := doJSON(t, ts, http.MethodGet,
		"/api/v1/securities/market-data/trades/"+instrID, nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	total, ok := result["total"].(float64)
	if !ok {
		t.Fatalf("expected numeric total field, got %T: %v", result["total"], result["total"])
	}
	if int(total) != 3 {
		t.Errorf("expected total=3, got %d", int(total))
	}

	data, ok := result["data"].([]interface{})
	if !ok {
		t.Fatalf("expected data array, got %T", result["data"])
	}
	if len(data) != 3 {
		t.Errorf("expected 3 trades in data array, got %d", len(data))
	}
}
