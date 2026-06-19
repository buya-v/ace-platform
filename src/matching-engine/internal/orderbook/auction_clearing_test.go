package orderbook

import (
	"fmt"
	"testing"

	"github.com/garudax-platform/matching-engine/internal/types"
)

// This file contains comprehensive tests for the call-auction clearing-price
// algorithm used by the opening and closing auctions of the MSE equities venue.
//
// The clearing-price (a.k.a. uncrossing / equilibrium) algorithm must:
//  1. Maximize matched (executable) volume across all candidate prices.
//  2. Break ties between equal-volume prices by proximity to the reference
//     price, then by the higher price.
//  3. Execute every fill at the single equilibrium price (no price
//     discrimination), in strict time priority.
//  4. Conserve volume: total bought == total sold == sum of trade quantities.
//  5. Leave unmatched residual quantity on the book (partial fills).
//
// The instrument symbol follows the MSE equities convention.
const clrInstrument = "MSE-EQ-GOBI"

// clearingSpec describes one resting order for a clearing-price scenario.
type clearingSpec struct {
	side types.Side
	px   string
	qty  uint64
	seq  uint64
}

// buildClearingLevels builds sorted bid/ask price levels (bids descending,
// asks ascending) from a list of order specs. RemainingQty is initialized to
// the full quantity, mirroring an order resting on the book during an auction
// call phase (no continuous matching has occurred yet).
func buildClearingLevels(t *testing.T, specs []clearingSpec) ([]*PriceLevel, []*PriceLevel) {
	t.Helper()
	bidLevels := make(map[int64]*PriceLevel)
	askLevels := make(map[int64]*PriceLevel)

	for i, s := range specs {
		price := mustParseDecimal(s.px)
		ord := &types.Order{
			OrderID:        fmt.Sprintf("ord-%d", i+1),
			InstrumentID:   clrInstrument,
			AccountID:      fmt.Sprintf("acct-%d", i+1),
			ParticipantID:  fmt.Sprintf("pp-%d", i+1),
			Side:           s.side,
			OrderType:      types.OrderTypeLimit,
			Price:          price,
			Quantity:       s.qty,
			RemainingQty:   s.qty,
			Status:         types.OrderStatusNew,
			SequenceNumber: s.seq,
		}
		raw := price.Raw()
		if s.side == types.SideBuy {
			if _, ok := bidLevels[raw]; !ok {
				bidLevels[raw] = NewPriceLevel(price)
			}
			bidLevels[raw].Enqueue(ord)
		} else {
			if _, ok := askLevels[raw]; !ok {
				askLevels[raw] = NewPriceLevel(price)
			}
			askLevels[raw].Enqueue(ord)
		}
	}

	bids := make([]*PriceLevel, 0, len(bidLevels))
	for _, l := range bidLevels {
		bids = append(bids, l)
	}
	sortLevels(bids, true) // descending

	asks := make([]*PriceLevel, 0, len(askLevels))
	for _, l := range askLevels {
		asks = append(asks, l)
	}
	sortLevels(asks, false) // ascending

	return bids, asks
}

// totalTradeQty sums the quantities of all trades.
func totalTradeQty(trades []types.Trade) uint64 {
	var total uint64
	for _, tr := range trades {
		total += tr.Quantity
	}
	return total
}

// --- Clearing price: volume maximization with a deep book ---

func TestClearingPriceMaximizesVolume(t *testing.T) {
	ae := newTestAuctionEngine()

	// Bids: 105@2, 103@5, 101@3   Asks: 100@4, 102@6, 104@2
	// Cumulative bid volume (price >= p) and ask volume (price <= p):
	//   p=100: bid=10 ask=4  -> match 4
	//   p=101: bid=10 ask=4  -> match 4
	//   p=102: bid=7  ask=10 -> match 7   <-- max
	//   p=103: bid=7  ask=10 -> match 7   <-- max
	//   p=104: bid=2  ask=12 -> match 2
	//   p=105: bid=2  ask=12 -> match 2
	// Max executable volume = 7 at {102, 103}.
	bids, asks := buildClearingLevels(t, []clearingSpec{
		{types.SideBuy, "105.00", 2, 1},
		{types.SideBuy, "103.00", 5, 2},
		{types.SideBuy, "101.00", 3, 3},
		{types.SideSell, "100.00", 4, 4},
		{types.SideSell, "102.00", 6, 5},
		{types.SideSell, "104.00", 2, 6},
	})

	// Reference exactly at 102 -> equilibrium must be 102.
	result := ae.RunAuction(clrInstrument, bids, asks, mustParseDecimal("102.00"))

	if result.EquilibriumPrice.String() != "102" {
		t.Errorf("expected equilibrium 102, got %s", result.EquilibriumPrice.String())
	}
	if got := totalTradeQty(result.Trades); got != 7 {
		t.Errorf("expected total matched volume 7, got %d", got)
	}
}

