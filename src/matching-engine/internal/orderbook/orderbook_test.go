package orderbook

import (
	"fmt"
	"testing"

	"github.com/garudax-platform/matching-engine/internal/types"
)

// testIDGen generates sequential IDs for testing.
type testIDGen struct {
	counter uint64
}

func (g *testIDGen) NewID() string {
	g.counter++
	return fmt.Sprintf("test-id-%d", g.counter)
}

func newTestBook() *OrderBook {
	var seq uint64
	return NewOrderBook("WHT-HRW-2026M07-UB", &testIDGen{}, &seq)
}

func mustParseDecimal(s string) types.Decimal {
	d, err := types.ParseDecimal(s)
	if err != nil {
		panic(err)
	}
	return d
}

func newLimitOrder(id string, side types.Side, price string, qty uint64) *types.Order {
	return &types.Order{
		OrderID:      id,
		InstrumentID: "WHT-HRW-2026M07-UB",
		AccountID:    "account-" + id,
		Side:         side,
		OrderType:    types.OrderTypeLimit,
		TimeInForce:  types.TIFDay,
		Price:        mustParseDecimal(price),
		Quantity:     qty,
	}
}

func newMarketOrder(id string, side types.Side, qty uint64) *types.Order {
	return &types.Order{
		OrderID:      id,
		InstrumentID: "WHT-HRW-2026M07-UB",
		AccountID:    "account-" + id,
		Side:         side,
		OrderType:    types.OrderTypeMarket,
		TimeInForce:  types.TIFDay,
		Quantity:     qty,
	}
}

// --- Basic Order Placement ---

func TestLimitOrderRestsOnBook(t *testing.T) {
	book := newTestBook()
	order := newLimitOrder("o1", types.SideBuy, "100.00", 10)

	result := book.SubmitOrder(order)

	if len(result.Trades) != 0 {
		t.Errorf("expected 0 trades, got %d", len(result.Trades))
	}
	if book.OrderCount() != 1 {
		t.Errorf("expected 1 order on book, got %d", book.OrderCount())
	}
	if book.BestBid().String() != "100" {
		t.Errorf("expected best bid 100, got %s", book.BestBid().String())
	}
}

func TestAskRestsOnBook(t *testing.T) {
	book := newTestBook()
	order := newLimitOrder("o1", types.SideSell, "105.00", 5)

	book.SubmitOrder(order)

	if book.OrderCount() != 1 {
		t.Errorf("expected 1 order, got %d", book.OrderCount())
	}
	if book.BestAsk().String() != "105" {
		t.Errorf("expected best ask 105, got %s", book.BestAsk().String())
	}
}

// --- Price-Time Priority Matching ---

func TestExactMatch(t *testing.T) {
	book := newTestBook()

	// Place a sell order
	sell := newLimitOrder("sell1", types.SideSell, "100.00", 10)
	book.SubmitOrder(sell)

	// Place a matching buy order
	buy := newLimitOrder("buy1", types.SideBuy, "100.00", 10)
	result := book.SubmitOrder(buy)

	if len(result.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(result.Trades))
	}

	trade := result.Trades[0]
	if trade.Quantity != 10 {
		t.Errorf("expected trade qty 10, got %d", trade.Quantity)
	}
	if trade.Price.String() != "100" {
		t.Errorf("expected trade price 100, got %s", trade.Price.String())
	}
	if trade.AggressorSide != types.SideBuy {
		t.Error("aggressor should be BUY")
	}
	if trade.BuyOrderID != "buy1" {
		t.Errorf("expected buy order ID buy1, got %s", trade.BuyOrderID)
	}
	if trade.SellOrderID != "sell1" {
		t.Errorf("expected sell order ID sell1, got %s", trade.SellOrderID)
	}

	if book.OrderCount() != 0 {
		t.Errorf("expected empty book, got %d orders", book.OrderCount())
	}
}

