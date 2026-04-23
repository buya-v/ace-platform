// Package engine_test provides comprehensive tests for the MatchingEngine.
package engine_test

import (
	"math"
	"testing"
	"time"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/kafka"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// ---- helpers ----------------------------------------------------------------

// testStores holds all four in-memory stores for easy wiring.
type testStores struct {
	inst *store.InMemoryInstrumentStore
	ord  *store.InMemoryOrderStore
	trd  *store.InMemoryTradeStore
	pos  *store.InMemoryPositionStore
}

const testInstID = "INST-TEST"

// newTestStores creates fresh empty stores.
func newTestStores() *testStores {
	return &testStores{
		inst: store.NewInMemoryInstrumentStore(),
		ord:  store.NewInMemoryOrderStore(),
		trd:  store.NewInMemoryTradeStore(),
		pos:  store.NewInMemoryPositionStore(),
	}
}

// createInstrument adds a test instrument to the instrument store.
func createInstrument(t *testing.T, s *testStores, id string, status types.TradingStatus) {
	t.Helper()
	inst := &types.Instrument{
		ID:            id,
		Ticker:        "TEST",
		Name:          "Test Corp",
		AssetClass:    types.AssetClassEquity,
		LotSize:       1,
		TickSize:      0.01,
		TradingStatus: status,
		ExchangeCode:  "MSE",
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	if err := s.inst.Create(inst); err != nil {
		t.Fatalf("createInstrument: %v", err)
	}
}

// newEngine creates a MatchingEngine backed by the given stores and an optional producer.
func newEngine(s *testStores, p kafka.Producer) *engine.MatchingEngine {
	return engine.NewMatchingEngine(s.inst, s.ord, s.trd, s.pos, p, nil)
}

// setup creates stores + ACTIVE instrument + engine with nil producer.
func setup(t *testing.T) (*engine.MatchingEngine, *testStores) {
	t.Helper()
	s := newTestStores()
	createInstrument(t, s, testInstID, types.TradingStatusActive)
	return newEngine(s, nil), s
}

// ts returns a stable RFC3339 timestamp offset by n seconds from a fixed epoch.
func ts(offset int) string {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return base.Add(time.Duration(offset) * time.Second).Format(time.RFC3339)
}

// limitOrder builds a PENDING LIMIT order with the given parameters.
func limitOrder(id, instID, participantID string, side types.OrderSide, qty int, price float64, createdAt string) *types.SecurityOrder {
	return &types.SecurityOrder{
		ID:            id,
		InstrumentID:  instID,
		ParticipantID: participantID,
		Side:          side,
		OrderType:     types.OrderTypeLimit,
		Quantity:      qty,
		Price:         price,
		Status:        types.OrderStatusPending,
		TimeInForce:   types.TimeInForceGTC,
		CreatedAt:     createdAt,
		UpdatedAt:     createdAt,
	}
}

// marketOrder builds a PENDING MARKET order.
func marketOrder(id, instID, participantID string, side types.OrderSide, qty int, createdAt string) *types.SecurityOrder {
	return &types.SecurityOrder{
		ID:            id,
		InstrumentID:  instID,
		ParticipantID: participantID,
		Side:          side,
		OrderType:     types.OrderTypeMarket,
		Quantity:      qty,
		Price:         0,
		Status:        types.OrderStatusPending,
		TimeInForce:   types.TimeInForceGTC,
		CreatedAt:     createdAt,
		UpdatedAt:     createdAt,
	}
}

// submit submits an order to the order store and fails the test on error.
func submit(t *testing.T, s *testStores, order *types.SecurityOrder) {
	t.Helper()
	if err := s.ord.Submit(order); err != nil {
		t.Fatalf("submit order %s: %v", order.ID, err)
	}
}

// submitAndMatch submits the incoming order to the store first (required for
// engine.Update calls), then calls MatchOrder.
func submitAndMatch(t *testing.T, eng *engine.MatchingEngine, s *testStores, order *types.SecurityOrder) ([]types.SecurityTrade, error) {
	t.Helper()
	submit(t, s, order)
	return eng.MatchOrder(order)
}

// ---- engine tests -----------------------------------------------------------

func TestMatchOrder_LimitBuySellExactCross(t *testing.T) {
	eng, s := setup(t)

	sell := limitOrder("sell-1", testInstID, "seller", types.OrderSideSell, 100, 50.00, ts(0))
	submit(t, s, sell)

	buy := limitOrder("buy-1", testInstID, "buyer", types.OrderSideBuy, 100, 50.00, ts(1))

	trades, err := submitAndMatch(t, eng, s, buy)
	if err != nil {
		t.Fatalf("MatchOrder: %v", err)
	}
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}
	tr := trades[0]
	if tr.Price != 50.00 {
		t.Errorf("trade price: want 50.00, got %v", tr.Price)
	}
	if tr.Quantity != 100 {
		t.Errorf("trade qty: want 100, got %d", tr.Quantity)
	}
	if tr.BuyOrderID != "buy-1" {
		t.Errorf("buy order id: want buy-1, got %s", tr.BuyOrderID)
	}
	if tr.SellOrderID != "sell-1" {
		t.Errorf("sell order id: want sell-1, got %s", tr.SellOrderID)
	}
	if buy.Status != types.OrderStatusFilled {
		t.Errorf("buy status: want FILLED, got %s", buy.Status)
	}
	storedSell, err := s.ord.Get("sell-1")
	if err != nil {
		t.Fatalf("get sell order: %v", err)
	}
	if storedSell.Status != types.OrderStatusFilled {
		t.Errorf("sell status: want FILLED, got %s", storedSell.Status)
	}
	// Verify trade persisted in trade store.
	storedTrade, err := s.trd.Get(tr.ID)
	if err != nil {
		t.Fatalf("get trade from store: %v", err)
	}
	if storedTrade.Quantity != 100 {
		t.Errorf("stored trade qty: want 100, got %d", storedTrade.Quantity)
	}
}

func TestMatchOrder_LimitBuyAboveSell(t *testing.T) {
	eng, s := setup(t)

	sell := limitOrder("sell-1", testInstID, "seller", types.OrderSideSell, 100, 50.00, ts(0))
	submit(t, s, sell)

	buy := limitOrder("buy-1", testInstID, "buyer", types.OrderSideBuy, 100, 55.00, ts(1))

	trades, err := submitAndMatch(t, eng, s, buy)
	if err != nil {
		t.Fatalf("MatchOrder: %v", err)
	}
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}
	// Trade executes at resting (sell) price, not the aggressor's (buy) price.
	if trades[0].Price != 50.00 {
		t.Errorf("trade price: want 50.00 (resting price), got %v", trades[0].Price)
	}
	if trades[0].Quantity != 100 {
		t.Errorf("trade qty: want 100, got %d", trades[0].Quantity)
	}
	if buy.Status != types.OrderStatusFilled {
		t.Errorf("buy status: want FILLED, got %s", buy.Status)
	}
}

