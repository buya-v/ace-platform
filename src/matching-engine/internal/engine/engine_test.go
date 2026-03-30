package engine

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/garudax-platform/matching-engine/internal/types"
)

type testIDGen struct {
	counter uint64
}

func (g *testIDGen) NewID() string {
	n := atomic.AddUint64(&g.counter, 1)
	return fmt.Sprintf("tid-%d", n)
}

func newTestEngine() *Engine {
	return NewEngine(&testIDGen{})
}

func mustParseDecimal(s string) types.Decimal {
	d, err := types.ParseDecimal(s)
	if err != nil {
		panic(err)
	}
	return d
}

func TestEngineBasicFlow(t *testing.T) {
	eng := newTestEngine()
	if err := eng.RegisterInstrument("WHEAT"); err != nil {
		t.Fatal(err)
	}

	var trades []types.Trade
	eng.SetTradeHandler(func(trade types.Trade) {
		trades = append(trades, trade)
	})

	// Submit sell
	sell := &types.Order{
		OrderID:      "sell-1",
		InstrumentID: "WHEAT",
		AccountID:    "seller",
		Side:         types.SideSell,
		OrderType:    types.OrderTypeLimit,
		TimeInForce:  types.TIFDay,
		Price:        mustParseDecimal("100"),
		Quantity:     10,
	}
	_, err := eng.SubmitOrder(sell)
	if err != nil {
		t.Fatal(err)
	}

	// Submit matching buy
	buy := &types.Order{
		OrderID:      "buy-1",
		InstrumentID: "WHEAT",
		AccountID:    "buyer",
		Side:         types.SideBuy,
		OrderType:    types.OrderTypeLimit,
		TimeInForce:  types.TIFDay,
		Price:        mustParseDecimal("100"),
		Quantity:     10,
	}
	result, err := eng.SubmitOrder(buy)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(result.Trades))
	}
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade via handler, got %d", len(trades))
	}
}

func TestEngineUnknownInstrument(t *testing.T) {
	eng := newTestEngine()

	order := &types.Order{
		OrderID:      "o1",
		InstrumentID: "UNKNOWN",
		Side:         types.SideBuy,
		OrderType:    types.OrderTypeLimit,
		Price:        mustParseDecimal("100"),
		Quantity:     10,
	}
	_, err := eng.SubmitOrder(order)
	if err == nil {
		t.Error("expected error for unknown instrument")
	}
}

func TestEngineCancelOrder(t *testing.T) {
	eng := newTestEngine()
	eng.RegisterInstrument("WHEAT")

	sell := &types.Order{
		OrderID:      "sell-1",
		InstrumentID: "WHEAT",
		AccountID:    "acc1",
		Side:         types.SideSell,
		OrderType:    types.OrderTypeLimit,
		TimeInForce:  types.TIFDay,
		Price:        mustParseDecimal("100"),
		Quantity:     10,
	}
	eng.SubmitOrder(sell)

	report, err := eng.CancelOrder("WHEAT", "sell-1")
	if err != nil {
		t.Fatal(err)
	}
	if report.ExecType != types.ExecTypeCancelled {
		t.Errorf("expected CANCELLED, got %d", report.ExecType)
	}
}

func TestEngineConcurrentOrders(t *testing.T) {
	eng := newTestEngine()
	eng.RegisterInstrument("WHEAT")

	// Pre-populate sell side
	for i := 0; i < 100; i++ {
		sell := &types.Order{
			OrderID:      fmt.Sprintf("sell-%d", i),
			InstrumentID: "WHEAT",
			AccountID:    fmt.Sprintf("seller-%d", i),
			Side:         types.SideSell,
			OrderType:    types.OrderTypeLimit,
			TimeInForce:  types.TIFDay,
			Price:        mustParseDecimal("100"),
			Quantity:     1,
		}
		eng.SubmitOrder(sell)
	}

	// Concurrent buy orders
	var wg sync.WaitGroup
	var totalTrades int64
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			buy := &types.Order{
				OrderID:      fmt.Sprintf("buy-%d", idx),
				InstrumentID: "WHEAT",
				AccountID:    fmt.Sprintf("buyer-%d", idx),
				Side:         types.SideBuy,
				OrderType:    types.OrderTypeLimit,
				TimeInForce:  types.TIFDay,
				Price:        mustParseDecimal("100"),
				Quantity:     5,
			}
			result, err := eng.SubmitOrder(buy)
			if err != nil {
				t.Errorf("concurrent submit error: %v", err)
				return
			}
			for _, trade := range result.Trades {
				atomic.AddInt64(&totalTrades, int64(trade.Quantity))
			}
		}(i)
	}
	wg.Wait()

	// All 50 buy quantities should match against the 100 sell quantities
	if totalTrades != 50 {
		t.Errorf("expected 50 total trade qty, got %d", totalTrades)
	}
}
