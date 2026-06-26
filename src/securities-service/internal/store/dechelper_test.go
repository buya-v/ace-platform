// Package store_test test helper — builds Decimal money values from float literals.
package store_test

import (
	"github.com/garudax-platform/decimal"
	"github.com/garudax-platform/securities-service/internal/types"
)

// decLit builds a Decimal from a numeric literal for test fixtures. It is the
// test-side counterpart to the production float<->Decimal boundary shims.
func decLit(f float64) types.Decimal {
	d, _ := decimal.NewFromFloat(f)
	return d
}
