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
	return engine.NewMatchingEngine(s.inst, s.ord, s.trd, s.pos, p, nil, nil)
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
// defaultTenantID is the tenant used for all engine tests.
const defaultTenantID = "ace-commodities"

func submitAndMatch(t *testing.T, eng *engine.MatchingEngine, s *testStores, order *types.SecurityOrder) ([]types.SecurityTrade, error) {
	t.Helper()
	submit(t, s, order)
	return eng.MatchOrder(defaultTenantID, order)
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
	_, err := eng.MatchOrder(defaultTenantID, order)
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
	_, err := eng.MatchOrder(defaultTenantID, order)
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
	topic := kafka.TopicTradeExecuted(defaultTenantID)
	prod.RegisterTopic(topic, 64)

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

	recs := prod.Records(topic)
	if len(recs) != 1 {
		t.Fatalf("expected 1 kafka record, got %d", len(recs))
	}
	if recs[0].Topic != topic {
		t.Errorf("topic: want %s, got %s", topic, recs[0].Topic)
	}
	if recs[0].Key != trades[0].ID {
		t.Errorf("partition key: want trade ID %s, got %s", trades[0].ID, recs[0].Key)
	}
}

// ── Self-Trade Prevention tests ───────────────────────────────────────────────

// createInstrumentWithSTP creates a test instrument with the given STP mode.
func createInstrumentWithSTP(t *testing.T, s *testStores, id string, stpMode types.STPMode) {
	t.Helper()
	inst := &types.Instrument{
		ID:            id,
		Ticker:        "STP-TEST",
		Name:          "STP Test Corp",
		AssetClass:    types.AssetClassEquity,
		LotSize:       1,
		TickSize:      0.01,
		TradingStatus: types.TradingStatusActive,
		ExchangeCode:  "MSE",
		STPMode:       stpMode,
		CreatedAt:     ts(0),
		UpdatedAt:     ts(0),
	}
	if err := s.inst.Create(inst); err != nil {
		t.Fatalf("createInstrumentWithSTP: %v", err)
	}
}

// TestSTP_CancelNewest verifies that when STPMode=STP_CANCEL_NEWEST and the
// incoming and resting orders share the same participant, the incoming order
// skips the resting order (no trade produced) and the resting order stays PENDING.
func TestSTP_CancelNewest(t *testing.T) {
	s := newTestStores()
	const stpInstID = "INST-STP-NEWEST"
	createInstrumentWithSTP(t, s, stpInstID, types.STPCancelNewest)
	eng := newEngine(s, nil)

	// Resting sell order from "trader-A".
	sell := limitOrder("sell-stp-newest", stpInstID, "trader-A", types.OrderSideSell, 100, 50.0, ts(0))
	submit(t, s, sell)

	// Incoming buy order from the SAME participant "trader-A".
	buy := limitOrder("buy-stp-newest", stpInstID, "trader-A", types.OrderSideBuy, 100, 50.0, ts(1))
	trades, err := submitAndMatch(t, eng, s, buy)
	if err != nil {
		t.Fatalf("MatchOrder: %v", err)
	}

	// STP_CANCEL_NEWEST: incoming skips the same-participant resting order.
	// No trade should be produced.
	if len(trades) != 0 {
		t.Errorf("STP_CANCEL_NEWEST: want 0 trades, got %d", len(trades))
	}

	// Resting sell should still be PENDING (not cancelled).
	storedSell, err := s.ord.Get("sell-stp-newest")
	if err != nil {
		t.Fatalf("get resting sell: %v", err)
	}
	if storedSell.Status != types.OrderStatusPending {
		t.Errorf("resting sell status: want PENDING, got %s", storedSell.Status)
	}

	// Incoming buy should remain PENDING (not filled, not cancelled).
	if buy.Status != types.OrderStatusPending {
		t.Errorf("incoming buy status: want PENDING, got %s", buy.Status)
	}
}

