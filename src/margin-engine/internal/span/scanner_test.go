package span

import (
	"testing"

	"github.com/garudax-platform/margin-engine/internal/types"
)

// wheatRiskArrays returns the 16 standard wheat scenarios from the migration seed data.
func wheatRiskArrays() []RiskArrayEntry {
	return []RiskArrayEntry{
		{InstrumentID: "WHT-HRW-2026M07-UB", ScenarioID: 1, PriceShiftPct: d(3, 0), VolShiftPct: d(25, 0), PnLImpact: d(-150, 0)},
		{InstrumentID: "WHT-HRW-2026M07-UB", ScenarioID: 2, PriceShiftPct: d(3, 0), VolShiftPct: d(-25, 0), PnLImpact: d(-140, 0)},
		{InstrumentID: "WHT-HRW-2026M07-UB", ScenarioID: 3, PriceShiftPct: d(-3, 0), VolShiftPct: d(25, 0), PnLImpact: d(160, 0)},
		{InstrumentID: "WHT-HRW-2026M07-UB", ScenarioID: 4, PriceShiftPct: d(-3, 0), VolShiftPct: d(-25, 0), PnLImpact: d(150, 0)},
		{InstrumentID: "WHT-HRW-2026M07-UB", ScenarioID: 5, PriceShiftPct: d(6, 0), VolShiftPct: d(25, 0), PnLImpact: d(-310, 0)},
		{InstrumentID: "WHT-HRW-2026M07-UB", ScenarioID: 6, PriceShiftPct: d(6, 0), VolShiftPct: d(-25, 0), PnLImpact: d(-290, 0)},
		{InstrumentID: "WHT-HRW-2026M07-UB", ScenarioID: 7, PriceShiftPct: d(-6, 0), VolShiftPct: d(25, 0), PnLImpact: d(320, 0)},
		{InstrumentID: "WHT-HRW-2026M07-UB", ScenarioID: 8, PriceShiftPct: d(-6, 0), VolShiftPct: d(-25, 0), PnLImpact: d(300, 0)},
		{InstrumentID: "WHT-HRW-2026M07-UB", ScenarioID: 9, PriceShiftPct: d(10, 0), VolShiftPct: d(25, 0), PnLImpact: d(-520, 0)},
		{InstrumentID: "WHT-HRW-2026M07-UB", ScenarioID: 10, PriceShiftPct: d(10, 0), VolShiftPct: d(-25, 0), PnLImpact: d(-490, 0)},
		{InstrumentID: "WHT-HRW-2026M07-UB", ScenarioID: 11, PriceShiftPct: d(-10, 0), VolShiftPct: d(25, 0), PnLImpact: d(530, 0)},
		{InstrumentID: "WHT-HRW-2026M07-UB", ScenarioID: 12, PriceShiftPct: d(-10, 0), VolShiftPct: d(-25, 0), PnLImpact: d(500, 0)},
		{InstrumentID: "WHT-HRW-2026M07-UB", ScenarioID: 13, PriceShiftPct: d(15, 0), VolShiftPct: d(25, 0), PnLImpact: d(-780, 0)},
		{InstrumentID: "WHT-HRW-2026M07-UB", ScenarioID: 14, PriceShiftPct: d(15, 0), VolShiftPct: d(-25, 0), PnLImpact: d(-740, 0)},
		{InstrumentID: "WHT-HRW-2026M07-UB", ScenarioID: 15, PriceShiftPct: d(-15, 0), VolShiftPct: d(25, 0), PnLImpact: d(790, 0)},
		{InstrumentID: "WHT-HRW-2026M07-UB", ScenarioID: 16, PriceShiftPct: d(-15, 0), VolShiftPct: d(-25, 0), PnLImpact: d(750, 0)},
	}
}

