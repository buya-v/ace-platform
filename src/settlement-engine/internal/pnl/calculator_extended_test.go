package pnl

import (
	"testing"
	"time"

	"github.com/garudax-platform/settlement-engine/internal/types"
	"github.com/garudax-platform/settlement-engine/internal/valuation"
)

func TestCalculateBatchErrorPropagation(t *testing.T) {
	// This covers the error path in CalculateBatch (was at 85.7%)
	store := valuation.NewStore()
	calc := NewCalculator(store)
	settleDate := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	// WHEAT has a price, CORN does not
	store.SetSettlementPrice("WHEAT-MAY26", settleDate, types.NewDecimal(1500, 0))

	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "WHEAT-MAY26", NetQuantity: 10},
		{ParticipantID: "P2", InstrumentID: "CORN-JUL26", NetQuantity: 5}, // missing price
	}

	_, err := calc.CalculateBatch(positions, settleDate)
	if err == nil {
		t.Fatal("expected error for missing price in batch")
	}
}

func TestCalculateBatchEmpty(t *testing.T) {
	store := valuation.NewStore()
	calc := NewCalculator(store)
	settleDate := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	records, err := calc.CalculateBatch(nil, settleDate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records, got %d", len(records))
	}
}

func TestCalculateDailyTableDriven(t *testing.T) {
	tests := []struct {
		name          string
		prevPrice     int64 // 0 means no previous price set
		currentPrice  int64
		entryPrice    int64
		quantity      int64
		wantVariation int64 // raw decimal value / 10000
	}{
		{
			name:          "long profit with previous price",
			prevPrice:     1500,
			currentPrice:  1520,
			quantity:      10,
			wantVariation: 200, // (1520-1500)*10
		},
		{
			name:          "long loss with previous price",
			prevPrice:     1500,
			currentPrice:  1480,
			quantity:      10,
			wantVariation: -200, // (1480-1500)*10
		},
		{
			name:          "short profit when price falls",
			prevPrice:     1500,
			currentPrice:  1480,
			quantity:      -5,
			wantVariation: 100, // (1480-1500)*(-5)
		},
		{
			name:          "short loss when price rises",
			prevPrice:     1500,
			currentPrice:  1520,
			quantity:      -5,
			wantVariation: -100, // (1520-1500)*(-5)
		},
		{
			name:          "new position uses entry price",
			prevPrice:     0, // no previous
			currentPrice:  800,
			entryPrice:    790,
			quantity:      20,
			wantVariation: 200, // (800-790)*20
		},
		{
			name:          "new short position uses entry price",
			prevPrice:     0,
			currentPrice:  800,
			entryPrice:    810,
			quantity:      -10,
			wantVariation: 100, // (800-810)*(-10)
		},
		{
			name:          "zero price difference",
			prevPrice:     1500,
			currentPrice:  1500,
			quantity:      100,
			wantVariation: 0,
		},
		{
			name:          "single contract",
			prevPrice:     1000,
			currentPrice:  1001,
			quantity:      1,
			wantVariation: 1,
		},
		{
			name:          "large position",
			prevPrice:     100,
			currentPrice:  101,
			quantity:      10000,
			wantVariation: 10000, // (101-100)*10000
		},
		{
			name:          "negative previous and current prices",
			prevPrice:     -50,
			currentPrice:  -30,
			quantity:      10,
			wantVariation: 200, // (-30-(-50))*10 = 20*10
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := valuation.NewStore()
			day1 := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
			day2 := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

			if tt.prevPrice != 0 {
				store.SetSettlementPrice("INST", day1, types.NewDecimal(tt.prevPrice, 0))
			}
			store.SetSettlementPrice("INST", day2, types.NewDecimal(tt.currentPrice, 0))

			calc := NewCalculator(store)
			pos := types.Position{
				ParticipantID: "P1",
				InstrumentID:  "INST",
				NetQuantity:   tt.quantity,
				AvgEntryPrice: types.NewDecimal(tt.entryPrice, 0),
			}

			rec, err := calc.CalculateDaily(pos, day2)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			expected := types.NewDecimal(tt.wantVariation, 0)
			if !rec.VariationMargin.Equal(expected) {
				t.Errorf("expected variation margin %s, got %s", expected, rec.VariationMargin)
			}
		})
	}
}

