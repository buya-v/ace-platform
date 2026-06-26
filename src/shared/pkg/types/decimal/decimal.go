// Package decimal provides the GarudaX platform's shared fixed-point money type.
//
// It replaces the six divergent Decimal copies that previously lived in each
// engine's internal/types package and the raw float64 money math in
// securities-service. See docs/specs/R001_shared_decimal_spec.md.
//
// A Decimal is a signed fixed-point number with exactly 4 fractional digits,
// stored internally as an int64 scaled by 10000 — i.e. Decimal(18,4). The
// representable range is approximately -9.22e14 .. 9.22e14.
//
// Correctness guarantees:
//   - No silent overflow. Every multiply/divide checks a 128-bit intermediate
//     (math/bits) and reports overflow rather than wrapping int64.
//   - No truncation bias. Fixed-point multiply and division round half-to-even
//     (banker's rounding) instead of truncating toward zero.
//   - No silent divide-by-zero. Division by zero is an error (Try* form) or a
//     panic (convenience form), never a zero result.
//
// Two API styles are provided:
//   - Try* methods return (Decimal, error) for callers at money boundaries that
//     must handle overflow/divide-by-zero explicitly.
//   - Convenience methods (Add, Sub, Mul*, Div*, Negate, Abs) panic on overflow
//     or divide-by-zero. A panic is a loud, traceable failure; the previous
//     behaviour silently corrupted balances. Prefer Try* where a bad input is
//     reachable from untrusted or unbounded data.
package decimal

import (
	"errors"
	"fmt"
	"math"
	"math/bits"
	"strconv"
	"strings"
)

// MarshalJSON renders the decimal as a JSON number (no quotes), preserving the
// exact 4-dp value, e.g. 12.5 not "12.5". This keeps the wire format compatible
// with clients that previously consumed a float64 price.
func (d Decimal) MarshalJSON() ([]byte, error) {
	return []byte(d.String()), nil
}

// UnmarshalJSON accepts either a JSON number (12.5) or a JSON string ("12.5"),
// parsing through the precision-preserving ParseDecimal path. Numbers with more
// than 4 fractional digits are rounded half-to-even, never truncated.
func (d *Decimal) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		return nil
	}
	s = strings.Trim(s, `"`)
	parsed, err := ParseDecimal(s)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

// Float64 returns the value as a float64 — for display or legitimately
// float-typed interop only. Never use the result for money arithmetic.
func (d Decimal) Float64() float64 {
	return float64(d.value) / float64(Scale)
}

// NewFromFloat builds a Decimal from a float64, rounding half-to-even to 4 dp.
// Use only at the boundary with float-typed inputs; prefer ParseDecimal for
// strings and the integer constructors for exact values.
func NewFromFloat(f float64) (Decimal, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return Decimal{}, ErrInvalidFormat
	}
	return ParseDecimal(strconv.FormatFloat(f, 'f', -1, 64))
}

// Scale is the fixed-point scale: 4 fractional digits (Decimal(18,4)).
const Scale int64 = 10000

const scaleDigits = 4

// Sentinel errors returned by the Try* API.
var (
	ErrOverflow      = errors.New("decimal: overflow")
	ErrDivideByZero  = errors.New("decimal: divide by zero")
	ErrInvalidFormat = errors.New("decimal: invalid format")
)

// Decimal is a signed fixed-point number with 4 fractional digits.
// The zero value is 0.0000 and is ready to use.
type Decimal struct {
	value int64 // realValue * Scale
}

// ── Constructors ────────────────────────────────────────────────────────────

// NewDecimal builds a Decimal from an integer and a 4-digit fractional part,
// e.g. NewDecimal(123, 4567) == 123.4567. It panics on overflow.
func NewDecimal(integer, fraction int64) Decimal {
	v, ok := mulInt64(integer, Scale)
	if !ok {
		panic(ErrOverflow)
	}
	s := v + fraction
	if (v > 0 && fraction > 0 && s < 0) || (v < 0 && fraction < 0 && s > 0) {
		panic(ErrOverflow)
	}
	return Decimal{value: s}
}

// DecimalFromInt builds a Decimal from a whole number. Panics on overflow.
func DecimalFromInt(v int64) Decimal {
	r, ok := mulInt64(v, Scale)
	if !ok {
		panic(ErrOverflow)
	}
	return Decimal{value: r}
}

// DecimalZero returns 0.0000.
func DecimalZero() Decimal { return Decimal{} }

// Zero is an alias for DecimalZero.
func Zero() Decimal { return Decimal{} }

// DecimalFromRaw builds a Decimal from the raw scaled int64 (the value already
// stored in protobufs / Postgres *_raw columns). No scaling is applied.
func DecimalFromRaw(raw int64) Decimal { return Decimal{value: raw} }

