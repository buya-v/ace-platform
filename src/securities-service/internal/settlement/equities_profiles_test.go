package settlement_test

// Tests for the production equities settlement-cycle API in equities.go:
// SettlementProfile date computation (incl. holidays), cycle parsing, clearing
// instructions, and the obligation settlement state machine. These exercise the
// exported package surface (the sibling equities_test.go validates the same date
// semantics via a local spec type, guarding against the two drifting apart).

import (
	"errors"
	"testing"
	"time"

	"github.com/garudax-platform/securities-service/internal/settlement"
	"github.com/garudax-platform/securities-service/internal/types"
)

func TestSettlementProfile_SettlementDate(t *testing.T) {
	cases := []struct {
		name    string
		profile settlement.SettlementProfile
		trade   string
		want    string
	}{
		{"T+1 mid-week", settlement.ProfileT1, "2026-06-15", "2026-06-16"},
		{"T+2 mid-week", settlement.ProfileT2, "2026-06-15", "2026-06-17"},
		{"T+1 Thu->Fri", settlement.ProfileT1, "2026-06-18", "2026-06-19"},
		{"T+2 Thu->Mon", settlement.ProfileT2, "2026-06-18", "2026-06-22"},
		{"T+1 Fri->Mon", settlement.ProfileT1, "2026-06-19", "2026-06-22"},
		{"T+2 Fri->Tue", settlement.ProfileT2, "2026-06-19", "2026-06-23"},
		{"T+3 mid-week", settlement.ProfileT3, "2026-06-15", "2026-06-18"},
		// T+0 on a weekday is same-day; Friday is a settlement day.
		{"T+0 Fri", settlement.ProfileT0, "2026-06-19", "2026-06-19"},
		{"T+0 mid-week", settlement.ProfileT0, "2026-06-15", "2026-06-15"},
		// T+0 on a Saturday must roll the landing date off the weekend to Monday.
		{"T+0 Sat->Mon", settlement.ProfileT0, "2026-06-20", "2026-06-22"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			td, err := time.Parse("2006-01-02", tc.trade)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got := tc.profile.SettlementDate(td); got != tc.want {
				t.Errorf("%s of %s: want %s, got %s", tc.profile.Name, tc.trade, tc.want, got)
			}
		})
	}
}

func TestSettlementProfile_WithHolidays(t *testing.T) {
	td := time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC) // Monday
	// Tue 2026-06-16 is a holiday: T+1 must skip it and land on Wed 2026-06-17.
	holidays := map[string]bool{"2026-06-16": true}
	if got := settlement.ProfileT1.SettlementDateWithHolidays(td, holidays); got != "2026-06-17" {
		t.Errorf("T+1 over holiday: want 2026-06-17, got %s", got)
	}
	// With no holidays, the result must equal the plain SettlementDate.
	if got := settlement.ProfileT2.SettlementDateWithHolidays(td, nil); got != settlement.ProfileT2.SettlementDate(td) {
		t.Errorf("nil holidays should match SettlementDate, got %s", got)
	}
}

