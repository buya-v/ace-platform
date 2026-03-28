package types

import "testing"

func TestNewDecimal(t *testing.T) {
	d := NewDecimal(5, 2500)
	if d.String() != "5.25" {
		t.Errorf("NewDecimal(5, 2500) = %s, want 5.25", d.String())
	}
}

func TestDecimalFromInt(t *testing.T) {
	d := DecimalFromInt(100)
	if d.Raw() != 1000000 {
		t.Errorf("Raw() = %d, want 1000000", d.Raw())
	}
	if d.String() != "100" {
		t.Errorf("String() = %s, want 100", d.String())
	}
}

func TestDecimalZero(t *testing.T) {
	d := DecimalZero()
	if !d.IsZero() {
		t.Error("DecimalZero should be zero")
	}
	if d.Raw() != 0 {
		t.Errorf("Raw() = %d, want 0", d.Raw())
	}
}

func TestDecimalFromRaw(t *testing.T) {
	d := DecimalFromRaw(12345)
	if d.Raw() != 12345 {
		t.Errorf("Raw() = %d, want 12345", d.Raw())
	}
}

func TestParseDecimal(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"100", "100"},
		{"100.50", "100.5"},
		{"0", "0"},
		{"-50", "-50"},
		{"-123.4567", "-123.4567"},
		{"0.0001", "0.0001"},
		{"999.99", "999.99"},
		{"  42  ", "42"},
		{"", "0"},
		{"1.123456789", "1.1234"}, // truncation
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			d, err := ParseDecimal(tt.input)
			if err != nil {
				t.Fatalf("ParseDecimal(%q): %v", tt.input, err)
			}
			if d.String() != tt.want {
				t.Errorf("got %s, want %s", d.String(), tt.want)
			}
		})
	}
}

func TestParseDecimalErrors(t *testing.T) {
	tests := []string{"abc", "12.34.56", "1.abcd"}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := ParseDecimal(input)
			if err == nil {
				t.Errorf("expected error for %q", input)
			}
		})
	}
}

func TestDecimalIsZero(t *testing.T) {
	if !DecimalZero().IsZero() {
		t.Error("zero should be zero")
	}
	if DecimalFromInt(1).IsZero() {
		t.Error("1 should not be zero")
	}
}

func TestDecimalIsNeg(t *testing.T) {
	if !DecimalFromInt(-1).IsNeg() {
		t.Error("-1 should be negative")
	}
	if DecimalFromInt(1).IsNeg() {
		t.Error("1 should not be negative")
	}
	if DecimalZero().IsNeg() {
		t.Error("0 should not be negative")
	}
}

func TestDecimalNegate(t *testing.T) {
	d := DecimalFromInt(100)
	neg := d.Negate()
	if neg.String() != "-100" {
		t.Errorf("Negate(100) = %s, want -100", neg.String())
	}
	if !neg.Negate().Equal(d) {
		t.Error("double negate should return original")
	}
}

func TestDecimalAdd(t *testing.T) {
	a := DecimalFromInt(100)
	b := DecimalFromInt(200)
	sum := a.Add(b)
	if sum.String() != "300" {
		t.Errorf("100 + 200 = %s, want 300", sum.String())
	}
}

func TestDecimalSub(t *testing.T) {
	a := DecimalFromInt(500)
	b := DecimalFromInt(200)
	diff := a.Sub(b)
	if diff.String() != "300" {
		t.Errorf("500 - 200 = %s, want 300", diff.String())
	}
}

func TestDecimalMulUint64(t *testing.T) {
	d := DecimalFromInt(50)
	result := d.MulUint64(10)
	if result.String() != "500" {
		t.Errorf("50 * 10 = %s, want 500", result.String())
	}
}

func TestDecimalAbs(t *testing.T) {
	pos := DecimalFromInt(50)
	neg := DecimalFromInt(-50)
	zero := DecimalZero()

	if !pos.Abs().Equal(DecimalFromInt(50)) {
		t.Error("Abs(50) != 50")
	}
	if !neg.Abs().Equal(DecimalFromInt(50)) {
		t.Error("Abs(-50) != 50")
	}
	if !zero.Abs().IsZero() {
		t.Error("Abs(0) != 0")
	}
}

func TestDecimalCmp(t *testing.T) {
	a := DecimalFromInt(100)
	b := DecimalFromInt(200)
	c := DecimalFromInt(100)

	if a.Cmp(b) != -1 {
		t.Errorf("Cmp(100, 200) = %d, want -1", a.Cmp(b))
	}
	if b.Cmp(a) != 1 {
		t.Errorf("Cmp(200, 100) = %d, want 1", b.Cmp(a))
	}
	if a.Cmp(c) != 0 {
		t.Errorf("Cmp(100, 100) = %d, want 0", a.Cmp(c))
	}
}

func TestDecimalLessThan(t *testing.T) {
	if !DecimalFromInt(1).LessThan(DecimalFromInt(2)) {
		t.Error("1 < 2 should be true")
	}
	if DecimalFromInt(2).LessThan(DecimalFromInt(1)) {
		t.Error("2 < 1 should be false")
	}
}

func TestDecimalGreaterThan(t *testing.T) {
	if !DecimalFromInt(2).GreaterThan(DecimalFromInt(1)) {
		t.Error("2 > 1 should be true")
	}
	if DecimalFromInt(1).GreaterThan(DecimalFromInt(2)) {
		t.Error("1 > 2 should be false")
	}
}

func TestDecimalEqual(t *testing.T) {
	if !DecimalFromInt(100).Equal(DecimalFromInt(100)) {
		t.Error("100 == 100 should be true")
	}
	if DecimalFromInt(100).Equal(DecimalFromInt(200)) {
		t.Error("100 == 200 should be false")
	}
}

func TestDecimalString(t *testing.T) {
	tests := []struct {
		d    Decimal
		want string
	}{
		{DecimalFromInt(0), "0"},
		{DecimalFromInt(100), "100"},
		{DecimalFromInt(-100), "-100"},
		{NewDecimal(5, 2500), "5.25"},
		{NewDecimal(5, 2500).Negate(), "-5.25"},
		{NewDecimal(0, 1), "0.0001"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.d.String(); got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestDecimalNegativeString(t *testing.T) {
	// Negative with fraction
	d := NewDecimal(10, 5000).Negate() // -10.5
	if d.String() != "-10.5" {
		t.Errorf("got %s, want -10.5", d.String())
	}
}
