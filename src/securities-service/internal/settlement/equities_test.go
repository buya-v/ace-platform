package settlement_test

// Equities-specific settlement-cycle tests (mse-equities flagship tenant).
//
// MSE equities settle on a T+2 profile; certain instruments (e.g. government
// bonds / money-market lines) settle T+1. This file validates that the
// settlement engine correctly drives obligations through the affirm → net →
// settle lifecycle when fed dates produced by each settlement profile, and
// that cycles for different settlement dates stay isolated from one another.
//
// The settlement engine settles by the SettlementDate carried on each trade,
// so the "profile" is the rule that maps a trade date to a settlement date.
// settlementProfile below encodes that rule (calendar offset + weekend roll)
// and acts as the executable spec the trade-capture layer is expected to honor.

import (
	"testing"
	"time"

	"github.com/garudax-platform/securities-service/internal/settlement"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// settlementProfile maps a trade date to a settlement date for an equities venue.
type settlementProfile struct {
	name        string
	offsetDays  int // business days from trade date to settlement date
	rollWeekend bool
}

var (
	profileT1 = settlementProfile{name: "T+1", offsetDays: 1, rollWeekend: true}
	profileT2 = settlementProfile{name: "T+2", offsetDays: 2, rollWeekend: true}
)

// settlementDate advances tradeDate by the profile's business-day offset,
// skipping Saturdays and Sundays when rollWeekend is set. Returns an ISO date.
func (p settlementProfile) settlementDate(tradeDate time.Time) string {
	d := tradeDate
	added := 0
	for added < p.offsetDays {
		d = d.AddDate(0, 0, 1)
		if p.rollWeekend && (d.Weekday() == time.Saturday || d.Weekday() == time.Sunday) {
			continue
		}
		added++
	}
	return d.Format("2006-01-02")
}

// makeOrderPair submits a buy/sell order pair and returns their IDs.
func makeOrderPair(t *testing.T, os *store.InMemoryOrderStore, idSuffix, inst, buyer, seller string, qty int, price float64) (string, string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	buy := &types.SecurityOrder{
		ID: "BUY-" + idSuffix, InstrumentID: inst, ParticipantID: buyer,
		Side: types.OrderSideBuy, OrderType: types.OrderTypeLimit,
		Quantity: qty, Price: decLit(price), Status: types.OrderStatusFilled, CreatedAt: now,
	}
	sell := &types.SecurityOrder{
		ID: "SELL-" + idSuffix, InstrumentID: inst, ParticipantID: seller,
		Side: types.OrderSideSell, OrderType: types.OrderTypeLimit,
		Quantity: qty, Price: decLit(price), Status: types.OrderStatusFilled, CreatedAt: now,
	}
	if err := os.Submit(buy); err != nil {
		t.Fatalf("submit buy %s: %v", idSuffix, err)
	}
	if err := os.Submit(sell); err != nil {
		t.Fatalf("submit sell %s: %v", idSuffix, err)
	}
	return buy.ID, sell.ID
}

// TestEquitiesSettlementProfile_DateComputation verifies the profile date rule
// for both T+1 and T+2, including weekend rolling.
func TestEquitiesSettlementProfile_DateComputation(t *testing.T) {
	cases := []struct {
		name    string
		profile settlementProfile
		trade   string // ISO trade date
		want    string // expected settlement date
	}{
		// Monday 2026-06-15 trade.
		{"T+1 mid-week", profileT1, "2026-06-15", "2026-06-16"},
		{"T+2 mid-week", profileT2, "2026-06-15", "2026-06-17"},
		// Thursday 2026-06-18 trade — T+2 must roll over Sat/Sun to Monday.
		{"T+1 Thu->Fri", profileT1, "2026-06-18", "2026-06-19"},
		{"T+2 Thu->Mon", profileT2, "2026-06-18", "2026-06-22"},
		// Friday 2026-06-19 trade — T+1 rolls to Monday.
		{"T+1 Fri->Mon", profileT1, "2026-06-19", "2026-06-22"},
		{"T+2 Fri->Tue", profileT2, "2026-06-19", "2026-06-23"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tradeDate, err := time.Parse("2006-01-02", tc.trade)
			if err != nil {
				t.Fatalf("parse trade date: %v", err)
			}
			got := tc.profile.settlementDate(tradeDate)
			if got != tc.want {
				t.Errorf("%s settlement of %s: want %s, got %s",
					tc.profile.name, tc.trade, tc.want, got)
			}
		})
	}
}

