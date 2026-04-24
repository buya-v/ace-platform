// Package engine — session manager controlling market phase transitions.
package engine

import (
	"fmt"
	"sync"

	"github.com/garudax-platform/securities-service/internal/types"
)

// SessionManager tracks the current trading session for each instrument and
// routes order submission to the appropriate engine (auction or continuous).
type SessionManager struct {
	mu             sync.RWMutex
	sessions       map[string]types.MarketSession
	auctionEngine  *AuctionEngine
	matchingEngine *MatchingEngine
}

// NewSessionManager creates a SessionManager wired to the given engines.
func NewSessionManager(auction *AuctionEngine, matching *MatchingEngine) *SessionManager {
	return &SessionManager{
		sessions:       make(map[string]types.MarketSession),
		auctionEngine:  auction,
		matchingEngine: matching,
	}
}

// GetSession returns the current session for an instrument. Defaults to CLOSED.
func (sm *SessionManager) GetSession(instrumentID string) types.MarketSession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	sess, ok := sm.sessions[instrumentID]
	if !ok {
		return types.SessionClosed
	}
	return sess
}

// GetAllSessions returns a snapshot of all instrument sessions.
func (sm *SessionManager) GetAllSessions() map[string]types.MarketSession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	result := make(map[string]types.MarketSession, len(sm.sessions))
	for k, v := range sm.sessions {
		result[k] = v
	}
	return result
}

// TransitionTo moves an instrument to a new market session.
// Valid transitions:
//
//	CLOSED         → PRE_OPEN
//	PRE_OPEN       → CONTINUOUS  (runs opening auction first)
//	CONTINUOUS     → CLOSING_AUCTION
//	CLOSING_AUCTION → CLOSED     (runs closing auction first)
//
// Returns the AuctionResult if an auction was executed during transition, nil otherwise.
func (sm *SessionManager) TransitionTo(instrumentID, tenantID string, newSession types.MarketSession) (*types.AuctionResult, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	current, ok := sm.sessions[instrumentID]
	if !ok {
		current = types.SessionClosed
	}

	// Validate the transition.
	if !isValidTransition(current, newSession) {
		return nil, fmt.Errorf("invalid session transition: %s → %s", current, newSession)
	}

	var auctionResult *types.AuctionResult

	switch {
	case current == types.SessionPreOpen && newSession == types.SessionContinuous:
		// Run opening auction before transitioning to continuous.
		_, result, err := sm.auctionEngine.RunAuction(instrumentID, tenantID)
		if err != nil {
			return nil, fmt.Errorf("opening auction failed: %w", err)
		}
		auctionResult = result

	case current == types.SessionClosingAuction && newSession == types.SessionClosed:
		// Run closing auction before transitioning to closed.
		_, result, err := sm.auctionEngine.RunAuction(instrumentID, tenantID)
		if err != nil {
			return nil, fmt.Errorf("closing auction failed: %w", err)
		}
		auctionResult = result
	}

	sm.sessions[instrumentID] = newSession
	return auctionResult, nil
}

// SubmitOrder routes an order through the appropriate engine based on the
// current market session for the order's instrument.
//
//	PRE_OPEN / CLOSING_AUCTION → AuctionEngine.CollectOrder
//	CONTINUOUS                 → MatchingEngine.MatchOrder
//	CLOSED                     → error
func (sm *SessionManager) SubmitOrder(order *types.SecurityOrder, tenantID string) ([]types.SecurityTrade, error) {
	sm.mu.RLock()
	session, ok := sm.sessions[order.InstrumentID]
	if !ok {
		session = types.SessionClosed
	}
	sm.mu.RUnlock()

	switch session {
	case types.SessionPreOpen, types.SessionClosingAuction:
		if err := sm.auctionEngine.CollectOrder(order); err != nil {
			return nil, fmt.Errorf("failed to collect auction order: %w", err)
		}
		return nil, nil

	case types.SessionContinuous:
		trades, err := sm.matchingEngine.MatchOrder(tenantID, order)
		if err != nil {
			return nil, fmt.Errorf("matching failed: %w", err)
		}
		return trades, nil

	case types.SessionClosed:
		return nil, fmt.Errorf("market is closed")

	default:
		return nil, fmt.Errorf("unknown session: %s", session)
	}
}

// isValidTransition checks whether a session transition is allowed.
func isValidTransition(from, to types.MarketSession) bool {
	switch {
	case from == types.SessionClosed && to == types.SessionPreOpen:
		return true
	case from == types.SessionPreOpen && to == types.SessionContinuous:
		return true
	case from == types.SessionContinuous && to == types.SessionClosingAuction:
		return true
	case from == types.SessionClosingAuction && to == types.SessionClosed:
		return true
	default:
		return false
	}
}
