package fix

import (
	"strconv"
	"testing"

	"github.com/garudax-platform/decimal"
)

// TestPriceRoundTripNoFloatDrift is the regression test for R021. It proves that
// carrying a FIX price tag through the shared Decimal type (ParseDecimal inbound,
// String() outbound) preserves the exact wire value, whereas the previous
// float64 path (strconv.ParseFloat → FormatFloat) silently corrupted large
// notionals. FIX tags are strings on the wire, so the price never has to touch
// binary floating point.
func TestPriceRoundTripNoFloatDrift(t *testing.T) {
	cases := []string{
		"275.50",
		"150.25",
		"0.0001",
		"8299.0001",
		"99999999.9999",
		"9007199254740.9931", // beyond float64's exact integer range — float drifts here
	}

	for _, in := range cases {
		// Inbound: parse the FIX tag into the order's Decimal price.
		msg := &FIXMessage{Fields: map[int]string{TagPrice: in}}
		price := GetDecimalTag(msg, TagPrice)

		// Outbound: render it back onto the wire via an ExecutionReport.
		exec := MapExecutionReport("ORD-1", "EXEC-1", "0", "0", SideBuy, 1, price, 0, 1)
		got := GetTag(exec, TagAvgPx)

		// The Decimal round-trip must equal the exact input (trailing-zero
		// normalization aside, which we compare via the Decimal itself).
		want := decimal.MustParse(in)
		if !price.Equal(want) {
			t.Errorf("inbound drift for %q: got %s, want %s", in, price.String(), want.String())
		}
		if got != want.String() {
			t.Errorf("outbound drift for %q: AvgPx=%q, want %q", in, got, want.String())
		}
	}
}

// TestPriceRoundTripBeatsFloat64 directly contrasts the Decimal path against the
// old float64 path for a value the old code mangled, locking in the fix.
func TestPriceRoundTripBeatsFloat64(t *testing.T) {
	const in = "9007199254740.9931"

	// Old behaviour: ParseFloat then FormatFloat to 4dp.
	f, _ := strconv.ParseFloat(in, 64)
	floatOut := strconv.FormatFloat(f, 'f', 4, 64)
	if floatOut == in {
		t.Skipf("float64 no longer drifts for %q on this platform; test value needs refreshing", in)
	}

	// New behaviour: Decimal preserves the exact value.
	d := decimal.MustParse(in)
	if d.String() != in {
		t.Errorf("Decimal path drifted: got %s, want %s", d.String(), in)
	}
	if floatOut == d.String() {
		t.Errorf("expected Decimal (%s) to differ from float64 (%s) for %q", d.String(), floatOut, in)
	}
}