func TestPartialFill(t *testing.T) {
	book := newTestBook()

	// Sell 10
	sell := newLimitOrder("sell1", types.SideSell, "100.00", 10)
	book.SubmitOrder(sell)

	// Buy 5 (partial fill)
	buy := newLimitOrder("buy1", types.SideBuy, "100.00", 5)
	result := book.SubmitOrder(buy)

	if len(result.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(result.Trades))
	}
	if result.Trades[0].Quantity != 5 {
		t.Errorf("expected trade qty 5, got %d", result.Trades[0].Quantity)
	}

	// Sell order should still be on book with 5 remaining
	if book.OrderCount() != 1 {
		t.Errorf("expected 1 order on book, got %d", book.OrderCount())
	}
	o, ok := book.GetOrder("sell1")
	if !ok {
		t.Fatal("sell1 should still be on book")
	}
	if o.RemainingQty != 5 {
		t.Errorf("expected remaining qty 5, got %d", o.RemainingQty)
	}
	if o.Status != types.OrderStatusPartiallyFilled {
		t.Errorf("expected PARTIALLY_FILLED, got %s", o.Status.String())
	}
}

func TestPricePriority(t *testing.T) {
	book := newTestBook()

	// Two sells at different prices
	sell1 := newLimitOrder("sell1", types.SideSell, "102.00", 10)
	sell2 := newLimitOrder("sell2", types.SideSell, "100.00", 10) // Better price
	book.SubmitOrder(sell1)
	book.SubmitOrder(sell2)

	// Buy should match against sell2 first (lower ask = better for buyer)
	buy := newLimitOrder("buy1", types.SideBuy, "102.00", 5)
	result := book.SubmitOrder(buy)

	if len(result.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(result.Trades))
	}
	if result.Trades[0].SellOrderID != "sell2" {
		t.Errorf("expected match against sell2 (better price), got %s", result.Trades[0].SellOrderID)
	}
	if result.Trades[0].Price.String() != "100" {
		t.Errorf("expected fill at resting price 100, got %s", result.Trades[0].Price.String())
	}
}

func TestTimePriority(t *testing.T) {
	book := newTestBook()

	// Two sells at same price
	sell1 := newLimitOrder("sell1", types.SideSell, "100.00", 10)
	sell2 := newLimitOrder("sell2", types.SideSell, "100.00", 10)
	book.SubmitOrder(sell1) // First (has time priority)
	book.SubmitOrder(sell2) // Second

	// Buy should match against sell1 first (FIFO)
	buy := newLimitOrder("buy1", types.SideBuy, "100.00", 5)
	result := book.SubmitOrder(buy)

	if len(result.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(result.Trades))
	}
	if result.Trades[0].SellOrderID != "sell1" {
		t.Errorf("expected match against sell1 (time priority), got %s", result.Trades[0].SellOrderID)
	}
}

func TestMultipleFillsAcrossLevels(t *testing.T) {
	book := newTestBook()

	// Sell orders at different prices
	sell1 := newLimitOrder("sell1", types.SideSell, "100.00", 5)
	sell2 := newLimitOrder("sell2", types.SideSell, "101.00", 5)
	book.SubmitOrder(sell1)
	book.SubmitOrder(sell2)

	// Buy 8 @ 101 — should fill 5 @ 100 from sell1, then 3 @ 101 from sell2
	buy := newLimitOrder("buy1", types.SideBuy, "101.00", 8)
	result := book.SubmitOrder(buy)

	if len(result.Trades) != 2 {
		t.Fatalf("expected 2 trades, got %d", len(result.Trades))
	}
	if result.Trades[0].Price.String() != "100" || result.Trades[0].Quantity != 5 {
		t.Errorf("first trade: expected 5@100, got %d@%s", result.Trades[0].Quantity, result.Trades[0].Price.String())
	}
	if result.Trades[1].Price.String() != "101" || result.Trades[1].Quantity != 3 {
		t.Errorf("second trade: expected 3@101, got %d@%s", result.Trades[1].Quantity, result.Trades[1].Price.String())
	}
}

// --- Market Orders ---

func TestMarketOrderFullFill(t *testing.T) {
	book := newTestBook()

	sell := newLimitOrder("sell1", types.SideSell, "100.00", 10)
	book.SubmitOrder(sell)

	buy := newMarketOrder("buy1", types.SideBuy, 10)
	result := book.SubmitOrder(buy)

	if len(result.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(result.Trades))
	}
	if result.Trades[0].Quantity != 10 {
		t.Errorf("expected trade qty 10, got %d", result.Trades[0].Quantity)
	}
	if book.OrderCount() != 0 {
		t.Errorf("expected empty book, got %d", book.OrderCount())
	}
}

