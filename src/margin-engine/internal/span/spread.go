package span

import (
	"github.com/garudax-platform/margin-engine/internal/types"
)

// SpreadCredit defines margin relief for offsetting positions in correlated instruments.
// When a participant is long one instrument and short a correlated instrument,
// the spread credit reduces the total margin requirement because the risk offsets.
type SpreadCredit struct {
	ID               string
	LongInstrumentID string        // Instrument expected to be held long
	ShortInstrumentID string       // Instrument expected to be held short
	CreditPct        types.Decimal // Percentage of margin saved (e.g., 50.0 = 50% reduction)
}

// SpreadCreditor calculates margin reductions from inter-commodity spread credits.
type SpreadCreditor struct {
	credits []SpreadCredit
}

// NewSpreadCreditor creates a new creditor with empty credit definitions.
// Use LoadCredits to populate.
func NewSpreadCreditor() *SpreadCreditor {
	return &SpreadCreditor{}
}

// LoadCredits loads spread credit definitions.
func (sc *SpreadCreditor) LoadCredits(credits []SpreadCredit) {
	sc.credits = make([]SpreadCredit, len(credits))
	copy(sc.credits, credits)
}

// ApplySpreadCredits calculates the total margin reduction from spread credits.
//
// For each spread credit definition, if the portfolio has a long position in the
// long instrument AND a short position in the short instrument (or vice versa),
// the margin reduction is:
//
//	min(|longQty|, |shortQty|) * averagePerContractMargin * creditPct / 100
//
// The averagePerContractMargin is approximated from the per-instrument scan results.
// If no matching spread is found, the reduction is zero.
//
// Parameters:
//   - positions: map of instrumentID -> net quantity
//   - perInstrumentMargin: map of instrumentID -> margin requirement for that instrument
//
// Returns the total margin reduction to subtract from the portfolio's initial margin.
func (sc *SpreadCreditor) ApplySpreadCredits(
	positions map[string]int64,
	perInstrumentMargin map[string]types.Decimal,
) types.Decimal {
	totalReduction := types.DecimalZero()

	for _, credit := range sc.credits {
		longQty, hasLong := positions[credit.LongInstrumentID]
		shortQty, hasShort := positions[credit.ShortInstrumentID]

		if !hasLong || !hasShort {
			continue
		}

		// Check for offsetting positions:
		// Standard case: long in longInstrument, short in shortInstrument
		// Reverse case: short in longInstrument, long in shortInstrument
		var spreadQty int64
		if longQty > 0 && shortQty < 0 {
			// Standard: long the long leg, short the short leg
			spreadQty = minAbs(longQty, shortQty)
		} else if longQty < 0 && shortQty > 0 {
			// Reverse spread: also valid for margin relief
			spreadQty = minAbs(longQty, shortQty)
		} else {
			// Both same direction -- no offsetting benefit
			continue
		}

		if spreadQty == 0 {
			continue
		}

		// Calculate per-spread-contract margin as average of the two legs
		longMargin, hasLM := perInstrumentMargin[credit.LongInstrumentID]
		shortMargin, hasSM := perInstrumentMargin[credit.ShortInstrumentID]
		if !hasLM || !hasSM {
			continue
		}

		// Average margin per contract across both legs
		// Then multiply by spread quantity and credit percentage
		combinedMargin := longMargin.Add(shortMargin)
		// creditPct is stored as e.g. 50.0 meaning 50%, so divide by 100
		// Multiply combined margin by spread quantity, then by creditPct/100
		reduction := combinedMargin.MulInt64(spreadQty).MulDecimal(credit.CreditPct).MulDecimal(
			types.NewDecimal(0, 100), // 0.01 = 1/100
		)

		totalReduction = totalReduction.Add(reduction)
	}

	return totalReduction
}

// minAbs returns the smaller of |a| and |b|.
func minAbs(a, b int64) int64 {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	if a < b {
		return a
	}
	return b
}
