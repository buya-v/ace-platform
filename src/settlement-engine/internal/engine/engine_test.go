package engine

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garudax-platform/settlement-engine/internal/dvp"
	"github.com/garudax-platform/settlement-engine/internal/payment"
	"github.com/garudax-platform/settlement-engine/internal/types"
	"github.com/garudax-platform/settlement-engine/internal/valuation"
)

// --- DVP test doubles for engine integration tests ---

type testDeliveryValidator struct {
	validateErr map[string]error
	confirmErr  map[string]error
}

func newTestDeliveryValidator() *testDeliveryValidator {
	return &testDeliveryValidator{
		validateErr: make(map[string]error),
		confirmErr:  make(map[string]error),
	}
}

func (v *testDeliveryValidator) ValidateReceipt(receipt types.DeliveryReceipt) error {
	if err, ok := v.validateErr[receipt.ReceiptID]; ok {
		return err
	}
	return nil
}

func (v *testDeliveryValidator) ConfirmDelivery(receipt types.DeliveryReceipt) (types.DeliveryReceipt, error) {
	if err, ok := v.confirmErr[receipt.ReceiptID]; ok {
		return receipt, err
	}
	receipt.Status = types.DeliveryConfirmed
	receipt.ConfirmedAt = time.Now()
	return receipt, nil
}

func (v *testDeliveryValidator) RollbackDelivery(_ types.DeliveryReceipt) error {
	return nil
}

type testPaymentLocker struct {
	counter uint64
	lockErr map[string]error
}

func newTestPaymentLocker() *testPaymentLocker {
	return &testPaymentLocker{lockErr: make(map[string]error)}
}

func (l *testPaymentLocker) LockFunds(participantID string, _ types.Decimal) (string, error) {
	if err, ok := l.lockErr[participantID]; ok {
		return "", err
	}
	n := atomic.AddUint64(&l.counter, 1)
	return fmt.Sprintf("lock-%d", n), nil
}

func (l *testPaymentLocker) ReleaseFunds(_ string) error { return nil }
func (l *testPaymentLocker) UnlockFunds(_ string) error  { return nil }

type seqIDGen struct {
	counter uint64
}

func (g *seqIDGen) NewID() string {
	n := atomic.AddUint64(&g.counter, 1)
	return fmt.Sprintf("test-%d", n)
}

func setupEngine(t *testing.T) (*Engine, *valuation.Store, *payment.InMemoryGateway) {
	t.Helper()
	priceStore := valuation.NewStore()
	gw := payment.NewInMemoryGateway()
	eng := NewEngine(priceStore, &seqIDGen{}, gw)
	return eng, priceStore, gw
}

func TestRunSettlementCycleSuccess(t *testing.T) {
	eng, priceStore, _ := setupEngine(t)

	day1 := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	priceStore.SetSettlementPrice("WHEAT-MAY26", day1, types.NewDecimal(1500, 0))
	priceStore.SetSettlementPrice("WHEAT-MAY26", day2, types.NewDecimal(1520, 0))

	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "WHEAT-MAY26", NetQuantity: 10},
		{ParticipantID: "P2", InstrumentID: "WHEAT-MAY26", NetQuantity: -10},
	}

	cycle, err := eng.RunSettlementCycle("cycle-1", day2, positions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cycle.Status != types.CycleStatusCompleted {
		t.Errorf("expected COMPLETED, got %s", cycle.Status)
	}
	if len(cycle.PnLRecords) != 2 {
		t.Errorf("expected 2 P&L records, got %d", len(cycle.PnLRecords))
	}
	if len(cycle.Instructions) != 2 {
		t.Errorf("expected 2 instructions, got %d", len(cycle.Instructions))
	}

	// Zero-sum: pay-in should equal pay-out
	if !cycle.TotalPayIn.Equal(cycle.TotalPayOut) {
		t.Errorf("expected zero-sum: pay_in=%s pay_out=%s", cycle.TotalPayIn, cycle.TotalPayOut)
	}

	// P1 gains (1520-1500)*10 = 200
	if !cycle.TotalPayOut.Equal(types.NewDecimal(200, 0)) {
		t.Errorf("expected total payout 200, got %s", cycle.TotalPayOut)
	}
}