func TestMatchOrder_NoMatch(t *testing.T) {
	eng, s := setup(t)

	sell := limitOrder("sell-1", testInstID, "seller", types.OrderSideSell, 100, 100.00, ts(0))
	submit(t, s, sell)

	buy := limitOrder("buy-1", testInstID, "buyer", types.OrderSideBuy, 100, 95.00, ts(1))

	trades, err := submitAndMatch(t, eng, s, buy)
	if err != nil {
		t.Fatalf("MatchOrder: %v", err)
	}
	if len(trades) != 0 {
		t.Errorf("expected 0 trades, got %d", len(trades))
	}
	if buy.Status != types.OrderStatusPending {
		t.Errorf("buy status: want PENDING, got %s", buy.Status)
	}
	storedSell, _ := s.ord.Get("sell-1")
	if storedSell.Status != types.OrderStatusPending {
		t.Errorf("sell status: want PENDING, got %s", storedSell.Status)
	}
}

func TestMatchOrder_PartialFill(t *testing.T) {
	eng, s := setup(t)

	// Sell 100, buy only 60 → sell is PARTIALLY_FILLED.
	sell := limitOrder("sell-1", testInstID, "seller", types.OrderSideSell, 100, 50.00, ts(0))
	submit(t, s, sell)

	buy := limitOrder("buy-1", testInstID, "buyer", types.OrderSideBuy, 60, 50.00, ts(1))

	trades, err := submitAndMatch(t, eng, s, buy)
	if err != nil {
		t.Fatalf("MatchOrder: %v", err)
	}
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}
	if trades[0].Quantity != 60 {
		t.Errorf("trade qty: want 60, got %d", trades[0].Quantity)
	}
	if buy.Status != types.OrderStatusFilled {
		t.Errorf("buy status: want FILLED, got %s", buy.Status)
	}

	storedSell, err := s.ord.Get("sell-1")
	if err != nil {
		t.Fatalf("get sell: %v", err)
	}
	if storedSell.Status != types.OrderStatusPartiallyFilled {
		t.Errorf("sell status: want PARTIALLY_FILLED, got %s", storedSell.Status)
	}
	if storedSell.FilledQuantity != 60 {
		t.Errorf("sell filled qty: want 60, got %d", storedSell.FilledQuantity)
	}
}

