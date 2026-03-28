package engine

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ace-platform/clearing-engine/internal/novation"
	"github.com/ace-platform/clearing-engine/internal/store"
	"github.com/ace-platform/clearing-engine/internal/types"
)

// failingStore is an ObligationStore that fails on Append after N successful appends.
type failingStore struct {
	*store.InMemoryObligationStore
	failAfter int
	count     int
}

func newFailingStore(failAfter int) *failingStore {
	return &failingStore{
		InMemoryObligationStore: store.NewInMemoryObligationStore(),
		failAfter:               failAfter,
	}
}

func (s *failingStore) Append(obl types.ClearingObligation) error {
	s.count++
	if s.count > s.failAfter {
		return fmt.Errorf("store: simulated write failure")
	}
	return s.InMemoryObligationStore.Append(obl)
}

type countIDGen struct{ counter uint64 }

func (g *countIDGen) NewID() string {
	n := atomic.AddUint64(&g.counter, 1)
	return fmt.Sprintf("obl-%d", n)
}

func makeTradeWithInstrument(id, buyer, seller, instrument string, price int64, qty uint64) types.Trade {
	return types.Trade{
		TradeID:             id,
		InstrumentID:        instrument,
		BuyOrderID:          "buy-" + id,
		SellOrderID:         "sell-" + id,
		BuyerParticipantID:  buyer,
		SellerParticipantID: seller,
		Price:               types.DecimalFromInt(price),
		Quantity:            qty,
		TradeValue:          types.DecimalFromInt(price).MulUint64(qty),
		AggressorSide:       types.SideBuy,
		SequenceNumber:      1,
		ExecutedAt:          time.Now(),
	}
}

// === ClearTrade error path tests ===

func TestClearTradeNovationFailure(t *testing.T) {
	eng := newTestEngine()

	// Empty trade ID → novation will fail
	trade := makeTrade("", "buyer", "seller", 500, 10)
	_, err := eng.ClearTrade(trade)
	if err == nil {
		t.Fatal("expected error for invalid trade (empty ID)")
	}
}

func TestClearTradeZeroQuantityFails(t *testing.T) {
	eng := newTestEngine()
	trade := makeTrade("t-1", "buyer", "seller", 500, 0)
	_, err := eng.ClearTrade(trade)
	if err == nil {
		t.Fatal("expected error for zero quantity trade")
	}
}

func TestClearTradeMissingBuyerFails(t *testing.T) {
	eng := newTestEngine()
	trade := makeTrade("t-1", "", "seller", 500, 10)
	_, err := eng.ClearTrade(trade)
	if err == nil {
		t.Fatal("expected error for missing buyer")
	}
}

func TestClearTradeMissingSellerFails(t *testing.T) {
	eng := newTestEngine()
	trade := makeTrade("t-1", "buyer", "", 500, 10)
	_, err := eng.ClearTrade(trade)
	if err == nil {
		t.Fatal("expected error for missing seller")
	}
}

func TestClearTradeStoreBuyerFailure(t *testing.T) {
	// Store fails on first append (buyer obligation)
	eng := NewEngine(&countIDGen{}, newFailingStore(0))
	trade := makeTrade("t-1", "buyer", "seller", 500, 10)

	_, err := eng.ClearTrade(trade)
	if err == nil {
		t.Fatal("expected error when store fails on buyer obligation")
	}
}

func TestClearTradeStoreSellerFailure(t *testing.T) {
	// Store succeeds on first append (buyer) but fails on second (seller)
	eng := NewEngine(&countIDGen{}, newFailingStore(1))
	trade := makeTrade("t-1", "buyer", "seller", 500, 10)

	_, err := eng.ClearTrade(trade)
	if err == nil {
		t.Fatal("expected error when store fails on seller obligation")
	}
}

func TestClearTradeIdempotencyMessage(t *testing.T) {
	eng := newTestEngine()
	trade := makeTrade("t-dup", "buyer", "seller", 500, 10)

	_, err := eng.ClearTrade(trade)
	if err != nil {
		t.Fatalf("first clear: %v", err)
	}

	_, err = eng.ClearTrade(trade)
	if err == nil {
		t.Fatal("expected error on duplicate")
	}
	// Verify the error message includes the trade ID
	expected := "clearing: trade t-dup already processed"
	if err.Error() != expected {
		t.Errorf("error = %q, want %q", err.Error(), expected)
	}
}

// === GetPositions / GetPositionsByInstrument ===

