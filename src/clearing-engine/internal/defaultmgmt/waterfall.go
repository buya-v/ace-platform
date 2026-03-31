package defaultmgmt

import (
	"fmt"
	"sort"
	"time"

	"github.com/garudax-platform/clearing-engine/internal/types"
)

// WaterfallLayer identifies a layer in the CCP default waterfall.
type WaterfallLayer int

const (
	// LayerDefaulterMargin is the defaulter's own margin/collateral (first loss).
	LayerDefaulterMargin WaterfallLayer = iota
	// LayerDefaulterFundContribution is the defaulter's default fund contribution.
	LayerDefaulterFundContribution
	// LayerCCPSkinInTheGame is the CCP's own capital pledged ahead of mutualized losses.
	LayerCCPSkinInTheGame
	// LayerNonDefaultingContributions is pro-rata allocation from non-defaulting members.
	LayerNonDefaultingContributions
	// LayerCCPAdditionalCapital is the CCP's additional capital (last resort before resolution).
	LayerCCPAdditionalCapital
)

// String returns a human-readable name for the waterfall layer.
func (l WaterfallLayer) String() string {
	switch l {
	case LayerDefaulterMargin:
		return "defaulter_margin"
	case LayerDefaulterFundContribution:
		return "defaulter_fund_contribution"
	case LayerCCPSkinInTheGame:
		return "ccp_skin_in_the_game"
	case LayerNonDefaultingContributions:
		return "non_defaulting_contributions"
	case LayerCCPAdditionalCapital:
		return "ccp_additional_capital"
	default:
		return "unknown"
	}
}

// LayerResult records how much loss was absorbed by a single waterfall layer.
type LayerResult struct {
	Layer    WaterfallLayer `json:"layer"`
	Name     string         `json:"name"`
	Absorbed types.Decimal  `json:"absorbed"`

	// ProRataDetails is populated only for LayerNonDefaultingContributions.
	// Maps participant ID to the amount they absorbed.
	ProRataDetails map[string]types.Decimal `json:"pro_rata_details,omitempty"`
}

// WaterfallResult captures the full outcome of executing the default waterfall.
type WaterfallResult struct {
	DefaultingParticipantID string        `json:"defaulting_participant_id"`
	TotalLoss               types.Decimal `json:"total_loss"`
	TotalAbsorbed           types.Decimal `json:"total_absorbed"`
	RemainingLoss           types.Decimal `json:"remaining_loss"`
	FullyCovered            bool          `json:"fully_covered"`
	Layers                  []LayerResult `json:"layers"`
	ExecutedAt              time.Time     `json:"executed_at"`
}

// DefaultWaterfall executes the CCP default management waterfall per IOSCO/PFMI.
// It is a pure calculation engine -- no side effects, no DB writes.
type DefaultWaterfall struct {
	fundMgr *DefaultFundManager
}

// NewDefaultWaterfall creates a new waterfall executor backed by the given fund manager.
func NewDefaultWaterfall(fundMgr *DefaultFundManager) *DefaultWaterfall {
	return &DefaultWaterfall{fundMgr: fundMgr}
}

// ExecuteWaterfall runs the five-layer default waterfall for a defaulting participant.
//
// The waterfall order (per IOSCO/PFMI Principle 4):
//  1. Defaulter's margin and collateral (passed as defaulterMargin parameter)
//  2. Defaulter's default fund contribution
//  3. CCP skin-in-the-game capital
//  4. Non-defaulting members' contributions (pro-rata by contribution size)
//  5. CCP additional capital
//
// Returns a WaterfallResult detailing how much each layer absorbed.
// The waterfall stops as soon as the loss is fully covered.
func (w *DefaultWaterfall) ExecuteWaterfall(
	defaultingParticipantID string,
	totalLoss types.Decimal,
	defaulterMargin types.Decimal,
) (*WaterfallResult, error) {
	if totalLoss.IsNeg() || totalLoss.IsZero() {
		return nil, fmt.Errorf("waterfall: total loss must be positive, got %s", totalLoss.String())
	}
	if defaulterMargin.IsNeg() {
		return nil, fmt.Errorf("waterfall: defaulter margin cannot be negative")
	}

	result := &WaterfallResult{
		DefaultingParticipantID: defaultingParticipantID,
		TotalLoss:               totalLoss,
		TotalAbsorbed:           types.DecimalZero(),
		Layers:                  make([]LayerResult, 0, 5),
		ExecutedAt:              time.Now(),
	}

	remaining := totalLoss

	// Layer 1: Defaulter's margin/collateral
	remaining = w.applyLayer(result, remaining, LayerDefaulterMargin, defaulterMargin, nil)
	if remaining.IsZero() {
		w.finalize(result, remaining)
		return result, nil
	}

	// Layer 2: Defaulter's default fund contribution
	defaulterContrib := w.fundMgr.GetContribution(defaultingParticipantID)
	remaining = w.applyLayer(result, remaining, LayerDefaulterFundContribution, defaulterContrib, nil)
	if remaining.IsZero() {
		w.finalize(result, remaining)
		return result, nil
	}

	// Layer 3: CCP skin-in-the-game
	ccpSkin := w.fundMgr.GetCCPSkinInTheGame()
	remaining = w.applyLayer(result, remaining, LayerCCPSkinInTheGame, ccpSkin, nil)
	if remaining.IsZero() {
		w.finalize(result, remaining)
		return result, nil
	}

	// Layer 4: Non-defaulting members' contributions (pro-rata)
	remaining = w.applyProRataLayer(result, remaining, defaultingParticipantID)
	if remaining.IsZero() {
		w.finalize(result, remaining)
		return result, nil
	}

	// Layer 5: CCP additional capital
	ccpAdditional := w.fundMgr.GetCCPAdditionalCapital()
	remaining = w.applyLayer(result, remaining, LayerCCPAdditionalCapital, ccpAdditional, nil)

	w.finalize(result, remaining)
	return result, nil
}

