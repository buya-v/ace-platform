package reconciliation

import (
	"math"
	"math/big"
	"math/rand"
	"testing"

	"github.com/garudax-platform/decimal"
)

// This file reconciles the settlement-cash math of the two services that sit on
// opposite sides of the old float-vs-int boundary, now that both use the shared
// decimal type (R003 engines, R004 securities-service).
//
// The exact one-line formulas, copied from each service's source:
//
//   securities-service  internal/settlement/engine.go:80
//                       NetAmount = trade.Price.MulInt64(int64(trade.Quantity))
//   securities-service  internal/settlement/equities.go:186
//                       NetAmount = trade.Price.MulInt64(int64(trade.Quantity))
//   settlement-engine   internal/valuation/valuation.go:96
//                       ValuePosition = markPrice.MulInt64(qty)
//
// They must agree to the cent for any (price, quantity). They are reproduced
// here (not imported) because both live in `internal/` packages that other
// modules cannot import; the shared decimal type they both call IS imported.

// securitiesNetAmount mirrors securities-service equities/engine settlement cash.
func securitiesNetAmount(price decimal.Decimal, quantity int) decimal.Decimal {
	return price.MulInt64(int64(quantity))
}

// settlementEngineCash mirrors settlement-engine's notional valuation. The engine
// values the absolute quantity; for a fully-matched buy/sell the magnitude is the
// same cash figure the securities service settles.
func settlementEngineCash(markPrice decimal.Decimal, qty int64) decimal.Decimal {
	if qty < 0 {
		qty = -qty
	}
	return markPrice.MulInt64(qty)
}

// TestReconcile_EquitiesCashLeg is the headline cross-service check: identical
// price and quantity yield identical settlement cash on both sides.
func TestReconcile_EquitiesCashLeg(t *testing.T) {
	cases := []struct {
		price string
		qty   int
		want  string
	}{
		{"17.3300", 137, "2374.21"},
		{"12.50", 100, "1250"},
		{"4.0150", 100, "401.5"},     // float-truncation trap (see regression test)
		{"0.0001", 1_000_000, "100"}, // tick-sized price, large qty
		{"999.9999", 4321, "4320999.5679"},
		{"1000", 1, "1000"},
	}
	for _, c := range cases {
		price := decimal.MustParse(c.price)
		sec := securitiesNetAmount(price, c.qty)
		eng := settlementEngineCash(price, int64(c.qty))
		if !sec.Equal(eng) {
			t.Errorf("reconcile %s x %d: securities=%s settlement=%s",
				c.price, c.qty, sec, eng)
		}
		if sec.String() != c.want {
			t.Errorf("reconcile %s x %d = %s, want %s", c.price, c.qty, sec, c.want)
		}
	}
}

// TestReconcile_FloatBoundaryRegression demonstrates *why* the migration was
// needed: the old securities-service path multiplied a float64 price by quantity
// and truncated to the storage scale, which disagreed with the settlement
// engine's integer math. price 4.015 is not exactly representable in float64 (it
// is fractionally below 4.015), so 4.015 * 100 = 401.49999999… and truncating to
// 4dp yields 401.4999 — one tick short of the true 401.5000. The shared decimal
// path is exact (401.5000) and both services now agree.
func TestReconcile_FloatBoundaryRegression(t *testing.T) {
	const priceStr = "4.0150"
	const qty = 100

	// Old, buggy float path: parse to float64, multiply, truncate to 4dp.
	f := 4.015
	oldTruncated := math.Trunc(f*float64(qty)*float64(decimal.Scale)) / float64(decimal.Scale)
	if oldTruncated != 401.4999 {
		t.Fatalf("precondition: expected float truncation to 401.4999, got %.4f", oldTruncated)
	}

	// New shared-decimal path on both sides.
	price := decimal.MustParse(priceStr)
	sec := securitiesNetAmount(price, qty)
	eng := settlementEngineCash(price, qty)

	if sec.String() != "401.5" || eng.String() != "401.5" {
		t.Fatalf("decimal path: securities=%s settlement=%s, want 401.5 both", sec, eng)
	}
	if sec.Float64() == oldTruncated {
		t.Fatalf("decimal path reproduced the float truncation bug (%.4f)", oldTruncated)
	}
}

// bondParams mirrors the fields securities-service uses to accrue bond interest.
type bondParams struct {
	parValue   decimal.Decimal
	couponRate float64 // fraction, e.g. 0.05
	basis      int64   // 360 or 365
}

// securitiesBondNetAmount mirrors securities-service settlement engine.go:80+145
// and engine.go:201-227: base cash + accrued interest, accrued computed by
// dividing by the day-count basis LAST to preserve precision (R004 decision #3).
func securitiesBondNetAmount(price decimal.Decimal, quantity int, b bondParams) (decimal.Decimal, decimal.Decimal) {
	const defaultDaysSinceLastCoupon = 30
	base := price.MulInt64(int64(quantity))
	couponFactor, err := decimal.NewFromFloat(b.couponRate)
	if err != nil {
		return base, decimal.DecimalZero()
	}
	accrued := b.parValue.MulDecimal(couponFactor).
		MulInt64(defaultDaysSinceLastCoupon).
		MulInt64(int64(quantity)).
		DivInt64(b.basis)
	return base.Add(accrued), accrued
}

