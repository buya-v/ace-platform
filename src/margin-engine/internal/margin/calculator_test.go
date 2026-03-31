package margin

import (
	"testing"

	"github.com/garudax-platform/margin-engine/internal/params"
	"github.com/garudax-platform/margin-engine/internal/span"
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

// --- SPAN scanner integration tests ---

func setupSPANScanner() *span.SPANScanner {
	s := span.NewSPANScanner()
	s.LoadRiskArrays([]span.RiskArrayEntry{
		// CORN risk arrays: simplified 4 scenarios for testing
		{InstrumentID: "CORN-2026-07", ScenarioID: 1, PriceShiftPct: types.NewDecimal(3, 0), VolShiftPct: types.NewDecimal(25, 0), PnLImpact: types.NewDecimal(-200, 0)},
		{InstrumentID: "CORN-2026-07", ScenarioID: 2, PriceShiftPct: types.NewDecimal(3, 0), VolShiftPct: types.NewDecimal(-25, 0), PnLImpact: types.NewDecimal(-180, 0)},
		{InstrumentID: "CORN-2026-07", ScenarioID: 3, PriceShiftPct: types.NewDecimal(-3, 0), VolShiftPct: types.NewDecimal(25, 0), PnLImpact: types.NewDecimal(210, 0)},
		{InstrumentID: "CORN-2026-07", ScenarioID: 4, PriceShiftPct: types.NewDecimal(-3, 0), VolShiftPct: types.NewDecimal(-25, 0), PnLImpact: types.NewDecimal(190, 0)},
	})
	return s
}

func TestCalculateWithSPANScanner(t *testing.T) {
	ps := setupParams()
	calc := NewCalculator(ps)
	calc.SetSPANScanner(setupSPANScanner())

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

	// With SPAN risk arrays: worst case is scenario 1, pnl_impact = -200 per contract
	// For 10 contracts: loss = 200 * 10 = 2000
	expected := types.DecimalFromInt(2000)
	if !req.ScanRisk.Equal(expected) {
		t.Errorf("expected SPAN scan risk %s, got %s", expected.String(), req.ScanRisk.String())
	}
}

func TestCalculateWithSPANFallbackForMissing(t *testing.T) {
	ps := setupParams()
	calc := NewCalculator(ps)
	calc.SetSPANScanner(setupSPANScanner()) // Only has CORN risk arrays

	// WHEAT has no risk arrays -- should fall back to percentage-based scanner
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

	// Should use fallback scanner, so scan risk should be non-zero
	if req.ScanRisk.IsZero() {
		t.Error("fallback scanner should produce non-zero scan risk for WHEAT")
	}
	// Delivery month charge should still be calculated
	if req.DeliveryMonth.IsZero() {
		t.Error("delivery month charge should be non-zero for WHEAT")
	}
}

func TestCalculateWithoutSPANMatchesOriginal(t *testing.T) {
	ps := setupParams()

	// Calculator without SPAN
	calcOriginal := NewCalculator(ps)
	// Calculator with SPAN but no risk arrays for the instrument
	calcSPAN := NewCalculator(ps)
	calcSPAN.SetSPANScanner(span.NewSPANScanner()) // Empty scanner

	pos := types.Position{
		ParticipantID: "P1",
		InstrumentID:  "CORN-2026-07",
		NetQuantity:   10,
		AvgEntryPrice: types.NewDecimal(450, 0),
	}

	reqOrig, err := calcOriginal.Calculate(pos)
	if err != nil {
		t.Fatalf("original calc error: %v", err)
	}
	reqSPAN, err := calcSPAN.Calculate(pos)
	if err != nil {
		t.Fatalf("SPAN calc error: %v", err)
	}

	// With empty SPAN scanner (no risk arrays), should fall back and match original
	if !reqOrig.ScanRisk.Equal(reqSPAN.ScanRisk) {
		t.Errorf("fallback should match original: orig=%s span=%s",
			reqOrig.ScanRisk.String(), reqSPAN.ScanRisk.String())
	}
}

func TestPortfolioWithSpreadCredits(t *testing.T) {
	ps := setupParams()
	calc := NewCalculator(ps)

	sc := span.NewSpreadCreditor()
	sc.LoadCredits([]span.SpreadCredit{
		{
			ID:                "SC1",
			LongInstrumentID:  "CORN-2026-07",
			ShortInstrumentID: "WHEAT-2026-09",
			CreditPct:         types.DecimalFromInt(50), // 50% credit
		},
	})
	calc.SetSpreadCreditor(sc)

	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "CORN-2026-07", NetQuantity: 10, AvgEntryPrice: types.NewDecimal(450, 0)},
		{ParticipantID: "P1", InstrumentID: "WHEAT-2026-09", NetQuantity: -5, AvgEntryPrice: types.NewDecimal(600, 0)},
	}

	// With spread credits
	pmWithCredit, err := calc.CalculatePortfolio("P1", positions, types.DecimalFromInt(1000000))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Without spread credits
	calcNoCredit := NewCalculator(ps)
	pmNoCredit, err := calcNoCredit.CalculatePortfolio("P1", positions, types.DecimalFromInt(1000000))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Total required with credits should be <= without credits
	if pmWithCredit.TotalRequired.GreaterThan(pmNoCredit.TotalRequired) {
		t.Errorf("spread credits should reduce margin: with=%s without=%s",
			pmWithCredit.TotalRequired.String(), pmNoCredit.TotalRequired.String())
	}

	// Excess should be >= with credits
	if pmWithCredit.ExcessDeficit.LessThan(pmNoCredit.ExcessDeficit) {
		t.Errorf("spread credits should improve excess: with=%s without=%s",
			pmWithCredit.ExcessDeficit.String(), pmNoCredit.ExcessDeficit.String())
	}
}

func TestPortfolioSingleInstrumentNoSpreadCredit(t *testing.T) {
	ps := setupParams()
	calc := NewCalculator(ps)

	sc := span.NewSpreadCreditor()
	sc.LoadCredits([]span.SpreadCredit{
		{
			ID:                "SC1",
			LongInstrumentID:  "CORN-2026-07",
			ShortInstrumentID: "WHEAT-2026-09",
			CreditPct:         types.DecimalFromInt(50),
		},
	})
	calc.SetSpreadCreditor(sc)

	// Single instrument -- no spread credit should apply
	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "CORN-2026-07", NetQuantity: 10, AvgEntryPrice: types.NewDecimal(450, 0)},
	}

	calcNoCredit := NewCalculator(ps)
	pmWithCredit, err := calc.CalculatePortfolio("P1", positions, types.DecimalFromInt(1000000))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pmNoCredit, err := calcNoCredit.CalculatePortfolio("P1", positions, types.DecimalFromInt(1000000))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Single instrument, no spread credit -- should be identical
	if !pmWithCredit.TotalRequired.Equal(pmNoCredit.TotalRequired) {
		t.Errorf("single-instrument portfolio should not get spread credit: with=%s without=%s",
			pmWithCredit.TotalRequired.String(), pmNoCredit.TotalRequired.String())
	}
}