// TestSTP_CancelNewest_DifferentParticipant confirms that STP_CANCEL_NEWEST
// does NOT prevent trades between different participants.
func TestSTP_CancelNewest_DifferentParticipant(t *testing.T) {
	s := newTestStores()
	const stpInstID = "INST-STP-NEWEST2"
	createInstrumentWithSTP(t, s, stpInstID, types.STPCancelNewest)
	eng := newEngine(s, nil)

	sell := limitOrder("sell-diff", stpInstID, "seller", types.OrderSideSell, 50, 50.0, ts(0))
	submit(t, s, sell)

	buy := limitOrder("buy-diff", stpInstID, "buyer", types.OrderSideBuy, 50, 50.0, ts(1))
	trades, err := submitAndMatch(t, eng, s, buy)
	if err != nil {
		t.Fatalf("MatchOrder: %v", err)
	}
	if len(trades) != 1 {
		t.Errorf("different participants: want 1 trade, got %d", len(trades))
	}
}

// TestSTP_CancelOldest verifies that when STPMode=STP_CANCEL_OLDEST and the
// incoming and resting orders share the same participant, the resting order is
// cancelled and the matching loop continues (incoming may fill against others).
func TestSTP_CancelOldest(t *testing.T) {
	s := newTestStores()
	const stpInstID = "INST-STP-OLDEST"
	createInstrumentWithSTP(t, s, stpInstID, types.STPCancelOldest)
	eng := newEngine(s, nil)

	// Same-participant resting sell — should be cancelled by STP.
	selfSell := limitOrder("sell-self", stpInstID, "trader-B", types.OrderSideSell, 100, 50.0, ts(0))
	submit(t, s, selfSell)

	// Different-participant resting sell — should be matched.
	otherSell := limitOrder("sell-other", stpInstID, "other-seller", types.OrderSideSell, 100, 50.0, ts(1))
	submit(t, s, otherSell)

	// Incoming buy from "trader-B" — same as selfSell.
	buy := limitOrder("buy-oldest", stpInstID, "trader-B", types.OrderSideBuy, 100, 50.0, ts(2))
	trades, err := submitAndMatch(t, eng, s, buy)
	if err != nil {
		t.Fatalf("MatchOrder: %v", err)
	}

	// STP_CANCEL_OLDEST: self sell is cancelled, matching continues with other sell.
	if len(trades) != 1 {
		t.Errorf("STP_CANCEL_OLDEST: want 1 trade (against other-seller), got %d", len(trades))
	}
	if len(trades) > 0 && trades[0].SellOrderID != "sell-other" {
		t.Errorf("trade should be against sell-other, got %s", trades[0].SellOrderID)
	}

	// The self-sell resting order must be CANCELLED.
	storedSelfSell, err := s.ord.Get("sell-self")
	if err != nil {
		t.Fatalf("get self sell: %v", err)
	}
	if storedSelfSell.Status != types.OrderStatusCancelled {
		t.Errorf("self sell status: want CANCELLED (STP_CANCEL_OLDEST), got %s", storedSelfSell.Status)
	}
}