// runProfileCycle is shared by the T+1 and T+2 lifecycle tests: it creates a
// single obligation whose settlement date is derived from the given profile,
// runs the cycle for that date, and asserts the obligation settles cleanly.
func runProfileCycle(t *testing.T, p settlementProfile) {
	t.Helper()
	orderStore, settlementStore := setupStores(t)
	buyID, sellID := makeOrderPair(t, orderStore, p.name+"-1", "MSE-APU", "BUYER-A", "SELLER-B", 100, 12.5)

	eng := settlement.NewSettlementEngine(orderStore, settlementStore)

	tradeDate := time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC) // Monday
	settlDate := p.settlementDate(tradeDate)

	trades := []types.SecurityTrade{{
		ID: "EQ-" + p.name, BuyOrderID: buyID, SellOrderID: sellID,
		InstrumentID: "MSE-APU", Price: decLit(12.5), Quantity: 100,
		TradeDate: tradeDate.Format("2006-01-02"), SettlementDate: settlDate,
		Status: types.TradeStatusPending, CreatedAt: tradeDate.Format(time.RFC3339),
	}}
	if err := eng.CreateObligationsFromTrades(trades); err != nil {
		t.Fatalf("CreateObligationsFromTrades: %v", err)
	}

	// Obligation should carry the profile-derived settlement date and be PENDING.
	pending, err := settlementStore.ListByDate(settlDate)
	if err != nil {
		t.Fatalf("ListByDate: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("%s: expected 1 obligation on %s, got %d", p.name, settlDate, len(pending))
	}
	if pending[0].SettlementDate != settlDate {
		t.Errorf("%s: obligation SettlementDate = %s, want %s", p.name, pending[0].SettlementDate, settlDate)
	}
	if pending[0].Status != types.SettlePending {
		t.Errorf("%s: expected SETTLE_PENDING before cycle, got %s", p.name, pending[0].Status)
	}
	if pending[0].NetAmount != decLit(1250.0) {
		t.Errorf("%s: NetAmount = %.2f, want 1250.00", p.name, pending[0].NetAmount.Float64())
	}

	result, err := eng.ProcessSettlementCycle(settlDate)
	if err != nil {
		t.Fatalf("%s: ProcessSettlementCycle: %v", p.name, err)
	}
	if result.Processed != 1 || result.Affirmed != 1 || result.Netted != 1 || result.Settled != 1 || result.Failed != 0 {
		t.Errorf("%s: unexpected result %+v (want processed/affirmed/netted/settled=1, failed=0)", p.name, result)
	}

	settled, _ := settlementStore.ListByDate(settlDate)
	if len(settled) != 1 || settled[0].Status != types.SettleSettled {
		t.Fatalf("%s: expected obligation SETTLE_SETTLED, got %+v", p.name, settled)
	}
}

// TestEquitiesSettlement_T1Profile validates the full lifecycle for a T+1 line.
func TestEquitiesSettlement_T1Profile(t *testing.T) { runProfileCycle(t, profileT1) }

// TestEquitiesSettlement_T2Profile validates the full lifecycle for a T+2 line.
func TestEquitiesSettlement_T2Profile(t *testing.T) { runProfileCycle(t, profileT2) }