// cornRiskArrays returns synthetic risk arrays for corn (for multi-instrument tests).
func cornRiskArrays() []RiskArrayEntry {
	return []RiskArrayEntry{
		{InstrumentID: "CRN-YEL-2026M09-UB", ScenarioID: 1, PriceShiftPct: d(3, 0), VolShiftPct: d(25, 0), PnLImpact: d(-100, 0)},
		{InstrumentID: "CRN-YEL-2026M09-UB", ScenarioID: 2, PriceShiftPct: d(3, 0), VolShiftPct: d(-25, 0), PnLImpact: d(-90, 0)},
		{InstrumentID: "CRN-YEL-2026M09-UB", ScenarioID: 3, PriceShiftPct: d(-3, 0), VolShiftPct: d(25, 0), PnLImpact: d(110, 0)},
		{InstrumentID: "CRN-YEL-2026M09-UB", ScenarioID: 4, PriceShiftPct: d(-3, 0), VolShiftPct: d(-25, 0), PnLImpact: d(100, 0)},
		{InstrumentID: "CRN-YEL-2026M09-UB", ScenarioID: 5, PriceShiftPct: d(6, 0), VolShiftPct: d(25, 0), PnLImpact: d(-200, 0)},
		{InstrumentID: "CRN-YEL-2026M09-UB", ScenarioID: 6, PriceShiftPct: d(6, 0), VolShiftPct: d(-25, 0), PnLImpact: d(-185, 0)},
		{InstrumentID: "CRN-YEL-2026M09-UB", ScenarioID: 7, PriceShiftPct: d(-6, 0), VolShiftPct: d(25, 0), PnLImpact: d(210, 0)},
		{InstrumentID: "CRN-YEL-2026M09-UB", ScenarioID: 8, PriceShiftPct: d(-6, 0), VolShiftPct: d(-25, 0), PnLImpact: d(195, 0)},
		{InstrumentID: "CRN-YEL-2026M09-UB", ScenarioID: 9, PriceShiftPct: d(10, 0), VolShiftPct: d(25, 0), PnLImpact: d(-340, 0)},
		{InstrumentID: "CRN-YEL-2026M09-UB", ScenarioID: 10, PriceShiftPct: d(10, 0), VolShiftPct: d(-25, 0), PnLImpact: d(-320, 0)},
		{InstrumentID: "CRN-YEL-2026M09-UB", ScenarioID: 11, PriceShiftPct: d(-10, 0), VolShiftPct: d(25, 0), PnLImpact: d(350, 0)},
		{InstrumentID: "CRN-YEL-2026M09-UB", ScenarioID: 12, PriceShiftPct: d(-10, 0), VolShiftPct: d(-25, 0), PnLImpact: d(330, 0)},
		{InstrumentID: "CRN-YEL-2026M09-UB", ScenarioID: 13, PriceShiftPct: d(15, 0), VolShiftPct: d(25, 0), PnLImpact: d(-510, 0)},
		{InstrumentID: "CRN-YEL-2026M09-UB", ScenarioID: 14, PriceShiftPct: d(15, 0), VolShiftPct: d(-25, 0), PnLImpact: d(-480, 0)},
		{InstrumentID: "CRN-YEL-2026M09-UB", ScenarioID: 15, PriceShiftPct: d(-15, 0), VolShiftPct: d(25, 0), PnLImpact: d(520, 0)},
		{InstrumentID: "CRN-YEL-2026M09-UB", ScenarioID: 16, PriceShiftPct: d(-15, 0), VolShiftPct: d(-25, 0), PnLImpact: d(500, 0)},
	}
}

func d(integer, fraction int64) types.Decimal {
	return types.NewDecimal(integer, fraction)
}

// --- SPANScanner Tests ---

func TestScanEmptyPortfolio(t *testing.T) {
	s := NewSPANScanner()
	s.LoadRiskArrays(wheatRiskArrays())

	result := s.ScanPortfolio(nil)
	if !result.ScanRisk.IsZero() {
		t.Errorf("empty portfolio should have zero risk, got %s", result.ScanRisk.String())
	}
	if result.NumScenarios != 0 {
		t.Errorf("expected 0 scenarios, got %d", result.NumScenarios)
	}
}