func TestMarketOrderPartialFillCancelsRemainder(t *testing.T) {
	book := newTestBook()

	sell := newLimitOrder("sell1", types.SideSell, "100.00", 5)
	book.SubmitOrder(sell)

	buy := newMarketOrder("buy1", types.SideBuy, 10)
	result := book.SubmitOrder(buy)

	if len(result.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(result.Trades))
	}
	if result.Trades[0].Quantity != 5 {
		t.Errorf("expected trade qty 5, got %d", result.Trades[0].Quantity)
	}

	// Should have a cancel exec report for the remaining 5
	hasCancelReport := false
	for _, report := range result.ExecutionReports {
		if report.OrderID == "buy1" && report.ExecType == types.ExecTypeCancelled {
			hasCancelReport = true
		}
	}
	if !hasCancelReport {
		t.Error("expected cancel execution report for unfilled market order remainder")
	}
}

func TestMarketOrderNoLiquidity(t *testing.T) {
	book := newTestBook()

	buy := newMarketOrder("buy1", types.SideBuy, 10)
	result := book.SubmitOrder(buy)

	if len(result.Trades) != 0 {
		t.Errorf("expected 0 trades, got %d", len(result.Trades))
	}

	// Market order with no liquidity should be cancelled
	hasCancelReport := false
	for _, report := range result.ExecutionReports {
		if report.OrderID == "buy1" && report.ExecType == types.ExecTypeCancelled {
			hasCancelReport = true
		}
	}
	if !hasCancelReport {
		t.Error("expected cancel for market order with no liquidity")
	}
}

// --- IOC Orders ---

func TestIOCPartialFill(t *testing.T) {
	book := newTestBook()

	sell := newLimitOrder("sell1", types.SideSell, "100.00", 5)
	book.SubmitOrder(sell)

	buy := &types.Order{
		OrderID:      "buy-ioc",
		InstrumentID: "WHT-HRW-2026M07-UB",
		AccountID:    "acc-ioc",
		Side:         types.SideBuy,
		OrderType:    types.OrderTypeLimit,
		TimeInForce:  types.TIFIOC,
		Price:        mustParseDecimal("100.00"),
		Quantity:     10,
	}
	result := book.SubmitOrder(buy)

	if len(result.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(result.Trades))
	}
	if result.Trades[0].Quantity != 5 {
		t.Errorf("expected trade qty 5, got %d", result.Trades[0].Quantity)
	}

	// IOC remainder should be cancelled, not resting
	if book.OrderCount() != 0 {
		t.Errorf("IOC should not rest on book, got %d orders", book.OrderCount())
	}
}

// --- FOK Orders ---

func TestFOKFullFill(t *testing.T) {
	book := newTestBook()

	sell := newLimitOrder("sell1", types.SideSell, "100.00", 10)
	book.SubmitOrder(sell)

	buy := &types.Order{
		OrderID:      "buy-fok",
		InstrumentID: "WHT-HRW-2026M07-UB",
		AccountID:    "acc-fok",
		Side:         types.SideBuy,
		OrderType:    types.OrderTypeLimit,
		TimeInForce:  types.TIFFOK,
		Price:        mustParseDecimal("100.00"),
		Quantity:     10,
	}
	result := book.SubmitOrder(buy)

	if len(result.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(result.Trades))
	}
	if result.Trades[0].Quantity != 10 {
		t.Errorf("expected full fill of 10, got %d", result.Trades[0].Quantity)
	}
}

func TestFOKCancelledIfCannotFill(t *testing.T) {
	book := newTestBook()

	sell := newLimitOrder("sell1", types.SideSell, "100.00", 5)
	book.SubmitOrder(sell)

	buy := &types.Order{
		OrderID:      "buy-fok",
		InstrumentID: "WHT-HRW-2026M07-UB",
		AccountID:    "acc-fok",
		Side:         types.SideBuy,
		OrderType:    types.OrderTypeLimit,
		TimeInForce:  types.TIFFOK,
		Price:        mustParseDecimal("100.00"),
		Quantity:     10, // Only 5 available
	}
	result := book.SubmitOrder(buy)

	if len(result.Trades) != 0 {
		t.Errorf("FOK should not produce trades when can't fill fully, got %d", len(result.Trades))
	}

	// sell1 should still be on the book unchanged
	if book.OrderCount() != 1 {
		t.Errorf("expected sell1 still on book, got %d orders", book.OrderCount())
	}
}

