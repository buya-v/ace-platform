// Package server — tests for history archive HTTP handlers.
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/middleware"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// newTestServerWithHistory creates a test server wired with a HistoryStore,
// OrderStore, and TradeStore so that the history endpoints are reachable.
func newTestServerWithHistory(t *testing.T) (*httptest.Server, *store.InMemoryHistoryStore, *store.InMemoryOrderStore, *store.InMemoryTradeStore) {
	t.Helper()
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	histStore := store.NewInMemoryHistoryStore()

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
	srv.SetHistoryStore(histStore)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	ts := httptest.NewServer(tenantMW(mux))
	t.Cleanup(ts.Close)
	return ts, histStore, orderStore, tradeStore
}

// ============================================================
// TestListHistoricalOrders
// ============================================================

func TestListHistoricalOrders(t *testing.T) {
	ts, histStore, _, _ := newTestServerWithHistory(t)

	// Pre-populate the history store directly.
	now := time.Now().UTC().Format(time.RFC3339)
	for i, id := range []string{"ord-h1", "ord-h2", "ord-h3"} {
		_ = i
		o := types.SecurityOrder{
			ID:            id,
			InstrumentID:  "INST-1",
			ParticipantID: "P1",
			Side:          types.OrderSideBuy,
			OrderType:     types.OrderTypeLimit,
			Quantity:      100,
			Price:         decLit(10.0),
			Status:        types.OrderStatusFilled,
			ArchivedAt:    now,
		}
		if err := histStore.ArchiveOrder(o); err != nil {
			t.Fatalf("pre-populate ArchiveOrder: %v", err)
		}
	}

	t.Run("GET returns all archived orders (no filters)", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/history/orders", nil)
		assertStatus(t, resp, http.StatusOK)

		var result []interface{}
		decodeBody(t, resp, &result)
		if len(result) != 3 {
			t.Errorf("expected 3 archived orders, got %d", len(result))
		}
	})

	t.Run("GET with date_from in far future returns empty", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/history/orders?date_from=2099-01-01T00:00:00Z", nil)
		assertStatus(t, resp, http.StatusOK)

		var result []interface{}
		decodeBody(t, resp, &result)
		if len(result) != 0 {
			t.Errorf("expected 0 orders for future dateFrom, got %d", len(result))
		}
	})

	t.Run("GET with date_to in distant past returns empty", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/history/orders?date_to=2000-01-01T00:00:00Z", nil)
		assertStatus(t, resp, http.StatusOK)

		var result []interface{}
		decodeBody(t, resp, &result)
		if len(result) != 0 {
			t.Errorf("expected 0 orders for past dateTo, got %d", len(result))
		}
	})

	t.Run("wrong method returns 405", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodDelete, "/api/v1/securities/history/orders", nil)
		assertStatus(t, resp, http.StatusMethodNotAllowed)
		resp.Body.Close()
	})
}

// ============================================================
// TestArchiveTrigger
// ============================================================

