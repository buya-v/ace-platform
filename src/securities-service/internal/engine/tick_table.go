// Package engine — tiered tick-size validation.
package engine

import (
	"fmt"
	"math"

	"github.com/garudax-platform/securities-service/internal/types"
)

// ValidateTickSize checks that price is a valid multiple of the applicable tick size.
// If tickTable is nil or has no tiers, the defaultTickSize is used.
// If defaultTickSize is also zero, no validation is performed.
func ValidateTickSize(price float64, tickTable *types.TickTable, defaultTickSize float64) error {
	if tickTable == nil || len(tickTable.Tiers) == 0 {
		// Fall back to the instrument default tick size.
		if defaultTickSize <= 0 {
			return nil
		}
		remainder := math.Remainder(price, defaultTickSize)
		if math.Abs(remainder) > 1e-9 {
			return fmt.Errorf("price %.2f must be a multiple of tick_size %.4f", price, defaultTickSize)
		}
		return nil
	}

	// Find the tier where MinPrice <= price < MaxPrice.
	// If price >= all MaxPrice values, use the last tier.
	var tier *types.TickTier
	for i := range tickTable.Tiers {
		t := &tickTable.Tiers[i]
		if price >= t.MinPrice && price < t.MaxPrice {
			tier = t
			break
		}
	}
	if tier == nil {
		// price >= all MaxPrice — use the last tier.
		tier = &tickTable.Tiers[len(tickTable.Tiers)-1]
	}

	remainder := math.Remainder(price, tier.TickSize)
	if math.Abs(remainder) > 1e-9 {
		return fmt.Errorf("price %.2f must be a multiple of tick_size %.4f (tier: %.2f-%.2f)",
			price, tier.TickSize, tier.MinPrice, tier.MaxPrice)
	}
	return nil
}
