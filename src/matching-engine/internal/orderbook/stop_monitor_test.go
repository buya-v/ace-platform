package orderbook

import (
	"testing"
	"time"

	"github.com/garudax-platform/matching-engine/internal/types"
)

func newStopLimitOrder(id string, side types.Side, limitPrice, stopPrice string, qty uint64) *types.Order {
	return &types.Order{
		OrderID:      id,
		InstrumentID: "WHT-HRW-2026M07-UB",
		AccountID:    "account-" + id,
		Side:         side,
		OrderType:    types.OrderTypeStopLimit,
		TimeInForce:  types.TIFDay,
		Price:        mustParseDecimal(limitPrice),
		StopPrice:    mustParseDecimal(stopPrice),
		Quantity:     qty,
		CreatedAt:    time.Now(),
	}
}

func newStopMarketOrder(id string, side types.Side, stopPrice string, qty uint64) *types.Order {
	return &types.Order{
		OrderID:      id,
		InstrumentID: "WHT-HRW-2026M07-UB",
		AccountID:    "account-" + id,
		Side:         side,
		OrderType:    types.OrderTypeStopMarket,
		TimeInForce:  types.TIFDay,
		StopPrice:    mustParseDecimal(stopPrice),
		Quantity:     qty,
		CreatedAt:    time.Now(),
	}
}

// --- Adding stop orders ---

func TestAddStopLimitOrder(t *testing.T) {
	book := newTestBook()
	sm := NewStopMonitor(book)

	stop := newStopLimitOrder("stop1", types.SideBuy, "105.00", "103.00", 10)
	report := sm.AddStopOrder(stop)

	if report.ExecType != types.ExecTypeNew {
		t.Errorf("expected NEW exec type, got %d", report.ExecType)
	}
	if report.OrderStatus != types.OrderStatusPendingNew {
		t.Errorf("expected PENDING_NEW status, got %s", report.OrderStatus.String())
	}
	if sm.StopOrderCount() != 1 {
		t.Errorf("expected 1 stop order, got %d", sm.StopOrderCount())
	}

	// Stop should NOT be on the main book
	if book.OrderCount() != 0 {
		t.Errorf("stop orders should not be on the main book, got %d", book.OrderCount())
	}
}

func TestAddStopMarketOrder(t *testing.T) {
	book := newTestBook()
	sm := NewStopMonitor(book)

	stop := newStopMarketOrder("stop1", types.SideSell, "95.00", 10)
	report := sm.AddStopOrder(stop)

	if report.ExecType != types.ExecTypeNew {
		t.Errorf("expected NEW, got %d", report.ExecType)
	}
	if sm.StopOrderCount() != 1 {
		t.Errorf("expected 1 stop, got %d", sm.StopOrderCount())
	}
}

// --- Validation ---

func TestRejectNonStopOrderType(t *testing.T) {
	book := newTestBook()
	sm := NewStopMonitor(book)

	order := newLimitOrder("bad1", types.SideBuy, "100.00", 10)
	report := sm.AddStopOrder(order)

	if report.ExecType != types.ExecTypeRejected {
		t.Error("expected rejection for non-stop order type")
	}
}

func TestRejectStopWithoutStopPrice(t *testing.T) {
	book := newTestBook()
	sm := NewStopMonitor(book)

	order := &types.Order{
		OrderID:   "bad2",
		Side:      types.SideBuy,
		OrderType: types.OrderTypeStopLimit,
		Price:     mustParseDecimal("100.00"),
		Quantity:  10,
		CreatedAt: time.Now(),
	}
	report := sm.AddStopOrder(order)

	if report.ExecType != types.ExecTypeRejected {
		t.Error("expected rejection for stop without stop price")
	}
}

func TestRejectStopWithZeroQuantity(t *testing.T) {
	book := newTestBook()
	sm := NewStopMonitor(book)

	order := &types.Order{
		OrderID:   "bad3",
		Side:      types.SideBuy,
		OrderType: types.OrderTypeStopMarket,
		StopPrice: mustParseDecimal("100.00"),
		Quantity:  0,
		CreatedAt: time.Now(),
	}
	report := sm.AddStopOrder(order)

	if report.ExecType != types.ExecTypeRejected {
		t.Error("expected rejection for zero quantity")
	}
}