func TestArchiveTrigger(t *testing.T) {
	ts, histStore, orderStore, tradeStore := newTestServerWithHistory(t)

	// Submit orders in various terminal and non-terminal states.
	terminalOrder := &types.SecurityOrder{
		ID:            "arc-ord-filled",
		InstrumentID:  "INST-1",
		ParticipantID: "P1",
		Side:          types.OrderSideBuy,
		OrderType:     types.OrderTypeLimit,
		Quantity:      100,
		Price:         decLit(10.0),
		Status:        types.OrderStatusFilled,
	}
	activeOrder := &types.SecurityOrder{
		ID:            "arc-ord-active",
		InstrumentID:  "INST-1",
		ParticipantID: "P1",
		Side:          types.OrderSideSell,
		OrderType:     types.OrderTypeLimit,
		Quantity:      50,
		Price:         decLit(10.0),
		Status:        types.OrderStatusPending,
	}
	if err := orderStore.Submit(terminalOrder); err != nil {
		t.Fatalf("Submit terminalOrder: %v", err)
	}
	if err := orderStore.Submit(activeOrder); err != nil {
		t.Fatalf("Submit activeOrder: %v", err)
	}

	settledTrade := &types.SecurityTrade{
		ID:           "arc-trd-settled",
		InstrumentID: "INST-1",
		BuyOrderID:   "arc-ord-filled",
		SellOrderID:  "sell-1",
		Price:        decLit(10.0),
		Quantity:     100,
		Status:       types.TradeStatusSettled,
	}
	pendingTrade := &types.SecurityTrade{
		ID:           "arc-trd-pending",
		InstrumentID: "INST-1",
		BuyOrderID:   "buy-2",
		SellOrderID:  "sell-2",
		Price:        decLit(10.0),
		Quantity:     50,
		Status:       types.TradeStatusPending,
	}
	if err := tradeStore.Create(settledTrade); err != nil {
		t.Fatalf("Create settledTrade: %v", err)
	}
	if err := tradeStore.Create(pendingTrade); err != nil {
		t.Fatalf("Create pendingTrade: %v", err)
	}

	t.Run("POST triggers archive and returns counts", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/history/archive", nil)
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		decodeBody(t, resp, &result)

		if result["archived_orders"] != float64(1) {
			t.Errorf("expected archived_orders=1, got %v", result["archived_orders"])
		}
		if result["archived_trades"] != float64(1) {
			t.Errorf("expected archived_trades=1, got %v", result["archived_trades"])
		}
		if _, ok := result["archived_at"].(string); !ok {
			t.Error("expected archived_at string in response")
		}
	})

	t.Run("archived records appear in history store", func(t *testing.T) {
		orders, err := histStore.ListOrders("", "")
		if err != nil {
			t.Fatalf("ListOrders: %v", err)
		}
		if len(orders) != 1 {
			t.Errorf("expected 1 archived order in history store, got %d", len(orders))
		}
		if orders[0].ID != "arc-ord-filled" {
			t.Errorf("expected archived order ID arc-ord-filled, got %q", orders[0].ID)
		}

		trades, err := histStore.ListTrades("", "")
		if err != nil {
			t.Fatalf("ListTrades: %v", err)
		}
		if len(trades) != 1 {
			t.Errorf("expected 1 archived trade in history store, got %d", len(trades))
		}
	})

	t.Run("wrong method on archive endpoint returns 405", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/history/archive", nil)
		assertStatus(t, resp, http.StatusMethodNotAllowed)
		resp.Body.Close()
	})
}

// ============================================================
// TestListHistoricalTrades
// ============================================================

func TestListHistoricalTrades(t *testing.T) {
	ts, histStore, _, _ := newTestServerWithHistory(t)

	now := time.Now().UTC().Format(time.RFC3339)
	for _, id := range []string{"trd-h1", "trd-h2"} {
		tr := types.SecurityTrade{
			ID:           id,
			InstrumentID: "INST-2",
			BuyOrderID:   "buy-1",
			SellOrderID:  "sell-1",
			Price:        decLit(20.0),
			Quantity:     50,
			Status:       types.TradeStatusSettled,
			ArchivedAt:   now,
		}
		if err := histStore.ArchiveTrade(tr); err != nil {
			t.Fatalf("ArchiveTrade: %v", err)
		}
	}

	t.Run("GET returns all archived trades", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/history/trades", nil)
		assertStatus(t, resp, http.StatusOK)

		var result []interface{}
		decodeBody(t, resp, &result)
		if len(result) != 2 {
			t.Errorf("expected 2 archived trades, got %d", len(result))
		}
	})

	t.Run("GET with future date_from returns empty", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/history/trades?date_from=2099-01-01T00:00:00Z", nil)
		assertStatus(t, resp, http.StatusOK)

		var result []interface{}
		decodeBody(t, resp, &result)
		if len(result) != 0 {
			t.Errorf("expected 0 trades for future dateFrom, got %d", len(result))
		}
	})

	t.Run("wrong method returns 405", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/history/trades", nil)
		assertStatus(t, resp, http.StatusMethodNotAllowed)
		resp.Body.Close()
	})
}