func TestMatchOrder_MultipleFills(t *testing.T) {
	// sell 30@50 + sell 30@51, buy 50@51
	// Expected: trade1 → 30@50 (best ask), trade2 → 20@51
	eng, s := setup(t)

	sell1 := limitOrder("sell-1", testInstID, "seller1", types.OrderSideSell, 30, 50.00, ts(0))
	sell2 := limitOrder("sell-2", testInstID, "seller2", types.OrderSideSell, 30, 51.00, ts(1))
	submit(t, s, sell1)
	submit(t, s, sell2)

	buy := limitOrder("buy-1", testInstID, "buyer", types.OrderSideBuy, 50, 51.00, ts(2))

	trades, err := submitAndMatch(t, eng, s, buy)
	if err != nil {
		t.Fatalf("MatchOrder: %v", err)
	}
	if len(trades) != 2 {
		t.Fatalf("expected 2 trades, got %d", len(trades))
	}

	// First trade against sell1 (lower price = better ask).
	if trades[0].Price != 50.00 {
		t.Errorf("trade[0] price: want 50.00, got %v", trades[0].Price)
	}
	if trades[0].Quantity != 30 {
		t.Errorf("trade[0] qty: want 30, got %d", trades[0].Quantity)
	}
	// Second trade against sell2.
	if trades[1].Price != 51.00 {
		t.Errorf("trade[1] price: want 51.00, got %v", trades[1].Price)
	}
	if trades[1].Quantity != 20 {
		t.Errorf("trade[1] qty: want 20, got %d", trades[1].Quantity)
	}

	if buy.Status != types.OrderStatusFilled {
		t.Errorf("buy status: want FILLED, got %s", buy.Status)
	}

	s1, _ := s.ord.Get("sell-1")
	if s1.Status != types.OrderStatusFilled {
		t.Errorf("sell1 status: want FILLED, got %s", s1.Status)
	}
	s2, _ := s.ord.Get("sell-2")
	if s2.Status != types.OrderStatusPartiallyFilled {
		t.Errorf("sell2 status: want PARTIALLY_FILLED, got %s", s2.Status)
	}
	if s2.FilledQuantity != 20 {
		t.Errorf("sell2 filled qty: want 20, got %d", s2.FilledQuantity)
	}
}

func TestMatchOrder_MarketBuy(t *testing.T) {
	eng, s := setup(t)

	sell := limitOrder("sell-1", testInstID, "seller", types.OrderSideSell, 100, 50.00, ts(0))
	submit(t, s, sell)

	buy := marketOrder("buy-mkt", testInstID, "buyer", types.OrderSideBuy, 50, ts(1))

	trades, err := submitAndMatch(t, eng, s, buy)
	if err != nil {
		t.Fatalf("MatchOrder: %v", err)
	}
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}
	// Market order trades at the resting (sell) price.
	if trades[0].Price != 50.00 {
		t.Errorf("trade price: want 50.00, got %v", trades[0].Price)
	}
	if trades[0].Quantity != 50 {
		t.Errorf("trade qty: want 50, got %d", trades[0].Quantity)
	}
}

