// Package membership provides participant tiering and membership lifecycle management.
package membership

import (
	"errors"
	"fmt"
	"time"
)

// Tier represents a participant's membership tier.
// Tier determines fee rates, position limits, and order size limits.
type Tier string

const (
	TierFarmer         Tier = "farmer"
	TierHedger         Tier = "hedger"
	TierSpeculator     Tier = "speculator"
	TierMarketMaker    Tier = "market_maker"
	TierClearingMember Tier = "clearing_member"
)

// ValidTiers is the set of all valid tier values.
var ValidTiers = map[Tier]bool{
	TierFarmer:         true,
	TierHedger:         true,
	TierSpeculator:     true,
	TierMarketMaker:    true,
	TierClearingMember: true,
}

// IsValidTier checks whether the given tier is a recognized value.
func IsValidTier(t Tier) bool {
	return ValidTiers[t]
}

// Status represents a member's lifecycle status.
type Status string

const (
	StatusPending    Status = "PENDING"
	StatusActive     Status = "ACTIVE"
	StatusSuspended  Status = "SUSPENDED"
	StatusTerminated Status = "TERMINATED"
)

// ValidStatuses is the set of all valid status values.
var ValidStatuses = map[Status]bool{
	StatusPending:    true,
	StatusActive:     true,
	StatusSuspended:  true,
	StatusTerminated: true,
}

// validTransitions defines which status transitions are allowed.
// Key is (from, to) pair.
var validTransitions = map[[2]Status]bool{
	{StatusPending, StatusActive}:      true, // on KYC approval
	{StatusActive, StatusSuspended}:    true,
	{StatusSuspended, StatusActive}:    true, // reinstatement
	{StatusActive, StatusTerminated}:   true,
	{StatusPending, StatusTerminated}:  true, // reject before activation
}

// IsValidTransition checks whether a status transition from -> to is allowed.
func IsValidTransition(from, to Status) bool {
	return validTransitions[[2]Status{from, to}]
}

// EntityType represents the legal entity type of a participant.
type EntityType string

const (
	EntityIndividual  EntityType = "individual"
	EntityCorporate   EntityType = "corporate"
	EntityCooperative EntityType = "cooperative"
	EntityFarmerGroup EntityType = "farmer_group"
)

// NetWorthCategory classifies the participant's net worth.
type NetWorthCategory string

const (
	NetWorthSmall  NetWorthCategory = "small"
	NetWorthMedium NetWorthCategory = "medium"
	NetWorthLarge  NetWorthCategory = "large"
)

// HistoryAction represents an action recorded in membership history.
type HistoryAction string

const (
	ActionCreated     HistoryAction = "CREATED"
	ActionTierChanged HistoryAction = "TIER_CHANGED"
	ActionSuspended   HistoryAction = "SUSPENDED"
	ActionReinstated  HistoryAction = "REINSTATED"
	ActionTerminated  HistoryAction = "TERMINATED"
	ActionActivated   HistoryAction = "ACTIVATED"
)

// Member represents a platform participant with tiering and lifecycle status.
type Member struct {
	ID               string           `json:"id"`
	UserID           string           `json:"user_id"`
	LegalName        string           `json:"legal_name"`
	EntityType       EntityType       `json:"entity_type,omitempty"`
	Tier             Tier             `json:"tier"`
	Status           Status           `json:"status"`
	OnboardedAt      *time.Time       `json:"onboarded_at,omitempty"`
	NetWorthCategory NetWorthCategory `json:"net_worth_category,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

// HistoryEntry represents a single membership lifecycle event.
type HistoryEntry struct {
	ID        string        `json:"id"`
	MemberID  string        `json:"member_id"`
	Action    HistoryAction `json:"action"`
	OldValue  string        `json:"old_value,omitempty"`
	NewValue  string        `json:"new_value,omitempty"`
	Reason    string        `json:"reason,omitempty"`
	ActorID   string        `json:"actor_id,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
}

// ListFilter holds optional filters for listing members.
type ListFilter struct {
	Status     *Status
	Tier       *Tier
	EntityType *EntityType
}

// Sentinel errors for membership operations.
var (
	ErrMemberNotFound     = errors.New("member not found")
	ErrInvalidTier        = errors.New("invalid tier")
	ErrInvalidTransition  = errors.New("invalid status transition")
	ErrMissingUserID      = errors.New("user_id is required")
	ErrMissingLegalName   = errors.New("legal_name is required")
	ErrSameTier           = errors.New("new tier is the same as current tier")
	ErrAlreadyTerminated  = errors.New("member is terminated")
	ErrNotActive          = errors.New("member is not active")
	ErrNotSuspended       = errors.New("member is not suspended")
	ErrNotPending         = errors.New("member is not pending")
)

// Validate checks that a Member has required fields and valid enum values.
func (m *Member) Validate() error {
	if m.UserID == "" {
		return ErrMissingUserID
	}
	if m.LegalName == "" {
		return ErrMissingLegalName
	}
	if !IsValidTier(m.Tier) {
		return fmt.Errorf("%w: %s", ErrInvalidTier, m.Tier)
	}
	return nil
}

// TierConfig holds the operational parameters for a given tier.
type TierConfig struct {
	Tier           Tier
	FeeRateBps     int // fee rate in basis points (1 bps = 0.01%)
	PositionLimit  int // max open positions
	OrderSizeLimit int // max single order quantity
}

// DefaultTierConfigs returns the default configuration for each tier.
func DefaultTierConfigs() map[Tier]TierConfig {
	return map[Tier]TierConfig{
		TierFarmer: {
			Tier:           TierFarmer,
			FeeRateBps:     5,
			PositionLimit:  10,
			OrderSizeLimit: 100,
		},
		TierHedger: {
			Tier:           TierHedger,
			FeeRateBps:     10,
			PositionLimit:  50,
			OrderSizeLimit: 500,
		},
		TierSpeculator: {
			Tier:           TierSpeculator,
			FeeRateBps:     25,
			PositionLimit:  100,
			OrderSizeLimit: 1000,
		},
		TierMarketMaker: {
			Tier:           TierMarketMaker,
			FeeRateBps:     3,
			PositionLimit:  500,
			OrderSizeLimit: 5000,
		},
		TierClearingMember: {
			Tier:           TierClearingMember,
			FeeRateBps:     2,
			PositionLimit:  1000,
			OrderSizeLimit: 10000,
		},
	}
}
