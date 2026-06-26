package decimal

import (
	"encoding/json"
	"errors"
	"math"
	"testing"
)

func TestConstructorsAndString(t *testing.T) {
	cases := []struct {
		d    Decimal
		want string
	}{
		{NewDecimal(123, 4567), "123.4567"},
		{NewDecimal(0, 0), "0"},
		{DecimalFromInt(100), "100"},
		{DecimalFromInt(-5), "-5"},
		{DecimalFromRaw(12345), "1.2345"},
		{DecimalFromRaw(-12300), "-1.23"},
		{DecimalZero(), "0"},
		{Zero(), "0"},
		{MustParse("3.5"), "3.5"},
	}
	for _, c := range cases {
		if got := c.d.String(); got != c.want {
			t.Errorf("String() = %q, want %q", got, c.want)
		}
	}
}

func TestParseRoundsHalfEvenNotTruncate(t *testing.T) {
	cases := []struct {
		in  string
		raw int64 // expected scaled value
	}{
		{"", 0},
		{"100", 1000000},
		{"-0.5", -5000},
		{"+1.25", 12500},
		{"123.4567", 1234567},
		// >4 fractional digits: must round half-even, NOT truncate (old bug)
		{"1.23455", 12346},  // 5th digit 5, tie, kept 5(odd) -> up to 6
		{"1.23445", 12344},  // tie, kept 4(even) -> stays 4
		{"1.234551", 12346}, // >half -> up
		{"1.234549", 12345}, // <half -> down
		{"0.00005", 0},      // tie at last place, even -> stays 0
		{"0.00015", 2},      // tie, kept 1(odd) -> up to 2
	}
	for _, c := range cases {
		d, err := ParseDecimal(c.in)
		if err != nil {
			t.Fatalf("ParseDecimal(%q) error: %v", c.in, err)
		}
		if d.Raw() != c.raw {
			t.Errorf("ParseDecimal(%q).Raw() = %d, want %d", c.in, d.Raw(), c.raw)
		}
	}
}

func TestParseInvalid(t *testing.T) {
	for _, in := range []string{"abc", "1.2.3", "1.", "-", "1.2x", "+"} {
		if _, err := ParseDecimal(in); err == nil {
			t.Errorf("ParseDecimal(%q) expected error, got nil", in)
		}
	}
}

func TestAddSubOverflow(t *testing.T) {
	max := DecimalFromRaw(math.MaxInt64)
	if _, err := max.TryAdd(DecimalFromRaw(1)); !errors.Is(err, ErrOverflow) {
		t.Errorf("TryAdd overflow: got %v", err)
	}
	min := DecimalFromRaw(math.MinInt64)
	if _, err := min.TrySub(DecimalFromRaw(1)); !errors.Is(err, ErrOverflow) {
		t.Errorf("TrySub overflow: got %v", err)
	}
	if got := DecimalFromRaw(3).Add(DecimalFromRaw(4)).Raw(); got != 7 {
		t.Errorf("Add = %d, want 7", got)
	}
	if got := DecimalFromRaw(10).Sub(DecimalFromRaw(4)).Raw(); got != 6 {
		t.Errorf("Sub = %d, want 6", got)
	}
}

func TestMulDecimalOverflowNotSilent(t *testing.T) {
	// The margin-engine bug: (a*b)/scale wraps int64 silently for large values.
	// 1e9 * 1e9 fixed-point = 1e18 which overflows the int64 raw range.
	big := DecimalFromInt(1_000_000_000) // raw 1e13
	if _, err := big.TryMulDecimal(big); !errors.Is(err, ErrOverflow) {
		t.Errorf("TryMulDecimal large notional: expected overflow, got %v", err)
	}
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("MulDecimal large notional: expected panic")
			}
		}()
		_ = big.MulDecimal(big)
	}()
}

func TestMulDecimalRoundsHalfEven(t *testing.T) {
	// 1.00005 * 1 would be 1.00005 -> rounds to 1.0000 (tie, even).
	// Use raw-level checks: a=10001 (1.0001), b=15000 (1.5) -> 1.00015 -> round to 1.0002 (tie odd? kept .0001 *1.5)
	a := MustParse("1.0001")
	b := MustParse("1.5")
	got := a.MulDecimal(b) // 1.0001 * 1.5 = 1.50015 -> half-even -> 1.5002 (kept 1, odd -> up)
	if got.String() != "1.5002" {
		t.Errorf("MulDecimal = %s, want 1.5002", got.String())
	}
	// Truncation would have given 1.5001 — confirm we did NOT truncate.
	if got.Raw() == 15001 {
		t.Errorf("MulDecimal truncated instead of rounding")
	}
}

