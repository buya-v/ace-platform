package orderbook

import (
	"fmt"
	"testing"
	"time"

	"github.com/garudax-platform/matching-engine/internal/types"
)

func newTestAuctionEngine() *AuctionEngine {
	var seq uint64
	return NewAuctionEngine(&testIDGen{}, &seq)
}

// buildAuctionBook creates bid and ask price levels from order specs.
// Each spec is {price, qty, sequenceNumber}.
func buildAuctionLevels(t *testing.T, orders []struct {
	side  types.Side
	price string
	qty   uint64
	seq   uint64
}) ([]*PriceLevel, []*PriceLevel) {
	t.Helper()
	bidLevels := make(map[int64]*PriceLevel)
	askLevels := make(map[int64]*PriceLevel)

	for i, spec := range orders {
		price := mustParseDecimal(spec.price)
		order := &types.Order{
			OrderID:        fmt.Sprintf("o%d", i+1),
			InstrumentID:   "WHT-HRW-2026M07-UB",
			AccountID:      fmt.Sprintf("acct-%d", i+1),
			Side:           spec.side,
			OrderType:      types.OrderTypeLimit,
			Price:          price,
			Quantity:       spec.qty,
			RemainingQty:   spec.qty,
			Status:         types.OrderStatusNew,
			SequenceNumber: spec.seq,
		}

		raw := price.Raw()
		if spec.side == types.SideBuy {
			if _, ok := bidLevels[raw]; !ok {
				bidLevels[raw] = NewPriceLevel(price)
			}
			bidLevels[raw].Enqueue(order)
		} else {
			if _, ok := askLevels[raw]; !ok {
				askLevels[raw] = NewPriceLevel(price)
			}
			askLevels[raw].Enqueue(order)
		}
	}

	// Sort: bids descending, asks ascending
	bids := make([]*PriceLevel, 0, len(bidLevels))
	for _, l := range bidLevels {
		bids = append(bids, l)
	}
	sortLevels(bids, true)

	asks := make([]*PriceLevel, 0, len(askLevels))
	for _, l := range askLevels {
		asks = append(asks, l)
	}
	sortLevels(asks, false)

	return bids, asks
}

func sortLevels(levels []*PriceLevel, descending bool) {
	for i := 0; i < len(levels); i++ {
		for j := i + 1; j < len(levels); j++ {
			swap := false
			if descending {
				swap = levels[j].Price.GreaterThan(levels[i].Price)
			} else {
				swap = levels[j].Price.LessThan(levels[i].Price)
			}
			if swap {
				levels[i], levels[j] = levels[j], levels[i]
			}
		}
	}
}

// --- Test: Single price equilibrium ---

func TestAuctionSingleEquilibriumPrice(t *testing.T) {
	ae := newTestAuctionEngine()

	// Bids: 100@10, Asks: 100@10 — single crossing price at 100
	bids, asks := buildAuctionLevels(t, []struct {
		side  types.Side
		price string
		qty   uint64
		seq   uint64
	}{
		{types.SideBuy, "100.00", 10, 1},
		{types.SideSell, "100.00", 10, 2},
	})

	ref := mustParseDecimal("99.00")
	result := ae.RunAuction("WHT-HRW-2026M07-UB", bids, asks, ref)

	if result.EquilibriumPrice.String() != "100" {
		t.Errorf("expected equilibrium price 100, got %s", result.EquilibriumPrice.String())
	}
	if len(result.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(result.Trades))
	}
	if result.Trades[0].Quantity != 10 {
		t.Errorf("expected trade qty 10, got %d", result.Trades[0].Quantity)
	}
	if result.Trades[0].TradeType != types.TradeTypeAuction {
		t.Errorf("expected TradeTypeAuction, got %d", result.Trades[0].TradeType)
	}
}

// --- Test: Multiple crossing prices — max volume selection ---