// TestReconcile_BondCashLegWithAccrued checks that the bond NetAmount (base +
// accrued) computed via the shared decimal path is exact and matches a big.Int
// oracle for the accrued component — the precision-sensitive divide-last path.
func TestReconcile_BondCashLegWithAccrued(t *testing.T) {
	b := bondParams{
		parValue:   decimal.MustParse("1000.0000"),
		couponRate: 0.05,
		basis:      365,
	}
	price := decimal.MustParse("100.0000")
	qty := 10

	net, accrued := securitiesBondNetAmount(price, qty, b)

	// Oracle for accrued: round_half_even(parRaw * couponRaw / Scale * 30 * qty / basis)
	// following the exact operation order (each intermediate is its own rounding step).
	couponFactor, err := decimal.NewFromFloat(b.couponRate)
	if err != nil {
		t.Fatal(err)
	}
	step1 := roundHalfEvenBig(
		new(big.Int).Mul(big.NewInt(b.parValue.Raw()), big.NewInt(couponFactor.Raw())),
		bigScale,
	) // parValue * couponFactor
	step2 := new(big.Int).Mul(step1, big.NewInt(30))
	step3 := new(big.Int).Mul(step2, big.NewInt(int64(qty)))
	oracleAccrued := roundHalfEvenBig(step3, big.NewInt(b.basis))

	if accrued.Raw() != oracleAccrued.Int64() {
		t.Errorf("accrued: got %s (raw %d), oracle raw %d", accrued, accrued.Raw(), oracleAccrued)
	}
	// NetAmount = base (1000.00) + accrued; base must be exact.
	base := price.MulInt64(int64(qty))
	if !net.Equal(base.Add(accrued)) {
		t.Errorf("net = %s, want base(%s)+accrued(%s)", net, base, accrued)
	}
}

// TestReconcile_NettedBatchSumsAreExact checks the property the netting step
// relies on: summing per-trade cash legs gives the same total whether you add
// the decimals or sum the raw products — no float drift accumulates over a batch.
func TestReconcile_NettedBatchSumsAreExact(t *testing.T) {
	rng := rand.New(rand.NewSource(seed + 100))
	const trades = 500

	sumDecimal := decimal.DecimalZero()
	sumRaw := big.NewInt(0)
	for i := 0; i < trades; i++ {
		// Money-sized price (<= ~1000.0000) and modest quantity so the batch
		// total stays well within int64.
		price := decimal.DecimalFromRaw(rng.Int63n(10_000_001)) // [0, 1000.0001)
		qty := rng.Int63n(1000) + 1

		leg := securitiesNetAmount(price, int(qty))
		// settlement engine computes the identical leg.
		if !leg.Equal(settlementEngineCash(price, qty)) {
			t.Fatalf("leg mismatch at trade %d: price=%s qty=%d", i, price, qty)
		}
		sumDecimal = sumDecimal.Add(leg)
		sumRaw.Add(sumRaw, new(big.Int).Mul(big.NewInt(price.Raw()), big.NewInt(qty)))
	}
	if !fitsInt64(sumRaw) {
		t.Fatalf("test setup overflowed int64 unexpectedly: %s", sumRaw)
	}
	if sumDecimal.Raw() != sumRaw.Int64() {
		t.Fatalf("batch sum drift: decimal=%d oracle=%s", sumDecimal.Raw(), sumRaw)
	}
}

// TestReconcile_RandomizedCashLegAgreement is a property sweep over the cash leg:
// for any money-sized price and quantity, the two services' formulas must produce
// identical results (or both overflow identically).
func TestReconcile_RandomizedCashLegAgreement(t *testing.T) {
	rng := rand.New(rand.NewSource(seed + 101))
	for i := 0; i < iterations; i++ {
		price := decimal.DecimalFromRaw(randRawWide(rng))
		qty := rng.Int63() % 1_000_000_000

		sec, errSec := func() (d decimal.Decimal, err error) {
			defer func() {
				if r := recover(); r != nil {
					err = decimal.ErrOverflow
				}
			}()
			return securitiesNetAmount(price, int(qty)), nil
		}()
		eng, errEng := func() (d decimal.Decimal, err error) {
			defer func() {
				if r := recover(); r != nil {
					err = decimal.ErrOverflow
				}
			}()
			return settlementEngineCash(price, qty), nil
		}()

		if (errSec == nil) != (errEng == nil) {
			t.Fatalf("overflow disagreement: price=%s qty=%d secErr=%v engErr=%v",
				price, qty, errSec, errEng)
		}
		if errSec == nil && !sec.Equal(eng) {
			t.Fatalf("cash leg mismatch: price=%s qty=%d securities=%s settlement=%s",
				price, qty, sec, eng)
		}
	}
}
