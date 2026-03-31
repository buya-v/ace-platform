package store

import (
	"testing"
	"time"

	"github.com/garudax-platform/margin-engine/internal/types"
)

func TestDecimalToString(t *testing.T) {
	tests := []struct {
		name     string
		input    types.Decimal
		expected string
	}{
		{"zero", types.DecimalZero(), "0"},
		{"integer", types.DecimalFromInt(100), "100"},
		{"fractional", types.NewDecimal(123, 4500), "123.45"},
		{"negative", types.DecimalFromInt(-50), "-50"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decimalToString(tt.input)
			if got != tt.expected {
				t.Errorf("decimalToString(%v) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNullableTime(t *testing.T) {
	t.Run("zero_time_returns_nil", func(t *testing.T) {
		result := nullableTime(time.Time{})
		if result != nil {
			t.Errorf("expected nil for zero time, got %v", result)
		}
	})

	t.Run("non_zero_time_returns_time", func(t *testing.T) {
		now := time.Now()
		result := nullableTime(now)
		if result == nil {
			t.Error("expected non-nil for non-zero time")
		}
		if result.(time.Time) != now {
			t.Errorf("expected %v, got %v", now, result)
		}
	})
}

func TestParseMarginCallStatus(t *testing.T) {
	tests := []struct {
		input    string
		expected types.MarginCallStatus
	}{
		{"ISSUED", types.MarginCallIssued},
		{"SATISFIED", types.MarginCallSatisfied},
		{"BREACHED", types.MarginCallBreached},
		{"PENDING", types.MarginCallPending},
		{"UNKNOWN", types.MarginCallPending},
		{"", types.MarginCallPending},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseMarginCallStatus(tt.input)
			if got != tt.expected {
				t.Errorf("parseMarginCallStatus(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

// TestPostgresPortfolioStoreInterface verifies the interface is satisfied at compile time.
func TestPostgresPortfolioStoreInterface(t *testing.T) {
	var _ PortfolioMarginStore = (*PostgresPortfolioStore)(nil)
}

// TestPostgresMarginCallStoreInterface verifies the interface is satisfied at compile time.
func TestPostgresMarginCallStoreInterface(t *testing.T) {
	var _ MarginCallStore = (*PostgresMarginCallStore)(nil)
}
