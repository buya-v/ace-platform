package store

import (
	"testing"

	"github.com/garudax-platform/market-data-service/internal/types"
)

// Compile-time interface compliance checks for in-memory stores.
var _ TradeRepository = (*TradeStore)(nil)
var _ CandleRepository = (*CandleStore)(nil)
var _ TickerRepository = (*TickerStore)(nil)

// Compile-time interface compliance checks for PostgreSQL stores.
var _ TradeRepository = (*PGTradeStore)(nil)
var _ CandleRepository = (*PGCandleStore)(nil)
var _ TickerRepository = (*PGTickerStore)(nil)

func TestDecimalToString(t *testing.T) {
	tests := []struct {
		input    types.Decimal
		expected string
	}{
		{types.DecimalFromInt(100), "100"},
		{types.DecimalZero(), "0"},
		{types.NewDecimal(105, 5000), "105.5"},
		{types.NewDecimal(99, 9900), "99.99"},
	}
	for _, tt := range tests {
		got := decimalToString(tt.input)
		if got != tt.expected {
			t.Errorf("decimalToString(%v) = %s, want %s", tt.input, got, tt.expected)
		}
	}
}

func TestMustParseDecimal(t *testing.T) {
	tests := []struct {
		input    string
		expected types.Decimal
	}{
		{"100", types.DecimalFromInt(100)},
		{"0", types.DecimalZero()},
		{"105.5", types.NewDecimal(105, 5000)},
		{"99.99", types.NewDecimal(99, 9900)},
		{"", types.DecimalZero()},
	}
	for _, tt := range tests {
		got := mustParseDecimal(tt.input)
		if !got.Equal(tt.expected) {
			t.Errorf("mustParseDecimal(%q) = %s, want %s", tt.input, got, tt.expected)
		}
	}
}

func TestParseInterval(t *testing.T) {
	tests := []struct {
		input    string
		expected types.CandleInterval
	}{
		{"1m", types.Interval1m},
		{"5m", types.Interval5m},
		{"15m", types.Interval15m},
		{"1h", types.Interval1h},
		{"4h", types.Interval4h},
		{"1d", types.Interval1d},
		{"unknown", types.Interval1m},
		{"", types.Interval1m},
	}
	for _, tt := range tests {
		got := parseInterval(tt.input)
		if got != tt.expected {
			t.Errorf("parseInterval(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestParseInterval_Roundtrip(t *testing.T) {
	for _, interval := range types.AllIntervals() {
		str := interval.String()
		parsed := parseInterval(str)
		if parsed != interval {
			t.Errorf("roundtrip failed: %d -> %s -> %d", interval, str, parsed)
		}
	}
}