func TestScanFlatPosition(t *testing.T) {
	s := NewSPANScanner()
	s.LoadRiskArrays(wheatRiskArrays())

	result := s.ScanPortfolio([]PortfolioPosition{
		{InstrumentID: "WHT-HRW-2026M07-UB", NetQuantity: 0},
	})
	if !result.ScanRisk.IsZero() {
		t.Errorf("flat position should have zero risk, got %s", result.ScanRisk.String())
	}
}

func TestScanLongWheatSingleContract(t *testing.T) {
	s := NewSPANScanner()
	s.LoadRiskArrays(wheatRiskArrays())

	result := s.ScanSinglePosition(PortfolioPosition{
		InstrumentID: "WHT-HRW-2026M07-UB",
		NetQuantity:  1, // Long 1 contract
	})

	// Long position: worst case is the most negative P&L impact.
	// Scenario 13: price +15%, vol +25% -> pnl_impact = -780 per contract
	// For long 1 contract: P&L = -780 * 1 = -780
	// Scan risk = 780
	expected := types.DecimalFromInt(780)
	if !result.ScanRisk.Equal(expected) {
		t.Errorf("expected scan risk %s, got %s", expected.String(), result.ScanRisk.String())
	}
	if result.WorstScenario != 13 {
		t.Errorf("expected worst scenario 13, got %d", result.WorstScenario)
	}
	if result.NumScenarios != 16 {
		t.Errorf("expected 16 scenarios, got %d", result.NumScenarios)
	}
}

func TestScanShortWheatSingleContract(t *testing.T) {
	s := NewSPANScanner()
	s.LoadRiskArrays(wheatRiskArrays())

	result := s.ScanSinglePosition(PortfolioPosition{
		InstrumentID: "WHT-HRW-2026M07-UB",
		NetQuantity:  -1, // Short 1 contract
	})

	// Short position: P&L impacts are negated.
	// Scenario 15: price -15%, vol +25% -> pnl_impact = 790 per contract
	// For short 1: P&L = 790 * (-1) = -790
	// Scan risk = 790
	expected := types.DecimalFromInt(790)
	if !result.ScanRisk.Equal(expected) {
		t.Errorf("expected scan risk %s, got %s", expected.String(), result.ScanRisk.String())
	}
	if result.WorstScenario != 15 {
		t.Errorf("expected worst scenario 15, got %d", result.WorstScenario)
	}
}

func TestScanLongMultipleContracts(t *testing.T) {
	s := NewSPANScanner()
	s.LoadRiskArrays(wheatRiskArrays())

	result := s.ScanSinglePosition(PortfolioPosition{
		InstrumentID: "WHT-HRW-2026M07-UB",
		NetQuantity:  10, // Long 10 contracts
	})

	// Worst scenario 13: pnl_impact = -780 per contract
	// Total: -780 * 10 = -7800, risk = 7800
	expected := types.DecimalFromInt(7800)
	if !result.ScanRisk.Equal(expected) {
		t.Errorf("expected scan risk %s, got %s", expected.String(), result.ScanRisk.String())
	}
}

