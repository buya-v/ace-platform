package margin

import (
	"time"

	"github.com/garudax-platform/margin-engine/internal/params"
	"github.com/garudax-platform/margin-engine/internal/scanner"
	"github.com/garudax-platform/margin-engine/internal/span"
	"github.com/garudax-platform/margin-engine/internal/types"
)

// Calculator computes SPAN-style margin requirements for positions.
// It supports two scanning modes:
//   - Risk-array mode (SPAN): uses pre-computed risk arrays when available
//   - Fallback mode: uses the existing scenario-generation scanner
//
// When a SPANScanner is configured and has risk arrays for an instrument,
// the risk-array mode is used. Otherwise, the fallback scanner is used.
// This allows gradual migration: instruments with risk arrays get SPAN pricing,
// while others keep the existing behavior.
type Calculator struct {
	scanner      *scanner.Scanner
	spanScanner  *span.SPANScanner
	spreadCredit *span.SpreadCreditor
	paramStore   *params.Store
}

func NewCalculator(paramStore *params.Store) *Calculator {
	return &Calculator{
		scanner:    scanner.New(),
		paramStore: paramStore,
	}
}

// SetSPANScanner configures the SPAN risk-array scanner.
// When set, instruments with loaded risk arrays will use SPAN scanning
// instead of the fallback percentage-based scanner.
func (c *Calculator) SetSPANScanner(s *span.SPANScanner) {
	c.spanScanner = s
}

// SetSpreadCreditor configures spread credit calculation for portfolio margin.
func (c *Calculator) SetSpreadCreditor(sc *span.SpreadCreditor) {
	c.spreadCredit = sc
}

// Calculate computes the margin requirement for a single position.
func (c *Calculator) Calculate(pos types.Position) (types.MarginRequirement, error) {
	ip, err := c.paramStore.Get(pos.InstrumentID)
	if err != nil {
		return types.MarginRequirement{}, err
	}

	// Step 1: SPAN scanning risk
	// Use risk-array scanner if available for this instrument, otherwise fallback
	var scanRisk types.Decimal
	if c.spanScanner != nil && c.spanScanner.HasRiskArray(pos.InstrumentID) {
		spanResult := c.spanScanner.ScanSinglePosition(span.PortfolioPosition{
			InstrumentID: pos.InstrumentID,
			NetQuantity:  pos.NetQuantity,
		})
		scanRisk = spanResult.ScanRisk
	} else {
		scanResult := c.scanner.Scan(pos, ip)
		scanRisk = scanResult.ScanRisk
	}

	// Step 2: Delivery month charge
	deliveryCharge := types.DecimalZero()
	if ip.IsDeliveryMonth {
		// delivery charge = deliveryChargeRate * spotPrice * contractSize * |netQty|
		absQty := abs64(pos.NetQuantity)
		deliveryCharge = ip.DeliveryCharge.MulDecimal(ip.SpotPrice).MulInt64(ip.ContractSize).MulInt64(absQty)
	}

	// Step 3: Initial margin = scanRisk + deliveryCharge
	// (InterMonth spread charge is zero for single-instrument portfolios)
	initialMargin := scanRisk.Add(deliveryCharge)

	// Step 4: Mark-to-market
	mtm := c.scanner.MarkToMarket(pos, ip)

	// Step 5: Total required = initialMargin - MtM
	// If MtM is positive (profit), it reduces the requirement.
	// If MtM is negative (loss), it increases the requirement.
	totalRequired := initialMargin.Sub(mtm)
	if totalRequired.IsNeg() {
		totalRequired = types.DecimalZero()
	}

	return types.MarginRequirement{
		ParticipantID: pos.ParticipantID,
		InstrumentID:  pos.InstrumentID,
		NetQuantity:   pos.NetQuantity,
		ScanRisk:      scanRisk,
		InterMonth:    types.DecimalZero(), // Single-instrument; T029+ could add spread margins
		DeliveryMonth: deliveryCharge,
		ShortOption:   types.DecimalZero(), // Futures only; no options yet
		InitialMargin: initialMargin,
		MarkToMarket:  mtm,
		TotalRequired: totalRequired,
		CalculatedAt:  time.Now(),
	}, nil
}

// CalculatePortfolio computes margin for all of a participant's positions.
// When a SPANScanner is configured, it performs portfolio-level SPAN scanning
// (evaluating all positions simultaneously across scenarios) and applies
// spread credits for offsetting positions.
func (c *Calculator) CalculatePortfolio(participantID string, positions []types.Position, collateral types.Decimal) (types.PortfolioMargin, error) {
	now := time.Now()
	pm := types.PortfolioMargin{
		ParticipantID:    participantID,
		CollateralOnHand: collateral,
		CalculatedAt:     now,
	}

	totalInitial := types.DecimalZero()
	totalMtM := types.DecimalZero()
	totalRequired := types.DecimalZero()

	// Collect per-instrument data for spread credit calculation
	positionQtys := make(map[string]int64)
	perInstrumentMargin := make(map[string]types.Decimal)

	for _, pos := range positions {
		if pos.NetQuantity == 0 {
			continue
		}
		req, err := c.Calculate(pos)
		if err != nil {
			return types.PortfolioMargin{}, err
		}
		pm.Requirements = append(pm.Requirements, req)
		totalInitial = totalInitial.Add(req.InitialMargin)
		totalMtM = totalMtM.Add(req.MarkToMarket)
		totalRequired = totalRequired.Add(req.TotalRequired)

		positionQtys[pos.InstrumentID] = pos.NetQuantity
		perInstrumentMargin[pos.InstrumentID] = req.ScanRisk
	}

	// Apply spread credits if configured
	if c.spreadCredit != nil && len(positionQtys) > 1 {
		spreadReduction := c.spreadCredit.ApplySpreadCredits(positionQtys, perInstrumentMargin)
		if spreadReduction.GreaterThan(types.DecimalZero()) {
			totalInitial = totalInitial.Sub(spreadReduction)
			if totalInitial.IsNeg() {
				totalInitial = types.DecimalZero()
			}
			totalRequired = totalRequired.Sub(spreadReduction)
			if totalRequired.IsNeg() {
				totalRequired = types.DecimalZero()
			}
		}
	}

	pm.TotalInitial = totalInitial
	pm.TotalMtM = totalMtM
	pm.TotalRequired = totalRequired
	pm.ExcessDeficit = collateral.Sub(totalRequired)

	return pm, nil
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
