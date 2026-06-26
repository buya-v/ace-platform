package reconciliation

import (
	"errors"
	"math"
	"testing"

	"github.com/garudax-platform/decimal"
)

// TestOverflow_LargeNotionalMulDecimal pins the margin-engine bug class: a
// notional large enough to overflow the int64 raw range must ERROR, never wrap.
// 1e9 * 1e9 in fixed-point = 1e18 scaled, which exceeds ~9.22e14 representable.
func TestOverflow_LargeNotionalMulDecimal(t *testing.T) {
	big := decimal.DecimalFromInt(1_000_000_000) // raw 1e13
	if _, err := big.TryMulDecimal(big); !errors.Is(err, decimal.ErrOverflow) {
		t.Fatalf("TryMulDecimal(1e9, 1e9): got err=%v, want ErrOverflow", err)
	}
	assertPanics(t, "MulDecimal large notional", func() { _ = big.MulDecimal(big) })
}

// TestOverflow_LargeNotionalMulInt64 confirms price * absurd-quantity overflows
// loudly rather than wrapping to a negative or truncated cash figure.
func TestOverflow_LargeNotionalMulInt64(t *testing.T) {
	price := decimal.MustParse("100.0000") // raw 1_000_000
	if _, err := price.TryMulInt64(math.MaxInt64); !errors.Is(err, decimal.ErrOverflow) {
		t.Fatalf("TryMulInt64(MaxInt64): got err=%v, want ErrOverflow", err)
	}
	assertPanics(t, "MulInt64 overflow", func() { _ = price.MulInt64(math.MaxInt64) })

	// Just inside the boundary must NOT error: MaxInt64 raw * 1 is fine.
	if _, err := decimal.DecimalFromRaw(math.MaxInt64).TryMulInt64(1); err != nil {
		t.Fatalf("TryMulInt64 at boundary: unexpected error %v", err)
	}
}

// TestOverflow_MulUint64 confirms the unsigned multiply path also reports
// overflow (the securities-service MulUint64 boundary).
func TestOverflow_MulUint64(t *testing.T) {
	if _, err := decimal.DecimalFromRaw(math.MaxInt64).TryMulUint64(2); !errors.Is(err, decimal.ErrOverflow) {
		t.Fatalf("TryMulUint64 overflow: got %v, want ErrOverflow", err)
	}
}

// TestOverflow_AddSubAtBoundary confirms add/sub at the int64 extremes error
// instead of wrapping across the sign boundary.
func TestOverflow_AddSubAtBoundary(t *testing.T) {
	max := decimal.DecimalFromRaw(math.MaxInt64)
	min := decimal.DecimalFromRaw(math.MinInt64)
	if _, err := max.TryAdd(decimal.DecimalFromRaw(1)); !errors.Is(err, decimal.ErrOverflow) {
		t.Fatalf("MaxInt64 + 1: got %v, want ErrOverflow", err)
	}
	if _, err := min.TrySub(decimal.DecimalFromRaw(1)); !errors.Is(err, decimal.ErrOverflow) {
		t.Fatalf("MinInt64 - 1: got %v, want ErrOverflow", err)
	}
	// MinInt64 negated overflows (no +MinInt64 counterpart).
	if _, err := min.TryNegate(); !errors.Is(err, decimal.ErrOverflow) {
		t.Fatalf("Negate(MinInt64): got %v, want ErrOverflow", err)
	}
}

// TestOverflow_ParseLargeNotional confirms parsing an out-of-range integer part
// reports an error rather than producing a wrapped value.
func TestOverflow_ParseLargeNotional(t *testing.T) {
	for _, s := range []string{
		"99999999999999999999",   // far beyond int64*Scale
		"1000000000000000",       // 1e15 * Scale overflows
		"-1000000000000000.5000", // negative side
	} {
		if _, err := decimal.ParseDecimal(s); err == nil {
			t.Errorf("ParseDecimal(%q): expected overflow/format error, got nil", s)
		}
	}
}

// TestDivideByZero_Errors confirms every division entry point treats a zero
// divisor as an error (Try*) or panic (convenience), never a zero result.
func TestDivideByZero_Errors(t *testing.T) {
	d := decimal.MustParse("123.45")

	if _, err := d.TryDivInt64(0); !errors.Is(err, decimal.ErrDivideByZero) {
		t.Fatalf("TryDivInt64(0): got %v, want ErrDivideByZero", err)
	}
	assertPanics(t, "DivInt64(0)", func() { _ = d.DivInt64(0) })
	assertPanics(t, "DivInt(0) alias", func() { _ = d.DivInt(0) })

	// Zero numerator divided by zero is still an error (not 0).
	if _, err := decimal.DecimalZero().TryDivInt64(0); !errors.Is(err, decimal.ErrDivideByZero) {
		t.Fatalf("0 / 0: got %v, want ErrDivideByZero", err)
	}
}

// TestDivideByZero_NonZeroDivisorOK is the negative control: a legitimate
// divisor must succeed and round correctly, proving the guard is not overbroad.
func TestDivideByZero_NonZeroDivisorOK(t *testing.T) {
	if got := decimal.DecimalFromInt(1).DivInt64(3).String(); got != "0.3333" {
		t.Fatalf("1/3 = %s, want 0.3333", got)
	}
	if got := decimal.DecimalFromInt(10).DivInt64(-2).String(); got != "-5" {
		t.Fatalf("10/-2 = %s, want -5", got)
	}
}

func assertPanics(t *testing.T, label string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("%s: expected panic, got none", label)
		}
	}()
	fn()
}