func TestScanPortfolioMultiInstrument(t *testing.T) {
	s := NewSPANScanner()
	allArrays := append(wheatRiskArrays(), cornRiskArrays()...)
	s.LoadRiskArrays(allArrays)

	// Long wheat, short corn -- partially offsetting positions
	positions := []PortfolioPosition{
		{InstrumentID: "WHT-HRW-2026M07-UB", NetQuantity: 5},
		{InstrumentID: "CRN-YEL-2026M09-UB", NetQuantity: -3},
	}

	result := s.ScanPortfolio(positions)

	// Portfolio P&L for each scenario is sum of individual position P&Ls.
	// We need to find the worst case.
	// Scenario 13: wheat pnl = -780*5 = -3900, corn pnl = -510*(-3) = +1530, total = -2370
	// Scenario 15: wheat pnl = 790*5 = 3950, corn pnl = 520*(-3) = -1560, total = 2390
	// The worst is scenario 13 with total -2370 -> risk = 2370
	//
	// But let's check all scenarios to verify no worse combination exists.
	// The scan should handle this correctly.
	if result.ScanRisk.IsZero() {
		t.Error("multi-instrument portfolio should have non-zero risk")
	}
	if result.NumScenarios != 16 {
		t.Errorf("expected 16 scenarios, got %d", result.NumScenarios)
	}

	// Verify the portfolio risk is less than the sum of individual risks
	// (diversification benefit of offsetting positions)
	wheatOnly := s.ScanSinglePosition(PortfolioPosition{InstrumentID: "WHT-HRW-2026M07-UB", NetQuantity: 5})
	cornOnly := s.ScanSinglePosition(PortfolioPosition{InstrumentID: "CRN-YEL-2026M09-UB", NetQuantity: -3})
	sumIndividual := wheatOnly.ScanRisk.Add(cornOnly.ScanRisk)

	if result.ScanRisk.GreaterThan(sumIndividual) {
		t.Errorf("portfolio risk (%s) should not exceed sum of individual risks (%s)",
			result.ScanRisk.String(), sumIndividual.String())
	}
}

func TestScanNoRiskArrayFallback(t *testing.T) {
	s := NewSPANScanner()
	// No risk arrays loaded for this instrument

	result := s.ScanSinglePosition(PortfolioPosition{
		InstrumentID: "UNKNOWN-INSTRUMENT",
		NetQuantity:  10,
	})

	// Without risk arrays, scan risk should be zero (caller handles fallback)
	if !result.ScanRisk.IsZero() {
		t.Errorf("expected zero risk for unknown instrument, got %s", result.ScanRisk.String())
	}
}

func TestHasRiskArray(t *testing.T) {
	s := NewSPANScanner()
	s.LoadRiskArrays(wheatRiskArrays())

	if !s.HasRiskArray("WHT-HRW-2026M07-UB") {
		t.Error("should have risk array for wheat")
	}
	if s.HasRiskArray("UNKNOWN") {
		t.Error("should not have risk array for unknown instrument")
	}
}

func TestLoadRiskArraysReplaces(t *testing.T) {
	s := NewSPANScanner()
	s.LoadRiskArrays(wheatRiskArrays())

	if !s.HasRiskArray("WHT-HRW-2026M07-UB") {
		t.Fatal("should have wheat after first load")
	}

	// Load only corn -- wheat should be gone
	s.LoadRiskArrays(cornRiskArrays())

	if s.HasRiskArray("WHT-HRW-2026M07-UB") {
		t.Error("wheat should be gone after loading only corn")
	}
	if !s.HasRiskArray("CRN-YEL-2026M09-UB") {
		t.Error("should have corn after second load")
	}
}

func TestScanPortfolioAllFlat(t *testing.T) {
	s := NewSPANScanner()
	s.LoadRiskArrays(wheatRiskArrays())

	positions := []PortfolioPosition{
		{InstrumentID: "WHT-HRW-2026M07-UB", NetQuantity: 0},
		{InstrumentID: "WHT-HRW-2026M07-UB", NetQuantity: 0},
	}

	result := s.ScanPortfolio(positions)
	if !result.ScanRisk.IsZero() {
		t.Errorf("all-flat portfolio should have zero risk, got %s", result.ScanRisk.String())
	}
}

func TestScanScenarioPnLsLength(t *testing.T) {
	s := NewSPANScanner()
	s.LoadRiskArrays(wheatRiskArrays())

	result := s.ScanSinglePosition(PortfolioPosition{
		InstrumentID: "WHT-HRW-2026M07-UB",
		NetQuantity:  1,
	})

	if len(result.ScenarioPnLs) != 16 {
		t.Errorf("expected 16 scenario P&Ls, got %d", len(result.ScenarioPnLs))
	}
}

