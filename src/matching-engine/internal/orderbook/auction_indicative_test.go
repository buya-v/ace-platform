package orderbook

import (
	"testing"

	"github.com/garudax-platform/matching-engine/internal/types"
)

// TestIndicativeMatchesRunAuctionPrice verifies the non-executing indicative
// uncross reports the same equilibrium price and volume that RunAuction would
// execute, but without mutating any order state.
func TestIndicativeMatchesRunAuctionPrice(t *testing.T) {
	specs := []clearingSpec{
		{types.SideBuy, "105.00", 2, 1},
		{types.SideBuy, "103.00", 5, 2},
		{types.SideBuy, "101.00", 3, 3},
		{types.SideSell, "100.00", 4, 4},
		{types.SideSell, "102.00", 6, 5},
		{types.SideSell, "104.00", 2, 6},
	}

	// Indicative pass.
	bids, asks := buildClearingLevels(t, specs)
	ind := newTestAuctionEngine().Indicative(bids, asks, mustParseDecimal("102.00"))
	if !ind.Crossed {
		t.Fatal("expected indicative to cross")
	}
	if ind.IndicativePrice.String() != "102" {
		t.Errorf("expected indicative price 102, got %s", ind.IndicativePrice.String())
	}
	if ind.IndicativeVolume != 7 {
		t.Errorf("expected indicative volume 7, got %d", ind.IndicativeVolume)
	}
	// At price 102: cumulative bid (>=102) = 105@2+103@5 = 7; ask (<=102) =
	// 100@4+102@6 = 10. Ask-side surplus of 3.
	if ind.ImbalanceSide != types.SideSell || ind.ImbalanceQty != 3 {
		t.Errorf("expected sell imbalance 3, got side=%s qty=%d", ind.ImbalanceSide, ind.ImbalanceQty)
	}

	// Indicative must not mutate orders: a fresh executing pass on an
	// equivalent book yields the same price and matched volume.
	xbids, xasks := buildClearingLevels(t, specs)
	run := newTestAuctionEngine().RunAuction(clrInstrument, xbids, xasks, mustParseDecimal("102.00"))
	if run.EquilibriumPrice.String() != ind.IndicativePrice.String() {
		t.Errorf("indicative price %s != executed price %s", ind.IndicativePrice.String(), run.EquilibriumPrice.String())
	}
	if got := totalTradeQty(run.Trades); got != ind.IndicativeVolume {
		t.Errorf("indicative volume %d != executed volume %d", ind.IndicativeVolume, got)
	}

	// The orders in the indicative book are untouched (no fills).
	for _, o := range append(collectOrders(bids), collectOrders(asks)...) {
		if o.FilledQty != 0 || o.RemainingQty != o.Quantity {
			t.Errorf("indicative mutated order %s: filled=%d remaining=%d", o.OrderID, o.FilledQty, o.RemainingQty)
		}
	}
}

// TestIndicativeNoCross reports an uncrossed book as not crossing with zero
// volume and no imbalance.
func TestIndicativeNoCross(t *testing.T) {
	bids, asks := buildClearingLevels(t, []clearingSpec{
		{types.SideBuy, "99.00", 5, 1},
		{types.SideSell, "101.00", 5, 2},
	})
	ind := newTestAuctionEngine().Indicative(bids, asks, mustParseDecimal("100.00"))
	if ind.Crossed {
		t.Error("expected no cross")
	}
	if ind.IndicativeVolume != 0 || ind.ImbalanceQty != 0 || ind.ImbalanceSide != types.SideUnspecified {
		t.Errorf("expected zero result, got %+v", ind)
	}
}

// TestIndicativeEmptyBook handles empty sides gracefully.
func TestIndicativeEmptyBook(t *testing.T) {
	ind := newTestAuctionEngine().Indicative(nil, nil, mustParseDecimal("100.00"))
	if ind.Crossed {
		t.Error("expected empty book to report no cross")
	}
}

