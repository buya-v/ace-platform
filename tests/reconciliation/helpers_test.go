// Package reconciliation holds property-based and cross-service reconciliation
// tests for the shared GarudaX money type (github.com/garudax-platform/decimal).
//
// These tests exercise the *real* shared decimal package — the same code that
// R003 wired into the six engines and R004 wired into securities-service — via a
// filesystem `replace` in go.mod. They live under tests/ (not in any service)
// because they validate behaviour that must hold identically across services:
//
//   - Mul/Div round-trip and rounding-direction invariants (property-based).
//   - Overflow on large notionals errors instead of silently wrapping int64.
//   - Divide-by-zero errors instead of returning zero.
//   - Cross-service reconciliation: a trade priced and quantified the same way
//     produces identical settlement cash in securities-service and the
//     settlement-engine (the previous float-vs-int boundary).
//
// The property tests use math/big as an independent oracle: the exact integer
// arithmetic is computed in big.Int and the half-even-rounded result is compared
// against what the decimal package returns. This catches both wrong rounding and
// silent overflow in a single randomized sweep.
package reconciliation

import (
	"math"
	"math/big"
	"math/rand"
	"testing"

	"github.com/garudax-platform/decimal"
)

// seed keeps the randomized property sweeps deterministic so a failure is
// reproducible and CI is stable. Change it locally to fuzz more inputs.
const seed = 0x6A52D5

// iterations is the number of random cases each property test runs.
const iterations = 20000

var bigScale = big.NewInt(decimal.Scale)
var bigMaxInt64 = big.NewInt(math.MaxInt64)
var bigMinInt64 = big.NewInt(math.MinInt64)

// fitsInt64 reports whether n is representable as an int64.
func fitsInt64(n *big.Int) bool {
	return n.Cmp(bigMinInt64) >= 0 && n.Cmp(bigMaxInt64) <= 0
}

// roundHalfEvenBig computes round(num/den) with banker's (half-to-even) rounding.
// It is the independent oracle the decimal package's mulDivRound must match.
// den must be non-zero.
func roundHalfEvenBig(num, den *big.Int) *big.Int {
	n := new(big.Int).Set(num)
	d := new(big.Int).Set(den)
	neg := (n.Sign() < 0) != (d.Sign() < 0)
	n.Abs(n)
	d.Abs(d)

	q := new(big.Int)
	r := new(big.Int)
	q.QuoRem(n, d, r) // q = n/d (trunc), r = n%d; 0 <= r < d (operands non-negative)

	twoR := new(big.Int).Lsh(r, 1) // 2*r
	switch twoR.Cmp(d) {
	case 1: // remainder > half -> round up
		q.Add(q, big.NewInt(1))
	case 0: // exactly half -> round to even
		if q.Bit(0) == 1 {
			q.Add(q, big.NewInt(1))
		}
	}
	if neg {
		q.Neg(q)
	}
	return q
}

// rawDecimal wraps a raw scaled int64 as a Decimal (no rescaling).
func rawDecimal(raw int64) decimal.Decimal { return decimal.DecimalFromRaw(raw) }

// randRawNarrow returns a raw int64 in a "money-sized" range (|v| < ~1e13, i.e.
// values up to ~1e9 at 4dp). Products of two such values stay well inside int64,
// so these inputs exercise the rounding paths without tripping overflow.
func randRawNarrow(rng *rand.Rand) int64 {
	v := rng.Int63n(20_000_000_000_001) - 10_000_000_000_000 // [-1e13, 1e13]
	return v
}

// randRawWide returns a raw int64 across the entire int64 range, so products and
// quotients frequently overflow — this is what drives the "errors not wraps" check.
func randRawWide(rng *rand.Rand) int64 {
	// Mix full-range values with values clustered near the int64 boundaries.
	switch rng.Intn(4) {
	case 0:
		return math.MaxInt64 - rng.Int63n(1_000_000)
	case 1:
		return math.MinInt64 + rng.Int63n(1_000_000)
	default:
		return rng.Int63() - rng.Int63() // roughly uniform, both signs
	}
}

// requireOracle asserts the decimal result matches the big.Int oracle, treating
// "expected doesn't fit int64" as a required overflow error.
func requireOracle(t *testing.T, label string, expected *big.Int, got decimal.Decimal, err error) {
	t.Helper()
	if fitsInt64(expected) {
		if err != nil {
			t.Fatalf("%s: unexpected error %v (expected %s)", label, err, expected)
		}
		if got.Raw() != expected.Int64() {
			t.Fatalf("%s: got raw %d, oracle %s", label, got.Raw(), expected)
		}
		return
	}
	// Expected value overflows int64 -> the API MUST report overflow, never wrap.
	if err == nil {
		t.Fatalf("%s: expected overflow error, got value raw=%d (oracle %s would wrap)",
			label, got.Raw(), expected)
	}
}