func TestMatchOrder_MarketBuyNoLiquidity(t *testing.T) {
	eng, s := setup(t)

	buy := marketOrder("buy-mkt", testInstID, "buyer", types.OrderSideBuy, 50, ts(0))

	trades, err := submitAndMatch(t, eng, s, buy)
	if err != nil {
		t.Fatalf("MatchOrder: %v", err)
	}
	if len(trades) != 0 {
		t.Errorf("expected 0 trades (no liquidity), got %d", len(trades))
	}
	if buy.Status != types.OrderStatusPending {
		t.Errorf("buy status: want PENDING, got %s", buy.Status)
	}
}

func TestMatchOrder_PositionUpdate(t *testing.T) {
	eng, s := setup(t)

	sell := limitOrder("sell-1", testInstID, "seller", types.OrderSideSell, 100, 50.00, ts(0))
	submit(t, s, sell)

	buy := limitOrder("buy-1", testInstID, "buyer", types.OrderSideBuy, 100, 50.00, ts(1))
	if _, err := submitAndMatch(t, eng, s, buy); err != nil {
		t.Fatalf("MatchOrder: %v", err)
	}

	// Buyer gets +100.
	buyerPos, err := s.pos.GetOrCreate("buyer", testInstID)
	if err != nil {
		t.Fatalf("get buyer position: %v", err)
	}
	if buyerPos.Quantity != 100 {
		t.Errorf("buyer qty: want 100, got %d", buyerPos.Quantity)
	}

	// Seller gets -100.
	sellerPos, err := s.pos.GetOrCreate("seller", testInstID)
	if err != nil {
		t.Fatalf("get seller position: %v", err)
	}
	if sellerPos.Quantity != -100 {
		t.Errorf("seller qty: want -100, got %d", sellerPos.Quantity)
	}
}

func TestMatchOrder_AvgCostCalculation(t *testing.T) {
	// Buy 100@50 then 100@60 → avg_cost = (100*50+100*60)/200 = 55.00.
	eng, s := setup(t)

	// First fill.
	sell1 := limitOrder("sell-1", testInstID, "seller1", types.OrderSideSell, 100, 50.00, ts(0))
	submit(t, s, sell1)
	buy1 := limitOrder("buy-1", testInstID, "buyer", types.OrderSideBuy, 100, 50.00, ts(1))
	if _, err := submitAndMatch(t, eng, s, buy1); err != nil {
		t.Fatalf("MatchOrder buy1: %v", err)
	}

	// Second fill — need a new resting sell.
	sell2 := limitOrder("sell-2", testInstID, "seller2", types.OrderSideSell, 100, 60.00, ts(2))
	submit(t, s, sell2)
	buy2 := limitOrder("buy-2", testInstID, "buyer", types.OrderSideBuy, 100, 60.00, ts(3))
	if _, err := submitAndMatch(t, eng, s, buy2); err != nil {
		t.Fatalf("MatchOrder buy2: %v", err)
	}

	pos, err := s.pos.GetOrCreate("buyer", testInstID)
	if err != nil {
		t.Fatalf("get buyer position: %v", err)
	}
	if pos.Quantity != 200 {
		t.Errorf("buyer qty: want 200, got %d", pos.Quantity)
	}
	const wantAvg = 55.00
	if math.Abs(pos.AvgCost-wantAvg) > 0.001 {
		t.Errorf("avg_cost: want %.2f, got %.4f", wantAvg, pos.AvgCost)
	}
}

func TestMatchOrder_HaltedInstrument(t *testing.T) {
	s := newTestStores()
	const haltedID = "INST-HALTED"
	createInstrument(t, s, haltedID, types.TradingStatusHalted)
	eng := newEngine(s, nil)

	order := limitOrder("buy-1", haltedID, "buyer", types.OrderSideBuy, 100, 50.00, ts(0))
	_, err := eng.MatchOrder(order)
	if err == nil {
		t.Fatal("expected error for HALTED instrument, got nil")
	}
}

