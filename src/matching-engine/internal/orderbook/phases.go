package orderbook

import (
	"fmt"
	"time"

	"github.com/garudax-platform/matching-engine/internal/types"
)

// MarketPhase represents a phase in the trading day lifecycle.
type MarketPhase int

const (
	PhasePreOpen        MarketPhase = 0
	PhaseOpeningAuction MarketPhase = 1
	PhaseContinuous     MarketPhase = 2
	PhaseClosingAuction MarketPhase = 3
	PhasePostClose      MarketPhase = 4
)

func (p MarketPhase) String() string {
	switch p {
	case PhasePreOpen:
		return "PRE_OPEN"
	case PhaseOpeningAuction:
		return "OPENING_AUCTION"
	case PhaseContinuous:
		return "CONTINUOUS"
	case PhaseClosingAuction:
		return "CLOSING_AUCTION"
	case PhasePostClose:
		return "POST_CLOSE"
	default:
		return "UNKNOWN"
	}
}

// AcceptsOrders returns true if the phase allows new order submission.
func (p MarketPhase) AcceptsOrders() bool {
	switch p {
	case PhasePreOpen, PhaseOpeningAuction, PhaseContinuous, PhaseClosingAuction:
		return true
	default:
		return false
	}
}

// AcceptsCancellations returns true if the phase allows order cancellation.
func (p MarketPhase) AcceptsCancellations() bool {
	switch p {
	case PhasePreOpen, PhaseOpeningAuction, PhaseContinuous, PhaseClosingAuction, PhasePostClose:
		return true
	default:
		return false
	}
}

// IsAuction returns true if the phase is an auction phase.
func (p MarketPhase) IsAuction() bool {
	return p == PhaseOpeningAuction || p == PhaseClosingAuction
}

// PhaseScheduleEntry defines a scheduled phase transition.
type PhaseScheduleEntry struct {
	Phase MarketPhase
	At    time.Time
}

// PhaseTransitionResult holds the output of a phase transition,
// including any auction uncrossing results.
type PhaseTransitionResult struct {
	PreviousPhase MarketPhase
	NewPhase      MarketPhase
	AuctionResult *AuctionResult // non-nil if an auction was uncrossed
}

// PhaseManager manages the market phase lifecycle for a single order book.
// It controls which operations are allowed in each phase and triggers
// auction uncrossing when leaving an auction phase.
type PhaseManager struct {
	currentPhase  MarketPhase
	auctionEngine *AuctionEngine
	schedule      []PhaseScheduleEntry // optional; for time-based transitions
}

// NewPhaseManager creates a PhaseManager starting in PRE_OPEN.
func NewPhaseManager(auctionEngine *AuctionEngine) *PhaseManager {
	return &PhaseManager{
		currentPhase:  PhasePreOpen,
		auctionEngine: auctionEngine,
	}
}

// CurrentPhase returns the current market phase.
func (pm *PhaseManager) CurrentPhase() MarketPhase {
	return pm.currentPhase
}

// SetSchedule configures the time-based phase schedule.
func (pm *PhaseManager) SetSchedule(schedule []PhaseScheduleEntry) {
	pm.schedule = schedule
}

// validTransitions defines the legal phase transitions.
var validTransitions = map[MarketPhase][]MarketPhase{
	PhasePreOpen:        {PhaseOpeningAuction},
	PhaseOpeningAuction: {PhaseContinuous},
	PhaseContinuous:     {PhaseClosingAuction},
	PhaseClosingAuction: {PhasePostClose},
	PhasePostClose:      {PhasePreOpen}, // next trading day
}

// isValidTransition checks if transitioning from -> to is allowed.
func isValidTransition(from, to MarketPhase) bool {
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	for _, p := range allowed {
		if p == to {
			return true
		}
	}
	return false
}

// TransitionTo moves the order book to a new phase. If leaving an auction
// phase, it runs the uncrossing algorithm against the book's orders and
// returns the auction result. The caller (OrderBook) is responsible for
// removing filled orders from the book after uncrossing.
func (pm *PhaseManager) TransitionTo(phase MarketPhase, book *OrderBook) (PhaseTransitionResult, error) {
	if !isValidTransition(pm.currentPhase, phase) {
		return PhaseTransitionResult{}, fmt.Errorf(
			"invalid phase transition from %s to %s", pm.currentPhase, phase,
		)
	}

	result := PhaseTransitionResult{
		PreviousPhase: pm.currentPhase,
		NewPhase:      phase,
	}

	// If leaving an auction phase, run uncrossing
	if pm.currentPhase.IsAuction() {
		auctionResult := pm.auctionEngine.RunAuction(
			book.InstrumentID,
			book.BidLevels(),
			book.AskLevels(),
			book.LastTradePrice,
		)
		if len(auctionResult.Trades) > 0 {
			result.AuctionResult = &auctionResult
			// Update last trade price
			book.LastTradePrice = auctionResult.EquilibriumPrice
		}
	}

	// Update the book state to match the new phase
	pm.currentPhase = phase
	book.State = phaseToBookState(phase)

	return result, nil
}

// BookStateForPhase returns the BookState corresponding to a MarketPhase.
func phaseToBookState(phase MarketPhase) types.BookState {
	switch phase {
	case PhasePreOpen:
		return types.BookStatePreOpen
	case PhaseOpeningAuction:
		return types.BookStateAuction
	case PhaseContinuous:
		return types.BookStateContinuous
	case PhaseClosingAuction:
		return types.BookStateAuction
	case PhasePostClose:
		return types.BookStateClosed
	default:
		return types.BookStateHalted
	}
}

// ShouldMatch returns true if orders should be matched immediately
// (continuous trading). During auction phases, orders are queued but
// not matched until uncrossing.
func (pm *PhaseManager) ShouldMatch() bool {
	return pm.currentPhase == PhaseContinuous
}

// CanSubmitOrder returns true if the current phase accepts new orders.
func (pm *PhaseManager) CanSubmitOrder() bool {
	return pm.currentPhase.AcceptsOrders()
}

// CanCancelOrder returns true if the current phase accepts cancellations.
func (pm *PhaseManager) CanCancelOrder() bool {
	return pm.currentPhase.AcceptsCancellations()
}

// CheckScheduledTransitions checks if any scheduled phase transition
// should occur based on the current time. Returns the target phase
// if a transition is due, or -1 if no transition is needed.
func (pm *PhaseManager) CheckScheduledTransitions(now time.Time) MarketPhase {
	if len(pm.schedule) == 0 {
		return MarketPhase(-1)
	}

	// Find the latest scheduled phase whose time has passed
	var latest *PhaseScheduleEntry
	for i := range pm.schedule {
		entry := &pm.schedule[i]
		if !now.Before(entry.At) {
			if latest == nil || entry.At.After(latest.At) {
				latest = entry
			}
		}
	}

	if latest == nil || latest.Phase == pm.currentPhase {
		return MarketPhase(-1)
	}

	return latest.Phase
}