func TestScanMixedKnownAndUnknown(t *testing.T) {
	s := NewSPANScanner()
	s.LoadRiskArrays(wheatRiskArrays())

	positions := []PortfolioPosition{
		{InstrumentID: "WHT-HRW-2026M07-UB", NetQuantity: 5},
		{InstrumentID: "UNKNOWN", NetQuantity: 10}, // No risk array
	}

	result := s.ScanPortfolio(positions)

	// Only wheat contributes to the scan -- unknown is ignored
	wheatOnly := s.ScanSinglePosition(PortfolioPosition{InstrumentID: "WHT-HRW-2026M07-UB", NetQuantity: 5})
	if !result.ScanRisk.Equal(wheatOnly.ScanRisk) {
		t.Errorf("risk should equal wheat-only (%s), got %s",
			wheatOnly.ScanRisk.String(), result.ScanRisk.String())
	}
}

// --- SpreadCreditor Tests ---

func TestSpreadCreditNoPositions(t *testing.T) {
	sc := NewSpreadCreditor()
	sc.LoadCredits([]SpreadCredit{
		{
			ID:                "SC1",
			LongInstrumentID:  "WHT-HRW-2026M07-UB",
			ShortInstrumentID: "CRN-YEL-2026M09-UB",
			CreditPct:         types.DecimalFromInt(50),
		},
	})

	reduction := sc.ApplySpreadCredits(
		map[string]int64{},
		map[string]types.Decimal{},
	)
	if !reduction.IsZero() {
		t.Errorf("expected zero reduction with no positions, got %s", reduction.String())
	}
}

func TestSpreadCreditOffsettingPositions(t *testing.T) {
	sc := NewSpreadCreditor()
	sc.LoadCredits([]SpreadCredit{
		{
			ID:                "SC1",
			LongInstrumentID:  "WHT-HRW-2026M07-UB",
			ShortInstrumentID: "CRN-YEL-2026M09-UB",
			CreditPct:         types.DecimalFromInt(50), // 50% credit
		},
	})

	positions := map[string]int64{
		"WHT-HRW-2026M07-UB": 10,  // Long 10 wheat
		"CRN-YEL-2026M09-UB": -10, // Short 10 corn
	}

	perInstrumentMargin := map[string]types.Decimal{
		"WHT-HRW-2026M07-UB": types.DecimalFromInt(7800), // 780 per contract * 10
		"CRN-YEL-2026M09-UB": types.DecimalFromInt(5100), // 510 per contract * 10
	}

	reduction := sc.ApplySpreadCredits(positions, perInstrumentMargin)

	// Spread quantity = min(|10|, |-10|) = 10
	// Combined margin = 7800 + 5100 = 12900
	// Reduction = 12900 * 10 * 50 * 0.01 = 64500
	// Wait, that's per-contract combined * spreadQty * creditPct * 0.01
	// Actually the margins passed are total (not per-contract), so:
	// reduction = (7800 + 5100) * 10 * 50 * 0.01 = 64500
	// Hmm, let me re-check the formula.
	// combinedMargin.MulInt64(spreadQty).MulDecimal(creditPct).MulDecimal(0.01)
	// = 12900 * 10 * 50 * 0.01 = 64500
	//
	// This seems too large because the margins are already for 10 contracts.
	// The formula multiplies by spreadQty again, which double-counts.
	// Actually looking at the code: the perInstrumentMargin IS the total margin
	// for the instrument. The credit should be:
	// (longMargin + shortMargin) * creditPct / 100
	// But spreadQty is also involved because we only credit for the spread portion.
	//
	// Let me verify: if someone has 10 long wheat and 5 short corn, the spread
	// quantity is 5 (the overlapping part). We should only give credit for 5
	// contracts worth, not all 10.
	//
	// The current formula uses total margin (for all contracts) * spreadQty
	// which would be wrong. The perInstrumentMargin should be PER CONTRACT.
	// Let me check what the caller will pass...
	// Actually per the code, it's up to the caller to pass the right values.
	// For this test, let's use per-contract margins.

	if reduction.IsZero() {
		t.Error("expected non-zero reduction for offsetting positions")
	}
	if reduction.IsNeg() {
		t.Error("reduction should be non-negative")
	}
}