func TestMatchOrder_PriceTimePriority(t *testing.T) {
	// Two sells at the same price — the earlier one must be matched first.
	eng, s := setup(t)

	early := limitOrder("sell-early", testInstID, "seller1", types.OrderSideSell, 50, 100.00, ts(0))
	late := limitOrder("sell-late", testInstID, "seller2", types.OrderSideSell, 50, 100.00, ts(5))
	submit(t, s, early)
	submit(t, s, late)

	buy := limitOrder("buy-1", testInstID, "buyer", types.OrderSideBuy, 50, 100.00, ts(10))
	trades, err := submitAndMatch(t, eng, s, buy)
	if err != nil {
		t.Fatalf("MatchOrder: %v", err)
	}
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}
	if trades[0].SellOrderID != "sell-early" {
		t.Errorf("expected sell-early matched, got %s", trades[0].SellOrderID)
	}

	earlyStored, _ := s.ord.Get("sell-early")
	if earlyStored.Status != types.OrderStatusFilled {
		t.Errorf("sell-early: want FILLED, got %s", earlyStored.Status)
	}
	lateStored, _ := s.ord.Get("sell-late")
	if lateStored.Status != types.OrderStatusPending {
		t.Errorf("sell-late: want PENDING, got %s", lateStored.Status)
	}
}

func TestMatchOrder_UnknownInstrument(t *testing.T) {
	eng, _ := setup(t)

	order := limitOrder("buy-1", "INST-MISSING", "buyer", types.OrderSideBuy, 100, 50.00, ts(0))
	_, err := eng.MatchOrder(order)
	if err == nil {
		t.Fatal("expected error for unknown instrument, got nil")
	}
}

func TestMatchOrder_IncomingSellMatchesBuy(t *testing.T) {
	// Resting buy, incoming sell — verify the "incoming SELL" branch and
	// that updatePositions assigns buyer/seller correctly.
	eng, s := setup(t)

	buy := limitOrder("buy-1", testInstID, "buyer", types.OrderSideBuy, 50, 75.00, ts(0))
	submit(t, s, buy)

	sell := limitOrder("sell-1", testInstID, "seller", types.OrderSideSell, 50, 75.00, ts(1))
	trades, err := submitAndMatch(t, eng, s, sell)
	if err != nil {
		t.Fatalf("MatchOrder: %v", err)
	}
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}
	if trades[0].Price != 75.00 {
		t.Errorf("price: want 75.00, got %v", trades[0].Price)
	}
	if trades[0].BuyOrderID != "buy-1" {
		t.Errorf("BuyOrderID: want buy-1, got %s", trades[0].BuyOrderID)
	}
	if trades[0].SellOrderID != "sell-1" {
		t.Errorf("SellOrderID: want sell-1, got %s", trades[0].SellOrderID)
	}
	if sell.Status != types.OrderStatusFilled {
		t.Errorf("sell status: want FILLED, got %s", sell.Status)
	}
	// Verify position: buyer +50, seller -50.
	buyerPos, _ := s.pos.GetOrCreate("buyer", testInstID)
	if buyerPos.Quantity != 50 {
		t.Errorf("buyer qty: want 50, got %d", buyerPos.Quantity)
	}
	sellerPos, _ := s.pos.GetOrCreate("seller", testInstID)
	if sellerPos.Quantity != -50 {
		t.Errorf("seller qty: want -50, got %d", sellerPos.Quantity)
	}
}

func TestMatchOrder_IncomingSellPartialFill(t *testing.T) {
	// Resting buy 30, incoming sell 50 — resting buy gets FILLED, sell PARTIALLY_FILLED.
	eng, s := setup(t)

	buy := limitOrder("buy-1", testInstID, "buyer", types.OrderSideBuy, 30, 80.00, ts(0))
	submit(t, s, buy)

	sell := limitOrder("sell-1", testInstID, "seller", types.OrderSideSell, 50, 80.00, ts(1))
	trades, err := submitAndMatch(t, eng, s, sell)
	if err != nil {
		t.Fatalf("MatchOrder: %v", err)
	}
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}
	if trades[0].Quantity != 30 {
		t.Errorf("trade qty: want 30, got %d", trades[0].Quantity)
	}
	storedBuy, _ := s.ord.Get("buy-1")
	if storedBuy.Status != types.OrderStatusFilled {
		t.Errorf("buy status: want FILLED, got %s", storedBuy.Status)
	}
	if sell.Status != types.OrderStatusPartiallyFilled {
		t.Errorf("sell status: want PARTIALLY_FILLED, got %s", sell.Status)
	}
}

