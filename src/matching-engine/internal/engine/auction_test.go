package engine

import (
	"testing"

	"github.com/garudax-platform/matching-engine/internal/orderbook"
	"github.com/garudax-platform/matching-engine/internal/types"
)

// auctionLimit builds a resting limit order for auction tests.
func auctionLimit(id, account string, side types.Side, price string, qty uint64) *types.Order {
	return &types.Order{
		OrderID:      id,
		InstrumentID: "MSE-EQ-GOBI",
		AccountID:    account,
		Side:         side,
		OrderType:    types.OrderTypeLimit,
		TimeInForce:  types.TIFDay,
		Price:        mustParseDecimal(price),
		Quantity:     qty,
	}
}

// registerInAuction registers an instrument and moves it into the requested
// auction phase, returning the engine.
func registerInAuction(t *testing.T, instrument string, at AuctionType) *Engine {
	t.Helper()
	eng := newTestEngine()
	if err := eng.RegisterInstrument(instrument); err != nil {
		t.Fatalf("register: %v", err)
	}
	if at == AuctionTypeClosing {
		// Closing auctions are entered from CONTINUOUS, which is reached by
		// closing an (empty) opening auction first.
		if err := eng.OpenAuction(instrument, AuctionTypeOpening); err != nil {
			t.Fatalf("open opening auction: %v", err)
		}
		if _, err := eng.CloseAuction(instrument); err != nil {
			t.Fatalf("close opening auction: %v", err)
		}
	}
	if err := eng.OpenAuction(instrument, at); err != nil {
		t.Fatalf("open %s auction: %v", at, err)
	}
	return eng
}

// TestOpeningAuctionEndToEnd accumulates a crossing book during the opening
// call phase, checks the indicative uncrossing, then closes the auction and
// verifies the clearing price, dispatched trades, and book reconciliation.
func TestOpeningAuctionEndToEnd(t *testing.T) {
	const inst = "MSE-EQ-GOBI"
	eng := registerInAuction(t, inst, AuctionTypeOpening)

	var dispatched []types.Trade
	eng.SetTradeHandler(func(tr types.Trade) { dispatched = append(dispatched, tr) })

	// Crossing book. Cumulative volumes:
	//   p=99.5:  bid=15 ask=4  -> 4
	//   p=100:   bid=15 ask=12 -> 12  <-- max
	//   p=100.5: bid=5  ask=12 -> 5
	// Equilibrium = 100, matched volume = 12.
	for _, o := range []*types.Order{
		auctionLimit("b1", "acc-b1", types.SideBuy, "100.50", 5),
		auctionLimit("b2", "acc-b2", types.SideBuy, "100.00", 10),
		auctionLimit("s1", "acc-s1", types.SideSell, "99.50", 4),
		auctionLimit("s2", "acc-s2", types.SideSell, "100.00", 8),
	} {
		if _, err := eng.SubmitOrder(o); err != nil {
			t.Fatalf("submit %s: %v", o.OrderID, err)
		}
	}

	// Orders rest during the call phase — no trades yet.
	if len(dispatched) != 0 {
		t.Fatalf("expected no trades during call phase, got %d", len(dispatched))
	}

	// Indicative uncrossing before closing.
	state, err := eng.IndicativeAuction(inst)
	if err != nil {
		t.Fatalf("indicative: %v", err)
	}
	if !state.Crossed {
		t.Fatal("expected indicative auction to cross")
	}
	if state.AuctionType != AuctionTypeOpening {
		t.Errorf("expected OPENING auction type, got %s", state.AuctionType)
	}
	if state.IndicativePrice.String() != "100" {
		t.Errorf("expected indicative price 100, got %s", state.IndicativePrice.String())
	}
	if state.IndicativeVolume != 12 {
		t.Errorf("expected indicative volume 12, got %d", state.IndicativeVolume)
	}
	// Eligible bid volume 15 vs ask 12 at price 100 -> buy-side surplus of 3.
	if state.ImbalanceSide != types.SideBuy || state.ImbalanceQty != 3 {
		t.Errorf("expected buy imbalance 3, got side=%s qty=%d", state.ImbalanceSide, state.ImbalanceQty)
	}
	if state.BidOrders != 2 || state.AskOrders != 2 {
		t.Errorf("expected 2 bid/2 ask orders, got %d/%d", state.BidOrders, state.AskOrders)
	}

	// Close the auction.
	res, err := eng.CloseAuction(inst)
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if !res.Crossed {
		t.Fatal("expected closing uncross to cross")
	}
	if res.ClearingPrice.String() != "100" {
		t.Errorf("expected clearing price 100, got %s", res.ClearingPrice.String())
	}
	if res.MatchedVolume != 12 {
		t.Errorf("expected matched volume 12, got %d", res.MatchedVolume)
	}
	if res.AuctionType != AuctionTypeOpening {
		t.Errorf("expected OPENING result type, got %s", res.AuctionType)
	}

	// Trades dispatched to the handler.
	var total uint64
	for _, tr := range dispatched {
		total += tr.Quantity
		if tr.TradeType != types.TradeTypeAuction {
			t.Errorf("expected auction trade type, got %d", tr.TradeType)
		}
		if !tr.Price.Equal(res.ClearingPrice) {
			t.Errorf("trade priced at %s, not clearing %s", tr.Price.String(), res.ClearingPrice.String())
		}
	}
	if total != 12 {
		t.Errorf("expected 12 total dispatched volume, got %d", total)
	}

	// Book reconciliation: b1, s1, s2 fully filled and removed; b2 partial (3) remains.
	if len(res.RemovedOrderIDs) != 3 {
		t.Errorf("expected 3 removed orders, got %d (%v)", len(res.RemovedOrderIDs), res.RemovedOrderIDs)
	}
	book, _ := eng.GetOrderBook(inst)
	if book.OrderCount() != 1 {
		t.Errorf("expected 1 residual order on book, got %d", book.OrderCount())
	}
	residual, ok := book.GetOrder("b2")
	if !ok {
		t.Fatal("expected b2 to remain on the book")
	}
	if residual.RemainingQty != 3 {
		t.Errorf("expected b2 residual qty 3, got %d", residual.RemainingQty)
	}
	// The reconciled bid level must reflect only the residual quantity.
	if len(book.BidLevels()) != 1 || book.BidLevels()[0].TotalQty != 3 {
		t.Errorf("expected single bid level with TotalQty 3, got %+v", book.BidLevels())
	}
	if len(book.AskLevels()) != 0 {
		t.Errorf("expected empty ask side after uncross, got %d levels", len(book.AskLevels()))
	}

	// Phase advanced to continuous, last trade price updated to the clearing price.
	phase, _ := eng.GetPhase(inst)
	if phase != orderbook.PhaseContinuous {
		t.Errorf("expected CONTINUOUS phase after opening auction, got %s", phase)
	}
	if book.LastTradePrice.String() != "100" {
		t.Errorf("expected last trade price 100, got %s", book.LastTradePrice.String())
	}
}

