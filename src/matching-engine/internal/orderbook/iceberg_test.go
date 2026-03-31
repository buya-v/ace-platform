package orderbook

import (
	"testing"

	"github.com/garudax-platform/matching-engine/internal/types"
)

func newIcebergOrder(id string, side types.Side, price string, totalQty, displayQty uint64) *types.Order {
	return &types.Order{
		OrderID:      id,
		InstrumentID: "WHT-HRW-2026M07-UB",
		AccountID:    "account-" + id,
		Side:         side,
		OrderType:    types.OrderTypeLimit,
		TimeInForce:  types.TIFDay,
		Price:        mustParseDecimal(price),
		Quantity:     totalQty,
		DisplayQty:   displayQty,
	}
}

// --- Iceberg order placement ---

func TestIcebergOrderRestsWithDisplayQty(t *testing.T) {
	book := newTestBook()
	order := newIcebergOrder("ice1", types.SideSell, "100.00", 100, 10)

	result := book.SubmitOrder(order)

	if len(result.Trades) != 0 {
		t.Errorf("expected 0 trades, got %d", len(result.Trades))
	}
	if book.OrderCount() != 1 {
		t.Errorf("expected 1 order on book, got %d", book.OrderCount())
	}

	// The visible quantity on the book should be displayQty (10), not totalQty (100)
	askLevels := book.AskLevels()
	if len(askLevels) != 1 {
		t.Fatalf("expected 1 ask level, got %d", len(askLevels))
	}
	if askLevels[0].TotalQty != 10 {
		t.Errorf("expected visible qty 10 on level, got %d", askLevels[0].TotalQty)
	}

	// Verify internal state
	o, _ := book.GetOrder("ice1")
	if o.RemainingQty != 10 {
		t.Errorf("expected remaining qty 10 (display), got %d", o.RemainingQty)
	}
	if o.HiddenQty != 90 {
		t.Errorf("expected hidden qty 90, got %d", o.HiddenQty)
	}
	if o.TotalQty != 100 {
		t.Errorf("expected total qty 100, got %d", o.TotalQty)
	}
}

// --- Iceberg replenishment ---

func TestIcebergReplenishesAfterDisplayFilled(t *testing.T) {
	book := newTestBook()

	// Place iceberg sell: total 50, display 10
	ice := newIcebergOrder("ice1", types.SideSell, "100.00", 50, 10)
	book.SubmitOrder(ice)

	// Buy 10 — fills the display slice
	buy := newLimitOrder("buy1", types.SideBuy, "100.00", 10)
	result := book.SubmitOrder(buy)

	if len(result.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(result.Trades))
	}
	if result.Trades[0].Quantity != 10 {
		t.Errorf("expected trade qty 10, got %d", result.Trades[0].Quantity)
	}

	// Iceberg should still be on the book, replenished
	if book.OrderCount() != 1 {
		t.Errorf("expected 1 order (replenished iceberg), got %d", book.OrderCount())
	}

	o, ok := book.GetOrder("ice1")
	if !ok {
		t.Fatal("iceberg should still be on book after replenishment")
	}
	if o.RemainingQty != 10 {
		t.Errorf("expected replenished remaining qty 10, got %d", o.RemainingQty)
	}
	if o.HiddenQty != 30 {
		t.Errorf("expected hidden qty 30 (50-10filled-10display), got %d", o.HiddenQty)
	}
	if o.FilledQty != 10 {
		t.Errorf("expected filled qty 10, got %d", o.FilledQty)
	}
}

func TestIcebergMultipleReplenishments(t *testing.T) {
	book := newTestBook()

	// Place iceberg sell: total 30, display 10
	ice := newIcebergOrder("ice1", types.SideSell, "100.00", 30, 10)
	book.SubmitOrder(ice)

	// Fill display slice 1 (10)
	buy1 := newLimitOrder("buy1", types.SideBuy, "100.00", 10)
	book.SubmitOrder(buy1)

	o, _ := book.GetOrder("ice1")
	if o.HiddenQty != 10 {
		t.Errorf("after first fill: expected hidden 10, got %d", o.HiddenQty)
	}

	// Fill display slice 2 (10)
	buy2 := newLimitOrder("buy2", types.SideBuy, "100.00", 10)
	book.SubmitOrder(buy2)

	o, _ = book.GetOrder("ice1")
	if o.HiddenQty != 0 {
		t.Errorf("after second fill: expected hidden 0, got %d", o.HiddenQty)
	}
	if o.RemainingQty != 10 {
		t.Errorf("after second fill: expected remaining 10, got %d", o.RemainingQty)
	}

	// Fill last display slice (10) — iceberg fully exhausted
	buy3 := newLimitOrder("buy3", types.SideBuy, "100.00", 10)
	result := book.SubmitOrder(buy3)

	if book.OrderCount() != 0 {
		t.Errorf("expected empty book after full iceberg fill, got %d", book.OrderCount())
	}

	// Last fill should be FILL exec type (not partial)
	hasFill := false
	for _, r := range result.ExecutionReports {
		if r.OrderID == "ice1" && r.ExecType == types.ExecTypeFill {
			hasFill = true
		}
	}
	if !hasFill {
		t.Error("expected FILL exec type for final iceberg exhaustion")
	}
}