// The same book should resolve the 102/103 tie toward 103 when the reference
// price favors the higher candidate. This isolates the reference-price
// tiebreaker from the volume-maximization step.
func TestClearingPriceTieResolvedTowardReference(t *testing.T) {
	ae := newTestAuctionEngine()

	bids, asks := buildClearingLevels(t, []clearingSpec{
		{types.SideBuy, "105.00", 2, 1},
		{types.SideBuy, "103.00", 5, 2},
		{types.SideBuy, "101.00", 3, 3},
		{types.SideSell, "100.00", 4, 4},
		{types.SideSell, "102.00", 6, 5},
		{types.SideSell, "104.00", 2, 6},
	})

	result := ae.RunAuction(clrInstrument, bids, asks, mustParseDecimal("103.50"))
	if result.EquilibriumPrice.String() != "103" {
		t.Errorf("expected equilibrium 103 (closest to ref 103.5), got %s", result.EquilibriumPrice.String())
	}
}

// --- Reference price tiebreaker: direction matters ---

func TestClearingReferenceTiebreakerDirections(t *testing.T) {
	// Bids 101@10, Asks 99@10. Both 99 and 101 are max-volume candidates (10).
	// The chosen equilibrium depends purely on the reference price.
	tests := []struct {
		name string
		ref  string
		want string
	}{
		{"ref below both -> lower price", "98.00", "99"},
		{"ref above both -> higher price", "102.00", "101"},
		{"ref equidistant -> higher price wins", "100.00", "101"},
		{"ref exactly on lower candidate", "99.00", "99"},
		{"ref exactly on higher candidate", "101.00", "101"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ae := newTestAuctionEngine()
			bids, asks := buildClearingLevels(t, []clearingSpec{
				{types.SideBuy, "101.00", 10, 1},
				{types.SideSell, "99.00", 10, 2},
			})
			result := ae.RunAuction(clrInstrument, bids, asks, mustParseDecimal(tt.ref))
			if result.EquilibriumPrice.String() != tt.want {
				t.Errorf("ref %s: expected equilibrium %s, got %s", tt.ref, tt.want, result.EquilibriumPrice.String())
			}
			// Regardless of price, executable volume is 10 in every case.
			if got := totalTradeQty(result.Trades); got != 10 {
				t.Errorf("ref %s: expected matched volume 10, got %d", tt.ref, got)
			}
		})
	}
}

// --- Volume conservation invariant ---