// ParseDecimal parses strings like "123.4567", "-0.5", "100", or "".
// Fractional digits beyond 4 are rounded half-to-even (never truncated).
// An empty string parses to zero.
func ParseDecimal(s string) (Decimal, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Decimal{}, nil
	}

	negative := false
	switch s[0] {
	case '-':
		negative = true
		s = s[1:]
	case '+':
		s = s[1:]
	}
	if s == "" {
		return Decimal{}, fmt.Errorf("%w: %q", ErrInvalidFormat, s)
	}

	parts := strings.SplitN(s, ".", 2)
	intPart, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return Decimal{}, fmt.Errorf("%w: %q: %v", ErrInvalidFormat, s, err)
	}

	var fracPart int64
	roundUp := false
	if len(parts) == 2 {
		fracStr := parts[1]
		if fracStr == "" || !allDigits(fracStr) {
			return Decimal{}, fmt.Errorf("%w: %q", ErrInvalidFormat, s)
		}
		keep := fracStr
		var rest string
		if len(fracStr) > scaleDigits {
			keep = fracStr[:scaleDigits]
			rest = fracStr[scaleDigits:]
		} else {
			for len(keep) < scaleDigits {
				keep += "0"
			}
		}
		fracPart, err = strconv.ParseInt(keep, 10, 64)
		if err != nil {
			return Decimal{}, fmt.Errorf("%w: %q: %v", ErrInvalidFormat, s, err)
		}
		roundUp = roundHalfEven(rest, fracPart)
	}

	scaled, ok := mulInt64(intPart, Scale)
	if !ok {
		return Decimal{}, fmt.Errorf("%w: %q", ErrOverflow, s)
	}
	val := scaled + fracPart // intPart and fracPart share sign handling below
	if val < scaled {        // fracPart is always >= 0 here, so val must grow
		return Decimal{}, fmt.Errorf("%w: %q", ErrOverflow, s)
	}
	if roundUp {
		if val == math.MaxInt64 {
			return Decimal{}, fmt.Errorf("%w: %q", ErrOverflow, s)
		}
		val++
	}
	if negative {
		val = -val
	}
	return Decimal{value: val}, nil
}

// MustParse is ParseDecimal that panics on error — for trusted literals/tests.
func MustParse(s string) Decimal {
	d, err := ParseDecimal(s)
	if err != nil {
		panic(err)
	}
	return d
}

// ── Accessors / predicates ──────────────────────────────────────────────────

// Raw returns the internal scaled int64 (wire/DB representation).
func (d Decimal) Raw() int64 { return d.value }

func (d Decimal) IsZero() bool { return d.value == 0 }
func (d Decimal) IsNeg() bool  { return d.value < 0 }
func (d Decimal) IsPos() bool  { return d.value > 0 }

// ── Comparison ──────────────────────────────────────────────────────────────

func (d Decimal) Cmp(o Decimal) int {
	switch {
	case d.value < o.value:
		return -1
	case d.value > o.value:
		return 1
	default:
		return 0
	}
}

func (d Decimal) LessThan(o Decimal) bool           { return d.value < o.value }
func (d Decimal) GreaterThan(o Decimal) bool        { return d.value > o.value }
func (d Decimal) Equal(o Decimal) bool              { return d.value == o.value }
func (d Decimal) GreaterThanOrEqual(o Decimal) bool { return d.value >= o.value }
func (d Decimal) LessThanOrEqual(o Decimal) bool    { return d.value <= o.value }

func (d Decimal) Max(o Decimal) Decimal {
	if d.value >= o.value {
		return d
	}
	return o
}

func (d Decimal) Min(o Decimal) Decimal {
	if d.value <= o.value {
		return d
	}
	return o
}

// ── Arithmetic (Try* = error; convenience = panic) ──────────────────────────

func (d Decimal) TryAdd(o Decimal) (Decimal, error) {
	s := d.value + o.value
	if (d.value > 0 && o.value > 0 && s < 0) || (d.value < 0 && o.value < 0 && s > 0) {
		return Decimal{}, ErrOverflow
	}
	return Decimal{value: s}, nil
}

func (d Decimal) Add(o Decimal) Decimal { return must(d.TryAdd(o)) }

func (d Decimal) TrySub(o Decimal) (Decimal, error) {
	r := d.value - o.value
	if (d.value >= 0 && o.value < 0 && r < 0) || (d.value < 0 && o.value > 0 && r > 0) {
		return Decimal{}, ErrOverflow
	}
	return Decimal{value: r}, nil
}

func (d Decimal) Sub(o Decimal) Decimal { return must(d.TrySub(o)) }

func (d Decimal) TryMulInt64(q int64) (Decimal, error) {
	v, ok := mulInt64(d.value, q)
	if !ok {
		return Decimal{}, ErrOverflow
	}
	return Decimal{value: v}, nil
}

func (d Decimal) MulInt64(q int64) Decimal { return must(d.TryMulInt64(q)) }

func (d Decimal) TryMulUint64(q uint64) (Decimal, error) {
	if d.value == 0 || q == 0 {
		return Decimal{}, nil
	}
	neg := d.value < 0
	hi, lo := bits.Mul64(abs64(d.value), q)
	if hi != 0 {
		return Decimal{}, ErrOverflow
	}
	v, ok := signedFromMag(lo, neg)
	if !ok {
		return Decimal{}, ErrOverflow
	}
	return Decimal{value: v}, nil
}

