package scanner

import (
	"testing"

	"github.com/garudax-platform/margin-engine/internal/params"
	"github.com/garudax-platform/margin-engine/internal/types"
)

// wheatDeliveryParams returns instrument params for a delivery-month wheat contract.
func wheatDeliveryParams() params.InstrumentParams {
	return params.InstrumentParams{
		InstrumentID:    "WHEAT-2026-09",
		PriceScanRange:  types.NewDecimal(4, 0),
		VolScanRange:    types.NewDecimal(0, 5000),
		SpotPrice:       types.NewDecimal(600, 0),
		ContractSize:    5000,
		DeliveryCharge:  types.NewDecimal(0, 500),
		IsDeliveryMonth: true,
		Scenarios:       params.DefaultScenarios(types.NewDecimal(4, 0), types.NewDecimal(0, 5000)),
	}
}

func TestScanTableDriven(t *testing.T) {
	s := New()

	tests := []struct {
		name        string
		pos         types.Position
		ip          params.InstrumentParams
		wantZero    bool
		wantMinRisk types.Decimal // scan risk should be >= this (if not wantZero)
	}{
		{
			name:     "flat position has zero risk",
			pos:      types.Position{InstrumentID: "CORN-2026-07", NetQuantity: 0},
			ip:       cornParams(),
			wantZero: true,
		},
		{
			name:        "long 1 corn",
			pos:         types.Position{InstrumentID: "CORN-2026-07", NetQuantity: 1, AvgEntryPrice: types.NewDecimal(450, 0)},
			ip:          cornParams(),
			wantMinRisk: types.NewDecimal(1, 0), // must be positive
		},
		{
			name:        "short 1 corn",
			pos:         types.Position{InstrumentID: "CORN-2026-07", NetQuantity: -1, AvgEntryPrice: types.NewDecimal(450, 0)},
			ip:          cornParams(),
			wantMinRisk: types.NewDecimal(1, 0),
		},
		{
			name:        "long 100 corn — large position",
			pos:         types.Position{InstrumentID: "CORN-2026-07", NetQuantity: 100, AvgEntryPrice: types.NewDecimal(450, 0)},
			ip:          cornParams(),
			wantMinRisk: types.DecimalFromInt(100000), // at least 100k risk
		},
		{
			name:        "short 50 wheat delivery month",
			pos:         types.Position{InstrumentID: "WHEAT-2026-09", NetQuantity: -50, AvgEntryPrice: types.NewDecimal(600, 0)},
			ip:          wheatDeliveryParams(),
			wantMinRisk: types.DecimalFromInt(100000),
		},
		{
			name:        "long 1 wheat delivery month",
			pos:         types.Position{InstrumentID: "WHEAT-2026-09", NetQuantity: 1, AvgEntryPrice: types.NewDecimal(600, 0)},
			ip:          wheatDeliveryParams(),
			wantMinRisk: types.NewDecimal(1, 0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.Scan(tt.pos, tt.ip)

			if tt.wantZero {
				if !result.ScanRisk.IsZero() {
					t.Errorf("expected zero scan risk, got %s", result.ScanRisk.String())
				}
				return
			}

			if result.ScanRisk.IsZero() {
				t.Error("expected non-zero scan risk")
			}
			if result.ScanRisk.IsNeg() {
				t.Errorf("scan risk must never be negative, got %s", result.ScanRisk.String())
			}
			if result.ScanRisk.LessThan(tt.wantMinRisk) {
				t.Errorf("scan risk %s below expected minimum %s", result.ScanRisk.String(), tt.wantMinRisk.String())
			}
		})
	}
}

func TestScanScenarioPnLsCount(t *testing.T) {
	s := New()
	ip := cornParams()

	tests := []struct {
		name string
		qty  int64
	}{
		{"flat", 0},
		{"long", 10},
		{"short", -5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := types.Position{InstrumentID: "CORN-2026-07", NetQuantity: tt.qty}
			result := s.Scan(pos, ip)

			if len(result.ScenarioPnLs) != len(ip.Scenarios) {
				t.Errorf("expected %d scenario PnLs, got %d", len(ip.Scenarios), len(result.ScenarioPnLs))
			}
		})
	}
}