// TestClosingAuctionUsesReferencePrice verifies the closing auction resolves a
// volume tie using the prior last trade price and finishes in POST_CLOSE.
func TestClosingAuctionUsesReferencePrice(t *testing.T) {
	const inst = "MSE-EQ-GOBI"
	eng := newTestEngine()
	if err := eng.RegisterInstrument(inst); err != nil {
		t.Fatalf("register: %v", err)
	}

	// Open then close an empty opening auction to reach CONTINUOUS.
	if err := eng.OpenAuction(inst, AuctionTypeOpening); err != nil {
		t.Fatalf("open opening: %v", err)
	}
	if _, err := eng.CloseAuction(inst); err != nil {
		t.Fatalf("close opening: %v", err)
	}

	// Establish a reference price via a continuous trade at 100.
	if _, err := eng.SubmitOrder(auctionLimit("seed-s", "acc-x", types.SideSell, "100.00", 1)); err != nil {
		t.Fatalf("seed sell: %v", err)
	}
	if _, err := eng.SubmitOrder(auctionLimit("seed-b", "acc-y", types.SideBuy, "100.00", 1)); err != nil {
		t.Fatalf("seed buy: %v", err)
	}
	book, _ := eng.GetOrderBook(inst)
	if book.LastTradePrice.String() != "100" {
		t.Fatalf("expected reference price 100 after seed trade, got %s", book.LastTradePrice.String())
	}

	// Enter the closing auction and accumulate a tie book: bids 101@10, asks
	// 99@10 -> candidates {99,101} both match 10. Reference 100 is equidistant,
	// so the higher price (101) wins.
	if err := eng.OpenAuction(inst, AuctionTypeClosing); err != nil {
		t.Fatalf("open closing: %v", err)
	}
	for _, o := range []*types.Order{
		auctionLimit("cb1", "acc-cb1", types.SideBuy, "101.00", 10),
		auctionLimit("cs1", "acc-cs1", types.SideSell, "99.00", 10),
	} {
		if _, err := eng.SubmitOrder(o); err != nil {
			t.Fatalf("submit %s: %v", o.OrderID, err)
		}
	}

	state, err := eng.IndicativeAuction(inst)
	if err != nil {
		t.Fatalf("indicative: %v", err)
	}
	if state.AuctionType != AuctionTypeClosing {
		t.Errorf("expected CLOSING auction type, got %s", state.AuctionType)
	}
	if state.IndicativePrice.String() != "101" {
		t.Errorf("expected indicative close 101, got %s", state.IndicativePrice.String())
	}
	// Balanced at the equilibrium: 10 vs 10.
	if state.ImbalanceSide != types.SideUnspecified || state.ImbalanceQty != 0 {
		t.Errorf("expected balanced book, got side=%s qty=%d", state.ImbalanceSide, state.ImbalanceQty)
	}

	res, err := eng.CloseAuction(inst)
	if err != nil {
		t.Fatalf("close closing: %v", err)
	}
	if res.ClearingPrice.String() != "101" {
		t.Errorf("expected closing price 101, got %s", res.ClearingPrice.String())
	}
	if res.MatchedVolume != 10 {
		t.Errorf("expected matched volume 10, got %d", res.MatchedVolume)
	}
	// Both sides fully filled and removed.
	if len(res.RemovedOrderIDs) != 2 {
		t.Errorf("expected 2 removed orders, got %d", len(res.RemovedOrderIDs))
	}
	if book.OrderCount() != 0 {
		t.Errorf("expected empty book after close, got %d orders", book.OrderCount())
	}

	phase, _ := eng.GetPhase(inst)
	if phase != orderbook.PhasePostClose {
		t.Errorf("expected POST_CLOSE phase, got %s", phase)
	}
	if book.LastTradePrice.String() != "101" {
		t.Errorf("expected official close 101, got %s", book.LastTradePrice.String())
	}
}

