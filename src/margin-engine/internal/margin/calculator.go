package margin

import (
	"time"

	"github.com/garudax-platform/margin-engine/internal/params"
	"github.com/garudax-platform/margin-engine/internal/scanner"
	"github.com/garudax-platform/margin-engine/internal/types"
)

// Calculator computes SPAN-style margin requirements for positions.
type Calculator struct {
	scanner    *scanner.Scanner
	paramStore *params.Store
}

func NewCalculator(paramStore *params.Store) *Calculator {
	return &Calculator{
		scanner:    scanner.New(),
		paramStore: paramStore,
	}
}

// Calculate computes the margin requirement for a single position.
func (c *Calculator) Calculate(pos types.Position) (types.MarginRequirement, error) {
	ip, err := c.paramStore.Get(pos.InstrumentID)
	if err != nil {
		return types.MarginRequirement{}, err
	}

	// Step 1: SPAN scanning risk
	scanResult := c.scanner.Scan(pos, ip)

	// Step 2: Delivery month charge
	deliveryCharge := types.DecimalZero()
	if ip.IsDeliveryMonth {
		// delivery charge = deliveryChargeRate * spotPrice * contractSize * |netQty|
		absQty := abs64(pos.NetQuantity)
		deliveryCharge = ip.DeliveryCharge.MulDecimal(ip.SpotPrice).MulInt64(ip.ContractSize).MulInt64(absQty)
	}

	// Step 3: Initial margin = scanRisk + deliveryCharge
	// (InterMonth spread charge is zero for single-instrument portfolios)
	initialMargin := scanResult.ScanRisk.Add(deliveryCharge)

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
		ScanRisk:      scanResult.ScanRisk,
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