func TestAuctionMultipleCrossingPricesMaxVolume(t *testing.T) {
	ae := newTestAuctionEngine()

	// Bids: 102@5, 101@10, 100@5
	// Asks: 99@3, 100@7, 101@10
	// At price 100: bid vol = 5+10+5=20, ask vol = 3+7=10, matchable = 10
	// At price 101: bid vol = 5+10=15, ask vol = 3+7+10=20, matchable = 15
	// At price 99:  bid vol = 20, ask vol = 3, matchable = 3
	// At price 102: bid vol = 5, ask vol = 20, matchable = 5
	// Max volume = 15 at price 101
	bids, asks := buildAuctionLevels(t, []struct {
		side  types.Side
		price string
		qty   uint64
		seq   uint64
	}{
		{types.SideBuy, "102.00", 5, 1},
		{types.SideBuy, "101.00", 10, 2},
		{types.SideBuy, "100.00", 5, 3},
		{types.SideSell, "99.00", 3, 4},
		{types.SideSell, "100.00", 7, 5},
		{types.SideSell, "101.00", 10, 6},
	})

	ref := mustParseDecimal("100.00")
	result := ae.RunAuction("WHT-HRW-2026M07-UB", bids, asks, ref)

	if result.EquilibriumPrice.String() != "101" {
		t.Errorf("expected equilibrium price 101, got %s", result.EquilibriumPrice.String())
	}

	// Total matched volume should be 15
	var totalQty uint64
	for _, trade := range result.Trades {
		totalQty += trade.Quantity
	}
	if totalQty != 15 {
		t.Errorf("expected total matched volume 15, got %d", totalQty)
	}
}

// --- Test: Reference price tiebreaker ---

func TestAuctionReferencePriceTiebreaker(t *testing.T) {
	ae := newTestAuctionEngine()

	// Bids: 102@10, Asks: 98@10
	// Both prices 99, 100, 101, 102 yield matchable volume = 10
	// But candidate prices are only those that exist in orders: 98 and 102
	// At 98:  bid vol = 10 (102 >= 98), ask vol = 10 (98 <= 98), matchable = 10
	// At 102: bid vol = 10 (102 >= 102), ask vol = 10 (98 <= 102), matchable = 10
	// With reference 99: dist(98,99) = 1, dist(102,99) = 3
	// Equilibrium = 98 (closest to reference)
	bids, asks := buildAuctionLevels(t, []struct {
		side  types.Side
		price string
		qty   uint64
		seq   uint64
	}{
		{types.SideBuy, "102.00", 10, 1},
		{types.SideSell, "98.00", 10, 2},
	})

	ref := mustParseDecimal("99.00")
	result := ae.RunAuction("WHT-HRW-2026M07-UB", bids, asks, ref)

	if result.EquilibriumPrice.String() != "98" {
		t.Errorf("expected equilibrium price 98 (closest to ref 99), got %s", result.EquilibriumPrice.String())
	}
}

// --- Test: Reference price equidistant — higher price wins ---

func TestAuctionEquidistantHigherPriceWins(t *testing.T) {
	ae := newTestAuctionEngine()

	// Bids: 102@10, 98@10. Asks: 98@5, 102@5
	// At 98: bid vol=20, ask vol=5, matchable=5
	// At 102: bid vol=10, ask vol=10, matchable=10
	// Max vol is 10 at 102 — no tiebreaker needed here.
	// Let's set up a case where equidistant matters:
	// Bids: 101@10, Asks: 99@10
	// At 99:  bid vol=10, ask vol=10, matchable=10
	// At 101: bid vol=10, ask vol=10, matchable=10
	// Reference = 100 → dist(99)=1, dist(101)=1 → tied → higher price 101 wins
	bids, asks := buildAuctionLevels(t, []struct {
		side  types.Side
		price string
		qty   uint64
		seq   uint64
	}{
		{types.SideBuy, "101.00", 10, 1},
		{types.SideSell, "99.00", 10, 2},
	})

	ref := mustParseDecimal("100.00")
	result := ae.RunAuction("WHT-HRW-2026M07-UB", bids, asks, ref)

	if result.EquilibriumPrice.String() != "101" {
		t.Errorf("expected equilibrium price 101 (higher wins on tie), got %s", result.EquilibriumPrice.String())
	}
}

// --- Test: Partial fills at equilibrium ---

func TestAuctionPartialFillsAtEquilibrium(t *testing.T) {
	ae := newTestAuctionEngine()

	// Bids: 100@20 (seq 1), Asks: 100@15 (seq 2)
	// Matchable volume = 15. Bid has 5 remaining.
	bids, asks := buildAuctionLevels(t, []struct {
		side  types.Side
		price string
		qty   uint64
		seq   uint64
	}{
		{types.SideBuy, "100.00", 20, 1},
		{types.SideSell, "100.00", 15, 2},
	})

	ref := mustParseDecimal("100.00")
	result := ae.RunAuction("WHT-HRW-2026M07-UB", bids, asks, ref)

	if len(result.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(result.Trades))
	}
	if result.Trades[0].Quantity != 15 {
		t.Errorf("expected trade qty 15, got %d", result.Trades[0].Quantity)
	}

	// Check execution reports: bid should be partial fill, ask should be full fill
	var bidReport, askReport *types.ExecutionReport
	for i := range result.ExecutionReports {
		r := &result.ExecutionReports[i]
		if r.Side == types.SideBuy {
			bidReport = r
		} else {
			askReport = r
		}
	}
	if bidReport == nil || askReport == nil {
		t.Fatal("expected execution reports for both sides")
	}
	if bidReport.ExecType != types.ExecTypePartialFill {
		t.Errorf("expected bid partial fill, got %d", bidReport.ExecType)
	}
	if bidReport.LeavesQty != 5 {
		t.Errorf("expected bid leaves qty 5, got %d", bidReport.LeavesQty)
	}
	if askReport.ExecType != types.ExecTypeFill {
		t.Errorf("expected ask full fill, got %d", askReport.ExecType)
	}
}

