// Package span implements a SPAN-like scenario-based margin model.
//
// Unlike the default scanner (which generates scenarios from PriceScanRange/VolScanRange),
// SPANScanner uses pre-computed risk arrays: each instrument has a fixed set of scenarios
// with pre-calculated P&L impacts. This mirrors how CME SPAN works in production, where
// risk arrays are published daily by the exchange.
//
// The scanning algorithm:
//  1. For each position, look up the risk array for that instrument
//  2. Multiply each scenario's pnl_impact by the position's net quantity
//  3. Sum across all positions to get portfolio-level P&L per scenario
//  4. The worst-case loss (most negative portfolio P&L) is the scanning risk
//
// If no risk array is configured for an instrument, the scanner falls back to
// the existing percentage-based calculation via the params.Store.
package span

import (
	"github.com/garudax-platform/margin-engine/internal/types"
)

// RiskArrayEntry represents a single scenario's pre-computed P&L impact
// for one contract of a given instrument.
type RiskArrayEntry struct {
	InstrumentID  string
	ScenarioID    int
	PriceShiftPct types.Decimal // Price shift as percentage (e.g., 3.0 = +3%)
	VolShiftPct   types.Decimal // Volatility shift as percentage (e.g., 25.0 = +25%)
	PnLImpact     types.Decimal // P&L impact per contract under this scenario
}

// PortfolioPosition represents a position for SPAN scanning purposes.
type PortfolioPosition struct {
	InstrumentID string
	NetQuantity  int64 // Positive = long, negative = short
}

// ScanResult holds the output of a SPAN portfolio scan.
type ScanResult struct {
	ScanRisk       types.Decimal   // Worst-case portfolio loss (always >= 0)
	WorstScenario  int             // 1-based scenario ID of worst case
	ScenarioPnLs   []types.Decimal // Portfolio-level P&L for each scenario
	NumScenarios   int             // Number of scenarios evaluated
}

// SPANScanner performs SPAN-like risk scanning using pre-computed risk arrays.
type SPANScanner struct {
	// riskArrays maps instrument_id -> scenario_id -> RiskArrayEntry
	riskArrays map[string]map[int]RiskArrayEntry
}

// NewSPANScanner creates a new scanner with empty risk arrays.
// Use LoadRiskArrays to populate before scanning.
func NewSPANScanner() *SPANScanner {
	return &SPANScanner{
		riskArrays: make(map[string]map[int]RiskArrayEntry),
	}
}

// LoadRiskArrays loads pre-computed risk array entries into the scanner.
// This replaces any previously loaded arrays. Entries are indexed by
// instrument ID and scenario ID for O(1) lookup during scanning.
func (s *SPANScanner) LoadRiskArrays(entries []RiskArrayEntry) {
	s.riskArrays = make(map[string]map[int]RiskArrayEntry, len(entries))
	for _, e := range entries {
		if _, ok := s.riskArrays[e.InstrumentID]; !ok {
			s.riskArrays[e.InstrumentID] = make(map[int]RiskArrayEntry)
		}
		s.riskArrays[e.InstrumentID][e.ScenarioID] = e
	}
}

// HasRiskArray returns true if pre-computed risk arrays exist for the given instrument.
func (s *SPANScanner) HasRiskArray(instrumentID string) bool {
	scenarios, ok := s.riskArrays[instrumentID]
	return ok && len(scenarios) > 0
}

// ScanPortfolio evaluates all positions against all scenarios simultaneously,
// computing portfolio-level P&L under each scenario. Returns the worst-case loss.
//
// This is the core SPAN algorithm: for each scenario, sum the P&L impact across
// all positions (pnl_impact * netQuantity), then take the maximum loss.
//
// Positions whose instruments have no risk array are skipped (the caller should
// fall back to percentage-based margin for those instruments).
func (s *SPANScanner) ScanPortfolio(positions []PortfolioPosition) ScanResult {
	if len(positions) == 0 {
		return ScanResult{}
	}

	// Collect all scenario IDs across all instruments
	scenarioIDs := s.collectScenarioIDs(positions)
	if len(scenarioIDs) == 0 {
		return ScanResult{}
	}

	numScenarios := len(scenarioIDs)
	portfolioPnLs := make([]types.Decimal, numScenarios)

	// For each scenario, sum P&L across all positions
	for i, scenarioID := range scenarioIDs {
		totalPnL := types.DecimalZero()
		for _, pos := range positions {
			if pos.NetQuantity == 0 {
				continue
			}
			scenarios, ok := s.riskArrays[pos.InstrumentID]
			if !ok {
				continue
			}
			entry, ok := scenarios[scenarioID]
			if !ok {
				continue
			}
			// P&L for this position under this scenario:
			// pnl_impact is per-contract, multiply by net quantity
			posPnL := entry.PnLImpact.MulInt64(pos.NetQuantity)
			totalPnL = totalPnL.Add(posPnL)
		}
		portfolioPnLs[i] = totalPnL
	}

	// Find worst-case loss (most negative P&L)
	worstLoss := types.DecimalZero()
	worstIdx := 0
	for i, pnl := range portfolioPnLs {
		loss := pnl.Negate() // Convert negative P&L to positive loss
		if loss.GreaterThan(worstLoss) {
			worstLoss = loss
			worstIdx = i
		}
	}

	// Scan risk is never negative
	if worstLoss.IsNeg() {
		worstLoss = types.DecimalZero()
	}

	worstScenarioID := 0
	if worstIdx < len(scenarioIDs) {
		worstScenarioID = scenarioIDs[worstIdx]
	}

	return ScanResult{
		ScanRisk:      worstLoss,
		WorstScenario: worstScenarioID,
		ScenarioPnLs:  portfolioPnLs,
		NumScenarios:  numScenarios,
	}
}

// ScanSinglePosition evaluates one position against its risk array.
// This is a convenience method equivalent to ScanPortfolio with a single position.
func (s *SPANScanner) ScanSinglePosition(pos PortfolioPosition) ScanResult {
	return s.ScanPortfolio([]PortfolioPosition{pos})
}

// collectScenarioIDs returns a sorted, deduplicated list of all scenario IDs
// present across the instruments in the given positions.
func (s *SPANScanner) collectScenarioIDs(positions []PortfolioPosition) []int {
	seen := make(map[int]bool)
	for _, pos := range positions {
		scenarios, ok := s.riskArrays[pos.InstrumentID]
		if !ok {
			continue
		}
		for id := range scenarios {
			seen[id] = true
		}
	}

	ids := make([]int, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}

	// Sort for deterministic ordering
	sortInts(ids)
	return ids
}

// sortInts sorts a slice of ints in ascending order (insertion sort, fine for <=16 scenarios).
func sortInts(s []int) {
	for i := 1; i < len(s); i++ {
		key := s[i]
		j := i - 1
		for j >= 0 && s[j] > key {
			s[j+1] = s[j]
			j--
		}
		s[j+1] = key
	}
}