// TestEquitiesSettlement_MixedProfiles_CyclesAreIsolated verifies that when a
// T+1 trade and a T+2 trade are captured on the same trade date, running the
// settlement cycle for the T+1 date settles ONLY the T+1 obligation and leaves
// the T+2 obligation untouched until its own settlement date arrives.
func TestEquitiesSettlement_MixedProfiles_CyclesAreIsolated(t *testing.T) {
	orderStore, settlementStore := setupStores(t)

	tradeDate := time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC) // Monday
	t1Date := profileT1.settlementDate(tradeDate)                     // 2026-06-16
	t2Date := profileT2.settlementDate(tradeDate)                     // 2026-06-17
	if t1Date == t2Date {
		t.Fatalf("test setup broken: T+1 (%s) and T+2 (%s) resolved to same date", t1Date, t2Date)
	}

	buyA, sellA := makeOrderPair(t, orderStore, "MIX-T1", "MSE-BOND", "BUYER-A", "SELLER-B", 10, 1000.0)
	buyB, sellB := makeOrderPair(t, orderStore, "MIX-T2", "MSE-APU", "BUYER-C", "SELLER-D", 50, 20.0)

	trades := []types.SecurityTrade{
		{ID: "T1-TRADE", BuyOrderID: buyA, SellOrderID: sellA, InstrumentID: "MSE-BOND", Price: decLit(1000.0), Quantity: 10, TradeDate: tradeDate.Format("2006-01-02"), SettlementDate: t1Date, Status: types.TradeStatusPending, CreatedAt: tradeDate.Format(time.RFC3339)},
		{ID: "T2-TRADE", BuyOrderID: buyB, SellOrderID: sellB, InstrumentID: "MSE-APU", Price: decLit(20.0), Quantity: 50, TradeDate: tradeDate.Format("2006-01-02"), SettlementDate: t2Date, Status: types.TradeStatusPending, CreatedAt: tradeDate.Format(time.RFC3339)},
	}
	if err := eng2(orderStore, settlementStore).CreateObligationsFromTrades(trades); err != nil {
		t.Fatalf("CreateObligationsFromTrades: %v", err)
	}

	// Run the T+1 cycle. Only the T+1 obligation should settle.
	eng := settlement.NewSettlementEngine(orderStore, settlementStore)
	r1, err := eng.ProcessSettlementCycle(t1Date)
	if err != nil {
		t.Fatalf("ProcessSettlementCycle(T+1): %v", err)
	}
	if r1.Processed != 1 || r1.Settled != 1 {
		t.Errorf("T+1 cycle: want processed=1 settled=1, got %+v", r1)
	}

	// The T+2 obligation must still be PENDING and unsettled.
	t2Obs, _ := settlementStore.ListByDate(t2Date)
	if len(t2Obs) != 1 {
		t.Fatalf("expected 1 obligation on T+2 date, got %d", len(t2Obs))
	}
	if t2Obs[0].Status != types.SettlePending {
		t.Errorf("T+2 obligation should remain SETTLE_PENDING after T+1 cycle, got %s", t2Obs[0].Status)
	}

	// Now run the T+2 cycle; the remaining obligation settles.
	r2, err := eng.ProcessSettlementCycle(t2Date)
	if err != nil {
		t.Fatalf("ProcessSettlementCycle(T+2): %v", err)
	}
	if r2.Processed != 1 || r2.Settled != 1 {
		t.Errorf("T+2 cycle: want processed=1 settled=1, got %+v", r2)
	}

	t1Obs, _ := settlementStore.ListByDate(t1Date)
	if len(t1Obs) != 1 || t1Obs[0].Status != types.SettleSettled {
		t.Errorf("T+1 obligation should be SETTLE_SETTLED, got %+v", t1Obs)
	}
	t2Obs, _ = settlementStore.ListByDate(t2Date)
	if len(t2Obs) != 1 || t2Obs[0].Status != types.SettleSettled {
		t.Errorf("T+2 obligation should be SETTLE_SETTLED, got %+v", t2Obs)
	}
}

