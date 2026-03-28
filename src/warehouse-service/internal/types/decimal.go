package types

import (
	"fmt"
	"strconv"
	"strings"
)

// Decimal represents a fixed-point decimal with 4 decimal places.
// Internally stored as int64 scaled by 10000.
type Decimal struct {
	value int64
}

const decimalScale = 10000

func NewDecimal(integer int64, fraction int64) Decimal {
	return Decimal{value: integer*decimalScale + fraction}
}

func DecimalFromInt(v int64) Decimal {
	return Decimal{value: v * decimalScale}
}

func DecimalZero() Decimal {
	return Decimal{value: 0}
}

func DecimalFromRaw(raw int64) Decimal {
	return Decimal{value: raw}
}

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

func (d Decimal) Raw() int64     { return d.value }
func (d Decimal) IsZero() bool   { return d.value == 0 }
func (d Decimal) IsNeg() bool    { return d.value < 0 }
func (d Decimal) IsPos() bool    { return d.value > 0 }
func (d Decimal) LessThan(o Decimal) bool { return d.value < o.value }
func (d Decimal) GreaterThan(o Decimal) bool { return d.value > o.value }

func (d Decimal) Add(o Decimal) Decimal { return Decimal{value: d.value + o.value} }
func (d Decimal) Sub(o Decimal) Decimal { return Decimal{value: d.value - o.value} }

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