// --- Order Cancellation ---

func TestCancelOrder(t *testing.T) {
	book := newTestBook()

	sell := newLimitOrder("sell1", types.SideSell, "100.00", 10)
	book.SubmitOrder(sell)

	report, err := book.CancelOrder("sell1")
	if err != nil {
		t.Fatalf("CancelOrder error: %v", err)
	}
	if report.ExecType != types.ExecTypeCancelled {
		t.Errorf("expected CANCELLED exec type, got %d", report.ExecType)
	}
	if book.OrderCount() != 0 {
		t.Errorf("expected empty book after cancel, got %d", book.OrderCount())
	}
}

func TestCancelNonExistentOrder(t *testing.T) {
	book := newTestBook()
	_, err := book.CancelOrder("nonexistent")
	if err == nil {
		t.Error("expected error for cancelling nonexistent order")
	}
}

func TestCancelAll(t *testing.T) {
	book := newTestBook()

	o1 := newLimitOrder("o1", types.SideBuy, "99.00", 10)
	o1.AccountID = "acc1"
	o2 := newLimitOrder("o2", types.SideBuy, "98.00", 10)
	o2.AccountID = "acc1"
	o3 := newLimitOrder("o3", types.SideSell, "101.00", 10)
	o3.AccountID = "acc2"

	book.SubmitOrder(o1)
	book.SubmitOrder(o2)
	book.SubmitOrder(o3)

	count, ids := book.CancelAll("acc1", types.SideUnspecified)
	if count != 2 {
		t.Errorf("expected 2 cancellations, got %d", count)
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 IDs, got %d", len(ids))
	}
	if book.OrderCount() != 1 {
		t.Errorf("expected 1 remaining order, got %d", book.OrderCount())
	}
}

// --- Modify Order (Cancel-Replace) ---

func TestModifyOrder(t *testing.T) {
	book := newTestBook()

	sell := newLimitOrder("sell1", types.SideSell, "100.00", 10)
	sell.AccountID = "acc1"
	book.SubmitOrder(sell)

	result, err := book.ModifyOrder("sell1", "acc1", mustParseDecimal("99.00"), 0)
	if err != nil {
		t.Fatalf("ModifyOrder error: %v", err)
	}

	// Should have a cancel report for the original + new/accept for the replacement
	hasCancelReport := false
	hasNewReport := false
	for _, r := range result.ExecutionReports {
		if r.OrderID == "sell1" && r.ExecType == types.ExecTypeCancelled {
			hasCancelReport = true
		}
		if r.ExecType == types.ExecTypeNew {
			hasNewReport = true
		}
	}
	if !hasCancelReport {
		t.Error("expected cancel report for original order")
	}
	if !hasNewReport {
		t.Error("expected new report for replacement order")
	}

	// Book should have 1 order at the new price
	if book.OrderCount() != 1 {
		t.Errorf("expected 1 order after modify, got %d", book.OrderCount())
	}
	if book.BestAsk().String() != "99" {
		t.Errorf("expected best ask at 99, got %s", book.BestAsk().String())
	}
}

// --- Self-Trade Prevention ---

func TestSTPCancelNewest(t *testing.T) {
	book := newTestBook()

	sell := newLimitOrder("sell1", types.SideSell, "100.00", 10)
	sell.AccountID = "same-account"
	book.SubmitOrder(sell)

	buy := newLimitOrder("buy1", types.SideBuy, "100.00", 10)
	buy.AccountID = "same-account"
	buy.STPMode = types.STPModeCancelNewest
	result := book.SubmitOrder(buy)

	if len(result.Trades) != 0 {
		t.Errorf("STP should prevent trade, got %d trades", len(result.Trades))
	}
	// Sell should still be on book
	if book.OrderCount() != 1 {
		t.Errorf("expected sell still on book, got %d", book.OrderCount())
	}
}

