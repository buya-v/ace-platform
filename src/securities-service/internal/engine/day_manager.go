// Package engine — day lifecycle manager for trading sessions.
package engine

import (
	"fmt"
	"sync"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// DayManager controls the overall trading day lifecycle, coordinating
// session transitions across all active instruments.
type DayManager struct {
	sessionManager  *SessionManager
	instrumentStore store.InstrumentStore
	currentState    types.DayState
	mu              sync.RWMutex
}

// NewDayManager creates a DayManager starting in DAY_CLOSED state.
func NewDayManager(sessionManager *SessionManager, instrumentStore store.InstrumentStore) *DayManager {
	return &DayManager{
		sessionManager:  sessionManager,
		instrumentStore: instrumentStore,
		currentState:    types.DayClosed,
	}
}

// GetState returns the current day state.
func (dm *DayManager) GetState() types.DayState {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.currentState
}

// StartDay transitions from DAY_CLOSED to DAY_PRE_OPEN.
// Iterates all ACTIVE instruments and transitions each to PRE_OPEN.
func (dm *DayManager) StartDay() error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.currentState != types.DayClosed {
		return fmt.Errorf("invalid day transition: %s -> DAY_PRE_OPEN (must be DAY_CLOSED)", dm.currentState)
	}

	instruments, err := dm.instrumentStore.List(store.InstrumentFilters{
		TradingStatus: types.TradingStatusActive,
	})
	if err != nil {
		return fmt.Errorf("failed to list instruments: %w", err)
	}

	for _, inst := range instruments {
		if _, err := dm.sessionManager.TransitionTo(inst.ID, "", types.SessionPreOpen); err != nil {
			return fmt.Errorf("failed to transition instrument %s to PRE_OPEN: %w", inst.ID, err)
		}
	}

	dm.currentState = types.DayPreOpen
	return nil
}

// StartTrading transitions from DAY_PRE_OPEN to DAY_TRADING.
// Transitions all ACTIVE instruments to CONTINUOUS (triggers opening auctions).
func (dm *DayManager) StartTrading() error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.currentState != types.DayPreOpen {
		return fmt.Errorf("invalid day transition: %s -> DAY_TRADING (must be DAY_PRE_OPEN)", dm.currentState)
	}

	instruments, err := dm.instrumentStore.List(store.InstrumentFilters{
		TradingStatus: types.TradingStatusActive,
	})
	if err != nil {
		return fmt.Errorf("failed to list instruments: %w", err)
	}

	for _, inst := range instruments {
		if _, err := dm.sessionManager.TransitionTo(inst.ID, "", types.SessionContinuous); err != nil {
			return fmt.Errorf("failed to transition instrument %s to CONTINUOUS: %w", inst.ID, err)
		}
	}

	dm.currentState = types.DayTrading
	return nil
}

// EndTrading transitions from DAY_TRADING to DAY_POST_CLOSE.
// Transitions all ACTIVE instruments to CLOSING_AUCTION then CLOSED.
func (dm *DayManager) EndTrading() error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.currentState != types.DayTrading {
		return fmt.Errorf("invalid day transition: %s -> DAY_POST_CLOSE (must be DAY_TRADING)", dm.currentState)
	}

	instruments, err := dm.instrumentStore.List(store.InstrumentFilters{
		TradingStatus: types.TradingStatusActive,
	})
	if err != nil {
		return fmt.Errorf("failed to list instruments: %w", err)
	}

	// First transition to CLOSING_AUCTION.
	for _, inst := range instruments {
		if _, err := dm.sessionManager.TransitionTo(inst.ID, "", types.SessionClosingAuction); err != nil {
			return fmt.Errorf("failed to transition instrument %s to CLOSING_AUCTION: %w", inst.ID, err)
		}
	}

	// Then transition to CLOSED (triggers closing auctions).
	for _, inst := range instruments {
		if _, err := dm.sessionManager.TransitionTo(inst.ID, "", types.SessionClosed); err != nil {
			return fmt.Errorf("failed to transition instrument %s to CLOSED: %w", inst.ID, err)
		}
	}

	dm.currentState = types.DayPostClose
	return nil
}

// EndDay transitions from DAY_POST_CLOSE to DAY_CLOSED.
func (dm *DayManager) EndDay() error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.currentState != types.DayPostClose {
		return fmt.Errorf("invalid day transition: %s -> DAY_CLOSED (must be DAY_POST_CLOSE)", dm.currentState)
	}

	dm.currentState = types.DayClosed
	return nil
}