func TestRunSettlementCyclePaymentFailure(t *testing.T) {
	eng, priceStore, gw := setupEngine(t)

	day1 := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	priceStore.SetSettlementPrice("WHEAT-MAY26", day1, types.NewDecimal(1500, 0))
	priceStore.SetSettlementPrice("WHEAT-MAY26", day2, types.NewDecimal(1520, 0))

	gw.SetFail("P2")

	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "WHEAT-MAY26", NetQuantity: 10},
		{ParticipantID: "P2", InstrumentID: "WHEAT-MAY26", NetQuantity: -10},
	}

	cycle, err := eng.RunSettlementCycle("cycle-1", day2, positions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cycle.Status != types.CycleStatusFailed {
		t.Errorf("expected FAILED, got %s", cycle.Status)
	}
	if cycle.Error == "" {
		t.Error("expected error message")
	}
}

func TestRunSettlementCycleMissingPrice(t *testing.T) {
	eng, _, _ := setupEngine(t)

	settleDate := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)
	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "WHEAT-MAY26", NetQuantity: 10},
	}

	_, err := eng.RunSettlementCycle("cycle-1", settleDate, positions)
	if err == nil {
		t.Fatal("expected error for missing settlement price")
	}
}

func TestRunSettlementCycleMultipleInstruments(t *testing.T) {
	eng, priceStore, _ := setupEngine(t)

	day1 := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	priceStore.SetSettlementPrice("WHEAT-MAY26", day1, types.NewDecimal(1500, 0))
	priceStore.SetSettlementPrice("WHEAT-MAY26", day2, types.NewDecimal(1520, 0))
	priceStore.SetSettlementPrice("CORN-JUL26", day1, types.NewDecimal(800, 0))
	priceStore.SetSettlementPrice("CORN-JUL26", day2, types.NewDecimal(790, 0))

	positions := []types.Position{
		// P1 long wheat (+200) and long corn (-100) = net +100
		{ParticipantID: "P1", InstrumentID: "WHEAT-MAY26", NetQuantity: 10},
		{ParticipantID: "P1", InstrumentID: "CORN-JUL26", NetQuantity: 10},
		// P2 short wheat (-200) and short corn (+100) = net -100
		{ParticipantID: "P2", InstrumentID: "WHEAT-MAY26", NetQuantity: -10},
		{ParticipantID: "P2", InstrumentID: "CORN-JUL26", NetQuantity: -10},
	}

	cycle, err := eng.RunSettlementCycle("cycle-1", day2, positions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cycle.Status != types.CycleStatusCompleted {
		t.Errorf("expected COMPLETED, got %s", cycle.Status)
	}
	if len(cycle.PnLRecords) != 4 {
		t.Errorf("expected 4 P&L records, got %d", len(cycle.PnLRecords))
	}
	// Net instructions: 2 participants, each with one net instruction
	if len(cycle.Instructions) != 2 {
		t.Errorf("expected 2 net instructions, got %d", len(cycle.Instructions))
	}
	if !cycle.TotalPayIn.Equal(cycle.TotalPayOut) {
		t.Errorf("expected zero-sum: pay_in=%s pay_out=%s", cycle.TotalPayIn, cycle.TotalPayOut)
	}
}

func TestGetCycle(t *testing.T) {
	eng, priceStore, _ := setupEngine(t)

	day1 := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)
	priceStore.SetSettlementPrice("WHEAT-MAY26", day1, types.NewDecimal(1500, 0))
	priceStore.SetSettlementPrice("WHEAT-MAY26", day2, types.NewDecimal(1520, 0))

	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "WHEAT-MAY26", NetQuantity: 10},
		{ParticipantID: "P2", InstrumentID: "WHEAT-MAY26", NetQuantity: -10},
	}

	eng.RunSettlementCycle("cycle-1", day2, positions)

	cycle, ok := eng.GetCycle("cycle-1")
	if !ok {
		t.Fatal("expected to find cycle-1")
	}
	if cycle.CycleID != "cycle-1" {
		t.Errorf("expected cycle ID cycle-1, got %s", cycle.CycleID)
	}

	_, ok = eng.GetCycle("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent cycle")
	}
}

