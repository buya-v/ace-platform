package engine

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garudax-platform/settlement-engine/internal/payment"
	"github.com/garudax-platform/settlement-engine/internal/types"
	"github.com/garudax-platform/settlement-engine/internal/valuation"
)

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
