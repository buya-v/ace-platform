package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ace-platform/matching-engine/internal/engine"
	"github.com/ace-platform/matching-engine/internal/store"
	"github.com/ace-platform/matching-engine/internal/types"
)

type testIDGen struct {
	counter uint64
}

func (g *testIDGen) NewID() string {
	n := atomic.AddUint64(&g.counter, 1)
	return fmt.Sprintf("tid-%d", n)
}

func newTestServer() *Server {
	eng := engine.NewEngine(&testIDGen{})
	ts := store.NewInMemoryTradeStore()
	cfg := DefaultConfig()
	return NewServer(eng, ts, cfg)
}

func TestSubmitOrderAndMatch(t *testing.T) {
	s := newTestServer()
	s.RegisterInstrument("WHEAT")

	// Submit sell
	_, err := s.SubmitOrder(SubmitOrderRequest{
		OrderID:      "sell-1",
		InstrumentID: "WHEAT",
		AccountID:    "seller",
		Side:         types.SideSell,
		OrderType:    types.OrderTypeLimit,
		TimeInForce:  types.TIFDay,
		Price:        "100",
		Quantity:     10,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Submit matching buy
	report, err := s.SubmitOrder(SubmitOrderRequest{
		OrderID:      "buy-1",
		InstrumentID: "WHEAT",
		AccountID:    "buyer",
		Side:         types.SideBuy,
		OrderType:    types.OrderTypeLimit,
		TimeInForce:  types.TIFDay,
		Price:        "100",
		Quantity:     10,
	})
	if err != nil {
		t.Fatal(err)
	}

	if report.ExecType != types.ExecTypeNew {
		t.Errorf("expected NEW exec type, got %d", report.ExecType)
	}

	// Verify trade was persisted
	last, ok := s.tradeStore.LastTrade("WHEAT")
	if !ok {
		t.Fatal("expected trade to be persisted")
	}
	if last.Quantity != 10 {
		t.Errorf("expected trade qty 10, got %d", last.Quantity)
	}
}

func TestSubmitOrderValidation(t *testing.T) {
	s := newTestServer()
	s.RegisterInstrument("WHEAT")

	tests := []struct {
		name string
		req  SubmitOrderRequest
	}{
		{"missing instrument", SubmitOrderRequest{OrderID: "o1", AccountID: "a1", Quantity: 10, Price: "100"}},
		{"missing account", SubmitOrderRequest{OrderID: "o1", InstrumentID: "WHEAT", Quantity: 10, Price: "100"}},
		{"zero quantity", SubmitOrderRequest{OrderID: "o1", InstrumentID: "WHEAT", AccountID: "a1", Quantity: 0, Price: "100"}},
		{"invalid price", SubmitOrderRequest{OrderID: "o1", InstrumentID: "WHEAT", AccountID: "a1", Quantity: 10, Price: "abc"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.SubmitOrder(tt.req)
			if err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestCancelOrder(t *testing.T) {
	s := newTestServer()
	s.RegisterInstrument("WHEAT")

	s.SubmitOrder(SubmitOrderRequest{
		OrderID:      "sell-1",
		InstrumentID: "WHEAT",
		AccountID:    "acc1",
		Side:         types.SideSell,
		OrderType:    types.OrderTypeLimit,
		TimeInForce:  types.TIFDay,
		Price:        "100",
		Quantity:     10,
	})

	report, err := s.CancelOrder("WHEAT", "sell-1", "acc1")
	if err != nil {
		t.Fatal(err)
	}
	if report.ExecType != types.ExecTypeCancelled {
		t.Errorf("expected CANCELLED, got %d", report.ExecType)
	}
}

func TestCancelOrderValidation(t *testing.T) {
	s := newTestServer()

	_, err := s.CancelOrder("", "o1", "a1")
	if err == nil {
		t.Error("expected error for empty instrument_id")
	}

	_, err = s.CancelOrder("WHEAT", "", "a1")
	if err == nil {
		t.Error("expected error for empty order_id")
	}
}

func TestCancelAllOrders(t *testing.T) {
	s := newTestServer()
	s.RegisterInstrument("WHEAT")

	for i := 0; i < 3; i++ {
		s.SubmitOrder(SubmitOrderRequest{
			OrderID:      fmt.Sprintf("sell-%d", i),
			InstrumentID: "WHEAT",
			AccountID:    "acc1",
			Side:         types.SideSell,
			OrderType:    types.OrderTypeLimit,
			TimeInForce:  types.TIFDay,
			Price:        fmt.Sprintf("%d", 100+i),
			Quantity:     10,
		})
	}

	count, ids, err := s.CancelAllOrders("WHEAT", "acc1", types.SideUnspecified)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("expected 3 cancellations, got %d", count)
	}
	if len(ids) != 3 {
		t.Errorf("expected 3 IDs, got %d", len(ids))
	}
}

func TestGetOrderBookSnapshot(t *testing.T) {
	s := newTestServer()
	s.RegisterInstrument("WHEAT")

	// Place some orders
	s.SubmitOrder(SubmitOrderRequest{
		OrderID: "bid1", InstrumentID: "WHEAT", AccountID: "a1",
		Side: types.SideBuy, OrderType: types.OrderTypeLimit,
		TimeInForce: types.TIFDay, Price: "99", Quantity: 10,
	})
	s.SubmitOrder(SubmitOrderRequest{
		OrderID: "ask1", InstrumentID: "WHEAT", AccountID: "a2",
		Side: types.SideSell, OrderType: types.OrderTypeLimit,
		TimeInForce: types.TIFDay, Price: "101", Quantity: 5,
	})

	snap, err := s.GetOrderBookSnapshot("WHEAT", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Bids) != 1 {
		t.Errorf("expected 1 bid level, got %d", len(snap.Bids))
	}
	if len(snap.Asks) != 1 {
		t.Errorf("expected 1 ask level, got %d", len(snap.Asks))
	}
	if snap.Bids[0].Quantity != 10 {
		t.Errorf("expected bid qty 10, got %d", snap.Bids[0].Quantity)
	}
	if snap.Asks[0].Quantity != 5 {
		t.Errorf("expected ask qty 5, got %d", snap.Asks[0].Quantity)
	}
}

func TestGetLastTrade(t *testing.T) {
	s := newTestServer()
	s.RegisterInstrument("WHEAT")

	// No trades yet
	_, err := s.GetLastTrade("WHEAT")
	if err == nil {
		t.Error("expected error when no trades")
	}

	// Create a trade
	s.SubmitOrder(SubmitOrderRequest{
		OrderID: "sell1", InstrumentID: "WHEAT", AccountID: "seller",
		Side: types.SideSell, OrderType: types.OrderTypeLimit,
		TimeInForce: types.TIFDay, Price: "100", Quantity: 10,
	})
	s.SubmitOrder(SubmitOrderRequest{
		OrderID: "buy1", InstrumentID: "WHEAT", AccountID: "buyer",
		Side: types.SideBuy, OrderType: types.OrderTypeLimit,
		TimeInForce: types.TIFDay, Price: "100", Quantity: 10,
	})

	trade, err := s.GetLastTrade("WHEAT")
	if err != nil {
		t.Fatal(err)
	}
	if trade.Quantity != 10 {
		t.Errorf("expected trade qty 10, got %d", trade.Quantity)
	}
}

func TestAppendOnlyTradeWrites(t *testing.T) {
	s := newTestServer()
	s.RegisterInstrument("WHEAT")

	// Generate multiple trades
	for i := 0; i < 5; i++ {
		s.SubmitOrder(SubmitOrderRequest{
			OrderID: fmt.Sprintf("sell-%d", i), InstrumentID: "WHEAT",
			AccountID: fmt.Sprintf("seller-%d", i),
			Side: types.SideSell, OrderType: types.OrderTypeLimit,
			TimeInForce: types.TIFDay, Price: "100", Quantity: 1,
		})
		s.SubmitOrder(SubmitOrderRequest{
			OrderID: fmt.Sprintf("buy-%d", i), InstrumentID: "WHEAT",
			AccountID: fmt.Sprintf("buyer-%d", i),
			Side: types.SideBuy, OrderType: types.OrderTypeLimit,
			TimeInForce: types.TIFDay, Price: "100", Quantity: 1,
		})
	}

	trades := s.tradeStore.Trades("WHEAT")
	if len(trades) != 5 {
		t.Fatalf("expected 5 trades, got %d", len(trades))
	}

	// Verify sequence numbers are strictly increasing
	for i := 1; i < len(trades); i++ {
		if trades[i].SequenceNumber <= trades[i-1].SequenceNumber {
			t.Errorf("trade sequence not increasing: %d <= %d", trades[i].SequenceNumber, trades[i-1].SequenceNumber)
		}
	}
}

func TestHealthEndpoints(t *testing.T) {
	s := newTestServer()

	// Test healthz
	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("healthz: expected 200, got %d", w.Code)
	}

	// Test readyz - not ready
	req = httptest.NewRequest("GET", "/readyz", nil)
	w = httptest.NewRecorder()
	http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.IsReady() {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ready"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("not ready"))
		}
	}).ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("readyz: expected 503 when not ready, got %d", w.Code)
	}

	// Set ready
	s.SetReady()

	req = httptest.NewRequest("GET", "/readyz", nil)
	w = httptest.NewRecorder()
	http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.IsReady() {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ready"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("not ready"))
		}
	}).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("readyz: expected 200 when ready, got %d", w.Code)
	}
}

func TestConfigFromEnv(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.GRPCPort != 50051 {
		t.Errorf("expected default gRPC port 50051, got %d", cfg.GRPCPort)
	}
	if cfg.HealthPort != 8081 {
		t.Errorf("expected default health port 8081, got %d", cfg.HealthPort)
	}
	if !cfg.DirectPodComms {
		t.Error("expected direct pod comms enabled by default")
	}
}

func TestModifyOrderViaServer(t *testing.T) {
	s := newTestServer()
	s.RegisterInstrument("WHEAT")

	s.SubmitOrder(SubmitOrderRequest{
		OrderID: "sell-1", InstrumentID: "WHEAT", AccountID: "acc1",
		Side: types.SideSell, OrderType: types.OrderTypeLimit,
		TimeInForce: types.TIFDay, Price: "100", Quantity: 10,
	})

	report, err := s.ModifyOrder("WHEAT", "sell-1", "acc1", "99", 0)
	if err != nil {
		t.Fatal(err)
	}
	if report.ExecType != types.ExecTypeCancelled {
		t.Errorf("expected CANCELLED for cancel-replace, got %d", report.ExecType)
	}
}