func TestCalculateDailyFlatPositionFields(t *testing.T) {
	store := valuation.NewStore()
	day1 := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)
	store.SetSettlementPrice("WHEAT", day1, types.NewDecimal(1500, 0))
	store.SetSettlementPrice("WHEAT", day2, types.NewDecimal(1520, 0))

	calc := NewCalculator(store)
	pos := types.Position{
		ParticipantID: "P1",
		InstrumentID:  "WHEAT",
		NetQuantity:   0,
	}

	rec, err := calc.CalculateDaily(pos, day2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.NetQuantity != 0 {
		t.Errorf("expected net quantity 0, got %d", rec.NetQuantity)
	}
	if rec.ParticipantID != "P1" {
		t.Errorf("expected participant P1, got %s", rec.ParticipantID)
	}
	if rec.InstrumentID != "WHEAT" {
		t.Errorf("expected instrument WHEAT, got %s", rec.InstrumentID)
	}
	if rec.CalculatedAt.IsZero() {
		t.Error("expected non-zero CalculatedAt")
	}
}

func TestCalculateBatchZeroSumMultiInstrument(t *testing.T) {
	store := valuation.NewStore()
	day1 := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	store.SetSettlementPrice("WHEAT", day1, types.NewDecimal(1500, 0))
	store.SetSettlementPrice("WHEAT", day2, types.NewDecimal(1520, 0))
	store.SetSettlementPrice("CORN", day1, types.NewDecimal(800, 0))
	store.SetSettlementPrice("CORN", day2, types.NewDecimal(790, 0))

	calc := NewCalculator(store)

	// Zero-sum: every long has a matching short
	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "WHEAT", NetQuantity: 10},
		{ParticipantID: "P2", InstrumentID: "WHEAT", NetQuantity: -10},
		{ParticipantID: "P1", InstrumentID: "CORN", NetQuantity: -5},
		{ParticipantID: "P2", InstrumentID: "CORN", NetQuantity: 5},
	}

	records, err := calc.CalculateBatch(positions, day2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	total := types.DecimalZero()
	for _, r := range records {
		total = total.Add(r.VariationMargin)
	}
	if !total.IsZero() {
		t.Errorf("expected zero-sum across all positions, got %s", total)
	}
}

func TestCalculateBatchErrorAtFirstPosition(t *testing.T) {
	store := valuation.NewStore()
	calc := NewCalculator(store)
	settleDate := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	// First position has missing price
	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "MISSING", NetQuantity: 10},
	}

	_, err := calc.CalculateBatch(positions, settleDate)
	if err == nil {
		t.Fatal("expected error for missing price")
	}
}

func TestNetByParticipantEmpty(t *testing.T) {
	nets := NetByParticipant(nil)
	if len(nets) != 0 {
		t.Errorf("expected empty map, got %d entries", len(nets))
	}
}

func TestNetByParticipantSingle(t *testing.T) {
	records := []types.PnLRecord{
		{ParticipantID: "P1", VariationMargin: types.NewDecimal(500, 0)},
	}

	nets := NetByParticipant(records)
	if len(nets) != 1 {
		t.Fatalf("expected 1 participant, got %d", len(nets))
	}
	if !nets["P1"].Equal(types.NewDecimal(500, 0)) {
		t.Errorf("expected P1 net 500, got %s", nets["P1"])
	}
}

func TestNetByParticipantManyInstruments(t *testing.T) {
	records := []types.PnLRecord{
		{ParticipantID: "P1", InstrumentID: "WHEAT", VariationMargin: types.NewDecimal(200, 0)},
		{ParticipantID: "P1", InstrumentID: "CORN", VariationMargin: types.NewDecimal(-50, 0)},
		{ParticipantID: "P1", InstrumentID: "SOYBEAN", VariationMargin: types.NewDecimal(100, 0)},
		{ParticipantID: "P2", InstrumentID: "WHEAT", VariationMargin: types.NewDecimal(-200, 0)},
		{ParticipantID: "P2", InstrumentID: "CORN", VariationMargin: types.NewDecimal(50, 0)},
		{ParticipantID: "P2", InstrumentID: "SOYBEAN", VariationMargin: types.NewDecimal(-100, 0)},
	}

	nets := NetByParticipant(records)

	p1Expected := types.NewDecimal(250, 0) // 200 - 50 + 100
	if !nets["P1"].Equal(p1Expected) {
		t.Errorf("expected P1 net %s, got %s", p1Expected, nets["P1"])
	}

	p2Expected := types.NewDecimal(-250, 0) // -200 + 50 - 100
	if !nets["P2"].Equal(p2Expected) {
		t.Errorf("expected P2 net %s, got %s", p2Expected, nets["P2"])
	}
}