func TestScanWorstScenarioIndex(t *testing.T) {
	s := New()
	ip := cornParams()

	pos := types.Position{
		InstrumentID:  "CORN-2026-07",
		NetQuantity:   10,
		AvgEntryPrice: types.NewDecimal(450, 0),
	}

	result := s.Scan(pos, ip)

	// WorstScenario should be a valid index
	if result.WorstScenario < 0 || result.WorstScenario >= len(ip.Scenarios) {
		t.Errorf("worst scenario index %d out of range [0, %d)", result.WorstScenario, len(ip.Scenarios))
	}

	// The PnL at worst scenario should be the most negative (or least positive)
	worstPnL := result.ScenarioPnLs[result.WorstScenario]
	for i, pnl := range result.ScenarioPnLs {
		if pnl.LessThan(worstPnL) {
			t.Errorf("scenario %d has PnL %s worse than 'worst' scenario %d with PnL %s",
				i, pnl.String(), result.WorstScenario, worstPnL.String())
		}
	}
}

func TestScanExtremePriceMoves(t *testing.T) {
	s := New()

	// Create params with very large price scan range
	ip := params.InstrumentParams{
		InstrumentID:   "EXTREME-TEST",
		PriceScanRange: types.NewDecimal(100, 0), // $100 scan range
		VolScanRange:   types.NewDecimal(1, 0),
		SpotPrice:      types.NewDecimal(500, 0),
		ContractSize:   1000,
		Scenarios:      params.DefaultScenarios(types.NewDecimal(100, 0), types.NewDecimal(1, 0)),
	}

	pos := types.Position{
		InstrumentID: "EXTREME-TEST",
		NetQuantity:  10,
	}

	result := s.Scan(pos, ip)

	// With $100 scan range, 1000 contract size, 10 qty:
	// Full down move: $100 * 1000 * 10 = $1,000,000
	if result.ScanRisk.IsZero() {
		t.Error("extreme price moves should produce non-zero risk")
	}
	if result.ScanRisk.IsNeg() {
		t.Error("scan risk must never be negative")
	}
}

func TestScanWithCustomScenarios(t *testing.T) {
	s := New()

	// Only positive scenarios — all profit for long position
	ip := params.InstrumentParams{
		InstrumentID: "CUSTOM-SCEN",
		ContractSize: 100,
		SpotPrice:    types.NewDecimal(50, 0),
		Scenarios: []params.PriceScenario{
			{PriceMove: types.NewDecimal(1, 0), Weight: types.DecimalFromInt(1)},
			{PriceMove: types.NewDecimal(2, 0), Weight: types.DecimalFromInt(1)},
			{PriceMove: types.NewDecimal(3, 0), Weight: types.DecimalFromInt(1)},
		},
	}

	pos := types.Position{
		InstrumentID: "CUSTOM-SCEN",
		NetQuantity:  1,
	}

	result := s.Scan(pos, ip)

	// All scenarios are price increases for a long position => all PnLs positive
	// Worst loss would be negative (no loss) but scan risk floor is zero
	if !result.ScanRisk.IsZero() {
		t.Errorf("all-profit scenarios should yield zero scan risk, got %s", result.ScanRisk.String())
	}
}

func TestScanWithMixedWeightScenarios(t *testing.T) {
	s := New()

	ip := params.InstrumentParams{
		InstrumentID: "WEIGHTED",
		ContractSize: 1000,
		SpotPrice:    types.NewDecimal(100, 0),
		Scenarios: []params.PriceScenario{
			// Small move, full weight
			{PriceMove: types.NewDecimal(1, 0), Weight: types.DecimalFromInt(1)},
			{PriceMove: types.NewDecimal(1, 0).Negate(), Weight: types.DecimalFromInt(1)},
			// Large move, reduced weight (0.3)
			{PriceMove: types.NewDecimal(10, 0), Weight: types.NewDecimal(0, 3000)},
			{PriceMove: types.NewDecimal(10, 0).Negate(), Weight: types.NewDecimal(0, 3000)},
		},
	}

	pos := types.Position{InstrumentID: "WEIGHTED", NetQuantity: 1}
	result := s.Scan(pos, ip)

	// Full-weight small down: -1 * 1000 * 1 * 1.0 = -1000 => loss = 1000
	// Reduced-weight large down: -10 * 1000 * 1 * 0.3 = -3000 => loss = 3000
	// The reduced-weight large move should be worst
	expectedRisk := types.NewDecimal(10, 0).MulInt64(1000).MulDecimal(types.NewDecimal(0, 3000))
	if !result.ScanRisk.Equal(expectedRisk) {
		t.Errorf("expected risk %s, got %s", expectedRisk.String(), result.ScanRisk.String())
	}
}