func TestGetPositions(t *testing.T) {
	eng := newTestEngine()

	// P1 trades in two instruments
	eng.ClearTrade(makeTrade("t-1", "P1", "P2", 500, 10))
	trade2 := makeTradeWithInstrument("t-2", "P1", "P3", "CORN-2026M12", 400, 5)
	eng.ClearTrade(trade2)

	positions := eng.GetPositions("P1")
	if len(positions) != 2 {
		t.Errorf("P1 positions = %d, want 2", len(positions))
	}

	// P2 only trades in one instrument
	positions = eng.GetPositions("P2")
	if len(positions) != 1 {
		t.Errorf("P2 positions = %d, want 1", len(positions))
	}

	// Unknown participant
	positions = eng.GetPositions("unknown")
	if len(positions) != 0 {
		t.Errorf("unknown positions = %d, want 0", len(positions))
	}
}

func TestGetPositionsByInstrument(t *testing.T) {
	eng := newTestEngine()

	// Two trades in WHT, one in CORN
	eng.ClearTrade(makeTrade("t-1", "P1", "P2", 500, 10))
	eng.ClearTrade(makeTrade("t-2", "P3", "P4", 510, 5))
	trade3 := makeTradeWithInstrument("t-3", "P1", "P5", "CORN-2026M12", 400, 8)
	eng.ClearTrade(trade3)

	whtPositions := eng.GetPositionsByInstrument("WHT-HRW-2026M07-UB")
	// P1, P2, P3, P4 all have WHT positions
	if len(whtPositions) != 4 {
		t.Errorf("WHT positions = %d, want 4", len(whtPositions))
	}

	cornPositions := eng.GetPositionsByInstrument("CORN-2026M12")
	// P1 and P5
	if len(cornPositions) != 2 {
		t.Errorf("CORN positions = %d, want 2", len(cornPositions))
	}

	// Unknown instrument
	unknownPositions := eng.GetPositionsByInstrument("UNKNOWN")
	if len(unknownPositions) != 0 {
		t.Errorf("unknown positions = %d, want 0", len(unknownPositions))
	}
}

// === End-to-end pipeline tests ===

func TestClearingPipelineEndToEnd(t *testing.T) {
	eng := newTestEngine()

	// Step 1: Clear multiple trades
	r1, _ := eng.ClearTrade(makeTrade("t-1", "P1", "P2", 500, 10))
	r2, _ := eng.ClearTrade(makeTrade("t-2", "P3", "P1", 520, 5))
	r3, _ := eng.ClearTrade(makeTrade("t-3", "P2", "P3", 510, 8))

	// Verify novation created obligations
	if r1.Novation.BuyerObligation.ObligationID == "" {
		t.Error("t-1: buyer obligation ID should not be empty")
	}
	if r2.Novation.SellerObligation.Side != types.SideSell {
		t.Error("t-2: seller side should be SELL")
	}
	if r3.Trade.TradeID != "t-3" {
		t.Error("t-3: trade ID mismatch")
	}

	// Step 2: Check positions
	p1, ok := eng.GetPosition("P1", "WHT-HRW-2026M07-UB")
	if !ok {
		t.Fatal("P1 position not found")
	}
	// P1: bought 10 (t-1), sold 5 (t-2) → net +5
	if p1.NetQuantity != 5 {
		t.Errorf("P1 net = %d, want 5", p1.NetQuantity)
	}

	// Step 3: Net obligations
	netResults := eng.NetObligations()
	if len(netResults) == 0 {
		t.Fatal("netting produced no results")
	}

	// All participants should appear in netting
	participants := make(map[string]bool)
	for _, r := range netResults {
		participants[r.ParticipantID] = true
	}
	for _, p := range []string{"P1", "P2", "P3"} {
		if !participants[p] {
			t.Errorf("participant %s missing from netting results", p)
		}
	}

	// Step 4: Check obligations by trade
	obls := eng.GetObligations("t-1")
	if len(obls) != 2 {
		t.Errorf("t-1 obligations = %d, want 2", len(obls))
	}
}

func TestNetObligationsByInstrumentFiltersNonNovated(t *testing.T) {
	// This tests the filter inside NetObligationsByInstrument that only
	// selects novated obligations from the instrument set
	eng := newTestEngine()

	eng.ClearTrade(makeTrade("t-1", "P1", "P2", 500, 10))

	// All obligations are novated, so netting should find them
	results := eng.NetObligationsByInstrument("WHT-HRW-2026M07-UB")
	if len(results) != 2 {
		t.Errorf("got %d results, want 2", len(results))
	}

	// Netting for non-existent instrument should return empty
	results = eng.NetObligationsByInstrument("NONEXISTENT")
	if len(results) != 0 {
		t.Errorf("got %d results for non-existent instrument, want 0", len(results))
	}
}