func TestClearingVolumeConservation(t *testing.T) {
	ae := newTestAuctionEngine()

	// Asymmetric, multi-level book with a buy-side imbalance.
	bids, asks := buildClearingLevels(t, []clearingSpec{
		{types.SideBuy, "50.00", 30, 1},
		{types.SideBuy, "49.50", 25, 2},
		{types.SideBuy, "49.00", 40, 3},
		{types.SideSell, "48.50", 20, 4},
		{types.SideSell, "49.00", 15, 5},
		{types.SideSell, "49.50", 10, 6},
	})

	result := ae.RunAuction(clrInstrument, bids, asks, mustParseDecimal("49.00"))

	// Every trade must conserve volume: buy fills == sell fills == trade qty.
	var buyQty, sellQty uint64
	buyFills := make(map[string]uint64)
	sellFills := make(map[string]uint64)
	for _, tr := range result.Trades {
		buyQty += tr.Quantity
		sellQty += tr.Quantity
		buyFills[tr.BuyOrderID] += tr.Quantity
		sellFills[tr.SellOrderID] += tr.Quantity
		// All trades execute at the single equilibrium price.
		if !tr.Price.Equal(result.EquilibriumPrice) {
			t.Errorf("trade %s executed at %s, not equilibrium %s",
				tr.TradeID, tr.Price.String(), result.EquilibriumPrice.String())
		}
		// Auction trades have no aggressor and are typed as auction trades.
		if tr.AggressorSide != types.SideUnspecified {
			t.Errorf("auction trade must have no aggressor, got %d", tr.AggressorSide)
		}
		if tr.TradeType != types.TradeTypeAuction {
			t.Errorf("expected TradeTypeAuction, got %d", tr.TradeType)
		}
		// Trade value must equal price * quantity at the equilibrium price.
		wantVal := result.EquilibriumPrice.MulUint64(tr.Quantity)
		if !tr.TradeValue.Equal(wantVal) {
			t.Errorf("trade value %s != price*qty %s", tr.TradeValue.String(), wantVal.String())
		}
	}

	if buyQty != sellQty {
		t.Fatalf("volume not conserved: bought %d, sold %d", buyQty, sellQty)
	}

	// No single order may be filled beyond its original quantity.
	allOrders := append(collectOrders(bids), collectOrders(asks)...)
	byID := make(map[string]*types.Order)
	for _, o := range allOrders {
		byID[o.OrderID] = o
	}
	for id, filled := range buyFills {
		if filled > byID[id].Quantity {
			t.Errorf("buy order %s overfilled: %d > %d", id, filled, byID[id].Quantity)
		}
	}
	for id, filled := range sellFills {
		if filled > byID[id].Quantity {
			t.Errorf("sell order %s overfilled: %d > %d", id, filled, byID[id].Quantity)
		}
	}
}

// collectOrders flattens all orders across the given price levels.
func collectOrders(levels []*PriceLevel) []*types.Order {
	var out []*types.Order
	for _, l := range levels {
		out = append(out, l.Orders()...)
	}
	return out
}

// --- Time priority across multiple price levels with cascading partial fills ---

func TestClearingTimePriorityCascade(t *testing.T) {
	ae := newTestAuctionEngine()

	// Three bids eligible at/above equilibrium, two asks below it.
	// Equilibrium volume is bounded by the smaller ask side; the earliest
	// bids (by sequence) must be filled first, the latest left partial/empty.
	//
	// Bids (all >= 100): 102@4 (seq1), 101@4 (seq2), 100@4 (seq3) = 12
	// Asks (all <= 100): 99@5 (seq4), 100@2 (seq5)               = 7
	// At p=100: bid=12 ask=7 -> match 7 (the maximum).
	bids, asks := buildClearingLevels(t, []clearingSpec{
		{types.SideBuy, "102.00", 4, 1},
		{types.SideBuy, "101.00", 4, 2},
		{types.SideBuy, "100.00", 4, 3},
		{types.SideSell, "99.00", 5, 4},
		{types.SideSell, "100.00", 2, 5},
	})

	result := ae.RunAuction(clrInstrument, bids, asks, mustParseDecimal("100.00"))

	if result.EquilibriumPrice.String() != "100" {
		t.Fatalf("expected equilibrium 100, got %s", result.EquilibriumPrice.String())
	}
	if got := totalTradeQty(result.Trades); got != 7 {
		t.Fatalf("expected matched volume 7, got %d", got)
	}

	// Time priority: seq1 bid (102@4) fully filled first, then seq2 (101@4)
	// fills the remaining 3. seq3 (100@4) must remain untouched.
	byID := make(map[string]*types.Order)
	for _, o := range collectOrders(bids) {
		byID[o.OrderID] = o
	}
	// ord-1 = 102@4 (seq1), ord-2 = 101@4 (seq2), ord-3 = 100@4 (seq3)
	if byID["ord-1"].FilledQty != 4 {
		t.Errorf("earliest bid should be fully filled (4), got %d", byID["ord-1"].FilledQty)
	}
	if byID["ord-2"].FilledQty != 3 {
		t.Errorf("second bid should be partially filled (3), got %d", byID["ord-2"].FilledQty)
	}
	if byID["ord-3"].FilledQty != 0 {
		t.Errorf("latest bid should be untouched (0), got %d", byID["ord-3"].FilledQty)
	}
	if byID["ord-3"].RemainingQty != 4 {
		t.Errorf("latest bid residual should remain 4, got %d", byID["ord-3"].RemainingQty)
	}
}