func TestMatchOrder_MarketSell(t *testing.T) {
	// Market sell against resting limit buy.
	eng, s := setup(t)

	buy := limitOrder("buy-1", testInstID, "buyer", types.OrderSideBuy, 100, 60.00, ts(0))
	submit(t, s, buy)

	sell := marketOrder("sell-mkt", testInstID, "seller", types.OrderSideSell, 40, ts(1))
	trades, err := submitAndMatch(t, eng, s, sell)
	if err != nil {
		t.Fatalf("MatchOrder: %v", err)
	}
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}
	// Trade at resting buy price.
	if trades[0].Price != 60.00 {
		t.Errorf("price: want 60.00, got %v", trades[0].Price)
	}
	if trades[0].Quantity != 40 {
		t.Errorf("qty: want 40, got %d", trades[0].Quantity)
	}
}

func TestMatchOrder_PartiallyFilledRestingOrder(t *testing.T) {
	// Submit a resting sell as PARTIALLY_FILLED — engine should still match it.
	eng, s := setup(t)

	// A sell that already has 40 filled out of 100.
	sell := &types.SecurityOrder{
		ID: "sell-partial", InstrumentID: testInstID, ParticipantID: "seller",
		Side: types.OrderSideSell, OrderType: types.OrderTypeLimit,
		Quantity: 100, FilledQuantity: 40, Price: 50.00,
		Status:      types.OrderStatusPartiallyFilled,
		TimeInForce: types.TimeInForceGTC, CreatedAt: ts(0), UpdatedAt: ts(0),
	}
	submit(t, s, sell)

	// Incoming buy for the remaining 60.
	buy := limitOrder("buy-1", testInstID, "buyer", types.OrderSideBuy, 60, 50.00, ts(1))
	trades, err := submitAndMatch(t, eng, s, buy)
	if err != nil {
		t.Fatalf("MatchOrder: %v", err)
	}
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}
	if trades[0].Quantity != 60 {
		t.Errorf("qty: want 60 (remaining of partial sell), got %d", trades[0].Quantity)
	}
	storedSell, _ := s.ord.Get("sell-partial")
	if storedSell.Status != types.OrderStatusFilled {
		t.Errorf("sell status: want FILLED, got %s", storedSell.Status)
	}
}

func TestMatchOrder_KafkaEventPublished(t *testing.T) {
	// Build everything in-line so we can access all stores and the producer.
	s := newTestStores()
	createInstrument(t, s, testInstID, types.TradingStatusActive)

	prod := kafka.NewChannelProducer(kafka.DefaultProducerConfig())
	prod.RegisterTopic(kafka.TopicTradeExecuted, 64)

	eng := newEngine(s, prod)

	sell := limitOrder("sell-1", testInstID, "seller", types.OrderSideSell, 100, 50.00, ts(0))
	submit(t, s, sell)
	buy := limitOrder("buy-1", testInstID, "buyer", types.OrderSideBuy, 100, 50.00, ts(1))

	trades, err := submitAndMatch(t, eng, s, buy)
	if err != nil {
		t.Fatalf("MatchOrder: %v", err)
	}
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}

	recs := prod.Records(kafka.TopicTradeExecuted)
	if len(recs) != 1 {
		t.Fatalf("expected 1 kafka record, got %d", len(recs))
	}
	if recs[0].Topic != kafka.TopicTradeExecuted {
		t.Errorf("topic: want %s, got %s", kafka.TopicTradeExecuted, recs[0].Topic)
	}
	if recs[0].Key != trades[0].ID {
		t.Errorf("partition key: want trade ID %s, got %s", trades[0].ID, recs[0].Key)
	}
}