func TestRejectStopLimitWithoutLimitPrice(t *testing.T) {
	book := newTestBook()
	sm := NewStopMonitor(book)

	order := &types.Order{
		OrderID:   "bad4",
		Side:      types.SideBuy,
		OrderType: types.OrderTypeStopLimit,
		StopPrice: mustParseDecimal("100.00"),
		Quantity:  10,
		CreatedAt: time.Now(),
	}
	report := sm.AddStopOrder(order)

	if report.ExecType != types.ExecTypeRejected {
		t.Error("expected rejection for stop-limit without limit price")
	}
}

// --- Triggering ---

func TestBuyStopTriggersOnPriceRise(t *testing.T) {
	book := newTestBook()
	sm := NewStopMonitor(book)

	// Place sell liquidity on the book
	sell := newLimitOrder("sell1", types.SideSell, "105.00", 10)
	book.SubmitOrder(sell)

	// Add buy stop: trigger at 103, limit 105
	stop := newStopLimitOrder("stop1", types.SideBuy, "105.00", "103.00", 5)
	sm.AddStopOrder(stop)

	// Price hasn't reached stop yet
	result := sm.OnTrade(mustParseDecimal("102.00"))
	if len(result.Trades) != 0 {
		t.Errorf("stop should not trigger at 102, got %d trades", len(result.Trades))
	}
	if sm.StopOrderCount() != 1 {
		t.Errorf("stop should still be pending, got %d", sm.StopOrderCount())
	}

	// Price rises to 103 — triggers
	result = sm.OnTrade(mustParseDecimal("103.00"))
	if sm.StopOrderCount() != 0 {
		t.Errorf("stop should be triggered and removed, got %d", sm.StopOrderCount())
	}

	// The triggered stop becomes a limit buy at 105, matches sell at 105
	if len(result.Trades) != 1 {
		t.Fatalf("expected 1 trade from triggered stop, got %d", len(result.Trades))
	}
	if result.Trades[0].Quantity != 5 {
		t.Errorf("expected trade qty 5, got %d", result.Trades[0].Quantity)
	}
	if result.Trades[0].Price.String() != "105" {
		t.Errorf("expected trade price 105, got %s", result.Trades[0].Price.String())
	}
}

func TestSellStopTriggersOnPriceDrop(t *testing.T) {
	book := newTestBook()
	sm := NewStopMonitor(book)

	// Place buy liquidity
	buy := newLimitOrder("buy1", types.SideBuy, "95.00", 10)
	book.SubmitOrder(buy)

	// Add sell stop: trigger at 97
	stop := newStopMarketOrder("stop1", types.SideSell, "97.00", 5)
	sm.AddStopOrder(stop)

	// Price above stop — no trigger
	result := sm.OnTrade(mustParseDecimal("98.00"))
	if len(result.Trades) != 0 {
		t.Errorf("stop should not trigger at 98")
	}

	// Price drops to 97 — triggers
	result = sm.OnTrade(mustParseDecimal("97.00"))
	if len(result.Trades) != 1 {
		t.Fatalf("expected 1 trade from triggered stop-market, got %d", len(result.Trades))
	}
	if result.Trades[0].Quantity != 5 {
		t.Errorf("expected qty 5, got %d", result.Trades[0].Quantity)
	}
	// Market sell matches at bid price 95
	if result.Trades[0].Price.String() != "95" {
		t.Errorf("expected fill at 95, got %s", result.Trades[0].Price.String())
	}
}

func TestMultipleStopsTriggeredByOnePrice(t *testing.T) {
	book := newTestBook()
	sm := NewStopMonitor(book)

	// Sell liquidity
	sell1 := newLimitOrder("sell1", types.SideSell, "110.00", 10)
	sell2 := newLimitOrder("sell2", types.SideSell, "112.00", 10)
	book.SubmitOrder(sell1)
	book.SubmitOrder(sell2)

	// Two buy stops at different trigger prices
	stop1 := newStopLimitOrder("stop1", types.SideBuy, "110.00", "105.00", 5)
	stop2 := newStopLimitOrder("stop2", types.SideBuy, "112.00", "108.00", 5)
	sm.AddStopOrder(stop1)
	sm.AddStopOrder(stop2)

	// Price jumps to 110 — both stops should trigger (105 and 108 both <= 110)
	result := sm.OnTrade(mustParseDecimal("110.00"))

	if sm.StopOrderCount() != 0 {
		t.Errorf("all stops should be triggered, %d remaining", sm.StopOrderCount())
	}
	if len(result.Trades) != 2 {
		t.Fatalf("expected 2 trades from triggered stops, got %d", len(result.Trades))
	}
}