// --- Wide-spread crossing: best bid far above best ask ---

func TestClearingWideSpreadCrossing(t *testing.T) {
	ae := newTestAuctionEngine()

	// A single aggressive bid engulfs a single low ask. Candidates are 90 and
	// 110, both yielding match=8. Reference 100 is equidistant -> higher (110).
	bids, asks := buildClearingLevels(t, []clearingSpec{
		{types.SideBuy, "110.00", 8, 1},
		{types.SideSell, "90.00", 8, 2},
	})

	result := ae.RunAuction(clrInstrument, bids, asks, mustParseDecimal("100.00"))
	if result.EquilibriumPrice.String() != "110" {
		t.Errorf("expected equilibrium 110, got %s", result.EquilibriumPrice.String())
	}
	if len(result.Trades) != 1 || result.Trades[0].Quantity != 8 {
		t.Errorf("expected single trade of 8, got %+v", result.Trades)
	}
}

// --- Touching market: best bid == best ask ---

func TestClearingTouchingPrices(t *testing.T) {
	ae := newTestAuctionEngine()

	// Best bid equals best ask (101). The crossing check uses >=, so this must
	// uncross. Multiple orders confirm aggregation at the single price.
	bids, asks := buildClearingLevels(t, []clearingSpec{
		{types.SideBuy, "101.00", 6, 1},
		{types.SideBuy, "101.00", 4, 2},
		{types.SideSell, "101.00", 7, 3},
	})

	result := ae.RunAuction(clrInstrument, bids, asks, mustParseDecimal("101.00"))
	if result.EquilibriumPrice.String() != "101" {
		t.Errorf("expected equilibrium 101, got %s", result.EquilibriumPrice.String())
	}
	if got := totalTradeQty(result.Trades); got != 7 {
		t.Errorf("expected matched volume 7 (ask-bounded), got %d", got)
	}
}

// --- Execution reports mirror the trades and carry correct cumulative state ---

func TestClearingExecutionReportsConsistency(t *testing.T) {
	ae := newTestAuctionEngine()

	bids, asks := buildClearingLevels(t, []clearingSpec{
		{types.SideBuy, "100.00", 10, 1},
		{types.SideSell, "100.00", 6, 2},
	})

	result := ae.RunAuction(clrInstrument, bids, asks, mustParseDecimal("100.00"))

	// One trade produces exactly two execution reports (one per side).
	if len(result.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(result.Trades))
	}
	if len(result.ExecutionReports) != 2 {
		t.Fatalf("expected 2 execution reports, got %d", len(result.ExecutionReports))
	}

	var buy, sell *types.ExecutionReport
	for i := range result.ExecutionReports {
		r := &result.ExecutionReports[i]
		switch r.Side {
		case types.SideBuy:
			buy = r
		case types.SideSell:
			sell = r
		}
		// Every report fills at the equilibrium price.
		if !r.LastPrice.Equal(result.EquilibriumPrice) {
			t.Errorf("report %s last price %s != equilibrium %s", r.OrderID, r.LastPrice.String(), result.EquilibriumPrice.String())
		}
		if r.LastQty != 6 {
			t.Errorf("report %s LastQty expected 6, got %d", r.OrderID, r.LastQty)
		}
	}
	if buy == nil || sell == nil {
		t.Fatal("expected one buy and one sell report")
	}
	// Buy: 10 ordered, 6 filled -> partial, 4 leaves.
	if buy.ExecType != types.ExecTypePartialFill || buy.LeavesQty != 4 || buy.CumulativeQty != 6 {
		t.Errorf("buy report mismatch: type=%d leaves=%d cum=%d", buy.ExecType, buy.LeavesQty, buy.CumulativeQty)
	}
	// Sell: 6 ordered, 6 filled -> full, 0 leaves.
	if sell.ExecType != types.ExecTypeFill || sell.LeavesQty != 0 || sell.CumulativeQty != 6 {
		t.Errorf("sell report mismatch: type=%d leaves=%d cum=%d", sell.ExecType, sell.LeavesQty, sell.CumulativeQty)
	}
}

// --- No executable volume when the book does not cross ---

