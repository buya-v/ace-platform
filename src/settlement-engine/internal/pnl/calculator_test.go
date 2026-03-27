package pnl

import (
	"testing"
	"time"

	"github.com/ace-platform/settlement-engine/internal/types"
	"github.com/ace-platform/settlement-engine/internal/valuation"
)

func setupPriceStore(t *testing.T) (*valuation.Store, time.Time) {
	t.Helper()
	store := valuation.NewStore()
	day1 := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	store.SetSettlementPrice("WHEAT-MAY26", day1, types.NewDecimal(1500, 0))
	store.SetSettlementPrice("WHEAT-MAY26", day2, types.NewDecimal(1520, 0))
	return store, day2
}

func TestCalculateDailyLongProfit(t *testing.T) {
	store, settleDate := setupPriceStore(t)
	calc := NewCalculator(store)

	pos := types.Position{
		ParticipantID: "P1",
		InstrumentID:  "WHEAT-MAY26",
		NetQuantity:   10,
		AvgEntryPrice: types.NewDecimal(1490, 0),
	}

	rec, err := calc.CalculateDaily(pos, settleDate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// (1520 - 1500) * 10 = 200
	expected := types.NewDecimal(200, 0)
	if !rec.VariationMargin.Equal(expected) {
		t.Errorf("expected variation margin %s, got %s", expected, rec.VariationMargin)
	}
	if rec.ParticipantID != "P1" {
		t.Errorf("expected participant P1, got %s", rec.ParticipantID)
	}
}

func TestCalculateDailyLongLoss(t *testing.T) {
	store := valuation.NewStore()
	day1 := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)
	store.SetSettlementPrice("WHEAT-MAY26", day1, types.NewDecimal(1500, 0))
	store.SetSettlementPrice("WHEAT-MAY26", day2, types.NewDecimal(1480, 0))

	calc := NewCalculator(store)
	pos := types.Position{
		ParticipantID: "P1",
		InstrumentID:  "WHEAT-MAY26",
		NetQuantity:   10,
	}

	rec, err := calc.CalculateDaily(pos, day2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// (1480 - 1500) * 10 = -200
	expected := types.NewDecimal(-200, 0)
	if !rec.VariationMargin.Equal(expected) {
		t.Errorf("expected variation margin %s, got %s", expected, rec.VariationMargin)
	}
}

func TestCalculateDailyShortProfit(t *testing.T) {
	store := valuation.NewStore()
	day1 := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)
	store.SetSettlementPrice("WHEAT-MAY26", day1, types.NewDecimal(1500, 0))
	store.SetSettlementPrice("WHEAT-MAY26", day2, types.NewDecimal(1480, 0))

	calc := NewCalculator(store)
	pos := types.Position{
		ParticipantID: "P1",
		InstrumentID:  "WHEAT-MAY26",
		NetQuantity:   -5, // short
	}

	rec, err := calc.CalculateDaily(pos, day2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// (1480 - 1500) * (-5) = 100 (short profits when price falls)
	expected := types.NewDecimal(100, 0)
	if !rec.VariationMargin.Equal(expected) {
		t.Errorf("expected variation margin %s, got %s", expected, rec.VariationMargin)
	}
}

func TestCalculateDailyNewPosition(t *testing.T) {
	// No previous day price — use entry price as reference
	store := valuation.NewStore()
	day2 := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)
	store.SetSettlementPrice("CORN-JUL26", day2, types.NewDecimal(800, 0))

	calc := NewCalculator(store)
	pos := types.Position{
		ParticipantID: "P2",
		InstrumentID:  "CORN-JUL26",
		NetQuantity:   20,
		AvgEntryPrice: types.NewDecimal(790, 0),
	}

	rec, err := calc.CalculateDaily(pos, day2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// (800 - 790) * 20 = 200
	expected := types.NewDecimal(200, 0)
	if !rec.VariationMargin.Equal(expected) {
		t.Errorf("expected variation margin %s, got %s", expected, rec.VariationMargin)
	}
}

func TestCalculateDailyFlatPosition(t *testing.T) {
	store, settleDate := setupPriceStore(t)
	calc := NewCalculator(store)

	pos := types.Position{
		ParticipantID: "P1",
		InstrumentID:  "WHEAT-MAY26",
		NetQuantity:   0,
	}

	rec, err := calc.CalculateDaily(pos, settleDate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !rec.VariationMargin.IsZero() {
		t.Errorf("expected zero variation margin, got %s", rec.VariationMargin)
	}
}

func TestCalculateDailyMissingPrice(t *testing.T) {
	store := valuation.NewStore()
	calc := NewCalculator(store)
	settleDate := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	pos := types.Position{
		ParticipantID: "P1",
		InstrumentID:  "WHEAT-MAY26",
		NetQuantity:   10,
	}

	_, err := calc.CalculateDaily(pos, settleDate)
	if err == nil {
		t.Fatal("expected error for missing settlement price")
	}
}

func TestCalculateBatch(t *testing.T) {
	store, settleDate := setupPriceStore(t)
	calc := NewCalculator(store)

	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "WHEAT-MAY26", NetQuantity: 10},
		{ParticipantID: "P2", InstrumentID: "WHEAT-MAY26", NetQuantity: -10},
	}

	records, err := calc.CalculateBatch(positions, settleDate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	// P1 long: (1520-1500)*10 = 200
	// P2 short: (1520-1500)*(-10) = -200
	// Zero-sum check
	total := records[0].VariationMargin.Add(records[1].VariationMargin)
	if !total.IsZero() {
		t.Errorf("expected zero-sum, got %s", total)
	}
}

func TestNetByParticipant(t *testing.T) {
	records := []types.PnLRecord{
		{ParticipantID: "P1", VariationMargin: types.NewDecimal(200, 0)},
		{ParticipantID: "P1", VariationMargin: types.NewDecimal(-50, 0)},
		{ParticipantID: "P2", VariationMargin: types.NewDecimal(-150, 0)},
	}

	nets := NetByParticipant(records)
	if len(nets) != 2 {
		t.Fatalf("expected 2 participants, got %d", len(nets))
	}

	p1Net := nets["P1"]
	expected := types.NewDecimal(150, 0)
	if !p1Net.Equal(expected) {
		t.Errorf("expected P1 net %s, got %s", expected, p1Net)
	}

	p2Net := nets["P2"]
	expected = types.NewDecimal(-150, 0)
	if !p2Net.Equal(expected) {
		t.Errorf("expected P2 net %s, got %s", expected, p2Net)
	}
}
