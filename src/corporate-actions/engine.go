// Package corporateactions models the state machine and financial calculations
// for processing corporate actions (dividends, stock splits, rights issues) on
// the GarudaX multi-tenant trading platform.
//
// The package is intentionally dependency-free (the platform's "zero-dep Go
// module" pattern). It exposes pure functions so that the lifecycle state
// transitions and the per-holder calculations can be verified in isolation,
// independent of any HTTP, store, or CSD wiring.
//
// Constants are kept wire-compatible with
// github.com/garudax-platform/securities-service/internal/types so the same
// values flow through the platform without translation.
//
// Platform invariant: tenant ID is never optional. Every corporate action must
// carry a non-empty TenantID, and holdings are only acted upon when their
// tenant matches the action's tenant.
package corporateactions

import (
	"errors"
	"math"
)

// ActionType enumerates the supported corporate action events.
type ActionType string

const (
	Dividend    ActionType = "CA_DIVIDEND"
	StockSplit  ActionType = "CA_STOCK_SPLIT"
	RightsIssue ActionType = "CA_RIGHTS_ISSUE"
	Merger      ActionType = "CA_MERGER"
)

// Status enumerates the lifecycle states of a corporate action.
type Status string

const (
	StatusAnnounced  Status = "ANNOUNCED"
	StatusProcessing Status = "PROCESSING"
	StatusCompleted  Status = "COMPLETED"
	StatusCancelled  Status = "CANCELLED"
)

// EntitlementStatus enumerates the lifecycle states of a holder entitlement.
type EntitlementStatus string

const (
	EntitlementPending EntitlementStatus = "PENDING"
	EntitlementPaid    EntitlementStatus = "PAID"
)

// Sentinel errors returned by the engine. Tests and callers should compare with
// errors.Is rather than matching on message text.
var (
	ErrMissingTenant     = errors.New("corporate action tenant_id is required")
	ErrMissingInstrument = errors.New("corporate action instrument_id is required")
	ErrWrongActionType   = errors.New("corporate action type does not match the requested calculation")
	ErrInvalidTransition = errors.New("invalid corporate action status transition")
	ErrNegativeDividend  = errors.New("dividend amount per share must not be negative")
	ErrInvalidRatio      = errors.New("ratio shares must be positive integers")
	ErrNegativePrice     = errors.New("price must not be negative")
)

// CorporateAction is the declared event for a single instrument within a tenant.
type CorporateAction struct {
	ID           string
	TenantID     string
	InstrumentID string
	ActionType   ActionType
	Status       Status
}

// Position is a participant's holding in an instrument at the record date.
type Position struct {
	ParticipantID string
	InstrumentID  string
	TenantID      string
	Quantity      int64
}

// Entitlement is a participant's cash/share entitlement resulting from a
// dividend.
type Entitlement struct {
	CorporateActionID string
	ParticipantID     string
	InstrumentID      string
	TenantID          string
	Quantity          int64   // holding at the record date
	Value             float64 // cash value of the entitlement
	Status            EntitlementStatus
}

// RightsEntitlement is a participant's right to subscribe to new shares from a
// rights issue.
type RightsEntitlement struct {
	CorporateActionID string
	ParticipantID     string
	InstrumentID      string
	TenantID          string
	HeldQuantity      int64
	RightsQuantity    int64   // number of new shares the holder may subscribe to
	SubscriptionCost  float64 // RightsQuantity * subscription price
	Status            EntitlementStatus
}

// DividendTerms describes a cash dividend.
type DividendTerms struct {
	AmountPerShare float64
}

// SplitTerms describes a stock split as a ratio of NewShares-for-OldShares.
// A forward 2-for-1 split is {NewShares: 2, OldShares: 1}; a 1-for-10 reverse
// split is {NewShares: 1, OldShares: 10}.
type SplitTerms struct {
	NewShares int64
	OldShares int64
}