func TestScanWithNoScenarios(t *testing.T) {
	s := New()

	ip := params.InstrumentParams{
		InstrumentID: "NO-SCEN",
		ContractSize: 1000,
		SpotPrice:    types.NewDecimal(100, 0),
		Scenarios:    []params.PriceScenario{}, // empty
	}

	pos := types.Position{InstrumentID: "NO-SCEN", NetQuantity: 5}
	result := s.Scan(pos, ip)

	if !result.ScanRisk.IsZero() {
		t.Errorf("no scenarios should yield zero risk, got %s", result.ScanRisk.String())
	}
	if len(result.ScenarioPnLs) != 0 {
		t.Errorf("expected 0 PnLs, got %d", len(result.ScenarioPnLs))
	}
}

func TestScanMultipleInstruments(t *testing.T) {
	s := New()

	cornIP := cornParams()
	wheatIP := wheatDeliveryParams()

	cornPos := types.Position{InstrumentID: "CORN-2026-07", NetQuantity: 10, AvgEntryPrice: types.NewDecimal(450, 0)}
	wheatPos := types.Position{InstrumentID: "WHEAT-2026-09", NetQuantity: -5, AvgEntryPrice: types.NewDecimal(600, 0)}

	cornResult := s.Scan(cornPos, cornIP)
	wheatResult := s.Scan(wheatPos, wheatIP)

	// Both should have non-zero risk
	if cornResult.ScanRisk.IsZero() {
		t.Error("corn position should have non-zero scan risk")
	}
	if wheatResult.ScanRisk.IsZero() {
		t.Error("wheat position should have non-zero scan risk")
	}

	// Wheat has higher price scan range ($4 vs $3) so with similar contract size
	// but different qty, risk will differ — just verify both are positive
	if cornResult.ScanRisk.IsNeg() || wheatResult.ScanRisk.IsNeg() {
		t.Error("scan risk must never be negative")
	}
}

func TestScanRiskScalesLinearly(t *testing.T) {
	s := New()
	ip := cornParams()

	pos1 := types.Position{InstrumentID: "CORN-2026-07", NetQuantity: 1}
	pos10 := types.Position{InstrumentID: "CORN-2026-07", NetQuantity: 10}

	risk1 := s.Scan(pos1, ip).ScanRisk
	risk10 := s.Scan(pos10, ip).ScanRisk

	// Risk for 10 contracts should be exactly 10x risk for 1 contract
	expected := risk1.MulInt64(10)
	if !risk10.Equal(expected) {
		t.Errorf("risk should scale linearly: 1-lot=%s, 10-lot=%s, expected 10-lot=%s",
			risk1.String(), risk10.String(), expected.String())
	}
}

