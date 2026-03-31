// Package defaultmgmt implements CCP default fund management and the
// default waterfall per IOSCO/PFMI Principle 4 (Credit Risk) and
// Principle 7 (Liquidity Risk). The default fund collects contributions
// from clearing members to mutualize losses when a member defaults.
package defaultmgmt

import (
	"fmt"
	"sync"

	"github.com/garudax-platform/clearing-engine/internal/types"
)

// DefaultFundManager manages participant contributions to the CCP default fund
// and CCP's own capital reserves. All amounts use the Decimal(18,4) type for
// consistency with clearing obligations.
type DefaultFundManager struct {
	mu sync.RWMutex

	// contributions maps participant ID to their default fund contribution amount
	contributions map[string]types.Decimal

	// ccpSkinInTheGame is the CCP's own capital pledged ahead of non-defaulting
	// members' contributions (IOSCO/PFMI Principle 4, Key Consideration 5)
	ccpSkinInTheGame types.Decimal

	// ccpAdditionalCapital is the CCP's additional capital available after
	// non-defaulting members' contributions are exhausted
	ccpAdditionalCapital types.Decimal
}

// NewDefaultFundManager creates a new fund manager with zero balances.
func NewDefaultFundManager() *DefaultFundManager {
	return &DefaultFundManager{
		contributions: make(map[string]types.Decimal),
	}
}

// AddContribution adds (or replaces) a participant's default fund contribution.
// Returns an error if amount is negative.
func (f *DefaultFundManager) AddContribution(participantID string, amount types.Decimal) error {
	if amount.IsNeg() {
		return fmt.Errorf("defaultfund: contribution cannot be negative for participant %s", participantID)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.contributions[participantID] = amount
	return nil
}

// GetContribution returns a participant's current default fund contribution.
// Returns zero if the participant has no contribution on file.
func (f *DefaultFundManager) GetContribution(participantID string) types.Decimal {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if amt, ok := f.contributions[participantID]; ok {
		return amt
	}
	return types.DecimalZero()
}

// GetTotalFund returns the sum of all participant contributions.
func (f *DefaultFundManager) GetTotalFund() types.Decimal {
	f.mu.RLock()
	defer f.mu.RUnlock()
	total := types.DecimalZero()
	for _, amt := range f.contributions {
		total = total.Add(amt)
	}
	return total
}

// GetParticipantCount returns the number of contributing participants.
func (f *DefaultFundManager) GetParticipantCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.contributions)
}

// SetCCPSkinInTheGame sets the CCP's own capital pledged into the waterfall.
func (f *DefaultFundManager) SetCCPSkinInTheGame(amount types.Decimal) error {
	if amount.IsNeg() {
		return fmt.Errorf("defaultfund: CCP skin-in-the-game cannot be negative")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ccpSkinInTheGame = amount
	return nil
}

// GetCCPSkinInTheGame returns the CCP's own capital in the waterfall.
func (f *DefaultFundManager) GetCCPSkinInTheGame() types.Decimal {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.ccpSkinInTheGame
}

// SetCCPAdditionalCapital sets the CCP's additional capital (last layer).
func (f *DefaultFundManager) SetCCPAdditionalCapital(amount types.Decimal) error {
	if amount.IsNeg() {
		return fmt.Errorf("defaultfund: CCP additional capital cannot be negative")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ccpAdditionalCapital = amount
	return nil
}

// GetCCPAdditionalCapital returns the CCP's additional capital.
func (f *DefaultFundManager) GetCCPAdditionalCapital() types.Decimal {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.ccpAdditionalCapital
}

// GetNonDefaultingContributions returns a map of participant ID to contribution
// for all participants EXCEPT the specified defaulting participant.
func (f *DefaultFundManager) GetNonDefaultingContributions(defaultingParticipantID string) map[string]types.Decimal {
	f.mu.RLock()
	defer f.mu.RUnlock()
	result := make(map[string]types.Decimal)
	for pid, amt := range f.contributions {
		if pid != defaultingParticipantID {
			result[pid] = amt
		}
	}
	return result
}