// --- Test: Empty auction — no crossing ---

func TestAuctionNoCrossing(t *testing.T) {
	ae := newTestAuctionEngine()

	// Bids: 98@10, Asks: 102@10 — best bid < best ask → no crossing
	bids, asks := buildAuctionLevels(t, []struct {
		side  types.Side
		price string
		qty   uint64
		seq   uint64
	}{
		{types.SideBuy, "98.00", 10, 1},
		{types.SideSell, "102.00", 10, 2},
	})

	ref := mustParseDecimal("100.00")
	result := ae.RunAuction("WHT-HRW-2026M07-UB", bids, asks, ref)

	if len(result.Trades) != 0 {
		t.Errorf("expected 0 trades for non-crossing book, got %d", len(result.Trades))
	}
	if !result.EquilibriumPrice.IsZero() {
		t.Errorf("expected zero equilibrium price, got %s", result.EquilibriumPrice.String())
	}
}

// --- Test: Empty sides ---

func TestAuctionEmptyBids(t *testing.T) {
	ae := newTestAuctionEngine()
	asks := []*PriceLevel{NewPriceLevel(mustParseDecimal("100.00"))}
	ref := mustParseDecimal("100.00")

	result := ae.RunAuction("WHT-HRW-2026M07-UB", nil, asks, ref)
	if len(result.Trades) != 0 {
		t.Errorf("expected 0 trades with no bids, got %d", len(result.Trades))
	}
}

func TestAuctionEmptyAsks(t *testing.T) {
	ae := newTestAuctionEngine()
	bids := []*PriceLevel{NewPriceLevel(mustParseDecimal("100.00"))}
	ref := mustParseDecimal("100.00")

	result := ae.RunAuction("WHT-HRW-2026M07-UB", bids, nil, ref)
	if len(result.Trades) != 0 {
		t.Errorf("expected 0 trades with no asks, got %d", len(result.Trades))
	}
}

// --- Test: Time priority in auction fills ---

func TestAuctionTimePriorityFills(t *testing.T) {
	ae := newTestAuctionEngine()

	// Two bids at same price, different sequence numbers.
	// The earlier bid (lower seq) should be filled first.
	bids, asks := buildAuctionLevels(t, []struct {
		side  types.Side
		price string
		qty   uint64
		seq   uint64
	}{
		{types.SideBuy, "100.00", 5, 1},  // first bid
		{types.SideBuy, "100.00", 5, 3},  // second bid
		{types.SideSell, "100.00", 7, 2}, // ask fills 7 of 10
	})

	ref := mustParseDecimal("100.00")
	result := ae.RunAuction("WHT-HRW-2026M07-UB", bids, asks, ref)

	// Should produce 2 trades: first bid fills 5, second bid fills 2
	if len(result.Trades) != 2 {
		t.Fatalf("expected 2 trades (time priority), got %d", len(result.Trades))
	}
	if result.Trades[0].Quantity != 5 {
		t.Errorf("expected first trade qty 5, got %d", result.Trades[0].Quantity)
	}
	if result.Trades[1].Quantity != 2 {
		t.Errorf("expected second trade qty 2, got %d", result.Trades[1].Quantity)
	}
}

// --- Test: All trades use equilibrium price, not individual order prices ---