func TestGetAllCycles(t *testing.T) {
	eng, priceStore, _ := setupEngine(t)

	day1 := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)
	priceStore.SetSettlementPrice("WHEAT-MAY26", day1, types.NewDecimal(1500, 0))
	priceStore.SetSettlementPrice("WHEAT-MAY26", day2, types.NewDecimal(1520, 0))

	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "WHEAT-MAY26", NetQuantity: 10},
		{ParticipantID: "P2", InstrumentID: "WHEAT-MAY26", NetQuantity: -10},
	}

	eng.RunSettlementCycle("cycle-1", day2, positions)
	eng.RunSettlementCycle("cycle-2", day2, positions)

	cycles := eng.GetAllCycles()
	if len(cycles) != 2 {
		t.Errorf("expected 2 cycles, got %d", len(cycles))
	}
}

func TestCycleHandler(t *testing.T) {
	eng, priceStore, _ := setupEngine(t)

	day1 := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)
	priceStore.SetSettlementPrice("WHEAT-MAY26", day1, types.NewDecimal(1500, 0))
	priceStore.SetSettlementPrice("WHEAT-MAY26", day2, types.NewDecimal(1520, 0))

	var handledCycle types.SettlementCycle
	eng.SetCycleHandler(func(cycle types.SettlementCycle) {
		handledCycle = cycle
	})

	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "WHEAT-MAY26", NetQuantity: 10},
		{ParticipantID: "P2", InstrumentID: "WHEAT-MAY26", NetQuantity: -10},
	}

	eng.RunSettlementCycle("cycle-1", day2, positions)

	if handledCycle.CycleID != "cycle-1" {
		t.Errorf("expected handler to receive cycle-1, got %s", handledCycle.CycleID)
	}
}

func TestSetSettlementPrice(t *testing.T) {
	eng, _, _ := setupEngine(t)

	date := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)
	eng.SetSettlementPrice("WHEAT-MAY26", date, types.NewDecimal(1520, 0))

	sp, err := eng.GetPriceStore().GetSettlementPrice("WHEAT-MAY26", date)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sp.SettlementPrice.Equal(types.NewDecimal(1520, 0)) {
		t.Errorf("expected 1520, got %s", sp.SettlementPrice)
	}
}

func TestConcurrentSettlementCycles(t *testing.T) {
	eng, priceStore, _ := setupEngine(t)

	day1 := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)
	priceStore.SetSettlementPrice("WHEAT-MAY26", day1, types.NewDecimal(1500, 0))
	priceStore.SetSettlementPrice("WHEAT-MAY26", day2, types.NewDecimal(1520, 0))

	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "WHEAT-MAY26", NetQuantity: 10},
		{ParticipantID: "P2", InstrumentID: "WHEAT-MAY26", NetQuantity: -10},
	}

	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			cycleID := fmt.Sprintf("cycle-%d", n)
			_, err := eng.RunSettlementCycle(cycleID, day2, positions)
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent settlement failed: %v", err)
	}

	cycles := eng.GetAllCycles()
	if len(cycles) != 10 {
		t.Errorf("expected 10 cycles, got %d", len(cycles))
	}
}

// --- Multi-instrument cycle tests ---

