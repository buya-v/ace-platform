package types

// Decimal is the platform's shared fixed-point money type. The previous
// per-service copy was replaced by github.com/garudax-platform/decimal during
// remediation R003 (see docs/specs/R001_shared_decimal_spec.md). This file
// re-exports it so existing types.Decimal / types.NewDecimal call sites compile
// unchanged while gaining checked-overflow, banker's rounding, and
// error-on-divide-by-zero semantics.

import "github.com/garudax-platform/decimal"

// Decimal aliases the shared money type (same underlying type, all methods).
type Decimal = decimal.Decimal

// Re-exported constructors (kept as vars so package-qualified call sites work).
var (
	NewDecimal       = decimal.NewDecimal
	DecimalFromInt   = decimal.DecimalFromInt
	DecimalZero      = decimal.DecimalZero
	DecimalFromRaw   = decimal.DecimalFromRaw
	ParseDecimal     = decimal.ParseDecimal
	MustParseDecimal = decimal.MustParse
)