// TestEquitiesSettlement_T2Netting_SameParties verifies that multiple T+2 trades
// between the same (instrument, buyer, seller) on the same settlement date are
// all driven to SETTLED in a single cycle (multilateral netting group).
func TestEquitiesSettlement_T2Netting_SameParties(t *testing.T) {
	orderStore, settlementStore := setupStores(t)

	tradeDate := time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC)
	settlDate := profileT2.settlementDate(tradeDate)

	buy1, sell1 := makeOrderPair(t, orderStore, "NET-1", "MSE-APU", "BUYER-A", "SELLER-B", 40, 11.0)
	buy2, sell2 := makeOrderPair(t, orderStore, "NET-2", "MSE-APU", "BUYER-A", "SELLER-B", 60, 11.5)

	eng := settlement.NewSettlementEngine(orderStore, settlementStore)
	trades := []types.SecurityTrade{
		{ID: "NT-1", BuyOrderID: buy1, SellOrderID: sell1, InstrumentID: "MSE-APU", Price: decLit(11.0), Quantity: 40, SettlementDate: settlDate, Status: types.TradeStatusPending, CreatedAt: tradeDate.Format(time.RFC3339)},
		{ID: "NT-2", BuyOrderID: buy2, SellOrderID: sell2, InstrumentID: "MSE-APU", Price: decLit(11.5), Quantity: 60, SettlementDate: settlDate, Status: types.TradeStatusPending, CreatedAt: tradeDate.Format(time.RFC3339)},
	}
	if err := eng.CreateObligationsFromTrades(trades); err != nil {
		t.Fatalf("CreateObligationsFromTrades: %v", err)
	}

	result, err := eng.ProcessSettlementCycle(settlDate)
	if err != nil {
		t.Fatalf("ProcessSettlementCycle: %v", err)
	}
	if result.Processed != 2 || result.Affirmed != 2 || result.Netted != 2 || result.Settled != 2 {
		t.Errorf("T+2 netting: want processed/affirmed/netted/settled=2, got %+v", result)
	}
}

// TestEquitiesSettlement_WrongDateCycle_NoOp verifies that running a cycle for a
// date that does not match the obligation's (profile-derived) settlement date
// settles nothing — guarding against premature settlement.
func TestEquitiesSettlement_WrongDateCycle_NoOp(t *testing.T) {
	orderStore, settlementStore := setupStores(t)

	tradeDate := time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC)
	settlDate := profileT2.settlementDate(tradeDate)
	tradeDateStr := tradeDate.Format("2006-01-02")

	buyID, sellID := makeOrderPair(t, orderStore, "WRONG", "MSE-APU", "BUYER-A", "SELLER-B", 100, 9.0)
	eng := settlement.NewSettlementEngine(orderStore, settlementStore)
	trades := []types.SecurityTrade{{
		ID: "WRONG-TRADE", BuyOrderID: buyID, SellOrderID: sellID, InstrumentID: "MSE-APU",
		Price: decLit(9.0), Quantity: 100, SettlementDate: settlDate, Status: types.TradeStatusPending,
		CreatedAt: tradeDate.Format(time.RFC3339),
	}}
	if err := eng.CreateObligationsFromTrades(trades); err != nil {
		t.Fatalf("CreateObligationsFromTrades: %v", err)
	}

	// Running the cycle on the trade date (T+0) must settle nothing.
	result, err := eng.ProcessSettlementCycle(tradeDateStr)
	if err != nil {
		t.Fatalf("ProcessSettlementCycle(T+0): %v", err)
	}
	if result.Processed != 0 || result.Settled != 0 {
		t.Errorf("T+0 cycle should be a no-op, got %+v", result)
	}

	// The obligation must remain PENDING on its real settlement date.
	obs, _ := settlementStore.ListByDate(settlDate)
	if len(obs) != 1 || obs[0].Status != types.SettlePending {
		t.Errorf("obligation should remain SETTLE_PENDING until its settlement date, got %+v", obs)
	}
}

// eng2 is a tiny constructor alias used where a freshly-wired engine is needed
// inline for obligation creation, keeping the mixed-profile test readable.
func eng2(os *store.InMemoryOrderStore, ss *store.InMemorySettlementStore) *settlement.SettlementEngine {
	return settlement.NewSettlementEngine(os, ss)
}