func (d Decimal) MulUint64(q uint64) Decimal { return must(d.TryMulUint64(q)) }

// TryMulDecimal multiplies two fixed-point values: round(a*b/Scale), half-even.
func (d Decimal) TryMulDecimal(o Decimal) (Decimal, error) {
	v, err := mulDivRound(d.value, o.value, Scale)
	if err != nil {
		return Decimal{}, err
	}
	return Decimal{value: v}, nil
}

func (d Decimal) MulDecimal(o Decimal) Decimal { return must(d.TryMulDecimal(o)) }

// TryDivInt64 divides by an integer: round(value/divisor), half-even.
func (d Decimal) TryDivInt64(divisor int64) (Decimal, error) {
	v, err := mulDivRound(d.value, 1, divisor)
	if err != nil {
		return Decimal{}, err
	}
	return Decimal{value: v}, nil
}

func (d Decimal) DivInt64(divisor int64) Decimal { return must(d.TryDivInt64(divisor)) }

// DivInt is an alias of DivInt64 (the old market-data name).
func (d Decimal) DivInt(divisor int64) Decimal { return d.DivInt64(divisor) }

func (d Decimal) TryNegate() (Decimal, error) {
	if d.value == math.MinInt64 {
		return Decimal{}, ErrOverflow
	}
	return Decimal{value: -d.value}, nil
}

func (d Decimal) Negate() Decimal { return must(d.TryNegate()) }

func (d Decimal) Abs() Decimal {
	if d.value < 0 {
		return d.Negate()
	}
	return d
}

// ── String ──────────────────────────────────────────────────────────────────

// String renders the decimal with trailing zeros trimmed, e.g. "123.45".
func (d Decimal) String() string {
	negative := d.value < 0
	v := abs64(d.value)
	intPart := v / uint64(Scale)
	fracPart := v % uint64(Scale)

	sign := ""
	if negative {
		sign = "-"
	}
	if fracPart == 0 {
		return fmt.Sprintf("%s%d", sign, intPart)
	}
	fracStr := strings.TrimRight(fmt.Sprintf("%04d", fracPart), "0")
	return fmt.Sprintf("%s%d.%s", sign, intPart, fracStr)
}

// ── internal helpers ────────────────────────────────────────────────────────

func must(d Decimal, err error) Decimal {
	if err != nil {
		panic(err)
	}
	return d
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return len(s) > 0
}

// abs64 returns the magnitude of x as uint64, handling math.MinInt64 safely.
func abs64(x int64) uint64 {
	if x < 0 {
		return uint64(-(x + 1)) + 1
	}
	return uint64(x)
}

// signedFromMag converts a magnitude+sign into an int64, reporting overflow.
func signedFromMag(mag uint64, neg bool) (int64, bool) {
	if neg {
		if mag > uint64(math.MaxInt64)+1 {
			return 0, false
		}
		if mag == uint64(math.MaxInt64)+1 {
			return math.MinInt64, true
		}
		return -int64(mag), true
	}
	if mag > uint64(math.MaxInt64) {
		return 0, false
	}
	return int64(mag), true
}

// mulInt64 multiplies two int64 values, reporting overflow.
func mulInt64(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	neg := (a < 0) != (b < 0)
	hi, lo := bits.Mul64(abs64(a), abs64(b))
	if hi != 0 {
		return 0, false
	}
	return signedFromMag(lo, neg)
}

// mulDivRound computes round(a*b/denom) with banker's (half-to-even) rounding,
// using a 128-bit intermediate. Reports ErrDivideByZero and ErrOverflow.
func mulDivRound(a, b, denom int64) (int64, error) {
	if denom == 0 {
		return 0, ErrDivideByZero
	}
	if a == 0 || b == 0 {
		return 0, nil
	}
	neg := ((a < 0) != (b < 0)) != (denom < 0)
	ua, ub, ud := abs64(a), abs64(b), abs64(denom)

	hi, lo := bits.Mul64(ua, ub)
	if hi >= ud { // quotient would not fit in 64 bits
		return 0, ErrOverflow
	}
	q, r := bits.Div64(hi, lo, ud)

	if r > 0 {
		rem := ud - r // safe: 0 < r < ud
		if r > rem || (r == rem && q&1 == 1) {
			if q == math.MaxUint64 {
				return 0, ErrOverflow
			}
			q++
		}
	}

	v, ok := signedFromMag(q, neg)
	if !ok {
		return 0, ErrOverflow
	}
	return v, nil
}

// roundHalfEven decides whether the kept fractional value should round up given
// the discarded digits `rest`. keptValue is used for the half-to-even tie-break.
func roundHalfEven(rest string, keptValue int64) bool {
	if rest == "" {
		return false
	}
	// Compare rest against the half-way point "5000…" of the same length.
	half := "5" + strings.Repeat("0", len(rest)-1)
	switch {
	case rest > half:
		return true
	case rest < half:
		return false
	default: // exactly half → round to even
		return keptValue&1 == 1
	}
}