// RightsTerms describes a rights issue offering NewShares for every OldShares
// held, purchasable at SubscriptionPrice.
type RightsTerms struct {
	NewShares         int64
	OldShares         int64
	SubscriptionPrice float64
}

// ----------------------------------------------------------------------------
// State machine
// ----------------------------------------------------------------------------

// transitions is the adjacency map of allowed status transitions.
//
//	ANNOUNCED  -> PROCESSING | CANCELLED
//	PROCESSING -> COMPLETED  | ANNOUNCED (rollback on failure)
//	COMPLETED  -> (terminal)
//	CANCELLED  -> (terminal)
var transitions = map[Status]map[Status]bool{
	StatusAnnounced: {
		StatusProcessing: true,
		StatusCancelled:  true,
	},
	StatusProcessing: {
		StatusCompleted: true,
		StatusAnnounced: true,
	},
	StatusCompleted: {},
	StatusCancelled: {},
}

// CanTransition reports whether a corporate action may move from->to.
func CanTransition(from, to Status) bool {
	return transitions[from][to]
}

// IsTerminal reports whether a status admits no further transitions.
func IsTerminal(s Status) bool {
	return len(transitions[s]) == 0
}

// Transition advances the corporate action to the target status in place.
// On an illegal transition it returns ErrInvalidTransition and leaves the
// action's Status unchanged.
func (ca *CorporateAction) Transition(to Status) error {
	if !CanTransition(ca.Status, to) {
		return ErrInvalidTransition
	}
	ca.Status = to
	return nil
}

// ----------------------------------------------------------------------------
// Calculations
// ----------------------------------------------------------------------------

// validate checks the platform invariants common to every calculation.
func (ca CorporateAction) validate(want ActionType) error {
	if ca.TenantID == "" {
		return ErrMissingTenant
	}
	if ca.InstrumentID == "" {
		return ErrMissingInstrument
	}
	if ca.ActionType != want {
		return ErrWrongActionType
	}
	return nil
}

// eligible reports whether a position participates in the given action: it must
// belong to the same tenant and instrument and carry a positive quantity.
func eligible(ca CorporateAction, p Position) bool {
	return p.TenantID == ca.TenantID &&
		p.InstrumentID == ca.InstrumentID &&
		p.Quantity > 0
}

// round2 rounds a monetary amount to two decimal places (half away from zero).
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// CalculateDividend produces one PENDING entitlement per eligible holder, with a
// cash value of quantity * amount-per-share rounded to two decimals. Holders in
// a different tenant or instrument, or with a non-positive quantity, are
// skipped. The returned slice is never nil.
func CalculateDividend(ca CorporateAction, terms DividendTerms, holders []Position) ([]Entitlement, error) {
	if err := ca.validate(Dividend); err != nil {
		return nil, err
	}
	if terms.AmountPerShare < 0 {
		return nil, ErrNegativeDividend
	}

	out := make([]Entitlement, 0, len(holders))
	for _, p := range holders {
		if !eligible(ca, p) {
			continue
		}
		out = append(out, Entitlement{
			CorporateActionID: ca.ID,
			ParticipantID:     p.ParticipantID,
			InstrumentID:      ca.InstrumentID,
			TenantID:          ca.TenantID,
			Quantity:          p.Quantity,
			Value:             round2(float64(p.Quantity) * terms.AmountPerShare),
			Status:            EntitlementPending,
		})
	}
	return out, nil
}

// SplitRatio returns the multiplicative factor NewShares/OldShares for the split.
func (t SplitTerms) Ratio() (float64, error) {
	if t.NewShares <= 0 || t.OldShares <= 0 {
		return 0, ErrInvalidRatio
	}
	return float64(t.NewShares) / float64(t.OldShares), nil
}