func TestClearingNoCrossProducesNoResult(t *testing.T) {
	ae := newTestAuctionEngine()

	bids, asks := buildClearingLevels(t, []clearingSpec{
		{types.SideBuy, "99.99", 100, 1},
		{types.SideSell, "100.01", 100, 2},
	})

	result := ae.RunAuction(clrInstrument, bids, asks, mustParseDecimal("100.00"))
	if len(result.Trades) != 0 {
		t.Errorf("expected no trades for non-crossing book, got %d", len(result.Trades))
	}
	if !result.EquilibriumPrice.IsZero() {
		t.Errorf("expected zero equilibrium for non-crossing book, got %s", result.EquilibriumPrice.String())
	}
	if len(result.ExecutionReports) != 0 {
		t.Errorf("expected no execution reports, got %d", len(result.ExecutionReports))
	}
}

// =====================================================================
// Full phase-lifecycle: opening and closing call auctions end-to-end.
// =====================================================================

// TestOpeningCallAuctionLifecycle drives a complete opening auction:
// PRE_OPEN -> OPENING_AUCTION (orders accumulate, no matching) -> CONTINUOUS
// (uncrossing fires). It verifies the clearing price, the trades, and that the
// book's LastTradePrice is updated to the equilibrium for downstream use.
func TestOpeningCallAuctionLifecycle(t *testing.T) {
	ae := newTestAuctionEngine()
	pm := NewPhaseManager(ae)
	book := newTestBook()
	book.State = types.BookStatePreOpen

	// PRE_OPEN -> OPENING_AUCTION.
	if _, err := pm.TransitionTo(PhaseOpeningAuction, book); err != nil {
		t.Fatalf("transition to opening auction failed: %v", err)
	}
	if pm.ShouldMatch() {
		t.Fatal("orders must not match during the opening auction call phase")
	}

	// Accumulate a crossing book during the call phase. Because the book is in
	// the auction state, these orders rest instead of matching continuously.
	for _, o := range []*types.Order{
		newLimitOrder("b1", types.SideBuy, "100.50", 5),
		newLimitOrder("b2", types.SideBuy, "100.00", 10),
		newLimitOrder("s1", types.SideSell, "99.50", 4),
		newLimitOrder("s2", types.SideSell, "100.00", 8),
	} {
		book.SubmitOrder(o)
	}
	// No continuous trades should have occurred yet.
	if !book.LastTradePrice.IsZero() {
		t.Fatalf("no trades expected during call phase, last trade price = %s", book.LastTradePrice.String())
	}

	// OPENING_AUCTION -> CONTINUOUS triggers the uncrossing.
	// Cumulative volumes:
	//   p=99.5:  bid=15 ask=4  -> 4
	//   p=100:   bid=15 ask=12 -> 12   <-- max
	//   p=100.5: bid=5  ask=12 -> 5
	// Equilibrium = 100, matched volume = 12.
	res, err := pm.TransitionTo(PhaseContinuous, book)
	if err != nil {
		t.Fatalf("transition to continuous failed: %v", err)
	}
	if res.AuctionResult == nil {
		t.Fatal("expected an auction result when leaving the opening auction")
	}
	if res.AuctionResult.EquilibriumPrice.String() != "100" {
		t.Errorf("expected opening clearing price 100, got %s", res.AuctionResult.EquilibriumPrice.String())
	}
	if got := totalTradeQty(res.AuctionResult.Trades); got != 12 {
		t.Errorf("expected opening matched volume 12, got %d", got)
	}
	// The clearing price becomes the new reference (last trade price).
	if book.LastTradePrice.String() != "100" {
		t.Errorf("expected book last trade price updated to 100, got %s", book.LastTradePrice.String())
	}
	if pm.CurrentPhase() != PhaseContinuous {
		t.Errorf("expected phase CONTINUOUS after uncrossing, got %s", pm.CurrentPhase())
	}
}

