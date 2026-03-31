package collateral

import (
	"log"

	"github.com/garudax-platform/margin-engine/internal/types"
)

// CompositeCollateralSource aggregates collateral from multiple sources.
// Each source is best-effort: if a source panics or returns an error,
// its contribution is treated as zero and the remaining sources are still summed.
//
// Typical composition:
//   - HTTPCollateralSource (clearing positions)
//   - WarehouseCollateralSource (pledged warehouse receipts)
//   - Future: CashCollateralSource (direct cash deposits)
type CompositeCollateralSource struct {
	sources []namedSource
}

// namedSource pairs a CollateralSource with a label for logging.
type namedSource struct {
	name   string
	source CollateralSource
}

// CollateralSource provides collateral balances. This mirrors the engine
// package interface so the composite can be used as a drop-in replacement.
type CollateralSource interface {
	GetCollateral(participantID string) types.Decimal
}

// NewCompositeCollateralSource creates a composite source with no initial sources.
// Use Add() to register sources.
func NewCompositeCollateralSource() *CompositeCollateralSource {
	return &CompositeCollateralSource{}
}

// Add registers a named collateral source. Sources are queried in the order
// they are added, but the order does not affect the result (pure summation).
func (c *CompositeCollateralSource) Add(name string, source CollateralSource) {
	c.sources = append(c.sources, namedSource{name: name, source: source})
}

// SourceCount returns the number of registered sources.
func (c *CompositeCollateralSource) SourceCount() int {
	return len(c.sources)
}

// GetCollateral sums collateral from all registered sources.
// Each source is called independently; failures in one source do not affect others.
// A source that panics contributes zero to the total.
func (c *CompositeCollateralSource) GetCollateral(participantID string) types.Decimal {
	total := types.DecimalZero()

	for _, ns := range c.sources {
		value := c.safeGetCollateral(ns, participantID)
		if !value.IsZero() {
			log.Printf("collateral: %s contributed %s for %s", ns.name, value.String(), participantID)
		}
		total = total.Add(value)
	}

	return total
}

// safeGetCollateral calls a source's GetCollateral with panic recovery.
func (c *CompositeCollateralSource) safeGetCollateral(ns namedSource, participantID string) (result types.Decimal) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("collateral: source %s panicked for %s: %v", ns.name, participantID, r)
			result = types.DecimalZero()
		}
	}()
	return ns.source.GetCollateral(participantID)
}