// SplitAdjustedQuantity converts a pre-split holding into its post-split whole
// share count, returning any fractional remainder separately (the basis for
// cash-in-lieu). A negative input quantity yields ErrInvalidRatio's sibling via
// the ratio check only; quantities are otherwise floored toward zero.
func SplitAdjustedQuantity(qty int64, terms SplitTerms) (newQty int64, fractional float64, err error) {
	ratio, err := terms.Ratio()
	if err != nil {
		return 0, 0, err
	}
	exact := float64(qty) * ratio
	newQty = int64(math.Floor(exact))
	fractional = round2(exact - float64(newQty))
	return newQty, fractional, nil
}

// SplitAdjustedPrice converts a pre-split price into its post-split price,
// preserving total market value: price * OldShares / NewShares.
func SplitAdjustedPrice(price float64, terms SplitTerms) (float64, error) {
	if price < 0 {
		return 0, ErrNegativePrice
	}
	ratio, err := terms.Ratio()
	if err != nil {
		return 0, err
	}
	return round2(price / ratio), nil
}

// ApplySplit returns copies of the eligible positions with their quantities
// adjusted by the split ratio. Ineligible positions (wrong tenant/instrument or
// non-positive quantity) are returned unchanged. The returned slice is never nil.
func ApplySplit(ca CorporateAction, terms SplitTerms, positions []Position) ([]Position, error) {
	if err := ca.validate(StockSplit); err != nil {
		return nil, err
	}
	if _, err := terms.Ratio(); err != nil {
		return nil, err
	}

	out := make([]Position, 0, len(positions))
	for _, p := range positions {
		if eligible(ca, p) {
			newQty, _, _ := SplitAdjustedQuantity(p.Quantity, terms)
			p.Quantity = newQty
		}
		out = append(out, p)
	}
	return out, nil
}

// CalculateRights produces one rights entitlement per eligible holder. The
// number of subscribable new shares is floor(held * NewShares / OldShares) and
// the subscription cost is that count multiplied by the subscription price.
// The returned slice is never nil.
func CalculateRights(ca CorporateAction, terms RightsTerms, holders []Position) ([]RightsEntitlement, error) {
	if err := ca.validate(RightsIssue); err != nil {
		return nil, err
	}
	if terms.NewShares <= 0 || terms.OldShares <= 0 {
		return nil, ErrInvalidRatio
	}
	if terms.SubscriptionPrice < 0 {
		return nil, ErrNegativePrice
	}

	ratio := float64(terms.NewShares) / float64(terms.OldShares)
	out := make([]RightsEntitlement, 0, len(holders))
	for _, p := range holders {
		if !eligible(ca, p) {
			continue
		}
		rights := int64(math.Floor(float64(p.Quantity) * ratio))
		out = append(out, RightsEntitlement{
			CorporateActionID: ca.ID,
			ParticipantID:     p.ParticipantID,
			InstrumentID:      ca.InstrumentID,
			TenantID:          ca.TenantID,
			HeldQuantity:      p.Quantity,
			RightsQuantity:    rights,
			SubscriptionCost:  round2(float64(rights) * terms.SubscriptionPrice),
			Status:            EntitlementPending,
		})
	}
	return out, nil
}

// TheoreticalExRightsPrice returns the TERP given the cum-rights market price
// and the rights terms:
//
//	TERP = (OldShares*cumPrice + NewShares*subscriptionPrice) / (OldShares+NewShares)
//
// It is the expected price of the share once it trades ex-rights.
func TheoreticalExRightsPrice(cumPrice float64, terms RightsTerms) (float64, error) {
	if cumPrice < 0 || terms.SubscriptionPrice < 0 {
		return 0, ErrNegativePrice
	}
	if terms.NewShares <= 0 || terms.OldShares <= 0 {
		return 0, ErrInvalidRatio
	}
	old := float64(terms.OldShares)
	nw := float64(terms.NewShares)
	terp := (old*cumPrice + nw*terms.SubscriptionPrice) / (old + nw)
	return round2(terp), nil
}
