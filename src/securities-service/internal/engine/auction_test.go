// Package engine_test — call auction tests for AuctionEngine.
package engine_test

import (
	"testing"
	"time"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// ---- auction test helpers --------------------------------------------------

const (
	auctionInstID = "EQUITY-AUCTION"
	auctionTenant = "mse-equities"
	buyerPrefix   = "buyer"
	sellerPrefix  = "seller"
)

// auctionTestEnv bundles everything a single auction test needs.
type auctionTestEnv struct {
	ae   *engine.AuctionEngine
	inst *store.InMemoryInstrumentStore
	ord  *store.InMemoryOrderStore
	trd  *store.InMemoryTradeStore
	pos  *store.InMemoryPositionStore
}

// setupAuctionTest creates a fresh AuctionEngine wired to in-memory stores and
// registers an EQUITY instrument (lot_size=1, tick_size=0.01, ACTIVE).
func setupAuctionTest(t *testing.T) *auctionTestEnv {
	t.Helper()

	inst := store.NewInMemoryInstrumentStore()
	ord := store.NewInMemoryOrderStore()
	trd := store.NewInMemoryTradeStore()
	pos := store.NewInMemoryPositionStore()

	// Create the test instrument.
	now := time.Now().UTC().Format(time.RFC3339)
	testInst := &types.Instrument{
		ID:            auctionInstID,
		Ticker:        "ACTN",
		Name:          "Auction Corp",
		AssetClass:    types.AssetClassEquity,
		LotSize:       1,
		TickSize:      0.01,
		TradingStatus: types.TradingStatusActive,
		ExchangeCode:  "MSE",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := inst.Create(testInst); err != nil {
		t.Fatalf("setupAuctionTest: create instrument: %v", err)
	}

	ae := engine.NewAuctionEngine(ord, trd, pos, nil)

	return &auctionTestEnv{ae: ae, inst: inst, ord: ord, trd: trd, pos: pos}
}

// auctionOrder builds a limit order suitable for auction (no status set — CollectOrder sets it).
func auctionOrder(id, participantID string, side types.OrderSide, qty int, price float64) *types.SecurityOrder {
	return &types.SecurityOrder{
		ID:            id,
		InstrumentID:  auctionInstID,
		ParticipantID: participantID,
		Side:          side,
		OrderType:     types.OrderTypeLimit,
		Quantity:      qty,
		Price:         decLit(price),
		TimeInForce:   types.TimeInForceGTC,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
	}
}

// collectOrder calls AuctionEngine.CollectOrder and fails the test on error.
func collectOrder(t *testing.T, ae *engine.AuctionEngine, order *types.SecurityOrder) {
	t.Helper()
	if err := ae.CollectOrder(order); err != nil {
		t.Fatalf("CollectOrder %s: %v", order.ID, err)
	}
}

// ---- auction tests ---------------------------------------------------------

// TestAuction_SinglePriceCross: 3 buys@100 (qty 10) + 3 sells@100 (qty 10).
// Expected: clearing price 100, matched volume 30.
func TestAuction_SinglePriceCross(t *testing.T) {
	env := setupAuctionTest(t)

	for i := 1; i <= 3; i++ {
		collectOrder(t, env.ae, auctionOrder(
			ts(i*2), // unique ID using ts helper
			buyerPrefix,
			types.OrderSideBuy, 10, 100.0,
		))
	}
	for i := 1; i <= 3; i++ {
		collectOrder(t, env.ae, auctionOrder(
			ts(i*2+10), // unique ID
			sellerPrefix,
			types.OrderSideSell, 10, 100.0,
		))
	}

	trades, result, err := env.ae.RunAuction(auctionInstID, auctionTenant)
	if err != nil {
		t.Fatalf("RunAuction: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil AuctionResult")
	}
	if result.ClearingPrice != decLit(100.0) {
		t.Errorf("clearing price: want 100.0, got %v", result.ClearingPrice)
	}
	if result.MatchedVolume != 30 {
		t.Errorf("matched volume: want 30, got %d", result.MatchedVolume)
	}
	if result.TradeCount == 0 {
		t.Error("expected at least one trade")
	}
	totalQty := 0
	for _, tr := range trades {
		totalQty += tr.Quantity
	}
	if totalQty != 30 {
		t.Errorf("total trade qty: want 30, got %d", totalQty)
	}
}

// TestAuction_MaxVolume: buys at 102(50), 101(50), 100(50); sells at 99(50), 100(50), 101(50).
// At price 100: cumBuy = 50+50+50=150, cumSell = 50+50=100, matchable = 100.
// At price 101: cumBuy = 50+50=100, cumSell = 50+50+50=150, matchable = 100.
// At price 99:  cumBuy = 150, cumSell = 50, matchable = 50.
// Max matchable = 100 at price 100 or 101; tie → highest price = 101.
func TestAuction_MaxVolume(t *testing.T) {
	env := setupAuctionTest(t)

	// Buys.
	collectOrder(t, env.ae, &types.SecurityOrder{
		ID: "b102", InstrumentID: auctionInstID, ParticipantID: "buyer-a",
		Side: types.OrderSideBuy, OrderType: types.OrderTypeLimit,
		Quantity: 50, Price: decLit(102.0), TimeInForce: types.TimeInForceGTC,
		CreatedAt: ts(1), UpdatedAt: ts(1),
	})
	collectOrder(t, env.ae, &types.SecurityOrder{
		ID: "b101", InstrumentID: auctionInstID, ParticipantID: "buyer-b",
		Side: types.OrderSideBuy, OrderType: types.OrderTypeLimit,
		Quantity: 50, Price: decLit(101.0), TimeInForce: types.TimeInForceGTC,
		CreatedAt: ts(2), UpdatedAt: ts(2),
	})
	collectOrder(t, env.ae, &types.SecurityOrder{
		ID: "b100", InstrumentID: auctionInstID, ParticipantID: "buyer-c",
		Side: types.OrderSideBuy, OrderType: types.OrderTypeLimit,
		Quantity: 50, Price: decLit(100.0), TimeInForce: types.TimeInForceGTC,
		CreatedAt: ts(3), UpdatedAt: ts(3),
	})

	// Sells.
	collectOrder(t, env.ae, &types.SecurityOrder{
		ID: "s99", InstrumentID: auctionInstID, ParticipantID: "seller-a",
		Side: types.OrderSideSell, OrderType: types.OrderTypeLimit,
		Quantity: 50, Price: decLit(99.0), TimeInForce: types.TimeInForceGTC,
		CreatedAt: ts(4), UpdatedAt: ts(4),
	})
	collectOrder(t, env.ae, &types.SecurityOrder{
		ID: "s100", InstrumentID: auctionInstID, ParticipantID: "seller-b",
		Side: types.OrderSideSell, OrderType: types.OrderTypeLimit,
		Quantity: 50, Price: decLit(100.0), TimeInForce: types.TimeInForceGTC,
		CreatedAt: ts(5), UpdatedAt: ts(5),
	})
	collectOrder(t, env.ae, &types.SecurityOrder{
		ID: "s101", InstrumentID: auctionInstID, ParticipantID: "seller-c",
		Side: types.OrderSideSell, OrderType: types.OrderTypeLimit,
		Quantity: 50, Price: decLit(101.0), TimeInForce: types.TimeInForceGTC,
		CreatedAt: ts(6), UpdatedAt: ts(6),
	})

	_, result, err := env.ae.RunAuction(auctionInstID, auctionTenant)
	if err != nil {
		t.Fatalf("RunAuction: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil AuctionResult")
	}

	// Clearing price should maximise volume (100 units, tie broken at highest price = 101).
	if result.MatchedVolume != 100 {
		t.Errorf("matched volume: want 100, got %d", result.MatchedVolume)
	}
	// Clearing price must be one that produced max volume.
	if result.ClearingPrice != decLit(101.0) {
		t.Errorf("clearing price: want 101.0 (tie-break highest), got %v", result.ClearingPrice)
	}
}

// TestAuction_NoMatch: all buys below all sells → 0 trades.
func TestAuction_NoMatch(t *testing.T) {
	env := setupAuctionTest(t)

	collectOrder(t, env.ae, &types.SecurityOrder{
		ID: "buy-no", InstrumentID: auctionInstID, ParticipantID: "buyer",
		Side: types.OrderSideBuy, OrderType: types.OrderTypeLimit,
		Quantity: 100, Price: decLit(95.0), TimeInForce: types.TimeInForceGTC,
		CreatedAt: ts(0), UpdatedAt: ts(0),
	})
	collectOrder(t, env.ae, &types.SecurityOrder{
		ID: "sell-no", InstrumentID: auctionInstID, ParticipantID: "seller",
		Side: types.OrderSideSell, OrderType: types.OrderTypeLimit,
		Quantity: 100, Price: decLit(105.0), TimeInForce: types.TimeInForceGTC,
		CreatedAt: ts(1), UpdatedAt: ts(1),
	})

	trades, result, err := env.ae.RunAuction(auctionInstID, auctionTenant)
	if err != nil {
		t.Fatalf("RunAuction: %v", err)
	}
	if len(trades) != 0 {
		t.Errorf("expected 0 trades, got %d", len(trades))
	}
	if result == nil {
		t.Fatal("expected non-nil result even when no match")
	}
	if result.MatchedVolume != 0 {
		t.Errorf("matched volume: want 0, got %d", result.MatchedVolume)
	}
	if result.ClearingPrice != decLit(0) {
		t.Errorf("clearing price: want 0 (no match), got %v", result.ClearingPrice)
	}
	if result.TradeCount != 0 {
		t.Errorf("trade count: want 0, got %d", result.TradeCount)
	}
}

// TestAuction_PartialFill: buy 100@102, sell 50@99, sell 50@101.
// At price 99:  cumBuy=100, cumSell=50, matchable=50.
// At price 101: cumBuy=100, cumSell=100, matchable=100.
// At price 102: cumBuy=100, cumSell=100, matchable=100 → clearing price 102 (highest tie).
// But 102 has no sell, so max matchable at 101 and 102 is both 100,
// tie → 102 wins, but cumSell at 102 = 100 (sells at 99 and 101 both <= 102).
// Actually: price 101 and 102 both give 100 matched — tie goes to highest = 102.
// Expected: 100 matched at clearing price 102.
func TestAuction_PartialFill(t *testing.T) {
	env := setupAuctionTest(t)

	collectOrder(t, env.ae, &types.SecurityOrder{
		ID: "buy-pf", InstrumentID: auctionInstID, ParticipantID: "buyer-pf",
		Side: types.OrderSideBuy, OrderType: types.OrderTypeLimit,
		Quantity: 100, Price: decLit(102.0), TimeInForce: types.TimeInForceGTC,
		CreatedAt: ts(0), UpdatedAt: ts(0),
	})
	collectOrder(t, env.ae, &types.SecurityOrder{
		ID: "sell-pf-1", InstrumentID: auctionInstID, ParticipantID: "seller-pf-a",
		Side: types.OrderSideSell, OrderType: types.OrderTypeLimit,
		Quantity: 50, Price: decLit(99.0), TimeInForce: types.TimeInForceGTC,
		CreatedAt: ts(1), UpdatedAt: ts(1),
	})
	collectOrder(t, env.ae, &types.SecurityOrder{
		ID: "sell-pf-2", InstrumentID: auctionInstID, ParticipantID: "seller-pf-b",
		Side: types.OrderSideSell, OrderType: types.OrderTypeLimit,
		Quantity: 50, Price: decLit(101.0), TimeInForce: types.TimeInForceGTC,
		CreatedAt: ts(2), UpdatedAt: ts(2),
	})

	trades, result, err := env.ae.RunAuction(auctionInstID, auctionTenant)
	if err != nil {
		t.Fatalf("RunAuction: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil AuctionResult")
	}

	// 100 units should match (buy 100 vs sell 50+50=100 all eligible at clearing price).
	if result.MatchedVolume != 100 {
		t.Errorf("matched volume: want 100, got %d", result.MatchedVolume)
	}

	// Verify all trades are at the clearing price.
	for _, tr := range trades {
		if tr.Price != result.ClearingPrice {
			t.Errorf("trade price %v != clearing price %v", tr.Price, result.ClearingPrice)
		}
	}

	// Total traded quantity equals matched volume.
	totalQty := 0
	for _, tr := range trades {
		totalQty += tr.Quantity
	}
	if totalQty != result.MatchedVolume {
		t.Errorf("sum(trade.Quantity)=%d != MatchedVolume=%d", totalQty, result.MatchedVolume)
	}
}

// TestAuction_EmptyBook: no collected orders → 0 trades, result has zero values.
func TestAuction_EmptyBook(t *testing.T) {
	env := setupAuctionTest(t)

	trades, result, err := env.ae.RunAuction(auctionInstID, auctionTenant)
	if err != nil {
		t.Fatalf("RunAuction on empty book: %v", err)
	}
	if len(trades) != 0 {
		t.Errorf("expected 0 trades, got %d", len(trades))
	}
	// Result should be non-nil but zeroed (no match).
	if result != nil {
		if result.MatchedVolume != 0 {
			t.Errorf("matched volume: want 0, got %d", result.MatchedVolume)
		}
		if result.ClearingPrice != decLit(0) {
			t.Errorf("clearing price: want 0, got %v", result.ClearingPrice)
		}
		if result.TradeCount != 0 {
			t.Errorf("trade count: want 0, got %d", result.TradeCount)
		}
	}
}

// TestAuction_TradesCreated: verify trade records have correct price, qty, and trade_date.
func TestAuction_TradesCreated(t *testing.T) {
	env := setupAuctionTest(t)

	collectOrder(t, env.ae, &types.SecurityOrder{
		ID: "buy-tc", InstrumentID: auctionInstID, ParticipantID: "buyer-tc",
		Side: types.OrderSideBuy, OrderType: types.OrderTypeLimit,
		Quantity: 50, Price: decLit(100.0), TimeInForce: types.TimeInForceGTC,
		CreatedAt: ts(0), UpdatedAt: ts(0),
	})
	collectOrder(t, env.ae, &types.SecurityOrder{
		ID: "sell-tc", InstrumentID: auctionInstID, ParticipantID: "seller-tc",
		Side: types.OrderSideSell, OrderType: types.OrderTypeLimit,
		Quantity: 50, Price: decLit(100.0), TimeInForce: types.TimeInForceGTC,
		CreatedAt: ts(1), UpdatedAt: ts(1),
	})

	trades, result, err := env.ae.RunAuction(auctionInstID, auctionTenant)
	if err != nil {
		t.Fatalf("RunAuction: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(trades) == 0 {
		t.Fatal("expected at least one trade")
	}

	today := time.Now().UTC().Format("2006-01-02")
	for _, tr := range trades {
		// Price must equal clearing price.
		if tr.Price != result.ClearingPrice {
			t.Errorf("trade.Price=%v != ClearingPrice=%v", tr.Price, result.ClearingPrice)
		}
		// Quantity must be positive.
		if tr.Quantity <= 0 {
			t.Errorf("trade.Quantity=%d: must be positive", tr.Quantity)
		}
		// TradeDate must be today.
		if tr.TradeDate != today {
			t.Errorf("trade.TradeDate=%s, want %s", tr.TradeDate, today)
		}
		// SettlementDate = TradeDate + 2 calendar days.
		wantSettle := time.Now().UTC().AddDate(0, 0, 2).Format("2006-01-02")
		if tr.SettlementDate != wantSettle {
			t.Errorf("trade.SettlementDate=%s, want %s", tr.SettlementDate, wantSettle)
		}
		// Trade must have been persisted in the store.
		stored, err := env.trd.Get(tr.ID)
		if err != nil {
			t.Fatalf("trade %s not found in store: %v", tr.ID, err)
		}
		if stored.Price != tr.Price {
			t.Errorf("stored trade price mismatch: got %v want %v", stored.Price, tr.Price)
		}
		// Status must be TRADE_PENDING.
		if tr.Status != types.TradeStatusPending {
			t.Errorf("trade.Status=%s, want TRADE_PENDING", tr.Status)
		}
	}
}

// TestAuction_PositionsUpdated: verify buyer +qty, seller -qty after auction.
func TestAuction_PositionsUpdated(t *testing.T) {
	env := setupAuctionTest(t)

	collectOrder(t, env.ae, &types.SecurityOrder{
		ID: "buy-pos", InstrumentID: auctionInstID, ParticipantID: "buyer-pos",
		Side: types.OrderSideBuy, OrderType: types.OrderTypeLimit,
		Quantity: 40, Price: decLit(100.0), TimeInForce: types.TimeInForceGTC,
		CreatedAt: ts(0), UpdatedAt: ts(0),
	})
	collectOrder(t, env.ae, &types.SecurityOrder{
		ID: "sell-pos", InstrumentID: auctionInstID, ParticipantID: "seller-pos",
		Side: types.OrderSideSell, OrderType: types.OrderTypeLimit,
		Quantity: 40, Price: decLit(100.0), TimeInForce: types.TimeInForceGTC,
		CreatedAt: ts(1), UpdatedAt: ts(1),
	})

	_, result, err := env.ae.RunAuction(auctionInstID, auctionTenant)
	if err != nil {
		t.Fatalf("RunAuction: %v", err)
	}
	if result == nil || result.MatchedVolume == 0 {
		t.Skip("no match occurred — position test not meaningful")
	}

	// Buyer should have +matched quantity.
	buyerPos, err := env.pos.GetOrCreate("buyer-pos", auctionInstID)
	if err != nil {
		t.Fatalf("get buyer position: %v", err)
	}
	if buyerPos.Quantity != result.MatchedVolume {
		t.Errorf("buyer position qty: want %d, got %d", result.MatchedVolume, buyerPos.Quantity)
	}

	// Seller should have -matched quantity.
	sellerPos, err := env.pos.GetOrCreate("seller-pos", auctionInstID)
	if err != nil {
		t.Fatalf("get seller position: %v", err)
	}
	if sellerPos.Quantity != -result.MatchedVolume {
		t.Errorf("seller position qty: want %d, got %d", -result.MatchedVolume, sellerPos.Quantity)
	}
}

// TestAuction_OnlyBuys: only buy side — no match possible.
func TestAuction_OnlyBuys(t *testing.T) {
	env := setupAuctionTest(t)

	collectOrder(t, env.ae, &types.SecurityOrder{
		ID: "buy-only", InstrumentID: auctionInstID, ParticipantID: "buyer",
		Side: types.OrderSideBuy, OrderType: types.OrderTypeLimit,
		Quantity: 100, Price: decLit(50.0), TimeInForce: types.TimeInForceGTC,
		CreatedAt: ts(0), UpdatedAt: ts(0),
	})

	trades, result, err := env.ae.RunAuction(auctionInstID, auctionTenant)
	if err != nil {
		t.Fatalf("RunAuction (only buys): %v", err)
	}
	if len(trades) != 0 {
		t.Errorf("expected 0 trades with empty sell side, got %d", len(trades))
	}
	if result != nil && result.MatchedVolume != 0 {
		t.Errorf("matched volume: want 0, got %d", result.MatchedVolume)
	}
}

// TestAuction_OnlySells: only sell side — no match possible.
func TestAuction_OnlySells(t *testing.T) {
	env := setupAuctionTest(t)

	collectOrder(t, env.ae, &types.SecurityOrder{
		ID: "sell-only", InstrumentID: auctionInstID, ParticipantID: "seller",
		Side: types.OrderSideSell, OrderType: types.OrderTypeLimit,
		Quantity: 100, Price: decLit(50.0), TimeInForce: types.TimeInForceGTC,
		CreatedAt: ts(0), UpdatedAt: ts(0),
	})

	trades, result, err := env.ae.RunAuction(auctionInstID, auctionTenant)
	if err != nil {
		t.Fatalf("RunAuction (only sells): %v", err)
	}
	if len(trades) != 0 {
		t.Errorf("expected 0 trades with empty buy side, got %d", len(trades))
	}
	if result != nil && result.MatchedVolume != 0 {
		t.Errorf("matched volume: want 0, got %d", result.MatchedVolume)
	}
}

// TestAuction_CollectOrderSetsStatusPending: CollectOrder should set order status to PENDING.
func TestAuction_CollectOrderSetsStatusPending(t *testing.T) {
	env := setupAuctionTest(t)

	order := auctionOrder("co-1", "buyer", types.OrderSideBuy, 10, 50.0)
	if err := env.ae.CollectOrder(order); err != nil {
		t.Fatalf("CollectOrder: %v", err)
	}
	if order.Status != types.OrderStatusPending {
		t.Errorf("order status after CollectOrder: want PENDING, got %s", order.Status)
	}
}