func TestDivByZeroIsError(t *testing.T) {
	if _, err := DecimalFromInt(10).TryDivInt64(0); !errors.Is(err, ErrDivideByZero) {
		t.Errorf("TryDivInt64(0): got %v, want ErrDivideByZero", err)
	}
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("DivInt64(0): expected panic, got none")
			}
		}()
		_ = DecimalFromInt(10).DivInt64(0)
	}()
}

func TestDivRoundsHalfEven(t *testing.T) {
	// 1.0000 / 3 = 0.33333... -> 0.3333
	if got := DecimalFromInt(1).DivInt64(3).String(); got != "0.3333" {
		t.Errorf("1/3 = %s, want 0.3333", got)
	}
	// raw 5 / 2 = 2.5 -> half-even -> 2 (even)
	if got := DecimalFromRaw(5).DivInt64(2).Raw(); got != 2 {
		t.Errorf("raw 5/2 = %d, want 2 (half-even)", got)
	}
	// raw 7 / 2 = 3.5 -> half-even -> 4 (even)
	if got := DecimalFromRaw(7).DivInt64(2).Raw(); got != 4 {
		t.Errorf("raw 7/2 = %d, want 4 (half-even)", got)
	}
	// negative divisor
	if got := DecimalFromInt(10).DivInt64(-2).String(); got != "-5" {
		t.Errorf("10/-2 = %s, want -5", got)
	}
}

func TestMulUint64AndInt64(t *testing.T) {
	price := MustParse("12.50")
	if got := price.MulUint64(4).String(); got != "50" {
		t.Errorf("12.50 * 4 = %s, want 50", got)
	}
	if got := price.MulInt64(-2).String(); got != "-25" {
		t.Errorf("12.50 * -2 = %s, want -25", got)
	}
	// overflow
	if _, err := DecimalFromRaw(math.MaxInt64).TryMulUint64(2); !errors.Is(err, ErrOverflow) {
		t.Errorf("TryMulUint64 overflow: got %v", err)
	}
}

func TestNegateAbsMinInt64(t *testing.T) {
	if _, err := DecimalFromRaw(math.MinInt64).TryNegate(); !errors.Is(err, ErrOverflow) {
		t.Errorf("TryNegate(MinInt64): expected overflow")
	}
	if got := DecimalFromInt(-7).Abs().String(); got != "7" {
		t.Errorf("Abs(-7) = %s, want 7", got)
	}
	if got := DecimalFromInt(5).Negate().String(); got != "-5" {
		t.Errorf("Negate(5) = %s, want -5", got)
	}
}

func TestComparisons(t *testing.T) {
	a := MustParse("1.5")
	b := MustParse("2.5")
	if !a.LessThan(b) || !b.GreaterThan(a) || a.Equal(b) {
		t.Errorf("comparison failed")
	}
	if a.Cmp(b) != -1 || b.Cmp(a) != 1 || a.Cmp(a) != 0 {
		t.Errorf("Cmp failed")
	}
	if a.Max(b).Cmp(b) != 0 || a.Min(b).Cmp(a) != 0 {
		t.Errorf("Max/Min failed")
	}
	if !a.LessThanOrEqual(a) || !a.GreaterThanOrEqual(a) {
		t.Errorf("LE/GE failed")
	}
}

// TestCrossServiceReconciliation guards the float-vs-int boundary that caused
// securities-service settlement cash to disagree with the settlement engine.
// price * quantity must be exact regardless of which multiply path is used.
func TestCrossServiceReconciliation(t *testing.T) {
	price := MustParse("17.3300")
	qty := int64(137)
	// settlement-engine style: MulInt64
	viaInt := price.MulInt64(qty)
	// securities style (now also Decimal): MulUint64
	viaUint := price.MulUint64(uint64(qty))
	if !viaInt.Equal(viaUint) {
		t.Fatalf("reconciliation mismatch: int=%s uint=%s", viaInt, viaUint)
	}
	if viaInt.String() != "2374.21" {
		t.Errorf("17.33 * 137 = %s, want 2374.21", viaInt.String())
	}
}