func TestProfileForCycle(t *testing.T) {
	cases := []struct {
		in      string
		wantOff int
		wantErr bool
	}{
		{"T+2", 2, false},
		{"t+1", 1, false},
		{" T2 ", 2, false},
		{"T+0", 0, false},
		{"T+5", 5, false},
		{"", 0, true},
		{"X+2", 0, true},
		{"T+abc", 0, true},
	}
	for _, tc := range cases {
		p, err := settlement.ProfileForCycle(tc.in)
		if tc.wantErr {
			if !errors.Is(err, settlement.ErrUnknownSettlementCycle) {
				t.Errorf("ProfileForCycle(%q): expected ErrUnknownSettlementCycle, got %v", tc.in, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("ProfileForCycle(%q): unexpected error %v", tc.in, err)
			continue
		}
		if p.OffsetDays != tc.wantOff {
			t.Errorf("ProfileForCycle(%q): offset = %d, want %d", tc.in, p.OffsetDays, tc.wantOff)
		}
	}
}

func TestProfileForCycleOrDefault(t *testing.T) {
	if p := settlement.ProfileForCycleOrDefault("garbage"); p.OffsetDays != settlement.DefaultEquitiesProfile.OffsetDays {
		t.Errorf("garbage cycle should fall back to default T+2, got %s", p.Name)
	}
	if p := settlement.ProfileForCycleOrDefault("T+1"); p.OffsetDays != 1 {
		t.Errorf("valid cycle should parse, got %s", p.Name)
	}
}

func TestBuildClearingInstruction(t *testing.T) {
	td := time.Date(2026, time.June, 18, 0, 0, 0, 0, time.UTC) // Thursday
	trade := types.SecurityTrade{ID: "TR-1", InstrumentID: "MSE-APU", Quantity: 100, Price: decLit(12.5)}
	ci := settlement.BuildClearingInstruction(trade, settlement.ProfileT2, td, nil)

	if ci.TradeID != "TR-1" || ci.InstrumentID != "MSE-APU" {
		t.Errorf("instruction identity mismatch: %+v", ci)
	}
	if ci.Profile != "T+2" {
		t.Errorf("profile = %s, want T+2", ci.Profile)
	}
	if ci.TradeDate != "2026-06-18" {
		t.Errorf("trade date = %s, want 2026-06-18", ci.TradeDate)
	}
	// Thursday + T+2 rolls over the weekend to Monday 2026-06-22.
	if ci.SettlementDate != "2026-06-22" {
		t.Errorf("settlement date = %s, want 2026-06-22", ci.SettlementDate)
	}
	if ci.NetAmount != decLit(1250.0) {
		t.Errorf("net amount = %.2f, want 1250.00", ci.NetAmount.Float64())
	}
}

func TestSettlementStateMachine_HappyPath(t *testing.T) {
	path := []types.SettlementStatus{
		types.SettlePending, types.SettleAffirmed, types.SettleNetted,
		types.SettleInstructed, types.SettleSettling, types.SettleSettled,
	}
	for i := 0; i+1 < len(path); i++ {
		if err := settlement.ValidateTransition(path[i], path[i+1]); err != nil {
			t.Errorf("transition %s -> %s should be valid: %v", path[i], path[i+1], err)
		}
	}
	// The engine's simplified shortcut NETTED -> SETTLED must also be legal.
	if !settlement.CanTransition(types.SettleNetted, types.SettleSettled) {
		t.Error("NETTED -> SETTLED should be permitted (engine happy path)")
	}
}

func TestSettlementStateMachine_IllegalTransitions(t *testing.T) {
	illegal := [][2]types.SettlementStatus{
		{types.SettlePending, types.SettleSettled},   // cannot skip the lifecycle
		{types.SettleSettled, types.SettlePending},   // terminal cannot reopen
		{types.SettleAffirmed, types.SettleAffirmed}, // no-op is not a transition
	}
	for _, tc := range illegal {
		if settlement.CanTransition(tc[0], tc[1]) {
			t.Errorf("%s -> %s should be illegal", tc[0], tc[1])
		}
		err := settlement.ValidateTransition(tc[0], tc[1])
		if err == nil || !errors.Is(err, settlement.ErrInvalidTransition) {
			t.Errorf("%s -> %s: expected ErrInvalidTransition, got %v", tc[0], tc[1], err)
		}
	}
}

func TestSettlementStateMachine_FailedIsRetryable(t *testing.T) {
	if settlement.IsTerminal(types.SettleFailed) {
		t.Error("FAILED should be retryable, not terminal")
	}
	if !settlement.IsTerminal(types.SettleSettled) {
		t.Error("SETTLED should be terminal")
	}
	if !settlement.CanTransition(types.SettleFailed, types.SettleInstructed) {
		t.Error("FAILED -> INSTRUCTED retry should be permitted")
	}
}