func TestAuctionAllTradesAtEquilibriumPrice(t *testing.T) {
	ae := newTestAuctionEngine()

	// Bids at 102 and 101, asks at 99 and 100.
	// Equilibrium should be computed, and ALL trades should be at that price.
	bids, asks := buildAuctionLevels(t, []struct {
		side  types.Side
		price string
		qty   uint64
		seq   uint64
	}{
		{types.SideBuy, "102.00", 5, 1},
		{types.SideBuy, "101.00", 5, 2},
		{types.SideSell, "99.00", 3, 3},
		{types.SideSell, "100.00", 4, 4},
	})

	ref := mustParseDecimal("100.00")
	result := ae.RunAuction("WHT-HRW-2026M07-UB", bids, asks, ref)

	for i, trade := range result.Trades {
		if !trade.Price.Equal(result.EquilibriumPrice) {
			t.Errorf("trade %d price %s != equilibrium %s", i, trade.Price.String(), result.EquilibriumPrice.String())
		}
	}
}

// --- Test: Phase transitions ---

func TestPhaseTransitionValidSequence(t *testing.T) {
	ae := newTestAuctionEngine()
	pm := NewPhaseManager(ae)

	if pm.CurrentPhase() != PhasePreOpen {
		t.Fatalf("expected initial phase PRE_OPEN, got %s", pm.CurrentPhase())
	}

	book := newTestBook()
	book.State = types.BookStatePreOpen

	// PRE_OPEN -> OPENING_AUCTION
	result, err := pm.TransitionTo(PhaseOpeningAuction, book)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.NewPhase != PhaseOpeningAuction {
		t.Errorf("expected OPENING_AUCTION, got %s", result.NewPhase)
	}
	if book.State != types.BookStateAuction {
		t.Errorf("expected book state AUCTION, got %d", book.State)
	}

	// OPENING_AUCTION -> CONTINUOUS
	result, err = pm.TransitionTo(PhaseContinuous, book)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if book.State != types.BookStateContinuous {
		t.Errorf("expected book state CONTINUOUS, got %d", book.State)
	}

	// CONTINUOUS -> CLOSING_AUCTION
	result, err = pm.TransitionTo(PhaseClosingAuction, book)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if book.State != types.BookStateAuction {
		t.Errorf("expected book state AUCTION, got %d", book.State)
	}

	// CLOSING_AUCTION -> POST_CLOSE
	result, err = pm.TransitionTo(PhasePostClose, book)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if book.State != types.BookStateClosed {
		t.Errorf("expected book state CLOSED, got %d", book.State)
	}
	_ = result
}

func TestPhaseTransitionInvalidSequence(t *testing.T) {
	ae := newTestAuctionEngine()
	pm := NewPhaseManager(ae)
	book := newTestBook()

	// PRE_OPEN -> CONTINUOUS should fail (must go through OPENING_AUCTION)
	_, err := pm.TransitionTo(PhaseContinuous, book)
	if err == nil {
		t.Error("expected error for invalid transition PRE_OPEN -> CONTINUOUS")
	}

	// PRE_OPEN -> POST_CLOSE should fail
	_, err = pm.TransitionTo(PhasePostClose, book)
	if err == nil {
		t.Error("expected error for invalid transition PRE_OPEN -> POST_CLOSE")
	}
}

// --- Test: Auction uncrossing on phase transition ---

func TestPhaseTransitionTriggersAuctionUncrossing(t *testing.T) {
	ae := newTestAuctionEngine()
	pm := NewPhaseManager(ae)
	book := newTestBook()
	book.State = types.BookStatePreOpen

	// Transition to OPENING_AUCTION
	_, err := pm.TransitionTo(PhaseOpeningAuction, book)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Add crossing orders to the book during auction
	book.State = types.BookStateAuction
	buy := newLimitOrder("b1", types.SideBuy, "100.00", 10)
	book.SubmitOrder(buy)
	sell := newLimitOrder("s1", types.SideSell, "100.00", 10)
	book.SubmitOrder(sell)

	// Transition to CONTINUOUS — should trigger uncrossing
	result, err := pm.TransitionTo(PhaseContinuous, book)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.AuctionResult == nil {
		t.Fatal("expected auction result on transition from OPENING_AUCTION")
	}
	if len(result.AuctionResult.Trades) == 0 {
		t.Error("expected trades from auction uncrossing")
	}
	if result.AuctionResult.EquilibriumPrice.String() != "100" {
		t.Errorf("expected equilibrium 100, got %s", result.AuctionResult.EquilibriumPrice.String())
	}
}

// --- Test: Phase controls ---

