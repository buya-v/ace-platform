package settlement

// Equities settlement-cycle profiles, clearing instructions, and the obligation
// settlement state machine for the mse-equities flagship tenant.
//
// MSE equities settle on a T+2 profile; certain fixed-income / money-market
// lines settle T+1 (and some same-day, T+0). The settlement engine settles by
// the SettlementDate carried on each obligation, so the "profile" is the rule
// that maps a trade date to a settlement date. SettlementProfile encodes that
// rule (business-day offset + weekend roll + optional holiday calendar) and is
// the single source of truth the trade-capture layer is expected to honor.
//
// This mirrors the executable spec in equities_test.go: with no holidays the
// SettlementDate result is identical to that spec, so the two cannot drift.

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/garudax-platform/securities-service/internal/types"
)

// isoDate is the canonical ISO-8601 date layout used across the settlement layer.
const isoDate = "2006-01-02"

// SettlementProfile maps a trade date to a settlement date for an equities venue.
//
//   - OffsetDays  number of business days from trade date to settlement date.
//   - RollWeekend when true, Saturdays and Sundays are skipped and do not count
//     toward the offset.
type SettlementProfile struct {
	Name        string
	OffsetDays  int
	RollWeekend bool
}

// Standard MSE settlement profiles. T+2 is the equities default; T+1 and T+0 are
// used for selected fixed-income / money-market lines.
var (
	ProfileT0 = SettlementProfile{Name: "T+0", OffsetDays: 0, RollWeekend: true}
	ProfileT1 = SettlementProfile{Name: "T+1", OffsetDays: 1, RollWeekend: true}
	ProfileT2 = SettlementProfile{Name: "T+2", OffsetDays: 2, RollWeekend: true}
	ProfileT3 = SettlementProfile{Name: "T+3", OffsetDays: 3, RollWeekend: true}
)

// DefaultEquitiesProfile is the settlement profile applied to MSE equities when
// an instrument does not declare a more specific settlement cycle.
var DefaultEquitiesProfile = ProfileT2

// knownProfiles indexes the standard profiles by their normalized cycle name.
var knownProfiles = map[string]SettlementProfile{
	"T+0": ProfileT0,
	"T+1": ProfileT1,
	"T+2": ProfileT2,
	"T+3": ProfileT3,
}

// ErrUnknownSettlementCycle is returned when a settlement-cycle string cannot be
// parsed into a known profile.
var ErrUnknownSettlementCycle = fmt.Errorf("settlement: unknown settlement cycle")

// SettlementDate advances tradeDate by the profile's business-day offset,
// skipping Saturdays and Sundays when RollWeekend is set, and returns an ISO
// date string. The final landing date is always rolled off a weekend when
// RollWeekend is set, so even a T+0 profile never lands on Sat/Sun.
func (p SettlementProfile) SettlementDate(tradeDate time.Time) string {
	return p.SettlementDateWithHolidays(tradeDate, nil)
}

// SettlementDateWithHolidays behaves like SettlementDate but additionally treats
// any date present in holidays (keyed by ISO "2006-01-02" string) as a non-
// settlement day: such days are skipped and do not count toward the offset, and
// the final landing date is rolled forward off any holiday. Passing a nil or
// empty map yields exactly the same result as SettlementDate.
func (p SettlementProfile) SettlementDateWithHolidays(tradeDate time.Time, holidays map[string]bool) string {
	d := tradeDate
	added := 0
	for added < p.OffsetDays {
		d = d.AddDate(0, 0, 1)
		if p.isNonSettlementDay(d, holidays) {
			continue
		}
		added++
	}
	// Roll the landing date forward off any weekend/holiday (covers T+0 and the
	// case where the computed date itself falls on a closed day).
	for p.isNonSettlementDay(d, holidays) {
		d = d.AddDate(0, 0, 1)
	}
	return d.Format(isoDate)
}

// isNonSettlementDay reports whether d is a day on which the venue does not
// settle, given the profile's weekend-roll flag and the supplied holiday set.
func (p SettlementProfile) isNonSettlementDay(d time.Time, holidays map[string]bool) bool {
	if p.RollWeekend && (d.Weekday() == time.Saturday || d.Weekday() == time.Sunday) {
		return true
	}
	if holidays != nil && holidays[d.Format(isoDate)] {
		return true
	}
	return false
}

// ProfileForCycle parses a settlement-cycle string such as "T+2", "T2", or "t+1"
// into the corresponding SettlementProfile. Whitespace and case are ignored and
// a missing "+" is tolerated. Unknown cycles return ErrUnknownSettlementCycle.
func ProfileForCycle(cycle string) (SettlementProfile, error) {
	norm := normalizeCycle(cycle)
	if norm == "" {
		return SettlementProfile{}, fmt.Errorf("%w: %q", ErrUnknownSettlementCycle, cycle)
	}
	if p, ok := knownProfiles[norm]; ok {
		return p, nil
	}
	// Accept any well-formed "T+N" beyond the pre-registered set.
	if strings.HasPrefix(norm, "T+") {
		if n, err := strconv.Atoi(norm[2:]); err == nil && n >= 0 {
			return SettlementProfile{Name: norm, OffsetDays: n, RollWeekend: true}, nil
		}
	}
	return SettlementProfile{}, fmt.Errorf("%w: %q", ErrUnknownSettlementCycle, cycle)
}