func TestMultiInstrumentCycleAggregation(t *testing.T) {
	eng, priceStore, _ := setupEngine(t)

	day1 := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	// Wheat: 1500 -> 1520 (+20 per contract)
	priceStore.SetSettlementPrice("WHEAT-MAY26", day1, types.NewDecimal(1500, 0))
	priceStore.SetSettlementPrice("WHEAT-MAY26", day2, types.NewDecimal(1520, 0))
	// Corn: 800 -> 790 (-10 per contract)
	priceStore.SetSettlementPrice("CORN-JUL26", day1, types.NewDecimal(800, 0))
	priceStore.SetSettlementPrice("CORN-JUL26", day2, types.NewDecimal(790, 0))
	// Gold: 2000 -> 2050 (+50 per contract)
	priceStore.SetSettlementPrice("GOLD-AUG26", day1, types.NewDecimal(2000, 0))
	priceStore.SetSettlementPrice("GOLD-AUG26", day2, types.NewDecimal(2050, 0))

	positions := []types.Position{
		// P1: long wheat (+200), long corn (-100), long gold (+500) = net +600
		{ParticipantID: "P1", InstrumentID: "WHEAT-MAY26", NetQuantity: 10},
		{ParticipantID: "P1", InstrumentID: "CORN-JUL26", NetQuantity: 10},
		{ParticipantID: "P1", InstrumentID: "GOLD-AUG26", NetQuantity: 10},
		// P2: short wheat (-200), short corn (+100), short gold (-500) = net -600
		{ParticipantID: "P2", InstrumentID: "WHEAT-MAY26", NetQuantity: -10},
		{ParticipantID: "P2", InstrumentID: "CORN-JUL26", NetQuantity: -10},
		{ParticipantID: "P2", InstrumentID: "GOLD-AUG26", NetQuantity: -10},
	}

	cycle, multiResult, err := eng.RunMultiInstrumentCycle("multi-1", day2, positions, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cycle.Status != types.CycleStatusCompleted {
		t.Errorf("expected COMPLETED, got %s (error: %s)", cycle.Status, cycle.Error)
	}

	// Should have 6 P&L records (3 instruments x 2 participants)
	if len(cycle.PnLRecords) != 6 {
		t.Errorf("expected 6 P&L records, got %d", len(cycle.PnLRecords))
	}

	// Should have results for 3 instruments
	if len(multiResult.InstrumentResults) != 3 {
		t.Errorf("expected 3 instrument results, got %d", len(multiResult.InstrumentResults))
	}

	// Net participant amounts: P1 = +600, P2 = -600
	p1Net := multiResult.NetParticipantAmounts["P1"]
	if !p1Net.Equal(types.NewDecimal(600, 0)) {
		t.Errorf("expected P1 net +600, got %s", p1Net)
	}
	p2Net := multiResult.NetParticipantAmounts["P2"]
	if !p2Net.Equal(types.NewDecimal(-600, 0)) {
		t.Errorf("expected P2 net -600, got %s", p2Net)
	}

	// Zero-sum check
	if !multiResult.AggregatedPayIn.Equal(multiResult.AggregatedPayOut) {
		t.Errorf("expected zero-sum: pay_in=%s pay_out=%s",
			multiResult.AggregatedPayIn, multiResult.AggregatedPayOut)
	}
	if !multiResult.AggregatedPayOut.Equal(types.NewDecimal(600, 0)) {
		t.Errorf("expected aggregated payout 600, got %s", multiResult.AggregatedPayOut)
	}
}

func TestMultiInstrumentCycleWithDVP(t *testing.T) {
	eng, priceStore, _ := setupEngine(t)

	validator := newTestDeliveryValidator()
	locker := newTestPaymentLocker()
	coord := dvp.NewDVPCoordinator(validator, locker, &seqIDGen{counter: 100})
	eng.SetDVPCoordinator(coord)

	// Register gold as physical delivery
	eng.RegisterInstrument(types.InstrumentConfig{
		InstrumentID: "GOLD-SPOT",
		Type:         types.InstrumentPhysicalDelivery,
		ContractUnit: "OZ",
		ContractSize: 100,
	})
	// Wheat is cash-settled (default, not registered)

	day1 := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	priceStore.SetSettlementPrice("WHEAT-MAY26", day1, types.NewDecimal(1500, 0))
	priceStore.SetSettlementPrice("WHEAT-MAY26", day2, types.NewDecimal(1520, 0))
	priceStore.SetSettlementPrice("GOLD-SPOT", day1, types.NewDecimal(2000, 0))
	priceStore.SetSettlementPrice("GOLD-SPOT", day2, types.NewDecimal(2050, 0))

	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "WHEAT-MAY26", NetQuantity: 10},
		{ParticipantID: "P2", InstrumentID: "WHEAT-MAY26", NetQuantity: -10},
		{ParticipantID: "P1", InstrumentID: "GOLD-SPOT", NetQuantity: 5},
		{ParticipantID: "P2", InstrumentID: "GOLD-SPOT", NetQuantity: -5},
	}

	deliveryReceipts := map[string][]types.DeliveryReceipt{
		"GOLD-SPOT": {
			{
				ReceiptID:    "receipt-gold-1",
				InstrumentID: "GOLD-SPOT",
				SellerID:     "P2",
				BuyerID:      "P1",
				Quantity:     500,
				WarehouseID:  "WH-GOLD",
				Status:       types.DeliveryPending,
				IssuedAt:     time.Now(),
			},
		},
	}

	cycle, multiResult, err := eng.RunMultiInstrumentCycle("multi-dvp-1", day2, positions, deliveryReceipts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cycle.Status != types.CycleStatusCompleted {
		t.Errorf("expected COMPLETED, got %s (error: %s)", cycle.Status, cycle.Error)
	}

	// Check wheat result (cash-settled, no DVP)
	wheatResult, ok := multiResult.InstrumentResults["WHEAT-MAY26"]
	if !ok {
		t.Fatal("expected wheat instrument result")
	}
	if wheatResult.InstrumentType != types.InstrumentCashSettled {
		t.Errorf("expected wheat cash-settled, got %s", wheatResult.InstrumentType)
	}
	if wheatResult.DVPResult != nil {
		t.Error("expected no DVP result for cash-settled wheat")
	}

	// Check gold result (physical delivery with DVP)
	goldResult, ok := multiResult.InstrumentResults["GOLD-SPOT"]
	if !ok {
		t.Fatal("expected gold instrument result")
	}
	if goldResult.InstrumentType != types.InstrumentPhysicalDelivery {
		t.Errorf("expected gold physical delivery, got %s", goldResult.InstrumentType)
	}
	if goldResult.DVPResult == nil {
		t.Fatal("expected DVP result for physical delivery gold")
	}
	if goldResult.DVPResult.Status != types.DVPSucceeded {
		t.Errorf("expected DVP SUCCEEDED, got %s (error: %s)",
			goldResult.DVPResult.Status, goldResult.DVPResult.Error)
	}
}