func TestNetByParticipantZeroSum(t *testing.T) {
	records := []types.PnLRecord{
		{ParticipantID: "P1", VariationMargin: types.NewDecimal(100, 0)},
		{ParticipantID: "P1", VariationMargin: types.NewDecimal(-100, 0)},
	}

	nets := NetByParticipant(records)
	if !nets["P1"].IsZero() {
		t.Errorf("expected P1 net zero, got %s", nets["P1"])
	}
}

func TestCalculateDailyRecordFields(t *testing.T) {
	store := valuation.NewStore()
	day1 := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)
	store.SetSettlementPrice("WHEAT", day1, types.NewDecimal(1500, 0))
	store.SetSettlementPrice("WHEAT", day2, types.NewDecimal(1520, 0))

	calc := NewCalculator(store)
	pos := types.Position{
		ParticipantID: "ACME",
		InstrumentID:  "WHEAT",
		NetQuantity:   7,
		AvgEntryPrice: types.NewDecimal(1490, 0),
	}

	rec, err := calc.CalculateDaily(pos, day2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.ParticipantID != "ACME" {
		t.Errorf("expected participant ACME, got %s", rec.ParticipantID)
	}
	if rec.InstrumentID != "WHEAT" {
		t.Errorf("expected instrument WHEAT, got %s", rec.InstrumentID)
	}
	if rec.NetQuantity != 7 {
		t.Errorf("expected net quantity 7, got %d", rec.NetQuantity)
	}
	if !rec.PreviousPrice.Equal(types.NewDecimal(1500, 0)) {
		t.Errorf("expected previous price 1500, got %s", rec.PreviousPrice)
	}
	if !rec.CurrentPrice.Equal(types.NewDecimal(1520, 0)) {
		t.Errorf("expected current price 1520, got %s", rec.CurrentPrice)
	}
	// (1520-1500)*7 = 140
	if !rec.VariationMargin.Equal(types.NewDecimal(140, 0)) {
		t.Errorf("expected variation margin 140, got %s", rec.VariationMargin)
	}
	if rec.CalculatedAt.IsZero() {
		t.Error("expected non-zero CalculatedAt")
	}
}

func TestCalculateDailyNewPositionShortLoss(t *testing.T) {
	store := valuation.NewStore()
	day := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)
	store.SetSettlementPrice("CORN", day, types.NewDecimal(800, 0))

	calc := NewCalculator(store)
	pos := types.Position{
		ParticipantID: "P1",
		InstrumentID:  "CORN",
		NetQuantity:   -10,
		AvgEntryPrice: types.NewDecimal(790, 0),
	}

	rec, err := calc.CalculateDaily(pos, day)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// New short: (800 - 790) * (-10) = -100 (loss)
	expected := types.NewDecimal(-100, 0)
	if !rec.VariationMargin.Equal(expected) {
		t.Errorf("expected variation margin %s, got %s", expected, rec.VariationMargin)
	}
}

func TestCalculateDailyFractionalPrices(t *testing.T) {
	store := valuation.NewStore()
	day1 := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	// Prices with fractional components
	store.SetSettlementPrice("OIL", day1, types.NewDecimal(75, 5000)) // 75.5
	store.SetSettlementPrice("OIL", day2, types.NewDecimal(76, 2500)) // 76.25

	calc := NewCalculator(store)
	pos := types.Position{
		ParticipantID: "P1",
		InstrumentID:  "OIL",
		NetQuantity:   4,
	}

	rec, err := calc.CalculateDaily(pos, day2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// (76.25 - 75.5) * 4 = 0.75 * 4 = 3.0
	expected := types.NewDecimal(3, 0)
	if !rec.VariationMargin.Equal(expected) {
		t.Errorf("expected variation margin %s, got %s", expected, rec.VariationMargin)
	}
}