func TestPhaseCanSubmitOrder(t *testing.T) {
	ae := newTestAuctionEngine()
	pm := NewPhaseManager(ae)

	// PRE_OPEN: can submit
	if !pm.CanSubmitOrder() {
		t.Error("PRE_OPEN should accept orders")
	}

	book := newTestBook()
	pm.TransitionTo(PhaseOpeningAuction, book)
	if !pm.CanSubmitOrder() {
		t.Error("OPENING_AUCTION should accept orders")
	}

	pm.TransitionTo(PhaseContinuous, book)
	if !pm.CanSubmitOrder() {
		t.Error("CONTINUOUS should accept orders")
	}

	pm.TransitionTo(PhaseClosingAuction, book)
	if !pm.CanSubmitOrder() {
		t.Error("CLOSING_AUCTION should accept orders")
	}

	pm.TransitionTo(PhasePostClose, book)
	if pm.CanSubmitOrder() {
		t.Error("POST_CLOSE should NOT accept orders")
	}
}

func TestPhaseCanCancelOrder(t *testing.T) {
	ae := newTestAuctionEngine()
	pm := NewPhaseManager(ae)

	// All phases should allow cancellation
	book := newTestBook()
	if !pm.CanCancelOrder() {
		t.Error("PRE_OPEN should accept cancellations")
	}
	pm.TransitionTo(PhaseOpeningAuction, book)
	if !pm.CanCancelOrder() {
		t.Error("OPENING_AUCTION should accept cancellations")
	}
	pm.TransitionTo(PhaseContinuous, book)
	if !pm.CanCancelOrder() {
		t.Error("CONTINUOUS should accept cancellations")
	}
	pm.TransitionTo(PhaseClosingAuction, book)
	if !pm.CanCancelOrder() {
		t.Error("CLOSING_AUCTION should accept cancellations")
	}
	pm.TransitionTo(PhasePostClose, book)
	if !pm.CanCancelOrder() {
		t.Error("POST_CLOSE should accept cancellations")
	}
}

func TestPhaseShouldMatch(t *testing.T) {
	ae := newTestAuctionEngine()
	pm := NewPhaseManager(ae)

	if pm.ShouldMatch() {
		t.Error("PRE_OPEN should not match")
	}

	book := newTestBook()
	pm.TransitionTo(PhaseOpeningAuction, book)
	if pm.ShouldMatch() {
		t.Error("OPENING_AUCTION should not match")
	}

	pm.TransitionTo(PhaseContinuous, book)
	if !pm.ShouldMatch() {
		t.Error("CONTINUOUS should match")
	}

	pm.TransitionTo(PhaseClosingAuction, book)
	if pm.ShouldMatch() {
		t.Error("CLOSING_AUCTION should not match")
	}
}

// --- Test: Scheduled transitions ---

func TestScheduledTransitions(t *testing.T) {
	ae := newTestAuctionEngine()
	pm := NewPhaseManager(ae)

	baseTime := time.Date(2026, 3, 31, 8, 0, 0, 0, time.UTC)

	pm.SetSchedule([]PhaseScheduleEntry{
		{Phase: PhaseOpeningAuction, At: baseTime.Add(30 * time.Minute)},
		{Phase: PhaseContinuous, At: baseTime.Add(1 * time.Hour)},
		{Phase: PhaseClosingAuction, At: baseTime.Add(4 * time.Hour)},
		{Phase: PhasePostClose, At: baseTime.Add(4*time.Hour + 15*time.Minute)},
	})

	// Before any scheduled transition
	phase := pm.CheckScheduledTransitions(baseTime)
	if phase != MarketPhase(-1) {
		t.Errorf("expected no transition before schedule, got %s", phase)
	}

	// At opening auction time
	phase = pm.CheckScheduledTransitions(baseTime.Add(30 * time.Minute))
	if phase != PhaseOpeningAuction {
		t.Errorf("expected OPENING_AUCTION at 8:30, got %d", phase)
	}

	// After continuous trading starts (at 9:15, both 8:30 and 9:00 have passed)
	phase = pm.CheckScheduledTransitions(baseTime.Add(75 * time.Minute))
	if phase != PhaseContinuous {
		t.Errorf("expected CONTINUOUS at 9:15, got %d", phase)
	}
}

// --- Test: MarketPhase string representation ---

func TestMarketPhaseString(t *testing.T) {
	tests := []struct {
		phase    MarketPhase
		expected string
	}{
		{PhasePreOpen, "PRE_OPEN"},
		{PhaseOpeningAuction, "OPENING_AUCTION"},
		{PhaseContinuous, "CONTINUOUS"},
		{PhaseClosingAuction, "CLOSING_AUCTION"},
		{PhasePostClose, "POST_CLOSE"},
		{MarketPhase(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		if tt.phase.String() != tt.expected {
			t.Errorf("phase %d: expected %s, got %s", tt.phase, tt.expected, tt.phase.String())
		}
	}
}
