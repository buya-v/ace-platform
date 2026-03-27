package scanner

import (
	"testing"

	"github.com/ace-platform/margin-engine/internal/params"
	"github.com/ace-platform/margin-engine/internal/types"
)

func cornParams() params.InstrumentParams {
	return params.InstrumentParams{
		InstrumentID:   "CORN-2026-07",
		PriceScanRange: types.NewDecimal(3, 0), // $3.00
		VolScanRange:   types.NewDecimal(0, 5000), // 0.50
		SpotPrice:      types.NewDecimal(450, 0), // $450.00
		ContractSize:   5000, // 5000 bushels
		Scenarios:      params.DefaultScenarios(types.NewDecimal(3, 0), types.NewDecimal(0, 5000)),
	}
}

func TestScanFlatPosition(t *testing.T) {
	s := New()
	pos := types.Position{
		ParticipantID: "P1",
		InstrumentID:  "CORN-2026-07",
		NetQuantity:   0,
	}

	result := s.Scan(pos, cornParams())
	if !result.ScanRisk.IsZero() {
		t.Errorf("flat position should have zero scan risk, got %s", result.ScanRisk.String())
	}
}

func TestScanLongPosition(t *testing.T) {
	s := New()
	pos := types.Position{
		ParticipantID: "P1",
		InstrumentID:  "CORN-2026-07",
		NetQuantity:   10, // Long 10 contracts
		AvgEntryPrice: types.NewDecimal(450, 0),
	}

	ip := cornParams()
	result := s.Scan(pos, ip)

	// Long position loses on price drops. Worst case for standard scenarios:
	// Full price drop: -$3.00 * 5000 * 10 = -$150,000
	// Scan risk should be $150,000
	expectedRisk := types.NewDecimal(3, 0).MulInt64(5000).MulInt64(10) // 150000
	if !result.ScanRisk.Equal(expectedRisk) {
		t.Errorf("expected scan risk %s, got %s", expectedRisk.String(), result.ScanRisk.String())
	}

	if result.ScanRisk.IsZero() {
		t.Error("long position should have non-zero scan risk")
	}
}

func TestScanShortPosition(t *testing.T) {
	s := New()
	pos := types.Position{
		ParticipantID: "P1",
		InstrumentID:  "CORN-2026-07",
		NetQuantity:   -5, // Short 5 contracts
		AvgEntryPrice: types.NewDecimal(450, 0),
	}

	ip := cornParams()
	result := s.Scan(pos, ip)

	// Short position loses on price rises. Worst case for standard scenarios:
	// Full price rise: $3.00 * 5000 * (-5) = -$75,000 loss
	// But extreme scenario: 3*$3.00 * 5000 * 5 = $225,000 * 0.30 weight = $67,500
	// Full price up is worse: $75,000 > $67,500
	expectedRisk := types.NewDecimal(3, 0).MulInt64(5000).MulInt64(5) // 75000
	if !result.ScanRisk.Equal(expectedRisk) {
		t.Errorf("expected scan risk %s, got %s", expectedRisk.String(), result.ScanRisk.String())
	}
}

func TestScanSymmetry(t *testing.T) {
	s := New()
	ip := cornParams()

	longPos := types.Position{NetQuantity: 1, InstrumentID: "CORN-2026-07"}
	shortPos := types.Position{NetQuantity: -1, InstrumentID: "CORN-2026-07"}

	longResult := s.Scan(longPos, ip)
	shortResult := s.Scan(shortPos, ip)

	// For futures with symmetric scenarios, long and short should have equal risk
	if !longResult.ScanRisk.Equal(shortResult.ScanRisk) {
		t.Errorf("scan risk should be symmetric: long=%s short=%s",
			longResult.ScanRisk.String(), shortResult.ScanRisk.String())
	}
}

func TestMarkToMarketProfit(t *testing.T) {
	s := New()
	pos := types.Position{
		NetQuantity:   10,
		AvgEntryPrice: types.NewDecimal(440, 0), // Bought at $440
		InstrumentID:  "CORN-2026-07",
	}
	ip := cornParams() // Spot at $450

	mtm := s.MarkToMarket(pos, ip)
	// MtM = ($450 - $440) * 5000 * 10 = $500,000
	expected := types.NewDecimal(10, 0).MulInt64(5000).MulInt64(10)
	if !mtm.Equal(expected) {
		t.Errorf("expected MtM %s, got %s", expected.String(), mtm.String())
	}
}

func TestMarkToMarketLoss(t *testing.T) {
	s := New()
	pos := types.Position{
		NetQuantity:   10,
		AvgEntryPrice: types.NewDecimal(460, 0), // Bought at $460
		InstrumentID:  "CORN-2026-07",
	}
	ip := cornParams() // Spot at $450

	mtm := s.MarkToMarket(pos, ip)
	// MtM = ($450 - $460) * 5000 * 10 = -$500,000
	if !mtm.IsNeg() {
		t.Errorf("expected negative MtM for losing position, got %s", mtm.String())
	}
}

func TestMarkToMarketFlat(t *testing.T) {
	s := New()
	pos := types.Position{NetQuantity: 0, InstrumentID: "CORN-2026-07"}
	ip := cornParams()

	mtm := s.MarkToMarket(pos, ip)
	if !mtm.IsZero() {
		t.Errorf("flat position MtM should be zero, got %s", mtm.String())
	}
}

func TestMarkToMarketShortProfit(t *testing.T) {
	s := New()
	pos := types.Position{
		NetQuantity:   -5,
		AvgEntryPrice: types.NewDecimal(460, 0), // Sold at $460
		InstrumentID:  "CORN-2026-07",
	}
	ip := cornParams() // Spot at $450

	mtm := s.MarkToMarket(pos, ip)
	// MtM = ($450 - $460) * 5000 * (-5) = $250,000 (profit for short when price drops)
	if mtm.IsNeg() || mtm.IsZero() {
		t.Errorf("short position should profit when price drops, got %s", mtm.String())
	}
}
