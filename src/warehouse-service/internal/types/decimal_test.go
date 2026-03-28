package types

import "testing"

func TestParseDecimal(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"100", "100"},
		{"100.5", "100.5"},
		{"100.1234", "100.1234"},
		{"0", "0"},
		{"", "0"},
		{"-50.25", "-50.25"},
		{"1000000", "1000000"},
	}
	for _, tc := range tests {
		d, err := ParseDecimal(tc.input)
		if err != nil {
			t.Errorf("ParseDecimal(%q): %v", tc.input, err)
			continue
		}
		if got := d.String(); got != tc.want {
			t.Errorf("ParseDecimal(%q).String() = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestDecimalArithmetic(t *testing.T) {
	a, _ := ParseDecimal("100.5")
	b, _ := ParseDecimal("50.25")

	sum := a.Add(b)
	if sum.String() != "150.75" {
		t.Errorf("100.5 + 50.25 = %s, want 150.75", sum.String())
	}

	diff := a.Sub(b)
	if diff.String() != "50.25" {
		t.Errorf("100.5 - 50.25 = %s, want 50.25", diff.String())
	}
}

func TestDecimalComparisons(t *testing.T) {
	a, _ := ParseDecimal("100")
	b, _ := ParseDecimal("200")
	c, _ := ParseDecimal("100")

	if !a.LessThan(b) {
		t.Error("expected 100 < 200")
	}
	if !b.GreaterThan(a) {
		t.Error("expected 200 > 100")
	}
	if a.LessThan(c) || a.GreaterThan(c) {
		t.Error("expected 100 == 100")
	}
}

func TestDecimalZero(t *testing.T) {
	z := DecimalZero()
	if !z.IsZero() {
		t.Error("expected zero")
	}
	if z.IsPos() {
		t.Error("zero should not be positive")
	}
	if z.IsNeg() {
		t.Error("zero should not be negative")
	}
}