func TestSTPCancelOldest(t *testing.T) {
	book := newTestBook()

	sell := newLimitOrder("sell1", types.SideSell, "100.00", 10)
	sell.AccountID = "same-account"
	book.SubmitOrder(sell)

	buy := newLimitOrder("buy1", types.SideBuy, "100.00", 10)
	buy.AccountID = "same-account"
	buy.STPMode = types.STPModeCancelOldest
	result := book.SubmitOrder(buy)

	if len(result.Trades) != 0 {
		t.Errorf("STP should prevent trade, got %d trades", len(result.Trades))
	}
	// Buy should rest on book (oldest was cancelled)
	if book.OrderCount() != 1 {
		t.Errorf("expected buy on book, got %d", book.OrderCount())
	}
}

func TestSTPCancelBoth(t *testing.T) {
	book := newTestBook()

	sell := newLimitOrder("sell1", types.SideSell, "100.00", 10)
	sell.AccountID = "same-account"
	book.SubmitOrder(sell)

	buy := newLimitOrder("buy1", types.SideBuy, "100.00", 10)
	buy.AccountID = "same-account"
	buy.STPMode = types.STPModeCancelBoth
	result := book.SubmitOrder(buy)

	if len(result.Trades) != 0 {
		t.Errorf("STP should prevent trade, got %d trades", len(result.Trades))
	}
	if book.OrderCount() != 0 {
		t.Errorf("expected empty book, got %d", book.OrderCount())
	}
}

// --- Validation ---

func TestRejectLimitOrderWithoutPrice(t *testing.T) {
	book := newTestBook()
	order := &types.Order{
		OrderID:      "bad1",
		InstrumentID: "WHT-HRW-2026M07-UB",
		AccountID:    "acc1",
		Side:         types.SideBuy,
		OrderType:    types.OrderTypeLimit,
		TimeInForce:  types.TIFDay,
		Quantity:     10,
		// Price is zero
	}
	result := book.SubmitOrder(order)

	if len(result.Trades) != 0 {
		t.Error("rejected order should produce no trades")
	}
	hasReject := false
	for _, r := range result.ExecutionReports {
		if r.ExecType == types.ExecTypeRejected {
			hasReject = true
		}
	}
	if !hasReject {
		t.Error("expected rejection exec report")
	}
}

func TestRejectZeroQuantity(t *testing.T) {
	book := newTestBook()
	order := newLimitOrder("bad1", types.SideBuy, "100.00", 0)
	result := book.SubmitOrder(order)

	hasReject := false
	for _, r := range result.ExecutionReports {
		if r.ExecType == types.ExecTypeRejected {
			hasReject = true
		}
	}
	if !hasReject {
		t.Error("expected rejection for zero quantity")
	}
}

func TestRejectInHaltedState(t *testing.T) {
	book := newTestBook()
	book.State = types.BookStateHalted

	order := newLimitOrder("o1", types.SideBuy, "100.00", 10)
	result := book.SubmitOrder(order)

	hasReject := false
	for _, r := range result.ExecutionReports {
		if r.ExecType == types.ExecTypeRejected {
			hasReject = true
		}
	}
	if !hasReject {
		t.Error("expected rejection when book is halted")
	}
}

// --- Execution Reports ---

func TestExecutionReportsForFill(t *testing.T) {
	book := newTestBook()

	sell := newLimitOrder("sell1", types.SideSell, "100.00", 10)
	book.SubmitOrder(sell)

	buy := newLimitOrder("buy1", types.SideBuy, "100.00", 10)
	result := book.SubmitOrder(buy)

	// Expected reports: NEW(buy), FILL(buy), FILL(sell)
	if len(result.ExecutionReports) < 3 {
		t.Fatalf("expected at least 3 exec reports, got %d", len(result.ExecutionReports))
	}

	// Check NEW report for incoming
	if result.ExecutionReports[0].ExecType != types.ExecTypeNew {
		t.Errorf("first report should be NEW, got %d", result.ExecutionReports[0].ExecType)
	}

	// Check fill reports
	fillReports := 0
	for _, r := range result.ExecutionReports {
		if r.ExecType == types.ExecTypeFill {
			fillReports++
			if r.LastQty != 10 {
				t.Errorf("expected fill qty 10, got %d", r.LastQty)
			}
		}
	}
	if fillReports != 2 {
		t.Errorf("expected 2 fill reports (one per side), got %d", fillReports)
	}
}