// TestIndicativeAuctionNoCross verifies a non-crossing book reports no
// executable volume and no imbalance.
func TestIndicativeAuctionNoCross(t *testing.T) {
	const inst = "MSE-EQ-GOBI"
	eng := registerInAuction(t, inst, AuctionTypeOpening)

	eng.SubmitOrder(auctionLimit("b", "acc-b", types.SideBuy, "98.00", 5))
	eng.SubmitOrder(auctionLimit("s", "acc-s", types.SideSell, "102.00", 5))

	state, err := eng.IndicativeAuction(inst)
	if err != nil {
		t.Fatalf("indicative: %v", err)
	}
	if state.Crossed {
		t.Error("expected non-crossing book to report Crossed=false")
	}
	if state.IndicativeVolume != 0 || state.ImbalanceQty != 0 {
		t.Errorf("expected zero volume/imbalance, got vol=%d imb=%d", state.IndicativeVolume, state.ImbalanceQty)
	}

	// Closing a non-crossing auction produces no trades but still advances phase.
	res, err := eng.CloseAuction(inst)
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if res.Crossed {
		t.Error("expected no uncross for non-crossing book")
	}
	if len(res.Trades) != 0 || len(res.RemovedOrderIDs) != 0 {
		t.Errorf("expected no trades/removals, got %d trades %d removed", len(res.Trades), len(res.RemovedOrderIDs))
	}
	if book, _ := eng.GetOrderBook(inst); book.OrderCount() != 2 {
		t.Errorf("expected both orders to remain, got %d", book.OrderCount())
	}
	phase, _ := eng.GetPhase(inst)
	if phase != orderbook.PhaseContinuous {
		t.Errorf("expected CONTINUOUS after empty opening uncross, got %s", phase)
	}
}

func TestOpenAuctionErrors(t *testing.T) {
	eng := newTestEngine()
	if err := eng.RegisterInstrument("INST"); err != nil {
		t.Fatal(err)
	}

	// Unknown instrument.
	if err := eng.OpenAuction("NOPE", AuctionTypeOpening); err == nil {
		t.Error("expected error for unknown instrument")
	}
	// Unknown auction type.
	if err := eng.OpenAuction("INST", AuctionTypeUnspecified); err == nil {
		t.Error("expected error for unspecified auction type")
	}
	// Invalid phase transition: opening auction requires PRE_OPEN. Move to
	// CONTINUOUS first, then attempt to open an opening auction.
	if err := eng.OpenAuction("INST", AuctionTypeOpening); err != nil {
		t.Fatalf("first open: %v", err)
	}
	if _, err := eng.CloseAuction("INST"); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := eng.OpenAuction("INST", AuctionTypeOpening); err == nil {
		t.Error("expected invalid-transition error opening an opening auction from CONTINUOUS")
	}
}

func TestAuctionQueriesRequireAuctionPhase(t *testing.T) {
	eng := newTestEngine()
	if err := eng.RegisterInstrument("INST"); err != nil {
		t.Fatal(err)
	}
	// Reach CONTINUOUS (a non-auction phase).
	if err := eng.OpenAuction("INST", AuctionTypeOpening); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.CloseAuction("INST"); err != nil {
		t.Fatal(err)
	}

	if _, err := eng.IndicativeAuction("INST"); err == nil {
		t.Error("expected error: indicative outside auction phase")
	}
	if _, err := eng.CloseAuction("INST"); err == nil {
		t.Error("expected error: close outside auction phase")
	}
	// Unknown instrument variants.
	if _, err := eng.IndicativeAuction("NOPE"); err == nil {
		t.Error("expected error for unknown instrument (indicative)")
	}
	if _, err := eng.CloseAuction("NOPE"); err == nil {
		t.Error("expected error for unknown instrument (close)")
	}
}

func TestAuctionTypeString(t *testing.T) {
	cases := map[AuctionType]string{
		AuctionTypeOpening:     "OPENING",
		AuctionTypeClosing:     "CLOSING",
		AuctionTypeUnspecified: "UNSPECIFIED",
	}
	for at, want := range cases {
		if at.String() != want {
			t.Errorf("AuctionType(%d).String() = %s, want %s", at, at.String(), want)
		}
	}
}