// TestSTP_CancelBoth verifies that when STPMode=STP_CANCEL_BOTH and the
// incoming and resting orders share the same participant, both are cancelled and
// no trades are produced.
func TestSTP_CancelBoth(t *testing.T) {
	s := newTestStores()
	const stpInstID = "INST-STP-BOTH"
	createInstrumentWithSTP(t, s, stpInstID, types.STPCancelBoth)
	eng := newEngine(s, nil)

	// Resting sell from "trader-C".
	sell := limitOrder("sell-both", stpInstID, "trader-C", types.OrderSideSell, 100, 50.0, ts(0))
	submit(t, s, sell)

	// Incoming buy from same participant "trader-C".
	buy := limitOrder("buy-both", stpInstID, "trader-C", types.OrderSideBuy, 100, 50.0, ts(1))
	trades, err := submitAndMatch(t, eng, s, buy)
	if err != nil {
		t.Fatalf("MatchOrder: %v", err)
	}

	// STP_CANCEL_BOTH: both orders cancelled, zero trades.
	if len(trades) != 0 {
		t.Errorf("STP_CANCEL_BOTH: want 0 trades, got %d", len(trades))
	}

	// Resting sell must be CANCELLED.
	storedSell, err := s.ord.Get("sell-both")
	if err != nil {
		t.Fatalf("get resting sell: %v", err)
	}
	if storedSell.Status != types.OrderStatusCancelled {
		t.Errorf("resting sell status: want CANCELLED, got %s", storedSell.Status)
	}

	// Incoming buy must also be CANCELLED.
	if buy.Status != types.OrderStatusCancelled {
		t.Errorf("incoming buy status: want CANCELLED, got %s", buy.Status)
	}
}

// TestSTP_CancelBoth_OnlyMatchesSelf verifies that STP_CANCEL_BOTH cancels
// the self-sell and stops matching, even when other sellers exist.
func TestSTP_CancelBoth_OnlyMatchesSelf(t *testing.T) {
	s := newTestStores()
	const stpInstID = "INST-STP-BOTH2"
	createInstrumentWithSTP(t, s, stpInstID, types.STPCancelBoth)
	eng := newEngine(s, nil)

	// Same-participant resting sell (best price — will be matched first).
	selfSell := limitOrder("sell-self-both", stpInstID, "trader-D", types.OrderSideSell, 50, 50.0, ts(0))
	submit(t, s, selfSell)

	// Different-participant resting sell at the same price.
	otherSell := limitOrder("sell-other-both", stpInstID, "other-seller", types.OrderSideSell, 50, 50.0, ts(1))
	submit(t, s, otherSell)

	// Incoming buy from "trader-D" — triggers STP with selfSell.
	buy := limitOrder("buy-cancel-both", stpInstID, "trader-D", types.OrderSideBuy, 100, 50.0, ts(2))
	trades, err := submitAndMatch(t, eng, s, buy)
	if err != nil {
		t.Fatalf("MatchOrder: %v", err)
	}

	// STP_CANCEL_BOTH stops matching after cancelling both orders.
	// The other-sell should not be matched since incoming is cancelled.
	if len(trades) != 0 {
		t.Errorf("STP_CANCEL_BOTH: want 0 trades, got %d", len(trades))
	}
	if buy.Status != types.OrderStatusCancelled {
		t.Errorf("incoming buy: want CANCELLED, got %s", buy.Status)
	}
}

// ── Iceberg order tests ───────────────────────────────────────────────────────

// icebergOrder builds a PENDING LIMIT iceberg order with an initial visible
// slice and a hidden reserve.
func icebergOrder(id, instID, participantID string, side types.OrderSide,
	total, visible, hidden int, price float64, createdAt string) *types.SecurityOrder {
	return &types.SecurityOrder{
		ID:              id,
		InstrumentID:    instID,
		ParticipantID:   participantID,
		Side:            side,
		OrderType:       types.OrderTypeLimit,
		Quantity:        total,
		Price:           price,
		VisibleQuantity: visible,
		HiddenQuantity:  hidden,
		Status:          types.OrderStatusPending,
		TimeInForce:     types.TimeInForceGTC,
		CreatedAt:       createdAt,
		UpdatedAt:       createdAt,
	}
}