func TestIcebergPartialReplenishWhenRemainingLessThanDisplay(t *testing.T) {
	book := newTestBook()

	// Place iceberg sell: total 25, display 10
	ice := newIcebergOrder("ice1", types.SideSell, "100.00", 25, 10)
	book.SubmitOrder(ice)

	// Fill first display (10)
	buy1 := newLimitOrder("buy1", types.SideBuy, "100.00", 10)
	book.SubmitOrder(buy1)

	o, _ := book.GetOrder("ice1")
	if o.RemainingQty != 10 {
		t.Errorf("expected remaining 10 after first fill, got %d", o.RemainingQty)
	}
	if o.HiddenQty != 5 {
		t.Errorf("expected hidden 5, got %d", o.HiddenQty)
	}

	// Fill second display (10)
	buy2 := newLimitOrder("buy2", types.SideBuy, "100.00", 10)
	book.SubmitOrder(buy2)

	// Now only 5 remain (less than display of 10)
	o, _ = book.GetOrder("ice1")
	if o.RemainingQty != 5 {
		t.Errorf("expected remaining 5 (partial replenish), got %d", o.RemainingQty)
	}
	if o.HiddenQty != 0 {
		t.Errorf("expected hidden 0, got %d", o.HiddenQty)
	}

	// Verify the visible qty on the level
	askLevels := book.AskLevels()
	if len(askLevels) != 1 {
		t.Fatalf("expected 1 ask level, got %d", len(askLevels))
	}
	if askLevels[0].TotalQty != 5 {
		t.Errorf("expected level qty 5, got %d", askLevels[0].TotalQty)
	}
}

func TestIcebergLosesTimePriorityOnReplenish(t *testing.T) {
	book := newTestBook()

	// Place iceberg sell: total 20, display 5
	ice := newIcebergOrder("ice1", types.SideSell, "100.00", 20, 5)
	book.SubmitOrder(ice)

	// Place regular sell at same price AFTER iceberg
	regular := newLimitOrder("reg1", types.SideSell, "100.00", 10)
	book.SubmitOrder(regular)

	// Buy 5 — fills iceberg display, iceberg replenishes and goes to back of queue
	buy1 := newLimitOrder("buy1", types.SideBuy, "100.00", 5)
	result := book.SubmitOrder(buy1)

	if len(result.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(result.Trades))
	}
	if result.Trades[0].SellOrderID != "ice1" {
		t.Errorf("first match should be against iceberg (has time priority), got %s", result.Trades[0].SellOrderID)
	}

	// Now buy 5 more — should match against regular (iceberg lost time priority)
	buy2 := newLimitOrder("buy2", types.SideBuy, "100.00", 5)
	result2 := book.SubmitOrder(buy2)

	if len(result2.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(result2.Trades))
	}
	if result2.Trades[0].SellOrderID != "reg1" {
		t.Errorf("second match should be against regular (iceberg lost priority), got %s", result2.Trades[0].SellOrderID)
	}
}

func TestIcebergExecReportShowsPartialFillDuringReplenishment(t *testing.T) {
	book := newTestBook()

	// Place iceberg: total 20, display 10
	ice := newIcebergOrder("ice1", types.SideSell, "100.00", 20, 10)
	book.SubmitOrder(ice)

	// Fill first display slice
	buy := newLimitOrder("buy1", types.SideBuy, "100.00", 10)
	result := book.SubmitOrder(buy)

	// The iceberg fill should be PARTIAL_FILL (not FILL) since hidden qty remains
	for _, r := range result.ExecutionReports {
		if r.OrderID == "ice1" {
			if r.ExecType == types.ExecTypeFill {
				t.Error("iceberg with hidden qty remaining should report PARTIAL_FILL, not FILL")
			}
		}
	}
}

