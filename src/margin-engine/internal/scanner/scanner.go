package scanner

import (
	"github.com/garudax-platform/margin-engine/internal/params"
	"github.com/garudax-platform/margin-engine/internal/types"
)

// Scanner implements the SPAN risk scanning algorithm.
// For each position, it evaluates profit/loss across all scenarios and
// takes the worst-case loss as the scanning risk.
type Scanner struct{}

func New() *Scanner {
	return &Scanner{}
}

// ScanResult holds the output of a single position scan.
type ScanResult struct {
	ScanRisk       types.Decimal // Worst-case loss (always >= 0)
	WorstScenario  int           // Index of worst-case scenario
	ScenarioPnLs   []types.Decimal // P&L for each scenario
}

// Scan evaluates a position against all SPAN scenarios and returns the scanning risk.
// For futures, the P&L under each scenario is: priceMove * contractSize * netQuantity.
// The scanning risk is the maximum loss (minimum P&L, negated) across all weighted scenarios.
func (s *Scanner) Scan(pos types.Position, ip params.InstrumentParams) ScanResult {
	if pos.NetQuantity == 0 {
		return ScanResult{
			ScanRisk:      types.DecimalZero(),
			ScenarioPnLs:  make([]types.Decimal, len(ip.Scenarios)),
		}
	}

	pnls := make([]types.Decimal, len(ip.Scenarios))
	worstLoss := types.DecimalZero()
	worstIdx := 0

	for i, scenario := range ip.Scenarios {
		// For futures: P&L = priceMove * contractSize * netQuantity
		// Positive netQuantity (long) profits from price increases
		// Negative netQuantity (short) profits from price decreases
		rawPnL := scenario.PriceMove.MulInt64(ip.ContractSize).MulInt64(pos.NetQuantity)

		// Apply scenario weight
		weightedPnL := rawPnL.MulDecimal(scenario.Weight)
		pnls[i] = weightedPnL

		// Track worst loss (most negative P&L = largest risk)
		loss := weightedPnL.Negate() // Convert loss to positive number
		if loss.GreaterThan(worstLoss) {
			worstLoss = loss
			worstIdx = i
		}
	}

	// Scan risk is never negative (worst case is zero loss)
	if worstLoss.IsNeg() {
		worstLoss = types.DecimalZero()
	}

	return ScanResult{
		ScanRisk:      worstLoss,
		WorstScenario: worstIdx,
		ScenarioPnLs:  pnls,
	}
}

// MarkToMarket calculates unrealized P&L for a position.
// MtM = (markPrice - avgEntryPrice) * contractSize * netQuantity
func (s *Scanner) MarkToMarket(pos types.Position, ip params.InstrumentParams) types.Decimal {
	if pos.NetQuantity == 0 {
		return types.DecimalZero()
	}
	priceDiff := ip.SpotPrice.Sub(pos.AvgEntryPrice)
	return priceDiff.MulInt64(ip.ContractSize).MulInt64(pos.NetQuantity)
}
