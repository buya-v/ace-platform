package types

import "testing"

func TestParseDecimal(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"100", "100"},
		{"100.50", "100.5"},
		{"100.5000", "100.5"},
		{"0.0001", "0.0001"},
		{"999999.9999", "999999.9999"},
		{"-50.25", "-50.25"},
		{"0", "0"},
		{"", "0"},
	}

	for _, tt := range tests {
		d, err := ParseDecimal(tt.input)
		if err != nil {
			t.Errorf("ParseDecimal(%q) error: %v", tt.input, err)
			continue
		}
		if got := d.String(); got != tt.want {
			t.Errorf("ParseDecimal(%q).String() = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDecimalComparison(t *testing.T) {
	a, _ := ParseDecimal("100.50")
	b, _ := ParseDecimal("100.25")
	c, _ := ParseDecimal("100.50")

	if !a.GreaterThan(b) {
		t.Error("100.50 should be > 100.25")
	}
	if !b.LessThan(a) {
		t.Error("100.25 should be < 100.50")
	}
	if !a.Equal(c) {
		t.Error("100.50 should equal 100.50")
	}
	if a.Cmp(b) != 1 {
		t.Error("Cmp(100.50, 100.25) should be 1")
	}
	if b.Cmp(a) != -1 {
		t.Error("Cmp(100.25, 100.50) should be -1")
	}
	if a.Cmp(c) != 0 {
		t.Error("Cmp(100.50, 100.50) should be 0")
	}
}

func TestDecimalMulUint64(t *testing.T) {
	price, _ := ParseDecimal("100.50")
	result := price.MulUint64(10)
	if result.String() != "1005" {
		t.Errorf("100.50 * 10 = %s, want 1005", result.String())
	}
}

func TestDecimalFromInt(t *testing.T) {
	d := DecimalFromInt(100)
	if d.String() != "100" {
		t.Errorf("DecimalFromInt(100) = %s, want 100", d.String())
	}
}