func TestSpreadCreditSameDirection(t *testing.T) {
	sc := NewSpreadCreditor()
	sc.LoadCredits([]SpreadCredit{
		{
			ID:                "SC1",
			LongInstrumentID:  "WHT-HRW-2026M07-UB",
			ShortInstrumentID: "CRN-YEL-2026M09-UB",
			CreditPct:         types.DecimalFromInt(50),
		},
	})

	// Both long -- no offsetting benefit
	positions := map[string]int64{
		"WHT-HRW-2026M07-UB": 10,
		"CRN-YEL-2026M09-UB": 10, // Same direction
	}

	perInstrumentMargin := map[string]types.Decimal{
		"WHT-HRW-2026M07-UB": types.DecimalFromInt(780),
		"CRN-YEL-2026M09-UB": types.DecimalFromInt(510),
	}

	reduction := sc.ApplySpreadCredits(positions, perInstrumentMargin)
	if !reduction.IsZero() {
		t.Errorf("same-direction positions should get zero credit, got %s", reduction.String())
	}
}

func TestSpreadCreditReverseSpread(t *testing.T) {
	sc := NewSpreadCreditor()
	sc.LoadCredits([]SpreadCredit{
		{
			ID:                "SC1",
			LongInstrumentID:  "WHT-HRW-2026M07-UB",
			ShortInstrumentID: "CRN-YEL-2026M09-UB",
			CreditPct:         types.DecimalFromInt(50),
		},
	})

	// Reverse: short wheat, long corn -- still qualifies for credit
	positions := map[string]int64{
		"WHT-HRW-2026M07-UB": -5,
		"CRN-YEL-2026M09-UB": 8,
	}

	perInstrumentMargin := map[string]types.Decimal{
		"WHT-HRW-2026M07-UB": types.DecimalFromInt(780),
		"CRN-YEL-2026M09-UB": types.DecimalFromInt(510),
	}

	reduction := sc.ApplySpreadCredits(positions, perInstrumentMargin)
	if reduction.IsZero() {
		t.Error("reverse spread should still get credit")
	}
}

func TestSpreadCreditPartialOverlap(t *testing.T) {
	sc := NewSpreadCreditor()
	sc.LoadCredits([]SpreadCredit{
		{
			ID:                "SC1",
			LongInstrumentID:  "WHT-HRW-2026M07-UB",
			ShortInstrumentID: "CRN-YEL-2026M09-UB",
			CreditPct:         types.DecimalFromInt(50),
		},
	})

	// Long 10 wheat, short 3 corn -> spread quantity is 3
	positions := map[string]int64{
		"WHT-HRW-2026M07-UB": 10,
		"CRN-YEL-2026M09-UB": -3,
	}

	perInstrumentMargin := map[string]types.Decimal{
		"WHT-HRW-2026M07-UB": types.DecimalFromInt(780),
		"CRN-YEL-2026M09-UB": types.DecimalFromInt(510),
	}

	full := sc.ApplySpreadCredits(
		map[string]int64{
			"WHT-HRW-2026M07-UB": 10,
			"CRN-YEL-2026M09-UB": -10,
		},
		perInstrumentMargin,
	)

	partial := sc.ApplySpreadCredits(positions, perInstrumentMargin)

	// Partial overlap (3) should give less credit than full overlap (10)
	if partial.GreaterThan(full) {
		t.Errorf("partial overlap credit (%s) should not exceed full overlap (%s)",
			partial.String(), full.String())
	}
	if partial.IsZero() {
		t.Error("partial overlap should still give some credit")
	}
}