// normalizeCycle canonicalizes a cycle string to the "T+N" form, or returns ""
// if it does not look like a cycle.
func normalizeCycle(cycle string) string {
	s := strings.ToUpper(strings.TrimSpace(cycle))
	s = strings.ReplaceAll(s, " ", "")
	if !strings.HasPrefix(s, "T") {
		return ""
	}
	rest := strings.TrimPrefix(s, "T")
	rest = strings.TrimPrefix(rest, "+")
	if rest == "" {
		return ""
	}
	if _, err := strconv.Atoi(rest); err != nil {
		return ""
	}
	return "T+" + rest
}

// ProfileForCycleOrDefault returns the profile for the given cycle string, or
// DefaultEquitiesProfile when the cycle is empty or cannot be parsed. Use this
// at trade capture where a missing/garbled cycle must still produce a sane date.
func ProfileForCycleOrDefault(cycle string) SettlementProfile {
	if p, err := ProfileForCycle(cycle); err == nil {
		return p
	}
	return DefaultEquitiesProfile
}

// ── Clearing instructions ─────────────────────────────────────────────────────

// ClearingInstruction is the post-trade record produced at trade capture: it
// pins the settlement profile and the resulting settlement date for a trade so
// that downstream clearing/settlement is fully deterministic and auditable.
type ClearingInstruction struct {
	TradeID        string  `json:"trade_id"`
	InstrumentID   string  `json:"instrument_id"`
	Profile        string  `json:"profile"`         // e.g. "T+2"
	TradeDate      string  `json:"trade_date"`      // ISO
	SettlementDate string  `json:"settlement_date"` // ISO, profile-derived
	Quantity       int     `json:"quantity"`
	Price          float64 `json:"price"`
	NetAmount      float64 `json:"net_amount"` // Price * Quantity
}

// BuildClearingInstruction derives the clearing instruction for a trade under
// the given settlement profile. tradeDate anchors the settlement-date offset.
// The returned instruction carries the profile-derived SettlementDate, which the
// trade-capture layer should stamp onto the SecurityTrade before obligations are
// created. Holidays (ISO-keyed) are optional; pass nil for weekend-only rolling.
func BuildClearingInstruction(trade types.SecurityTrade, profile SettlementProfile, tradeDate time.Time, holidays map[string]bool) ClearingInstruction {
	return ClearingInstruction{
		TradeID:        trade.ID,
		InstrumentID:   trade.InstrumentID,
		Profile:        profile.Name,
		TradeDate:      tradeDate.Format(isoDate),
		SettlementDate: profile.SettlementDateWithHolidays(tradeDate, holidays),
		Quantity:       trade.Quantity,
		Price:          trade.Price,
		NetAmount:      trade.Price * float64(trade.Quantity),
	}
}

// ── Settlement state machine ──────────────────────────────────────────────────

// settlementTransitions is the directed graph of legal settlement-obligation
// state transitions. It models the full DvP lifecycle:
//
//	PENDING → AFFIRMED → NETTED → INSTRUCTED → SETTLING → SETTLED
//
// with FAILED reachable from the instructed/settling stages and a retry edge
// from FAILED back to INSTRUCTED. The engine's simplified cycle takes the
// PENDING → AFFIRMED → NETTED → SETTLED happy path, which this graph permits.
var settlementTransitions = map[types.SettlementStatus][]types.SettlementStatus{
	types.SettlePending:    {types.SettleAffirmed, types.SettleFailed},
	types.SettleAffirmed:   {types.SettleNetted, types.SettleFailed},
	types.SettleNetted:     {types.SettleInstructed, types.SettleSettled, types.SettleFailed},
	types.SettleInstructed: {types.SettleSettling, types.SettleSettled, types.SettleFailed},
	types.SettleSettling:   {types.SettleSettled, types.SettleFailed},
	types.SettleSettled:    {},                       // terminal
	types.SettleFailed:     {types.SettleInstructed}, // retry after resolution
}

// ErrInvalidTransition is returned when a settlement-status transition is not
// permitted by the state machine.
var ErrInvalidTransition = fmt.Errorf("settlement: invalid status transition")

// CanTransition reports whether moving an obligation from->to is permitted.
// A no-op transition (from == to) is not considered valid.
func CanTransition(from, to types.SettlementStatus) bool {
	for _, next := range settlementTransitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

// ValidateTransition returns nil when from->to is a legal settlement transition,
// or an error wrapping ErrInvalidTransition otherwise. Callers driving an
// obligation's status should gate UpdateStatus on this to prevent illegal jumps
// (e.g. PENDING → SETTLED) that would corrupt the settlement audit trail.
func ValidateTransition(from, to types.SettlementStatus) error {
	if CanTransition(from, to) {
		return nil
	}
	return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, from, to)
}

// IsTerminal reports whether a settlement status is terminal (no outgoing
// transitions). SETTLED is terminal; FAILED is recoverable (retryable).
func IsTerminal(status types.SettlementStatus) bool {
	return len(settlementTransitions[status]) == 0
}
