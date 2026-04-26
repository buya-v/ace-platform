package settlement_test

import (
	"testing"
	"time"

	"github.com/garudax-platform/securities-service/internal/settlement"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

func setupStores(t *testing.T) (*store.InMemoryOrderStore, *store.InMemorySettlementStore) {
	t.Helper()
	return store.NewInMemoryOrderStore(), store.NewInMemorySettlementStore()
}

func makeBuySellOrders(t *testing.T, orderStore *store.InMemoryOrderStore) (string, string) {
	t.Helper()
	buyOrder := &types.SecurityOrder{
		ID:            "BUY-001",
		InstrumentID:  "INST-001",
		ParticipantID: "BUYER-A",
		Side:          types.OrderSideBuy,
		OrderType:     types.OrderTypeLimit,
		Quantity:      100,
		Price:         50.0,
		Status:        types.OrderStatusFilled,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	sellOrder := &types.SecurityOrder{
		ID:            "SELL-001",
		InstrumentID:  "INST-001",
		ParticipantID: "SELLER-B",
		Side:          types.OrderSideSell,
		OrderType:     types.OrderTypeLimit,
		Quantity:      100,
		Price:         50.0,
		Status:        types.OrderStatusFilled,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	if err := orderStore.Submit(buyOrder); err != nil {
		t.Fatal(err)
	}
	if err := orderStore.Submit(sellOrder); err != nil {
		t.Fatal(err)
	}
	return buyOrder.ID, sellOrder.ID
}

func TestCreateObligationsFromTrades(t *testing.T) {
	orderStore, settlementStore := setupStores(t)
	buyID, sellID := makeBuySellOrders(t, orderStore)

	eng := settlement.NewSettlementEngine(orderStore, settlementStore)

	now := time.Now().UTC()
	trades := []types.SecurityTrade{
		{
			ID:             "TRADE-001",
			BuyOrderID:     buyID,
			SellOrderID:    sellID,
			InstrumentID:   "INST-001",
			Price:          50.0,
			Quantity:       100,
			TradeDate:      now.Format("2006-01-02"),
			SettlementDate: now.AddDate(0, 0, 2).Format("2006-01-02"),
			Status:         types.TradeStatusPending,
			CreatedAt:      now.Format(time.RFC3339),
		},
	}

	if err := eng.CreateObligationsFromTrades(trades); err != nil {
		t.Fatalf("CreateObligationsFromTrades: %v", err)
	}

	// Verify obligation was created.
	settlDate := now.AddDate(0, 0, 2).Format("2006-01-02")
	obligations, err := settlementStore.ListByDate(settlDate)
	if err != nil {
		t.Fatalf("ListByDate: %v", err)
	}
	if len(obligations) != 1 {
		t.Fatalf("expected 1 obligation, got %d", len(obligations))
	}

	ob := obligations[0]
	if ob.TradeID != "TRADE-001" {
		t.Errorf("expected TradeID TRADE-001, got %s", ob.TradeID)
	}
	if ob.BuyerParticipantID != "BUYER-A" {
		t.Errorf("expected BuyerParticipantID BUYER-A, got %s", ob.BuyerParticipantID)
	}
	if ob.SellerParticipantID != "SELLER-B" {
		t.Errorf("expected SellerParticipantID SELLER-B, got %s", ob.SellerParticipantID)
	}
	if ob.Quantity != 100 {
		t.Errorf("expected Quantity 100, got %d", ob.Quantity)
	}
	if ob.Price != 50.0 {
		t.Errorf("expected Price 50.0, got %f", ob.Price)
	}
	if ob.NetAmount != 5000.0 {
		t.Errorf("expected NetAmount 5000.0, got %f", ob.NetAmount)
	}
	if ob.Status != types.SettlePending {
		t.Errorf("expected status SETTLE_PENDING, got %s", ob.Status)
	}
}

func TestCreateObligationsFromTrades_InvalidBuyOrder(t *testing.T) {
	orderStore, settlementStore := setupStores(t)
	eng := settlement.NewSettlementEngine(orderStore, settlementStore)

	trades := []types.SecurityTrade{
		{
			ID:           "TRADE-X",
			BuyOrderID:   "NONEXISTENT",
			SellOrderID:  "ALSO-NONEXISTENT",
			InstrumentID: "INST-001",
		},
	}

	err := eng.CreateObligationsFromTrades(trades)
	if err == nil {
		t.Fatal("expected error for nonexistent buy order")
	}
}

func TestProcessSettlementCycle(t *testing.T) {
	orderStore, settlementStore := setupStores(t)
	buyID, sellID := makeBuySellOrders(t, orderStore)

	eng := settlement.NewSettlementEngine(orderStore, settlementStore)

	now := time.Now().UTC()
	settlDate := now.AddDate(0, 0, 2).Format("2006-01-02")

	trades := []types.SecurityTrade{
		{
			ID:             "TRADE-001",
			BuyOrderID:     buyID,
			SellOrderID:    sellID,
			InstrumentID:   "INST-001",
			Price:          50.0,
			Quantity:       100,
			TradeDate:      now.Format("2006-01-02"),
			SettlementDate: settlDate,
			Status:         types.TradeStatusPending,
			CreatedAt:      now.Format(time.RFC3339),
		},
	}

	if err := eng.CreateObligationsFromTrades(trades); err != nil {
		t.Fatalf("CreateObligationsFromTrades: %v", err)
	}

	result, err := eng.ProcessSettlementCycle(settlDate)
	if err != nil {
		t.Fatalf("ProcessSettlementCycle: %v", err)
	}

	if result.Date != settlDate {
		t.Errorf("expected date %s, got %s", settlDate, result.Date)
	}
	if result.Processed != 1 {
		t.Errorf("expected processed=1, got %d", result.Processed)
	}
	if result.Affirmed != 1 {
		t.Errorf("expected affirmed=1, got %d", result.Affirmed)
	}
	if result.Netted != 1 {
		t.Errorf("expected netted=1, got %d", result.Netted)
	}
	if result.Settled != 1 {
		t.Errorf("expected settled=1, got %d", result.Settled)
	}
	if result.Failed != 0 {
		t.Errorf("expected failed=0, got %d", result.Failed)
	}

	// Verify final status is SETTLED.
	obligations, _ := settlementStore.ListByDate(settlDate)
	if len(obligations) != 1 {
		t.Fatalf("expected 1 obligation, got %d", len(obligations))
	}
	if obligations[0].Status != types.SettleSettled {
		t.Errorf("expected status SETTLE_SETTLED, got %s", obligations[0].Status)
	}
}

func TestProcessSettlementCycle_MultipleTrades_SameParties(t *testing.T) {
	orderStore, settlementStore := setupStores(t)

	// Create two buy/sell order pairs for the same parties.
	buy1 := &types.SecurityOrder{ID: "BUY-1", InstrumentID: "INST-001", ParticipantID: "BUYER-A", Side: types.OrderSideBuy, Quantity: 50, Price: 100.0, Status: types.OrderStatusFilled, CreatedAt: time.Now().Format(time.RFC3339)}
	sell1 := &types.SecurityOrder{ID: "SELL-1", InstrumentID: "INST-001", ParticipantID: "SELLER-B", Side: types.OrderSideSell, Quantity: 50, Price: 100.0, Status: types.OrderStatusFilled, CreatedAt: time.Now().Format(time.RFC3339)}
	buy2 := &types.SecurityOrder{ID: "BUY-2", InstrumentID: "INST-001", ParticipantID: "BUYER-A", Side: types.OrderSideBuy, Quantity: 30, Price: 105.0, Status: types.OrderStatusFilled, CreatedAt: time.Now().Format(time.RFC3339)}
	sell2 := &types.SecurityOrder{ID: "SELL-2", InstrumentID: "INST-001", ParticipantID: "SELLER-B", Side: types.OrderSideSell, Quantity: 30, Price: 105.0, Status: types.OrderStatusFilled, CreatedAt: time.Now().Format(time.RFC3339)}
	for _, o := range []*types.SecurityOrder{buy1, sell1, buy2, sell2} {
		if err := orderStore.Submit(o); err != nil {
			t.Fatal(err)
		}
	}

	eng := settlement.NewSettlementEngine(orderStore, settlementStore)
	settlDate := "2026-04-25"

	trades := []types.SecurityTrade{
		{ID: "T1", BuyOrderID: "BUY-1", SellOrderID: "SELL-1", InstrumentID: "INST-001", Price: 100.0, Quantity: 50, SettlementDate: settlDate, Status: types.TradeStatusPending, CreatedAt: time.Now().Format(time.RFC3339)},
		{ID: "T2", BuyOrderID: "BUY-2", SellOrderID: "SELL-2", InstrumentID: "INST-001", Price: 105.0, Quantity: 30, SettlementDate: settlDate, Status: types.TradeStatusPending, CreatedAt: time.Now().Format(time.RFC3339)},
	}
	if err := eng.CreateObligationsFromTrades(trades); err != nil {
		t.Fatalf("CreateObligationsFromTrades: %v", err)
	}

	result, err := eng.ProcessSettlementCycle(settlDate)
	if err != nil {
		t.Fatalf("ProcessSettlementCycle: %v", err)
	}

	if result.Processed != 2 {
		t.Errorf("expected processed=2, got %d", result.Processed)
	}
	if result.Affirmed != 2 {
		t.Errorf("expected affirmed=2, got %d", result.Affirmed)
	}
	// Both share same (instrument, buyer, seller) so they get netted together.
	if result.Netted != 2 {
		t.Errorf("expected netted=2, got %d", result.Netted)
	}
	if result.Settled != 2 {
		t.Errorf("expected settled=2, got %d", result.Settled)
	}
}

func TestProcessSettlementCycle_EmptyDate(t *testing.T) {
	orderStore, settlementStore := setupStores(t)
	eng := settlement.NewSettlementEngine(orderStore, settlementStore)

	result, err := eng.ProcessSettlementCycle("2026-12-31")
	if err != nil {
		t.Fatalf("ProcessSettlementCycle: %v", err)
	}
	if result.Processed != 0 {
		t.Errorf("expected processed=0 for empty date, got %d", result.Processed)
	}
}

func TestProcessSettlementCycle_AlreadyAffirmed(t *testing.T) {
	orderStore, settlementStore := setupStores(t)
	buyID, sellID := makeBuySellOrders(t, orderStore)
	eng := settlement.NewSettlementEngine(orderStore, settlementStore)

	settlDate := "2026-04-25"
	trades := []types.SecurityTrade{
		{ID: "T-AF", BuyOrderID: buyID, SellOrderID: sellID, InstrumentID: "INST-001", Price: 10.0, Quantity: 10, SettlementDate: settlDate, Status: types.TradeStatusPending, CreatedAt: time.Now().Format(time.RFC3339)},
	}
	if err := eng.CreateObligationsFromTrades(trades); err != nil {
		t.Fatal(err)
	}

	// Run cycle once to settle.
	result1, err := eng.ProcessSettlementCycle(settlDate)
	if err != nil {
		t.Fatal(err)
	}
	if result1.Settled != 1 {
		t.Fatalf("first cycle: expected settled=1, got %d", result1.Settled)
	}

	// Run cycle again — already settled, nothing to do.
	result2, err := eng.ProcessSettlementCycle(settlDate)
	if err != nil {
		t.Fatal(err)
	}
	if result2.Affirmed != 0 {
		t.Errorf("second cycle: expected affirmed=0, got %d", result2.Affirmed)
	}
	if result2.Settled != 0 {
		t.Errorf("second cycle: expected settled=0, got %d", result2.Settled)
	}
}

// ── Accrued interest tests (T5) ───────────────────────────────────────────────

// TestSettlement_BondAccruedInterest verifies that when a bond instrument is
// traded and the settlement engine has a BondStore configured, the settlement
// obligation's NetAmount includes accrued interest computed via the bond's
// day-count convention (ACT/365, defaulting to 30 days since last coupon).
//
// Formula (ACT/365, 30 days):
//
//	accrued = 30 * couponRate * parValue / 365 * quantity
func TestSettlement_BondAccruedInterest(t *testing.T) {
	orderStore, settlementStore := setupStores(t)

	const bondInstID = "BOND-001"
	const couponRate = 0.05  // 5% annual
	const parValue = 1000.0  // MNT 1,000 par
	const tradeQty = 10      // 10 bonds
	const tradePricePerBond = 990.0

	// Create buy/sell orders for the bond instrument.
	buyOrder := &types.SecurityOrder{
		ID:            "BOND-BUY-001",
		InstrumentID:  bondInstID,
		ParticipantID: "BUYER-BOND",
		Side:          types.OrderSideBuy,
		OrderType:     types.OrderTypeLimit,
		Quantity:      tradeQty,
		Price:         tradePricePerBond,
		Status:        types.OrderStatusFilled,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	sellOrder := &types.SecurityOrder{
		ID:            "BOND-SELL-001",
		InstrumentID:  bondInstID,
		ParticipantID: "SELLER-BOND",
		Side:          types.OrderSideSell,
		OrderType:     types.OrderTypeLimit,
		Quantity:      tradeQty,
		Price:         tradePricePerBond,
		Status:        types.OrderStatusFilled,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	if err := orderStore.Submit(buyOrder); err != nil {
		t.Fatalf("submit buy order: %v", err)
	}
	if err := orderStore.Submit(sellOrder); err != nil {
		t.Fatalf("submit sell order: %v", err)
	}

	// Set up bond store with the test bond.
	bondStore := store.NewInMemoryBondStore()
	bond := &types.Bond{
		ID:                 bondInstID,
		CouponRate:         couponRate,
		ParValue:           parValue,
		DayCountConvention: types.DayCountACT365,
	}
	if err := bondStore.Create(bond); err != nil {
		t.Fatalf("create bond: %v", err)
	}

	// Wire settlement engine with bond store.
	eng := settlement.NewSettlementEngine(orderStore, settlementStore)
	eng.SetBondStore(bondStore)

	settlDate := "2026-04-28"
	trades := []types.SecurityTrade{
		{
			ID:             "BOND-TRADE-001",
			BuyOrderID:     buyOrder.ID,
			SellOrderID:    sellOrder.ID,
			InstrumentID:   bondInstID,
			Price:          tradePricePerBond,
			Quantity:       tradeQty,
			TradeDate:      "2026-04-26",
			SettlementDate: settlDate,
			Status:         types.TradeStatusPending,
			CreatedAt:      time.Now().UTC().Format(time.RFC3339),
		},
	}
	if err := eng.CreateObligationsFromTrades(trades); err != nil {
		t.Fatalf("CreateObligationsFromTrades: %v", err)
	}

	// Run settlement cycle — this triggers accrued interest calculation.
	result, err := eng.ProcessSettlementCycle(settlDate)
	if err != nil {
		t.Fatalf("ProcessSettlementCycle: %v", err)
	}
	if result.Settled != 1 {
		t.Fatalf("expected settled=1, got %d", result.Settled)
	}

	// Fetch the settled obligation and verify NetAmount includes accrued interest.
	obligations, err := settlementStore.ListByDate(settlDate)
	if err != nil {
		t.Fatalf("ListByDate: %v", err)
	}
	if len(obligations) != 1 {
		t.Fatalf("expected 1 obligation, got %d", len(obligations))
	}

	ob := obligations[0]

	// Base net amount: price * quantity.
	baseAmount := tradePricePerBond * float64(tradeQty)

	// Expected accrued interest per bond (ACT/365, 30 days default):
	//   accrued_per_bond = 30 * 0.05 * 1000 / 365 = 4.1095...
	//   total accrued    = accrued_per_bond * quantity = 41.095...
	const defaultDays = 30
	accruedPerBond := float64(defaultDays) * couponRate * parValue / 365.0
	totalAccrued := accruedPerBond * float64(tradeQty)
	expectedNetAmount := baseAmount + totalAccrued

	if ob.AccruedInterest == 0 {
		t.Error("expected non-zero AccruedInterest for bond settlement")
	}
	const epsilon = 0.001
	diff := ob.AccruedInterest - totalAccrued
	if diff < -epsilon || diff > epsilon {
		t.Errorf("AccruedInterest: want %.4f, got %.4f", totalAccrued, ob.AccruedInterest)
	}

	netDiff := ob.NetAmount - expectedNetAmount
	if netDiff < -epsilon || netDiff > epsilon {
		t.Errorf("NetAmount: want %.4f (base %.2f + accrued %.4f), got %.4f",
			expectedNetAmount, baseAmount, totalAccrued, ob.NetAmount)
	}
}