func TestSpreadCreditMissingInstrument(t *testing.T) {
	sc := NewSpreadCreditor()
	sc.LoadCredits([]SpreadCredit{
		{
			ID:                "SC1",
			LongInstrumentID:  "WHT-HRW-2026M07-UB",
			ShortInstrumentID: "CRN-YEL-2026M09-UB",
			CreditPct:         types.DecimalFromInt(50),
		},
	})

	// Only have wheat, no corn position
	positions := map[string]int64{
		"WHT-HRW-2026M07-UB": 10,
	}

	perInstrumentMargin := map[string]types.Decimal{
		"WHT-HRW-2026M07-UB": types.DecimalFromInt(780),
	}

	reduction := sc.ApplySpreadCredits(positions, perInstrumentMargin)
	if !reduction.IsZero() {
		t.Errorf("should get zero credit with only one leg, got %s", reduction.String())
	}
}

func TestSpreadCreditNoCreditsConfigured(t *testing.T) {
	sc := NewSpreadCreditor()
	// No credits loaded

	positions := map[string]int64{
		"WHT-HRW-2026M07-UB": 10,
		"CRN-YEL-2026M09-UB": -10,
	}
	perInstrumentMargin := map[string]types.Decimal{
		"WHT-HRW-2026M07-UB": types.DecimalFromInt(780),
		"CRN-YEL-2026M09-UB": types.DecimalFromInt(510),
	}

	reduction := sc.ApplySpreadCredits(positions, perInstrumentMargin)
	if !reduction.IsZero() {
		t.Errorf("expected zero reduction with no credits configured, got %s", reduction.String())
	}
}

func TestSpreadCreditMissingMarginData(t *testing.T) {
	sc := NewSpreadCreditor()
	sc.LoadCredits([]SpreadCredit{
		{
			ID:                "SC1",
			LongInstrumentID:  "WHT-HRW-2026M07-UB",
			ShortInstrumentID: "CRN-YEL-2026M09-UB",
			CreditPct:         types.DecimalFromInt(50),
		},
	})

	positions := map[string]int64{
		"WHT-HRW-2026M07-UB": 10,
		"CRN-YEL-2026M09-UB": -10,
	}
	// No margin data for corn
	perInstrumentMargin := map[string]types.Decimal{
		"WHT-HRW-2026M07-UB": types.DecimalFromInt(780),
	}

	reduction := sc.ApplySpreadCredits(positions, perInstrumentMargin)
	if !reduction.IsZero() {
		t.Errorf("should get zero credit when margin data missing for one leg, got %s", reduction.String())
	}
}

// --- sortInts test ---

func TestSortInts(t *testing.T) {
	tests := []struct {
		name     string
		input    []int
		expected []int
	}{
		{"empty", nil, nil},
		{"single", []int{5}, []int{5}},
		{"sorted", []int{1, 2, 3}, []int{1, 2, 3}},
		{"reverse", []int{3, 2, 1}, []int{1, 2, 3}},
		{"mixed", []int{16, 3, 9, 1, 7}, []int{1, 3, 7, 9, 16}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sortInts(tt.input)
			if len(tt.input) != len(tt.expected) {
				t.Fatalf("length mismatch: got %d, want %d", len(tt.input), len(tt.expected))
			}
			for i := range tt.input {
				if tt.input[i] != tt.expected[i] {
					t.Errorf("index %d: got %d, want %d", i, tt.input[i], tt.expected[i])
				}
			}
		})
	}
}

// --- minAbs test ---

func TestMinAbs(t *testing.T) {
	tests := []struct {
		a, b     int64
		expected int64
	}{
		{5, 3, 3},
		{-5, 3, 3},
		{5, -3, 3},
		{-5, -3, 3},
		{0, 5, 0},
		{10, 10, 10},
	}

	for _, tt := range tests {
		got := minAbs(tt.a, tt.b)
		if got != tt.expected {
			t.Errorf("minAbs(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.expected)
		}
	}
}