func TestRawRoundTrip(t *testing.T) {
	for _, raw := range []int64{0, 1, -1, 12345, -98765, math.MaxInt64, math.MinInt64} {
		if got := DecimalFromRaw(raw).Raw(); got != raw {
			t.Errorf("raw round-trip %d -> %d", raw, got)
		}
	}
}

func TestPredicates(t *testing.T) {
	if !DecimalZero().IsZero() || DecimalFromInt(1).IsZero() {
		t.Errorf("IsZero failed")
	}
	if !DecimalFromInt(-1).IsNeg() || DecimalFromInt(1).IsNeg() {
		t.Errorf("IsNeg failed")
	}
	if !DecimalFromInt(1).IsPos() || DecimalFromInt(-1).IsPos() {
		t.Errorf("IsPos failed")
	}
}

func TestDivIntAlias(t *testing.T) {
	if got := DecimalFromInt(10).DivInt(4).String(); got != "2.5" {
		t.Errorf("DivInt = %s, want 2.5", got)
	}
}

func TestConstructorOverflowPanics(t *testing.T) {
	mustPanic(t, "NewDecimal", func() { NewDecimal(math.MaxInt64, 0) })
	mustPanic(t, "DecimalFromInt", func() { DecimalFromInt(math.MaxInt64) })
	mustPanic(t, "MustParse", func() { MustParse("not-a-number") })
}

func TestMaxMinReverse(t *testing.T) {
	a := MustParse("1")
	b := MustParse("2")
	if a.Max(b).Cmp(b) != 0 {
		t.Errorf("Max reverse failed")
	}
	if b.Min(a).Cmp(a) != 0 {
		t.Errorf("Min reverse failed")
	}
}

func TestParseOverflow(t *testing.T) {
	// integer part overflows int64*Scale
	if _, err := ParseDecimal("99999999999999999999"); err == nil {
		t.Errorf("expected overflow/format error for huge int part")
	}
}

func TestMinInt64Magnitude(t *testing.T) {
	// abs64 / signedFromMag round-trip at the int64 boundary via String.
	if got := DecimalFromRaw(math.MinInt64).String(); got == "" {
		t.Errorf("MinInt64 String empty")
	}
	// DivInt64 of MinInt64 by 1 must overflow (magnitude MaxInt64+1, neg ok)
	if got := DecimalFromRaw(math.MinInt64).DivInt64(1).Raw(); got != math.MinInt64 {
		t.Errorf("MinInt64/1 = %d, want MinInt64", got)
	}
}

func mustPanic(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("%s: expected panic, got none", name)
		}
	}()
	fn()
}

func TestJSONRoundTrip(t *testing.T) {
	type wrap struct {
		Price Decimal `json:"price"`
	}
	// marshal emits a bare JSON number
	b, err := json.Marshal(wrap{Price: MustParse("12.5")})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"price":12.5}` {
		t.Errorf("Marshal = %s, want {\"price\":12.5}", b)
	}
	// unmarshal from a number
	var w wrap
	if err := json.Unmarshal([]byte(`{"price":17.33}`), &w); err != nil {
		t.Fatal(err)
	}
	if w.Price.String() != "17.33" {
		t.Errorf("Unmarshal number = %s, want 17.33", w.Price)
	}
	// unmarshal from a string and from null
	if err := json.Unmarshal([]byte(`{"price":"4.25"}`), &w); err != nil {
		t.Fatal(err)
	}
	if w.Price.String() != "4.25" {
		t.Errorf("Unmarshal string = %s, want 4.25", w.Price)
	}
	// >4 dp number rounds, not truncates
	if err := json.Unmarshal([]byte(`{"price":1.234551}`), &w); err != nil {
		t.Fatal(err)
	}
	if w.Price.String() != "1.2346" {
		t.Errorf("Unmarshal long = %s, want 1.2346", w.Price)
	}
}

func TestFloatInterop(t *testing.T) {
	d := MustParse("12.34")
	if d.Float64() != 12.34 {
		t.Errorf("Float64 = %v, want 12.34", d.Float64())
	}
	got, err := NewFromFloat(12.34)
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "12.34" {
		t.Errorf("NewFromFloat = %s, want 12.34", got)
	}
	if _, err := NewFromFloat(math.Inf(1)); err == nil {
		t.Errorf("NewFromFloat(Inf) expected error")
	}
}