func TestMarkToMarketTableDriven(t *testing.T) {
	s := New()

	tests := []struct {
		name     string
		pos      types.Position
		ip       params.InstrumentParams
		wantSign int // -1 negative, 0 zero, 1 positive
	}{
		{
			name:     "flat position",
			pos:      types.Position{NetQuantity: 0},
			ip:       cornParams(),
			wantSign: 0,
		},
		{
			name:     "long at-the-money",
			pos:      types.Position{NetQuantity: 10, AvgEntryPrice: types.NewDecimal(450, 0)},
			ip:       cornParams(), // spot = 450
			wantSign: 0,
		},
		{
			name:     "long in-the-money",
			pos:      types.Position{NetQuantity: 10, AvgEntryPrice: types.NewDecimal(440, 0)},
			ip:       cornParams(), // spot = 450 > 440
			wantSign: 1,
		},
		{
			name:     "long out-of-the-money",
			pos:      types.Position{NetQuantity: 10, AvgEntryPrice: types.NewDecimal(460, 0)},
			ip:       cornParams(), // spot = 450 < 460
			wantSign: -1,
		},
		{
			name:     "short in-the-money (price dropped)",
			pos:      types.Position{NetQuantity: -10, AvgEntryPrice: types.NewDecimal(460, 0)},
			ip:       cornParams(), // spot = 450 < 460 => short profits
			wantSign: 1,
		},
		{
			name:     "short out-of-the-money (price rose)",
			pos:      types.Position{NetQuantity: -10, AvgEntryPrice: types.NewDecimal(440, 0)},
			ip:       cornParams(), // spot = 450 > 440 => short loses
			wantSign: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mtm := s.MarkToMarket(tt.pos, tt.ip)

			switch tt.wantSign {
			case 0:
				if !mtm.IsZero() {
					t.Errorf("expected zero MtM, got %s", mtm.String())
				}
			case 1:
				if !mtm.GreaterThan(types.DecimalZero()) {
					t.Errorf("expected positive MtM, got %s", mtm.String())
				}
			case -1:
				if !mtm.IsNeg() {
					t.Errorf("expected negative MtM, got %s", mtm.String())
				}
			}
		})
	}
}

func TestMarkToMarketExactValues(t *testing.T) {
	s := New()

	ip := params.InstrumentParams{
		InstrumentID: "EXACT-TEST",
		SpotPrice:    types.NewDecimal(100, 0),
		ContractSize: 100,
	}

	tests := []struct {
		name     string
		qty      int64
		entry    types.Decimal
		expected types.Decimal
	}{
		{
			name:     "long 1, entry 90, spot 100",
			qty:      1,
			entry:    types.NewDecimal(90, 0),
			expected: types.NewDecimal(10, 0).MulInt64(100).MulInt64(1), // (100-90)*100*1 = 1000
		},
		{
			name:     "short 2, entry 110, spot 100",
			qty:      -2,
			entry:    types.NewDecimal(110, 0),
			expected: types.NewDecimal(10, 0).MulInt64(100).MulInt64(2), // (100-110)*100*(-2) = 2000
		},
		{
			name:     "long 5, entry 100, spot 100",
			qty:      5,
			entry:    types.NewDecimal(100, 0),
			expected: types.DecimalZero(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := types.Position{
				InstrumentID:  "EXACT-TEST",
				NetQuantity:   tt.qty,
				AvgEntryPrice: tt.entry,
			}
			mtm := s.MarkToMarket(pos, ip)
			if !mtm.Equal(tt.expected) {
				t.Errorf("expected MtM %s, got %s", tt.expected.String(), mtm.String())
			}
		})
	}
}

func TestScanSingleScenario(t *testing.T) {
	s := New()

	// Single scenario: price drop of $5
	ip := params.InstrumentParams{
		InstrumentID: "SINGLE",
		ContractSize: 100,
		SpotPrice:    types.NewDecimal(50, 0),
		Scenarios: []params.PriceScenario{
			{PriceMove: types.NewDecimal(5, 0).Negate(), Weight: types.DecimalFromInt(1)},
		},
	}

	// Long position loses on price drop
	pos := types.Position{InstrumentID: "SINGLE", NetQuantity: 2}
	result := s.Scan(pos, ip)

	// PnL = -5 * 100 * 2 * 1.0 = -1000 => loss = 1000
	expectedRisk := types.NewDecimal(5, 0).MulInt64(100).MulInt64(2)
	if !result.ScanRisk.Equal(expectedRisk) {
		t.Errorf("expected risk %s, got %s", expectedRisk.String(), result.ScanRisk.String())
	}
	if result.WorstScenario != 0 {
		t.Errorf("expected worst scenario 0, got %d", result.WorstScenario)
	}
}