// --- Price Improvement ---

func TestPriceImprovement(t *testing.T) {
	book := newTestBook()

	// Sell at 99
	sell := newLimitOrder("sell1", types.SideSell, "99.00", 10)
	book.SubmitOrder(sell)

	// Buy at 100 — should get price improvement (fill at 99)
	buy := newLimitOrder("buy1", types.SideBuy, "100.00", 10)
	result := book.SubmitOrder(buy)

	if len(result.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(result.Trades))
	}
	if result.Trades[0].Price.String() != "99" {
		t.Errorf("expected fill at resting price 99, got %s", result.Trades[0].Price.String())
	}
}

// --- Book State After Operations ---

func TestBookLevelsAfterMatchExhausted(t *testing.T) {
	book := newTestBook()

	sell1 := newLimitOrder("sell1", types.SideSell, "100.00", 5)
	sell2 := newLimitOrder("sell2", types.SideSell, "101.00", 5)
	book.SubmitOrder(sell1)
	book.SubmitOrder(sell2)

	// Buy all at level 100
	buy := newLimitOrder("buy1", types.SideBuy, "100.00", 5)
	book.SubmitOrder(buy)

	// Level 100 should be gone, only 101 remains
	if len(book.AskLevels()) != 1 {
		t.Errorf("expected 1 ask level, got %d", len(book.AskLevels()))
	}
	if book.BestAsk().String() != "101" {
		t.Errorf("expected best ask 101, got %s", book.BestAsk().String())
	}
}

func TestNoMatchWhenPricesDoNotCross(t *testing.T) {
	book := newTestBook()

	sell := newLimitOrder("sell1", types.SideSell, "101.00", 10)
	book.SubmitOrder(sell)

	// Buy at 100 < ask at 101 → no match
	buy := newLimitOrder("buy1", types.SideBuy, "100.00", 10)
	result := book.SubmitOrder(buy)

	if len(result.Trades) != 0 {
		t.Errorf("expected no trades, got %d", len(result.Trades))
	}
	if book.OrderCount() != 2 {
		t.Errorf("expected 2 orders on book, got %d", book.OrderCount())
	}
}

// --- Last Trade Price ---

func TestLastTradePriceUpdated(t *testing.T) {
	book := newTestBook()

	sell := newLimitOrder("sell1", types.SideSell, "100.50", 10)
	book.SubmitOrder(sell)

	buy := newLimitOrder("buy1", types.SideBuy, "100.50", 5)
	book.SubmitOrder(buy)

	if book.LastTradePrice.String() != "100.5" {
		t.Errorf("expected last trade price 100.5, got %s", book.LastTradePrice.String())
	}
}

// --- Trade Value ---

func TestTradeValueCalculation(t *testing.T) {
	book := newTestBook()

	sell := newLimitOrder("sell1", types.SideSell, "100.00", 10)
	book.SubmitOrder(sell)

	buy := newLimitOrder("buy1", types.SideBuy, "100.00", 10)
	result := book.SubmitOrder(buy)

	if len(result.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(result.Trades))
	}
	// 100.0000 * 10 = 1000.0000 internally, displayed as "1000"
	if result.Trades[0].TradeValue.String() != "1000" {
		t.Errorf("expected trade value 1000, got %s", result.Trades[0].TradeValue.String())
	}
}

// --- Sequence Numbers ---

func TestTradeSequenceNumbers(t *testing.T) {
	book := newTestBook()

	sell1 := newLimitOrder("sell1", types.SideSell, "100.00", 10)
	sell2 := newLimitOrder("sell2", types.SideSell, "101.00", 10)
	book.SubmitOrder(sell1)
	book.SubmitOrder(sell2)

	buy := newLimitOrder("buy1", types.SideBuy, "101.00", 15)
	result := book.SubmitOrder(buy)

	if len(result.Trades) != 2 {
		t.Fatalf("expected 2 trades, got %d", len(result.Trades))
	}
	if result.Trades[0].SequenceNumber >= result.Trades[1].SequenceNumber {
		t.Error("trade sequence numbers should be strictly increasing")
	}
}