func TestTradeHandlerReceivesCorrectData(t *testing.T) {
	eng := newTestEngine()

	var receivedTrades []string
	var mu sync.Mutex
	eng.SetTradeHandler(func(trade types.Trade, result novation.NovationResult) {
		mu.Lock()
		defer mu.Unlock()
		receivedTrades = append(receivedTrades, trade.TradeID)

		// Verify novation result has both obligations
		if result.BuyerObligation.ObligationID == "" {
			t.Error("handler: buyer obligation ID empty")
		}
		if result.SellerObligation.ObligationID == "" {
			t.Error("handler: seller obligation ID empty")
		}
	})

	eng.ClearTrade(makeTrade("t-1", "P1", "P2", 500, 10))
	eng.ClearTrade(makeTrade("t-2", "P3", "P4", 600, 20))

	mu.Lock()
	defer mu.Unlock()
	if len(receivedTrades) != 2 {
		t.Fatalf("handler called %d times, want 2", len(receivedTrades))
	}
	if receivedTrades[0] != "t-1" || receivedTrades[1] != "t-2" {
		t.Errorf("handler trades = %v, want [t-1, t-2]", receivedTrades)
	}
}

func TestClearTradeWithoutHandler(t *testing.T) {
	// Engine should work fine without a handler set
	eng := newTestEngine()
	result, err := eng.ClearTrade(makeTrade("t-1", "P1", "P2", 500, 10))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
}

func TestPositionUpdatesAfterClearing(t *testing.T) {
	eng := newTestEngine()

	// Trade 1: P1 buys 10 @ 500
	r1, _ := eng.ClearTrade(makeTrade("t-1", "P1", "P2", 500, 10))
	if r1.BuyerPosition.NetQuantity != 10 {
		t.Errorf("after t-1: P1 net = %d, want 10", r1.BuyerPosition.NetQuantity)
	}
	if r1.SellerPosition.NetQuantity != -10 {
		t.Errorf("after t-1: P2 net = %d, want -10", r1.SellerPosition.NetQuantity)
	}

	// Trade 2: P1 sells 5 @ 600 to P3
	r2, _ := eng.ClearTrade(makeTrade("t-2", "P3", "P1", 600, 5))
	if r2.SellerPosition.NetQuantity != 5 {
		t.Errorf("after t-2: P1 net = %d, want 5", r2.SellerPosition.NetQuantity)
	}

	// Trade 3: P1 sells remaining 5 @ 550 to go flat
	r3, _ := eng.ClearTrade(makeTrade("t-3", "P2", "P1", 550, 5))
	if r3.SellerPosition.NetQuantity != 0 {
		t.Errorf("after t-3: P1 net = %d, want 0 (flat)", r3.SellerPosition.NetQuantity)
	}
}

func TestConcurrentClearingWithPositionVerification(t *testing.T) {
	eng := newTestEngine()

	var wg sync.WaitGroup
	numTrades := 50

	for i := 0; i < numTrades; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			trade := makeTrade(
				fmt.Sprintf("t-%d", idx),
				"P1", // all trades same buyer
				"P2", // all trades same seller
				500,
				1, // 1 unit each
			)
			eng.ClearTrade(trade)
		}(i)
	}

	wg.Wait()

	// P1 should have bought exactly numTrades units
	pos, ok := eng.GetPosition("P1", "WHT-HRW-2026M07-UB")
	if !ok {
		t.Fatal("P1 position not found")
	}
	if pos.NetQuantity != int64(numTrades) {
		t.Errorf("P1 net = %d, want %d", pos.NetQuantity, numTrades)
	}
}

func TestClearingResultFields(t *testing.T) {
	eng := newTestEngine()
	trade := makeTrade("t-result", "buyer-1", "seller-1", 750, 20)

	result, err := eng.ClearTrade(trade)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify ClearingResult contains original trade
	if result.Trade.TradeID != "t-result" {
		t.Errorf("result trade ID = %s, want t-result", result.Trade.TradeID)
	}

	// Verify novation result
	if result.Novation.BuyerObligation.ParticipantID != "buyer-1" {
		t.Errorf("buyer = %s, want buyer-1", result.Novation.BuyerObligation.ParticipantID)
	}
	if result.Novation.SellerObligation.ParticipantID != "seller-1" {
		t.Errorf("seller = %s, want seller-1", result.Novation.SellerObligation.ParticipantID)
	}

	// Verify positions
	if result.BuyerPosition.TotalBuyQty != 20 {
		t.Errorf("buyer total buy = %d, want 20", result.BuyerPosition.TotalBuyQty)
	}
	if result.SellerPosition.TotalSellQty != 20 {
		t.Errorf("seller total sell = %d, want 20", result.SellerPosition.TotalSellQty)
	}
}

func TestGetObligationsNotFound(t *testing.T) {
	eng := newTestEngine()
	obls := eng.GetObligations("nonexistent")
	if len(obls) != 0 {
		t.Errorf("got %d obligations, want 0", len(obls))
	}
}

func TestNetObligationsEmpty(t *testing.T) {
	eng := newTestEngine()
	results := eng.NetObligations()
	if len(results) != 0 {
		t.Errorf("got %d netting results, want 0", len(results))
	}
}

// Ensure novation import is used
var _ novation.NovationResult
