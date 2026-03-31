package store

import (
	"testing"
	"time"

	"github.com/garudax-platform/settlement-engine/internal/types"
)

// TestDecimalToString verifies the decimal-to-SQL string conversion.
func TestDecimalToString(t *testing.T) {
	tests := []struct {
		name string
		dec  types.Decimal
		want string
	}{
		{"zero", types.DecimalZero(), "0"},
		{"integer", types.DecimalFromInt(500), "500"},
		{"with fraction", types.NewDecimal(100, 5000), "100.5"},
		{"negative", types.DecimalFromInt(-250), "-250"},
		{"from raw", types.DecimalFromRaw(12345), "1.2345"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decimalToString(tt.dec)
			if got != tt.want {
				t.Errorf("decimalToString(%v) = %q, want %q", tt.dec, got, tt.want)
			}
		})
	}
}

// TestPostgresCycleStoreCreation verifies construction with nil db.
func TestPostgresCycleStoreCreation(t *testing.T) {
	store := NewPostgresCycleStore(nil)
	if store == nil {
		t.Fatal("NewPostgresCycleStore returned nil")
	}
	if store.db != nil {
		t.Fatal("expected nil db")
	}
}

// TestPostgresInstructionStoreCreation verifies construction.
func TestPostgresInstructionStoreCreation(t *testing.T) {
	store := NewPostgresInstructionStore(nil)
	if store == nil {
		t.Fatal("NewPostgresInstructionStore returned nil")
	}
	if store.db != nil {
		t.Fatal("expected nil db")
	}
}

// TestPostgresPriceStoreCreation verifies construction.
func TestPostgresPriceStoreCreation(t *testing.T) {
	store := NewPostgresPriceStore(nil)
	if store == nil {
		t.Fatal("NewPostgresPriceStore returned nil")
	}
	if store.db != nil {
		t.Fatal("expected nil db")
	}
}