// TestIceberg_BasicMatch verifies that a sell-20 incoming order only matches
// the visible slice of an iceberg buy (100 total: 20 visible + 80 hidden) and
// that visible replenishes to 20 from hidden after the fill.
func TestIceberg_BasicMatch(t *testing.T) {
	eng, s := setup(t)

	// Resting iceberg BUY: 100 total, 20 visible, 80 hidden, price 50.
	ice := icebergOrder("ice-buy-1", testInstID, "buyer", types.OrderSideBuy, 100, 20, 80, 50.00, ts(0))
	submit(t, s, ice)

	// Incoming SELL for exactly the visible slice (20).
	sell := limitOrder("sell-1", testInstID, "seller", types.OrderSideSell, 20, 50.00, ts(1))
	trades, err := submitAndMatch(t, eng, s, sell)
	if err != nil {
		t.Fatalf("MatchOrder: %v", err)
	}
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}
	if trades[0].Quantity != 20 {
		t.Errorf("trade qty: want 20, got %d", trades[0].Quantity)
	}

	// Incoming sell should be fully FILLED.
	if sell.Status != types.OrderStatusFilled {
		t.Errorf("sell status: want FILLED, got %s", sell.Status)
	}

	// Resting iceberg should be PARTIALLY_FILLED.
	storedIce, err := s.ord.Get("ice-buy-1")
	if err != nil {
		t.Fatalf("get iceberg order: %v", err)
	}
	if storedIce.Status != types.OrderStatusPartiallyFilled {
		t.Errorf("iceberg status: want PARTIALLY_FILLED, got %s", storedIce.Status)
	}
	if storedIce.FilledQuantity != 20 {
		t.Errorf("iceberg filled qty: want 20, got %d", storedIce.FilledQuantity)
	}
	// After filling 20 from visible (which was 20), visible exhausted → replenish
	// min(fillQty=20, hiddenQty=80)=20 from hidden. So visible=20, hidden=60.
	if storedIce.VisibleQuantity != 20 {
		t.Errorf("iceberg visible after replenish: want 20, got %d", storedIce.VisibleQuantity)
	}
	if storedIce.HiddenQuantity != 60 {
		t.Errorf("iceberg hidden after replenish: want 60, got %d", storedIce.HiddenQuantity)
	}
}

// TestIceberg_FullDrain verifies that buying the full iceberg quantity (100)
// consumes both visible and hidden slices, leaving the iceberg FILLED.
func TestIceberg_FullDrain(t *testing.T) {
	eng, s := setup(t)

	// Resting iceberg SELL: 100 total, 20 visible, 80 hidden, price 50.
	ice := icebergOrder("ice-sell-1", testInstID, "seller", types.OrderSideSell, 100, 20, 80, 50.00, ts(0))
	submit(t, s, ice)

	// Incoming BUY for the full iceberg quantity.
	// The engine matches visible slices iteratively — but since this is a single
	// matching pass, the incoming BUY will consume 20 (visible) in this pass.
	// We need to drain completely: submit buy for 20, confirm replenish, repeat.
	// To fully drain in one pass we rely on the engine replenish loop within
	// a single MatchOrder call when visible becomes 0.
	// The current engine replenishes once per resting order per match loop
	// iteration, so the simplest full-drain test uses qty=20 (visible only).
	// For a full 100-qty drain, we run 5 sequential buy orders.
	totalFilled := 0
	for round := 1; round <= 5; round++ {
		buyID := "buy-drain-" + string(rune('0'+round))
		buy := limitOrder(buyID, testInstID, "buyer"+string(rune('0'+round)), types.OrderSideBuy, 20, 50.00, ts(round))
		trades, err := submitAndMatch(t, eng, s, buy)
		if err != nil {
			t.Fatalf("round %d MatchOrder: %v", round, err)
		}
		if len(trades) != 1 {
			t.Fatalf("round %d: expected 1 trade, got %d", round, len(trades))
		}
		totalFilled += trades[0].Quantity
	}

	if totalFilled != 100 {
		t.Errorf("total filled across all rounds: want 100, got %d", totalFilled)
	}

	// After 5 fills of 20 each the iceberg must be FILLED.
	storedIce, err := s.ord.Get("ice-sell-1")
	if err != nil {
		t.Fatalf("get iceberg order: %v", err)
	}
	if storedIce.Status != types.OrderStatusFilled {
		t.Errorf("iceberg status after full drain: want FILLED, got %s", storedIce.Status)
	}
	if storedIce.VisibleQuantity != 0 {
		t.Errorf("visible after full drain: want 0, got %d", storedIce.VisibleQuantity)
	}
	if storedIce.HiddenQuantity != 0 {
		t.Errorf("hidden after full drain: want 0, got %d", storedIce.HiddenQuantity)
	}
}