// TestClosingCallAuctionLifecycle drives a complete closing auction:
// ... -> CONTINUOUS -> CLOSING_AUCTION (orders accumulate) -> POST_CLOSE
// (uncrossing fires). It verifies the closing clearing price uses the prior
// last trade price as the reference for tie resolution.
func TestClosingCallAuctionLifecycle(t *testing.T) {
	ae := newTestAuctionEngine()
	pm := NewPhaseManager(ae)
	book := newTestBook()
	book.State = types.BookStatePreOpen

	// Fast-forward through the opening with no orders so the opening auction is
	// a no-op, then seed a reference price for the close.
	if _, err := pm.TransitionTo(PhaseOpeningAuction, book); err != nil {
		t.Fatalf("transition to opening auction failed: %v", err)
	}
	if _, err := pm.TransitionTo(PhaseContinuous, book); err != nil {
		t.Fatalf("transition to continuous failed: %v", err)
	}
	// Seed the reference price (as a real continuous session would).
	book.LastTradePrice = mustParseDecimal("100.00")

	// CONTINUOUS -> CLOSING_AUCTION. The book state becomes auction, so the
	// orders below rest and accumulate for the closing uncrossing.
	if _, err := pm.TransitionTo(PhaseClosingAuction, book); err != nil {
		t.Fatalf("transition to closing auction failed: %v", err)
	}
	if pm.ShouldMatch() {
		t.Fatal("orders must not match during the closing auction call phase")
	}

	// Construct a book where two prices tie on volume, so the reference price
	// (100) decides the close. Bids 101@10, Asks 99@10 -> {99,101} tie at 10;
	// ref 100 is equidistant -> higher price 101 wins.
	for _, o := range []*types.Order{
		newLimitOrder("cb1", types.SideBuy, "101.00", 10),
		newLimitOrder("cs1", types.SideSell, "99.00", 10),
	} {
		book.SubmitOrder(o)
	}

	// CLOSING_AUCTION -> POST_CLOSE triggers the closing uncrossing.
	res, err := pm.TransitionTo(PhasePostClose, book)
	if err != nil {
		t.Fatalf("transition to post-close failed: %v", err)
	}
	if res.AuctionResult == nil {
		t.Fatal("expected an auction result when leaving the closing auction")
	}
	if res.AuctionResult.EquilibriumPrice.String() != "101" {
		t.Errorf("expected closing price 101 (tie resolved by ref 100), got %s", res.AuctionResult.EquilibriumPrice.String())
	}
	if got := totalTradeQty(res.AuctionResult.Trades); got != 10 {
		t.Errorf("expected closing matched volume 10, got %d", got)
	}
	if book.State != types.BookStateClosed {
		t.Errorf("expected book state CLOSED after post-close, got %d", book.State)
	}
	// The closing price updates the last trade price (the official close).
	if book.LastTradePrice.String() != "101" {
		t.Errorf("expected closing price recorded as 101, got %s", book.LastTradePrice.String())
	}
}

// TestClosingAuctionNoCrossLeavesReferenceUntouched verifies that an
// uncrossing with no executable volume does not corrupt the last trade price.
func TestClosingAuctionNoCrossLeavesReferenceUntouched(t *testing.T) {
	ae := newTestAuctionEngine()
	pm := NewPhaseManager(ae)
	book := newTestBook()
	book.State = types.BookStatePreOpen

	mustTransition(t, pm, PhaseOpeningAuction, book)
	mustTransition(t, pm, PhaseContinuous, book)
	book.LastTradePrice = mustParseDecimal("100.00")
	mustTransition(t, pm, PhaseClosingAuction, book)

	// Non-crossing book at the close.
	book.SubmitOrder(newLimitOrder("cb", types.SideBuy, "98.00", 5))
	book.SubmitOrder(newLimitOrder("cs", types.SideSell, "102.00", 5))

	res, err := pm.TransitionTo(PhasePostClose, book)
	if err != nil {
		t.Fatalf("transition to post-close failed: %v", err)
	}
	if res.AuctionResult != nil {
		t.Errorf("expected no auction result for non-crossing close, got %+v", res.AuctionResult)
	}
	// Reference price must be preserved (only updated on real trades).
	if book.LastTradePrice.String() != "100" {
		t.Errorf("expected last trade price preserved at 100, got %s", book.LastTradePrice.String())
	}
}

func mustTransition(t *testing.T, pm *PhaseManager, phase MarketPhase, book *OrderBook) {
	t.Helper()
	if _, err := pm.TransitionTo(phase, book); err != nil {
		t.Fatalf("transition to %s failed: %v", phase, err)
	}
}