// applyLayer absorbs as much of the remaining loss as possible from the given
// available amount. Returns the new remaining loss after absorption.
func (w *DefaultWaterfall) applyLayer(
	result *WaterfallResult,
	remaining types.Decimal,
	layer WaterfallLayer,
	available types.Decimal,
	proRata map[string]types.Decimal,
) types.Decimal {
	if available.IsZero() {
		result.Layers = append(result.Layers, LayerResult{
			Layer:          layer,
			Name:           layer.String(),
			Absorbed:       types.DecimalZero(),
			ProRataDetails: proRata,
		})
		return remaining
	}

	absorbed := available
	if available.GreaterThan(remaining) {
		absorbed = remaining
	}

	result.Layers = append(result.Layers, LayerResult{
		Layer:          layer,
		Name:           layer.String(),
		Absorbed:       absorbed,
		ProRataDetails: proRata,
	})

	return remaining.Sub(absorbed)
}

// applyProRataLayer distributes the remaining loss across non-defaulting members
// proportionally to their default fund contributions.
func (w *DefaultWaterfall) applyProRataLayer(
	result *WaterfallResult,
	remaining types.Decimal,
	defaultingParticipantID string,
) types.Decimal {
	contribs := w.fundMgr.GetNonDefaultingContributions(defaultingParticipantID)
	if len(contribs) == 0 {
		result.Layers = append(result.Layers, LayerResult{
			Layer:    LayerNonDefaultingContributions,
			Name:     LayerNonDefaultingContributions.String(),
			Absorbed: types.DecimalZero(),
		})
		return remaining
	}

	// Calculate total non-defaulting contributions
	totalContrib := types.DecimalZero()
	for _, amt := range contribs {
		totalContrib = totalContrib.Add(amt)
	}

	// Determine how much of the pool is needed
	poolUsed := totalContrib
	if totalContrib.GreaterThan(remaining) {
		poolUsed = remaining
	}

	// Pro-rata allocation: each member absorbs (their_contribution / total_contributions) * poolUsed
	// We use raw int64 arithmetic for pro-rata to avoid floating point.
	proRata := make(map[string]types.Decimal)
	totalAllocated := types.DecimalZero()

	// Sort participant IDs for deterministic output
	sortedIDs := make([]string, 0, len(contribs))
	for pid := range contribs {
		sortedIDs = append(sortedIDs, pid)
	}
	sort.Strings(sortedIDs)

	for i, pid := range sortedIDs {
		amt := contribs[pid]
		if i == len(sortedIDs)-1 {
			// Last participant gets the remainder to avoid rounding errors
			share := poolUsed.Sub(totalAllocated)
			proRata[pid] = share
			totalAllocated = totalAllocated.Add(share)
		} else {
			// share = poolUsed * (participant_contribution / total_contributions)
			// Using raw values: share_raw = poolUsed_raw * participant_raw / total_raw
			shareRaw := poolUsed.Raw() * amt.Raw() / totalContrib.Raw()
			share := types.DecimalFromRaw(shareRaw)
			proRata[pid] = share
			totalAllocated = totalAllocated.Add(share)
		}
	}

	return w.applyLayer(result, remaining, LayerNonDefaultingContributions, totalAllocated, proRata)
}

// finalize sets the final result fields.
func (w *DefaultWaterfall) finalize(result *WaterfallResult, remaining types.Decimal) {
	totalAbsorbed := types.DecimalZero()
	for _, layer := range result.Layers {
		totalAbsorbed = totalAbsorbed.Add(layer.Absorbed)
	}
	result.TotalAbsorbed = totalAbsorbed
	result.RemainingLoss = remaining
	result.FullyCovered = remaining.IsZero()
}
