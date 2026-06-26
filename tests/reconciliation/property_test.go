package reconciliation

import (
	"errors"
	"math"
	"math/big"
	"math/rand"
	"testing"

	"github.com/garudax-platform/decimal"
)

// TestProperty_MulInt64MatchesOracle sweeps random (decimal, int64) pairs and
// asserts MulInt64 equals the exact big.Int product — or reports overflow when
// the product doesn't fit int64. This is the core "rounding-direction" (here:
// exact, no rounding) plus "overflow errors not wraps" invariant for MulInt64.
func TestProperty_MulInt64MatchesOracle(t *testing.T) {
	rng := rand.New(rand.NewSource(seed))
	for i := 0; i < iterations; i++ {
		raw := randRawWide(rng)
		q := randRawWide(rng)
		d := rawDecimal(raw)

		oracle := new(big.Int).Mul(big.NewInt(raw), big.NewInt(q))
		got, err := d.TryMulInt64(q)
		requireOracle(t, "MulInt64", oracle, got, err)
	}
}

// TestProperty_MulDecimalRoundsHalfEven sweeps random decimal pairs and asserts
// MulDecimal == round_half_even(a*b / Scale) computed in big.Int, or overflow.
// This pins the rounding *direction* (banker's rounding) for every input, not
// just the handful of hand-picked cases in the unit suite.
func TestProperty_MulDecimalRoundsHalfEven(t *testing.T) {
	rng := rand.New(rand.NewSource(seed + 1))
	for i := 0; i < iterations; i++ {
		ra := randRawWide(rng)
		rb := randRawWide(rng)
		a := rawDecimal(ra)
		b := rawDecimal(rb)

		product := new(big.Int).Mul(big.NewInt(ra), big.NewInt(rb))
		oracle := roundHalfEvenBig(product, bigScale)

		got, err := a.TryMulDecimal(b)
		requireOracle(t, "MulDecimal", oracle, got, err)
	}
}

// TestProperty_DivInt64RoundsHalfEven sweeps random (decimal, divisor) pairs and
// asserts DivInt64 == round_half_even(raw/divisor), or reports the single
// MinInt64/-1 overflow, or divide-by-zero for divisor 0.
func TestProperty_DivInt64RoundsHalfEven(t *testing.T) {
	rng := rand.New(rand.NewSource(seed + 2))
	for i := 0; i < iterations; i++ {
		raw := randRawWide(rng)
		div := randRawWide(rng)
		d := rawDecimal(raw)

		got, err := d.TryDivInt64(div)
		if div == 0 {
			if !errors.Is(err, decimal.ErrDivideByZero) {
				t.Fatalf("DivInt64(0): got err=%v, want ErrDivideByZero", err)
			}
			continue
		}
		oracle := roundHalfEvenBig(big.NewInt(raw), big.NewInt(div))
		requireOracle(t, "DivInt64", oracle, got, err)
	}
}

// TestProperty_MulDivRoundTripExact asserts the exact round-trip invariant:
// d.MulInt64(q).DivInt64(q) == d for every non-zero q (when the product fits).
// MulInt64 is exact and DivInt64 then divides an exact multiple, so there is NO
// rounding loss — the recovered value must equal the original bit-for-bit.
func TestProperty_MulDivRoundTripExact(t *testing.T) {
	rng := rand.New(rand.NewSource(seed + 3))
	checked := 0
	for i := 0; i < iterations; i++ {
		// Keep operands narrow so raw*q stays inside int64 and the round-trip
		// is defined (overflow is covered separately).
		raw := randRawNarrow(rng)
		q := rng.Int63n(2_000_000) - 1_000_000 // [-1e6, 1e6]
		if q == 0 {
			continue
		}
		product := new(big.Int).Mul(big.NewInt(raw), big.NewInt(q))
		if !fitsInt64(product) {
			continue
		}
		d := rawDecimal(raw)
		back := d.MulInt64(q).DivInt64(q)
		if !back.Equal(d) {
			t.Fatalf("round-trip: (%s * %d) / %d = %s, want %s", d, q, q, back, d)
		}
		checked++
	}
	if checked < iterations/2 {
		t.Fatalf("round-trip exercised only %d cases; range too tight", checked)
	}
}

// TestProperty_DivMulRoundTripBounded asserts the bounded inverse: dividing then
// multiplying recovers the original to within half a divisor (the precision lost
// by a single half-even rounding). |d.DivInt64(q).MulInt64(q) - d| <= |q|/2.
func TestProperty_DivMulRoundTripBounded(t *testing.T) {
	rng := rand.New(rand.NewSource(seed + 4))
	for i := 0; i < iterations; i++ {
		raw := randRawNarrow(rng)
		q := rng.Int63n(1_000_000) + 1 // [1, 1e6], positive
		d := rawDecimal(raw)

		back := d.DivInt64(q).MulInt64(q)
		diff := new(big.Int).Sub(big.NewInt(back.Raw()), big.NewInt(raw))
		diff.Abs(diff)
		halfQ := new(big.Int).Div(big.NewInt(q), big.NewInt(2))
		// round-to-nearest error is <= q/2 (with ties allowed at exactly q/2).
		bound := new(big.Int).Add(halfQ, big.NewInt(1))
		if diff.Cmp(bound) > 0 {
			t.Fatalf("div/mul bound: |%d - %d| = %s > q/2(=%d)", back.Raw(), raw, diff, q)
		}
	}
}