// TestParseCycleStatus verifies all status string conversions.
func TestParseCycleStatus(t *testing.T) {
	tests := []struct {
		input string
		want  types.SettlementCycleStatus
	}{
		{"PENDING", types.CycleStatusPending},
		{"VALUING", types.CycleStatusValuing},
		{"CALCULATED", types.CycleStatusCalculated},
		{"SETTLING", types.CycleStatusSettling},
		{"COMPLETED", types.CycleStatusCompleted},
		{"FAILED", types.CycleStatusFailed},
		{"UNKNOWN", types.CycleStatusPending},
		{"", types.CycleStatusPending},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseCycleStatus(tt.input)
			if got != tt.want {
				t.Errorf("parseCycleStatus(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestParsePayDirection verifies direction string conversions.
func TestParsePayDirection(t *testing.T) {
	tests := []struct {
		input string
		want  types.PayDirection
	}{
		{"PAY_IN", types.PayIn},
		{"PAY_OUT", types.PayOut},
		{"INVALID", types.PayIn},
		{"", types.PayIn},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parsePayDirection(tt.input)
			if got != tt.want {
				t.Errorf("parsePayDirection(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestParseInstructionStatus verifies instruction status string conversions.
func TestParseInstructionStatus(t *testing.T) {
	tests := []struct {
		input string
		want  types.SettlementInstructionStatus
	}{
		{"PENDING", types.InstructionPending},
		{"SUBMITTED", types.InstructionSubmitted},
		{"CONFIRMED", types.InstructionConfirmed},
		{"FAILED", types.InstructionFailed},
		{"UNKNOWN", types.InstructionPending},
		{"", types.InstructionPending},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseInstructionStatus(tt.input)
			if got != tt.want {
				t.Errorf("parseInstructionStatus(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestCycleFieldMapping verifies that cycle fields serialize correctly for SQL.
func TestCycleFieldMapping(t *testing.T) {
	now := time.Now()
	cycle := types.SettlementCycle{
		CycleID:     "cycle-1",
		SettleDate:  now,
		Status:      types.CycleStatusCompleted,
		TotalPayIn:  types.NewDecimal(1000, 0),
		TotalPayOut: types.NewDecimal(1000, 0),
		StartedAt:   now,
		CompletedAt: now,
		Error:       "",
	}

	if cycle.Status.String() != "COMPLETED" {
		t.Errorf("status string = %q, want COMPLETED", cycle.Status.String())
	}
	if decimalToString(cycle.TotalPayIn) != "1000" {
		t.Errorf("total payin = %q, want 1000", decimalToString(cycle.TotalPayIn))
	}
	if decimalToString(cycle.TotalPayOut) != "1000" {
		t.Errorf("total payout = %q, want 1000", decimalToString(cycle.TotalPayOut))
	}
}

// TestInstructionFieldMapping verifies that instruction fields serialize correctly.
func TestInstructionFieldMapping(t *testing.T) {
	now := time.Now()
	inst := types.SettlementInstruction{
		InstructionID: "si-1",
		CycleID:       "cycle-1",
		ParticipantID: "P1",
		Direction:     types.PayIn,
		Amount:        types.NewDecimal(500, 2500),
		Status:        types.InstructionConfirmed,
		CreatedAt:     now,
		SubmittedAt:   now,
		ConfirmedAt:   now,
	}

	if inst.Direction.String() != "PAY_IN" {
		t.Errorf("direction string = %q, want PAY_IN", inst.Direction.String())
	}
	if inst.Status.String() != "CONFIRMED" {
		t.Errorf("status string = %q, want CONFIRMED", inst.Status.String())
	}
	if decimalToString(inst.Amount) != "500.25" {
		t.Errorf("amount = %q, want 500.25", decimalToString(inst.Amount))
	}
}

// TestPriceFieldMapping verifies settlement price field serialization.
func TestPriceFieldMapping(t *testing.T) {
	sp := types.SettlementPrice{
		InstrumentID:    "WHEAT-MAY26",
		SettleDate:      time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC),
		SettlementPrice: types.NewDecimal(1520, 0),
		PreviousPrice:   types.NewDecimal(1500, 0),
	}

	if decimalToString(sp.SettlementPrice) != "1520" {
		t.Errorf("settlement price = %q, want 1520", decimalToString(sp.SettlementPrice))
	}
	if decimalToString(sp.PreviousPrice) != "1500" {
		t.Errorf("previous price = %q, want 1500", decimalToString(sp.PreviousPrice))
	}
}

// TestCycleStatusRoundTrip verifies that status strings round-trip correctly.
func TestCycleStatusRoundTrip(t *testing.T) {
	statuses := []types.SettlementCycleStatus{
		types.CycleStatusPending,
		types.CycleStatusValuing,
		types.CycleStatusCalculated,
		types.CycleStatusSettling,
		types.CycleStatusCompleted,
		types.CycleStatusFailed,
	}
	for _, s := range statuses {
		t.Run(s.String(), func(t *testing.T) {
			got := parseCycleStatus(s.String())
			if got != s {
				t.Errorf("round trip failed: %d -> %q -> %d", s, s.String(), got)
			}
		})
	}
}

// TestInstructionStatusRoundTrip verifies status strings round-trip correctly.
func TestInstructionStatusRoundTrip(t *testing.T) {
	statuses := []types.SettlementInstructionStatus{
		types.InstructionPending,
		types.InstructionSubmitted,
		types.InstructionConfirmed,
		types.InstructionFailed,
	}
	for _, s := range statuses {
		t.Run(s.String(), func(t *testing.T) {
			got := parseInstructionStatus(s.String())
			if got != s {
				t.Errorf("round trip failed: %d -> %q -> %d", s, s.String(), got)
			}
		})
	}
}

// TestPayDirectionRoundTrip verifies direction strings round-trip correctly.
func TestPayDirectionRoundTrip(t *testing.T) {
	directions := []types.PayDirection{
		types.PayIn,
		types.PayOut,
	}
	for _, d := range directions {
		t.Run(d.String(), func(t *testing.T) {
			got := parsePayDirection(d.String())
			if got != d {
				t.Errorf("round trip failed: %d -> %q -> %d", d, d.String(), got)
			}
		})
	}
}