func TestStopMarketNoLiquidity(t *testing.T) {
	book := newTestBook()
	sm := NewStopMonitor(book)

	// Stop-market sell with no buy liquidity
	stop := newStopMarketOrder("stop1", types.SideSell, "95.00", 10)
	sm.AddStopOrder(stop)

	result := sm.OnTrade(mustParseDecimal("94.00"))

	// Stop triggers but no match (no liquidity)
	if sm.StopOrderCount() != 0 {
		t.Errorf("stop should be triggered, got %d remaining", sm.StopOrderCount())
	}
	if len(result.Trades) != 0 {
		t.Errorf("expected 0 trades (no liquidity), got %d", len(result.Trades))
	}
	// Should have cancel report for unfilled market order
	hasCancelReport := false
	for _, r := range result.ExecutionReports {
		if r.OrderID == "stop1" && r.ExecType == types.ExecTypeCancelled {
			hasCancelReport = true
		}
	}
	if !hasCancelReport {
		t.Error("expected cancel report for triggered stop-market with no liquidity")
	}
}

// --- Cancel stop ---

func TestCancelStopOrder(t *testing.T) {
	book := newTestBook()
	sm := NewStopMonitor(book)

	stop := newStopLimitOrder("stop1", types.SideBuy, "105.00", "103.00", 10)
	sm.AddStopOrder(stop)

	report, ok := sm.CancelStopOrder("stop1")
	if !ok {
		t.Fatal("expected cancel to succeed")
	}
	if report.ExecType != types.ExecTypeCancelled {
		t.Errorf("expected CANCELLED, got %d", report.ExecType)
	}
	if sm.StopOrderCount() != 0 {
		t.Errorf("expected 0 stops, got %d", sm.StopOrderCount())
	}
}

func TestCancelNonExistentStop(t *testing.T) {
	book := newTestBook()
	sm := NewStopMonitor(book)

	_, ok := sm.CancelStopOrder("nonexistent")
	if ok {
		t.Error("expected cancel to fail for nonexistent stop")
	}
}

// --- GetStopOrder ---

func TestGetStopOrder(t *testing.T) {
	book := newTestBook()
	sm := NewStopMonitor(book)

	stop := newStopLimitOrder("stop1", types.SideBuy, "105.00", "103.00", 10)
	sm.AddStopOrder(stop)

	got, ok := sm.GetStopOrder("stop1")
	if !ok {
		t.Fatal("expected to find stop order")
	}
	if got.OrderID != "stop1" {
		t.Errorf("expected stop1, got %s", got.OrderID)
	}
	if got.Status != types.OrderStatusPendingNew {
		t.Errorf("expected PENDING_NEW, got %s", got.Status.String())
	}
}

// --- Stop order does not affect main book ---

func TestStopOrderDoesNotAffectBookState(t *testing.T) {
	book := newTestBook()
	sm := NewStopMonitor(book)

	// Add stops
	sm.AddStopOrder(newStopLimitOrder("stop1", types.SideBuy, "105.00", "103.00", 10))
	sm.AddStopOrder(newStopMarketOrder("stop2", types.SideSell, "95.00", 10))

	// Book should be empty
	if book.OrderCount() != 0 {
		t.Errorf("stops should not be on main book, got %d", book.OrderCount())
	}
	if len(book.BidLevels()) != 0 {
		t.Errorf("expected no bid levels, got %d", len(book.BidLevels()))
	}
	if len(book.AskLevels()) != 0 {
		t.Errorf("expected no ask levels, got %d", len(book.AskLevels()))
	}
}

func TestStopLimitRestsOnBookIfNoMatch(t *testing.T) {
	book := newTestBook()
	sm := NewStopMonitor(book)

	// Stop-limit buy: trigger at 103, limit at 102 — no sell liquidity at 102
	stop := newStopLimitOrder("stop1", types.SideBuy, "102.00", "103.00", 10)
	sm.AddStopOrder(stop)

	// Trigger the stop
	result := sm.OnTrade(mustParseDecimal("103.00"))

	// No trades (no sell at 102 or below)
	if len(result.Trades) != 0 {
		t.Errorf("expected 0 trades, got %d", len(result.Trades))
	}

	// But the converted limit order should now rest on the book
	if book.OrderCount() != 1 {
		t.Errorf("triggered stop-limit should rest on book, got %d orders", book.OrderCount())
	}
	if book.BestBid().String() != "102" {
		t.Errorf("expected best bid at 102, got %s", book.BestBid().String())
	}
}