// TestRemoveFilledOrdersReconcilesBook verifies that after an auction
// uncrossing fills orders in place, RemoveFilledOrders drops fully-filled
// orders, recomputes level aggregates from residuals, and removes empty levels.
func TestRemoveFilledOrdersReconcilesBook(t *testing.T) {
	book := newTestBook()
	book.State = types.BookStatePreOpen
	ae := newTestAuctionEngine()
	pm := NewPhaseManager(ae)

	mustTransition(t, pm, PhaseOpeningAuction, book)

	// Accumulate a crossing book. Equilibrium 100, matched 12 (ask-bounded).
	for _, o := range []*types.Order{
		newLimitOrder("b1", types.SideBuy, "100.50", 5),
		newLimitOrder("b2", types.SideBuy, "100.00", 10),
		newLimitOrder("s1", types.SideSell, "99.50", 4),
		newLimitOrder("s2", types.SideSell, "100.00", 8),
	} {
		book.SubmitOrder(o)
	}
	if book.OrderCount() != 4 {
		t.Fatalf("expected 4 resting orders, got %d", book.OrderCount())
	}

	mustTransition(t, pm, PhaseContinuous, book) // triggers uncrossing

	removed := book.RemoveFilledOrders()
	// b1(5), s1(4), s2(8) fully filled; b2 partially filled (3 remaining).
	if len(removed) != 3 {
		t.Errorf("expected 3 removed orders, got %d (%v)", len(removed), removed)
	}
	if book.OrderCount() != 1 {
		t.Errorf("expected 1 residual order, got %d", book.OrderCount())
	}
	if _, ok := book.GetOrder("b2"); !ok {
		t.Error("expected b2 to survive as residual")
	}
	for _, id := range []string{"b1", "s1", "s2"} {
		if _, ok := book.GetOrder(id); ok {
			t.Errorf("expected %s to be removed from the index", id)
		}
	}
	// Ask side fully consumed; bid side has a single level with the residual 3.
	if len(book.AskLevels()) != 0 {
		t.Errorf("expected empty ask side, got %d levels", len(book.AskLevels()))
	}
	if len(book.BidLevels()) != 1 {
		t.Fatalf("expected 1 bid level, got %d", len(book.BidLevels()))
	}
	if got := book.BidLevels()[0].TotalQty; got != 3 {
		t.Errorf("expected residual level TotalQty 3, got %d", got)
	}
	if got := book.BidLevels()[0].OrderCount; got != 1 {
		t.Errorf("expected residual level OrderCount 1, got %d", got)
	}

	// Idempotent: a second sweep with no filled orders removes nothing.
	if again := book.RemoveFilledOrders(); len(again) != 0 {
		t.Errorf("expected no-op on second sweep, removed %v", again)
	}
}

// TestRemoveFilledOnLevel exercises PriceLevel.RemoveFilled directly.
func TestRemoveFilledOnLevel(t *testing.T) {
	level := NewPriceLevel(mustParseDecimal("100.00"))
	full := newLimitOrder("full", types.SideBuy, "100.00", 5)
	full.RemainingQty = 5
	partial := newLimitOrder("partial", types.SideBuy, "100.00", 5)
	partial.RemainingQty = 5
	level.Enqueue(full)
	level.Enqueue(partial)

	// Simulate an in-place auction fill.
	full.Fill(5)    // fully filled
	partial.Fill(2) // 3 remaining

	removed := level.RemoveFilled()
	if len(removed) != 1 || removed[0] != "full" {
		t.Errorf("expected to remove 'full', got %v", removed)
	}
	if level.TotalQty != 3 {
		t.Errorf("expected TotalQty 3, got %d", level.TotalQty)
	}
	if level.OrderCount != 1 {
		t.Errorf("expected OrderCount 1, got %d", level.OrderCount)
	}
	if level.IsEmpty() {
		t.Error("level should not be empty (residual remains)")
	}
}