func TestIcebergWithMarketOrderSweep(t *testing.T) {
	book := newTestBook()

	// Place iceberg sell: total 30, display 10
	ice := newIcebergOrder("ice1", types.SideSell, "100.00", 30, 10)
	book.SubmitOrder(ice)

	// Market buy 25 — should fill display(10), replenish, fill display(10), replenish, fill 5
	buy := newMarketOrder("buy1", types.SideBuy, 25)
	result := book.SubmitOrder(buy)

	// Should produce 3 trades: 10, 10, 5
	if len(result.Trades) != 3 {
		t.Fatalf("expected 3 trades (iceberg replenishment), got %d", len(result.Trades))
	}
	if result.Trades[0].Quantity != 10 {
		t.Errorf("trade 1: expected qty 10, got %d", result.Trades[0].Quantity)
	}
	if result.Trades[1].Quantity != 10 {
		t.Errorf("trade 2: expected qty 10, got %d", result.Trades[1].Quantity)
	}
	if result.Trades[2].Quantity != 5 {
		t.Errorf("trade 3: expected qty 5, got %d", result.Trades[2].Quantity)
	}

	// Iceberg should have 5 remaining on book
	o, ok := book.GetOrder("ice1")
	if !ok {
		t.Fatal("iceberg should still be on book")
	}
	if o.RemainingQty != 5 {
		t.Errorf("expected remaining 5, got %d", o.RemainingQty)
	}
	if o.FilledQty != 25 {
		t.Errorf("expected filled 25, got %d", o.FilledQty)
	}
}

// --- Iceberg validation ---

func TestIcebergRejectMarketOrder(t *testing.T) {
	book := newTestBook()
	order := &types.Order{
		OrderID:      "bad-ice",
		InstrumentID: "WHT-HRW-2026M07-UB",
		AccountID:    "acc1",
		Side:         types.SideBuy,
		OrderType:    types.OrderTypeMarket,
		TimeInForce:  types.TIFDay,
		Quantity:     100,
		DisplayQty:   10,
	}
	result := book.SubmitOrder(order)

	hasReject := false
	for _, r := range result.ExecutionReports {
		if r.ExecType == types.ExecTypeRejected {
			hasReject = true
		}
	}
	if !hasReject {
		t.Error("expected rejection for market iceberg order")
	}
}

func TestIcebergRejectDisplayGeTotal(t *testing.T) {
	book := newTestBook()
	order := &types.Order{
		OrderID:      "bad-ice2",
		InstrumentID: "WHT-HRW-2026M07-UB",
		AccountID:    "acc1",
		Side:         types.SideBuy,
		OrderType:    types.OrderTypeLimit,
		TimeInForce:  types.TIFDay,
		Price:        mustParseDecimal("100.00"),
		Quantity:     10,
		DisplayQty:   10, // display == total: not a valid iceberg
	}
	result := book.SubmitOrder(order)

	hasReject := false
	for _, r := range result.ExecutionReports {
		if r.ExecType == types.ExecTypeRejected {
			hasReject = true
		}
	}
	if !hasReject {
		t.Error("expected rejection when display qty >= total qty")
	}
}

func TestNonIcebergOrderUnchanged(t *testing.T) {
	book := newTestBook()
	// Regular limit order (displayQty = 0) should work unchanged
	order := newLimitOrder("reg1", types.SideBuy, "100.00", 10)
	book.SubmitOrder(order)

	o, _ := book.GetOrder("reg1")
	if o.DisplayQty != 0 {
		t.Errorf("non-iceberg should have display 0, got %d", o.DisplayQty)
	}
	if o.TotalQty != 0 {
		t.Errorf("non-iceberg should have total 0, got %d", o.TotalQty)
	}
	if o.HiddenQty != 0 {
		t.Errorf("non-iceberg should have hidden 0, got %d", o.HiddenQty)
	}
}

func TestIcebergCancellation(t *testing.T) {
	book := newTestBook()

	ice := newIcebergOrder("ice1", types.SideSell, "100.00", 50, 10)
	book.SubmitOrder(ice)

	report, err := book.CancelOrder("ice1")
	if err != nil {
		t.Fatalf("cancel error: %v", err)
	}
	if report.ExecType != types.ExecTypeCancelled {
		t.Errorf("expected CANCELLED, got %d", report.ExecType)
	}
	if book.OrderCount() != 0 {
		t.Errorf("expected empty book, got %d", book.OrderCount())
	}
}
