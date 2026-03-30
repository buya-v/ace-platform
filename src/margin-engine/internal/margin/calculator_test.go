package margin

import (
	"testing"

	"github.com/garudax-platform/margin-engine/internal/params"
	"github.com/garudax-platform/margin-engine/internal/types"
)

func setupParams() *params.Store {
	store := params.NewStore()
	store.Set(params.InstrumentParams{
		InstrumentID:   "CORN-2026-07",
		PriceScanRange: types.NewDecimal(3, 0),    // $3.00
		VolScanRange:   types.NewDecimal(0, 5000),  // 0.50
		SpotPrice:      types.NewDecimal(450, 0),   // $450.00
		ContractSize:   5000,
	})
	store.Set(params.InstrumentParams{
		InstrumentID:    "WHEAT-2026-09",
		PriceScanRange:  types.NewDecimal(4, 0),    // $4.00
		VolScanRange:    types.NewDecimal(0, 5000),
		SpotPrice:       types.NewDecimal(600, 0),  // $600.00
		ContractSize:    5000,
		DeliveryCharge:  types.NewDecimal(0, 500),  // 0.05 = 5%
		IsDeliveryMonth: true,
	})
	return store
}

func TestCalculateSinglePosition(t *testing.T) {
	calc := NewCalculator(setupParams())

	pos := types.Position{
		ParticipantID: "P1",
		InstrumentID:  "CORN-2026-07",
		NetQuantity:   10,
		AvgEntryPrice: types.NewDecimal(450, 0),
	}

	req, err := calc.Calculate(pos)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.ParticipantID != "P1" {
		t.Errorf("expected participant P1, got %s", req.ParticipantID)
	}
	if req.ScanRisk.IsZero() {
		t.Error("scan risk should be non-zero for non-flat position")
	}
	if !req.DeliveryMonth.IsZero() {
		t.Error("CORN is not in delivery month, delivery charge should be zero")
	}
	if !req.InterMonth.IsZero() {
		t.Error("inter-month should be zero for single instrument")
	}
	if req.TotalRequired.IsZero() {
		t.Error("total required should be non-zero")
	}
}

func TestCalculateDeliveryMonth(t *testing.T) {
	calc := NewCalculator(setupParams())

	pos := types.Position{
		ParticipantID: "P1",
		InstrumentID:  "WHEAT-2026-09",
		NetQuantity:   5,
		AvgEntryPrice: types.NewDecimal(600, 0),
	}

	req, err := calc.Calculate(pos)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.DeliveryMonth.IsZero() {
		t.Error("delivery month charge should be non-zero for delivery month instrument")
	}
	// Delivery charge = 0.05 * $600 * 5000 * 5 = $750,000
	expectedDelivery := types.NewDecimal(0, 500).MulDecimal(types.NewDecimal(600, 0)).MulInt64(5000).MulInt64(5)
	if !req.DeliveryMonth.Equal(expectedDelivery) {
		t.Errorf("expected delivery charge %s, got %s", expectedDelivery.String(), req.DeliveryMonth.String())
	}
}

func TestCalculateMissingInstrument(t *testing.T) {
	calc := NewCalculator(setupParams())

	pos := types.Position{
		ParticipantID: "P1",
		InstrumentID:  "NONEXISTENT",
		NetQuantity:   1,
	}

	_, err := calc.Calculate(pos)
	if err == nil {
		t.Error("expected error for missing instrument params")
	}
}

func TestCalculatePortfolio(t *testing.T) {
	calc := NewCalculator(setupParams())

	positions := []types.Position{
		{
			ParticipantID: "P1",
			InstrumentID:  "CORN-2026-07",
			NetQuantity:   10,
			AvgEntryPrice: types.NewDecimal(450, 0),
		},
		{
			ParticipantID: "P1",
			InstrumentID:  "WHEAT-2026-09",
			NetQuantity:   -3,
			AvgEntryPrice: types.NewDecimal(600, 0),
		},
	}

	collateral := types.DecimalFromInt(1000000) // $1M
	pm, err := calc.CalculatePortfolio("P1", positions, collateral)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pm.ParticipantID != "P1" {
		t.Errorf("expected participant P1, got %s", pm.ParticipantID)
	}
	if len(pm.Requirements) != 2 {
		t.Errorf("expected 2 requirements, got %d", len(pm.Requirements))
	}
	if pm.TotalRequired.IsZero() {
		t.Error("total required should be non-zero")
	}
	if !pm.CollateralOnHand.Equal(collateral) {
		t.Errorf("expected collateral %s, got %s", collateral.String(), pm.CollateralOnHand.String())
	}
}

func TestCalculatePortfolioSkipsFlat(t *testing.T) {
	calc := NewCalculator(setupParams())

	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "CORN-2026-07", NetQuantity: 0},
		{ParticipantID: "P1", InstrumentID: "WHEAT-2026-09", NetQuantity: 5, AvgEntryPrice: types.NewDecimal(600, 0)},
	}

	pm, err := calc.CalculatePortfolio("P1", positions, types.DecimalZero())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only the non-flat position should produce a requirement
	if len(pm.Requirements) != 1 {
		t.Errorf("expected 1 requirement (flat skipped), got %d", len(pm.Requirements))
	}
}

func TestCalculatePortfolioExcess(t *testing.T) {
	calc := NewCalculator(setupParams())

	positions := []types.Position{
		{
			ParticipantID: "P1",
			InstrumentID:  "CORN-2026-07",
			NetQuantity:   1,
			AvgEntryPrice: types.NewDecimal(450, 0),
		},
	}

	// Very large collateral should result in positive excess
	collateral := types.DecimalFromInt(10000000) // $10M
	pm, err := calc.CalculatePortfolio("P1", positions, collateral)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pm.ExcessDeficit.IsNeg() {
		t.Errorf("should have excess with $10M collateral, got deficit: %s", pm.ExcessDeficit.String())
	}
}

func TestCalculatePortfolioDeficit(t *testing.T) {
	calc := NewCalculator(setupParams())

	positions := []types.Position{
		{
			ParticipantID: "P1",
			InstrumentID:  "CORN-2026-07",
			NetQuantity:   100, // Large position
			AvgEntryPrice: types.NewDecimal(450, 0),
		},
	}

	// Zero collateral should result in deficit
	pm, err := calc.CalculatePortfolio("P1", positions, types.DecimalZero())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !pm.ExcessDeficit.IsNeg() {
		t.Errorf("should have deficit with zero collateral, got: %s", pm.ExcessDeficit.String())
	}
}