func TestMultiInstrumentCycleDVPFailure(t *testing.T) {
	eng, priceStore, _ := setupEngine(t)

	validator := newTestDeliveryValidator()
	locker := newTestPaymentLocker()
	coord := dvp.NewDVPCoordinator(validator, locker, &seqIDGen{counter: 200})
	eng.SetDVPCoordinator(coord)

	eng.RegisterInstrument(types.InstrumentConfig{
		InstrumentID: "GOLD-SPOT",
		Type:         types.InstrumentPhysicalDelivery,
	})

	day1 := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	priceStore.SetSettlementPrice("WHEAT-MAY26", day1, types.NewDecimal(1500, 0))
	priceStore.SetSettlementPrice("WHEAT-MAY26", day2, types.NewDecimal(1520, 0))
	priceStore.SetSettlementPrice("GOLD-SPOT", day1, types.NewDecimal(2000, 0))
	priceStore.SetSettlementPrice("GOLD-SPOT", day2, types.NewDecimal(2050, 0))

	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "WHEAT-MAY26", NetQuantity: 10},
		{ParticipantID: "P2", InstrumentID: "WHEAT-MAY26", NetQuantity: -10},
		{ParticipantID: "P1", InstrumentID: "GOLD-SPOT", NetQuantity: 5},
		{ParticipantID: "P2", InstrumentID: "GOLD-SPOT", NetQuantity: -5},
	}

	// Delivery validation will fail for gold
	validator.validateErr["receipt-gold-1"] = fmt.Errorf("gold purity below 99.5%%")

	deliveryReceipts := map[string][]types.DeliveryReceipt{
		"GOLD-SPOT": {
			{
				ReceiptID:    "receipt-gold-1",
				InstrumentID: "GOLD-SPOT",
				SellerID:     "P2",
				BuyerID:      "P1",
				Quantity:     500,
				Status:       types.DeliveryPending,
				IssuedAt:     time.Now(),
			},
		},
	}

	cycle, multiResult, err := eng.RunMultiInstrumentCycle("multi-dvp-fail", day2, positions, deliveryReceipts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Cycle should fail because one instrument's DVP failed
	if cycle.Status != types.CycleStatusFailed {
		t.Errorf("expected FAILED, got %s", cycle.Status)
	}

	// Gold DVP should have failed
	goldResult := multiResult.InstrumentResults["GOLD-SPOT"]
	if goldResult.DVPResult.Status == types.DVPSucceeded {
		t.Error("expected gold DVP to fail")
	}
	if goldResult.Error == "" {
		t.Error("expected error on gold instrument result")
	}

	// Wheat should still have valid P&L
	wheatResult := multiResult.InstrumentResults["WHEAT-MAY26"]
	if len(wheatResult.PnLRecords) != 2 {
		t.Errorf("expected 2 wheat P&L records, got %d", len(wheatResult.PnLRecords))
	}
}