// TestIceberg_RegularOrder verifies that a regular (non-iceberg) order is
// matched normally without any visible/hidden accounting.
func TestIceberg_RegularOrder(t *testing.T) {
	eng, s := setup(t)

	// Resting non-iceberg SELL (VisibleQuantity=0, HiddenQuantity=0).
	sell := limitOrder("sell-reg", testInstID, "seller", types.OrderSideSell, 50, 50.00, ts(0))
	// Confirm these fields are zero (default for limitOrder helper).
	if sell.VisibleQuantity != 0 || sell.HiddenQuantity != 0 {
		t.Fatal("regular order must have VisibleQuantity=0 and HiddenQuantity=0")
	}
	submit(t, s, sell)

	// Incoming regular BUY for 50.
	buy := limitOrder("buy-reg", testInstID, "buyer", types.OrderSideBuy, 50, 50.00, ts(1))
	trades, err := submitAndMatch(t, eng, s, buy)
	if err != nil {
		t.Fatalf("MatchOrder: %v", err)
	}
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}
	if trades[0].Quantity != 50 {
		t.Errorf("trade qty: want 50, got %d", trades[0].Quantity)
	}
	if buy.Status != types.OrderStatusFilled {
		t.Errorf("buy status: want FILLED, got %s", buy.Status)
	}
	storedSell, _ := s.ord.Get("sell-reg")
	if storedSell.Status != types.OrderStatusFilled {
		t.Errorf("sell status: want FILLED, got %s", storedSell.Status)
	}
	// Verify no iceberg fields were set on the stored sell.
	if storedSell.VisibleQuantity != 0 {
		t.Errorf("VisibleQuantity: want 0 (non-iceberg), got %d", storedSell.VisibleQuantity)
	}
	if storedSell.HiddenQuantity != 0 {
		t.Errorf("HiddenQuantity: want 0 (non-iceberg), got %d", storedSell.HiddenQuantity)
	}
}

// TestSTP_DifferentParticipants verifies that when buyers and sellers have
// different participant IDs, normal matching proceeds regardless of STP mode.
func TestSTP_DifferentParticipants(t *testing.T) {
	s := newTestStores()
	const stpInstID = "INST-STP-DIFF"
	createInstrumentWithSTP(t, s, stpInstID, types.STPCancelBoth)
	eng := newEngine(s, nil)

	sell := limitOrder("sell-diff-part", stpInstID, "seller-X", types.OrderSideSell, 100, 75.0, ts(0))
	submit(t, s, sell)

	buy := limitOrder("buy-diff-part", stpInstID, "buyer-Y", types.OrderSideBuy, 100, 75.0, ts(1))
	trades, err := submitAndMatch(t, eng, s, buy)
	if err != nil {
		t.Fatalf("MatchOrder: %v", err)
	}
	if len(trades) != 1 {
		t.Fatalf("different participants with STP_CANCEL_BOTH: want 1 trade, got %d", len(trades))
	}
	if trades[0].Price != 75.0 {
		t.Errorf("trade price: want 75.0, got %v", trades[0].Price)
	}
	if trades[0].Quantity != 100 {
		t.Errorf("trade qty: want 100, got %d", trades[0].Quantity)
	}
	if buy.Status != types.OrderStatusFilled {
		t.Errorf("buy status: want FILLED, got %s", buy.Status)
	}
	storedSell, _ := s.ord.Get("sell-diff-part")
	if storedSell.Status != types.OrderStatusFilled {
		t.Errorf("sell status: want FILLED, got %s", storedSell.Status)
	}
}
