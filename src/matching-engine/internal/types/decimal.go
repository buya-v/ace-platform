package types

import (
	"fmt"
	"strconv"
	"strings"
)

// Decimal represents a fixed-point decimal with 4 decimal places.
// Internally stored as int64 scaled by 10000. This gives us Decimal(18,4)
// range: -922,337,203,685,477.5808 to 922,337,203,685,477.5807
// which is more than sufficient for commodity prices.
type Decimal struct {
	value int64 // price * 10000
}

const decimalScale = 10000

// NewDecimal creates a Decimal from an integer and fractional part.
func NewDecimal(integer int64, fraction int64) Decimal {
	return Decimal{value: integer*decimalScale + fraction}
}

// DecimalFromInt creates a Decimal from a whole number.
func DecimalFromInt(v int64) Decimal {
	return Decimal{value: v * decimalScale}
}

// Zero returns a zero Decimal.
func DecimalZero() Decimal {
	return Decimal{value: 0}
}

// DecimalFromRaw creates a Decimal from the raw internal value.
func DecimalFromRaw(raw int64) Decimal {
	return Decimal{value: raw}
}

// ParseDecimal parses a decimal string like "123.4567" or "100".
func ParseDecimal(s string) (Decimal, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return DecimalZero(), nil
	}

	negative := false
	if strings.HasPrefix(s, "-") {
		negative = true
		s = s[1:]
	}

	parts := strings.SplitN(s, ".", 2)
	intPart, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return DecimalZero(), fmt.Errorf("invalid decimal %q: %w", s, err)
	}

	var fracPart int64
	if len(parts) == 2 {
		fracStr := parts[1]
		// Pad or truncate to 4 digits
		if len(fracStr) > 4 {
			fracStr = fracStr[:4]
		}
		for len(fracStr) < 4 {
			fracStr += "0"
		}
		fracPart, err = strconv.ParseInt(fracStr, 10, 64)
		if err != nil {
			return DecimalZero(), fmt.Errorf("invalid decimal fraction %q: %w", s, err)
		}
	}

	val := intPart*decimalScale + fracPart
	if negative {
		val = -val
	}
	return Decimal{value: val}, nil
}

// Raw returns the internal scaled integer value.
func (d Decimal) Raw() int64 {
	return d.value
}

// IsZero returns true if the decimal is zero.
func (d Decimal) IsZero() bool {
	return d.value == 0
}

// Cmp compares two decimals. Returns -1, 0, or 1.
func (d Decimal) Cmp(other Decimal) int {
	if d.value < other.value {
		return -1
	}
	if d.value > other.value {
		return 1
	}
	return 0
}

// LessThan returns true if d < other.
func (d Decimal) LessThan(other Decimal) bool {
	return d.value < other.value
}

// GreaterThan returns true if d > other.
func (d Decimal) GreaterThan(other Decimal) bool {
	return d.value > other.value
}

// Equal returns true if d == other.
func (d Decimal) Equal(other Decimal) bool {
	return d.value == other.value
}

// GreaterThanOrEqual returns true if d >= other.
func (d Decimal) GreaterThanOrEqual(other Decimal) bool {
	return d.value >= other.value
}

// LessThanOrEqual returns true if d <= other.
func (d Decimal) LessThanOrEqual(other Decimal) bool {
	return d.value <= other.value
}

// Mul multiplies the decimal by a uint64 quantity and returns the result.
// Used for trade_value = price * quantity.
func (d Decimal) MulUint64(qty uint64) Decimal {
	return Decimal{value: d.value * int64(qty)}
}

// String returns the decimal as a string with up to 4 decimal places.
func (d Decimal) String() string {
	negative := d.value < 0
	v := d.value
	if negative {
		v = -v
	}
	intPart := v / decimalScale
	fracPart := v % decimalScale

	sign := ""
	if negative {
		sign = "-"
	}

	if fracPart == 0 {
		return fmt.Sprintf("%s%d", sign, intPart)
	}

	fracStr := fmt.Sprintf("%04d", fracPart)
	fracStr = strings.TrimRight(fracStr, "0")
	return fmt.Sprintf("%s%d.%s", sign, intPart, fracStr)
}

// Abs returns the absolute value.
func (d Decimal) Abs() Decimal {
	if d.value < 0 {
		return Decimal{value: -d.value}
	}
	return d
}

// Sub subtracts other from d.
func (d Decimal) Sub(other Decimal) Decimal {
	return Decimal{value: d.value - other.value}
}