func TestMultiInstrumentCycleMissingPriceForOneInstrument(t *testing.T) {
	eng, priceStore, _ := setupEngine(t)

	day1 := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	// Only set wheat prices, not corn
	priceStore.SetSettlementPrice("WHEAT-MAY26", day1, types.NewDecimal(1500, 0))
	priceStore.SetSettlementPrice("WHEAT-MAY26", day2, types.NewDecimal(1520, 0))

	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "WHEAT-MAY26", NetQuantity: 10},
		{ParticipantID: "P2", InstrumentID: "WHEAT-MAY26", NetQuantity: -10},
		{ParticipantID: "P1", InstrumentID: "CORN-JUL26", NetQuantity: 5},
		{ParticipantID: "P2", InstrumentID: "CORN-JUL26", NetQuantity: -5},
	}

	cycle, multiResult, err := eng.RunMultiInstrumentCycle("multi-partial", day2, positions, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should fail because corn price is missing
	if cycle.Status != types.CycleStatusFailed {
		t.Errorf("expected FAILED, got %s", cycle.Status)
	}

	// Wheat should have valid results
	wheatResult := multiResult.InstrumentResults["WHEAT-MAY26"]
	if wheatResult.Error != "" {
		t.Errorf("expected no error on wheat, got %s", wheatResult.Error)
	}
	if len(wheatResult.PnLRecords) != 2 {
		t.Errorf("expected 2 wheat P&L records, got %d", len(wheatResult.PnLRecords))
	}

	// Corn should have an error
	cornResult := multiResult.InstrumentResults["CORN-JUL26"]
	if cornResult.Error == "" {
		t.Error("expected error on corn result")
	}
}

func TestRegisterInstrument(t *testing.T) {
	eng, _, _ := setupEngine(t)

	eng.RegisterInstrument(types.InstrumentConfig{
		InstrumentID: "GOLD-SPOT",
		Type:         types.InstrumentPhysicalDelivery,
		ContractUnit: "OZ",
		ContractSize: 100,
	})

	cfg := eng.getInstrumentConfig("GOLD-SPOT")
	if cfg.Type != types.InstrumentPhysicalDelivery {
		t.Errorf("expected PHYSICAL_DELIVERY, got %s", cfg.Type)
	}
	if cfg.ContractUnit != "OZ" {
		t.Errorf("expected OZ, got %s", cfg.ContractUnit)
	}

	// Unregistered instrument defaults to cash-settled
	cfg = eng.getInstrumentConfig("UNKNOWN-INSTR")
	if cfg.Type != types.InstrumentCashSettled {
		t.Errorf("expected CASH_SETTLED for unregistered instrument, got %s", cfg.Type)
	}
}

func TestMultiInstrumentCycleEmptyPositions(t *testing.T) {
	eng, _, _ := setupEngine(t)

	day2 := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	cycle, multiResult, err := eng.RunMultiInstrumentCycle("empty-1", day2, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cycle.Status != types.CycleStatusCompleted {
		t.Errorf("expected COMPLETED for empty positions, got %s", cycle.Status)
	}
	if len(multiResult.InstrumentResults) != 0 {
		t.Errorf("expected 0 instrument results, got %d", len(multiResult.InstrumentResults))
	}
}