// TestProperty_AddSubInverse asserts (a+b)-b == a whenever no overflow occurs.
func TestProperty_AddSubInverse(t *testing.T) {
	rng := rand.New(rand.NewSource(seed + 5))
	for i := 0; i < iterations; i++ {
		a := rawDecimal(randRawNarrow(rng))
		b := rawDecimal(randRawNarrow(rng))
		// narrow inputs: a+b cannot overflow int64.
		if got := a.Add(b).Sub(b); !got.Equal(a) {
			t.Fatalf("add/sub inverse: (%s + %s) - %s = %s, want %s", a, b, b, got, a)
		}
	}
}

// TestProperty_AddOverflowErrorsNotWraps confirms wide-range addition either
// returns the exact sum or reports overflow — it never wraps to a wrong sign.
func TestProperty_AddOverflowErrorsNotWraps(t *testing.T) {
	rng := rand.New(rand.NewSource(seed + 6))
	for i := 0; i < iterations; i++ {
		ra := randRawWide(rng)
		rb := randRawWide(rng)
		oracle := new(big.Int).Add(big.NewInt(ra), big.NewInt(rb))
		got, err := rawDecimal(ra).TryAdd(rawDecimal(rb))
		requireOracle(t, "Add", oracle, got, err)
	}
}

// TestProperty_MulDecimalCommutative asserts a*b == b*a for all inputs, including
// when both overflow (both must error identically).
func TestProperty_MulDecimalCommutative(t *testing.T) {
	rng := rand.New(rand.NewSource(seed + 7))
	for i := 0; i < iterations; i++ {
		a := rawDecimal(randRawWide(rng))
		b := rawDecimal(randRawWide(rng))
		ab, errAB := a.TryMulDecimal(b)
		ba, errBA := b.TryMulDecimal(a)
		if (errAB == nil) != (errBA == nil) {
			t.Fatalf("commutativity error mismatch: a*b err=%v, b*a err=%v", errAB, errBA)
		}
		if errAB == nil && !ab.Equal(ba) {
			t.Fatalf("commutativity: %s*%s=%s but %s*%s=%s", a, b, ab, b, a, ba)
		}
	}
}

// TestProperty_NegateInvolution asserts -(-d) == d for all non-MinInt64 values,
// and that MinInt64 negation reports overflow rather than returning MinInt64.
func TestProperty_NegateInvolution(t *testing.T) {
	rng := rand.New(rand.NewSource(seed + 8))
	for i := 0; i < iterations; i++ {
		raw := randRawWide(rng)
		d := rawDecimal(raw)
		neg, err := d.TryNegate()
		if raw == math.MinInt64 {
			if !errors.Is(err, decimal.ErrOverflow) {
				t.Fatalf("Negate(MinInt64): want overflow, got %v", err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("Negate(%s): unexpected error %v", d, err)
		}
		if back, err2 := neg.TryNegate(); err2 != nil || !back.Equal(d) {
			t.Fatalf("involution: -(-%s) = %s (err %v), want %s", d, back, err2, d)
		}
	}
}

// TestProperty_CmpTrichotomy asserts Cmp is a total order consistent with the
// boolean comparators for every random pair.
func TestProperty_CmpTrichotomy(t *testing.T) {
	rng := rand.New(rand.NewSource(seed + 9))
	for i := 0; i < iterations; i++ {
		a := rawDecimal(randRawWide(rng))
		b := rawDecimal(randRawWide(rng))
		c := a.Cmp(b)
		switch {
		case c < 0:
			if !a.LessThan(b) || a.GreaterThanOrEqual(b) || a.Equal(b) {
				t.Fatalf("trichotomy <: a=%s b=%s", a, b)
			}
		case c > 0:
			if !a.GreaterThan(b) || a.LessThanOrEqual(b) || a.Equal(b) {
				t.Fatalf("trichotomy >: a=%s b=%s", a, b)
			}
		default:
			if !a.Equal(b) || a.LessThan(b) || a.GreaterThan(b) {
				t.Fatalf("trichotomy ==: a=%s b=%s", a, b)
			}
		}
		// Antisymmetry: cmp(a,b) == -cmp(b,a).
		if a.Cmp(b) != -b.Cmp(a) {
			t.Fatalf("antisymmetry: a=%s b=%s", a, b)
		}
	}
}
