// Package engine — permission checking for exchange participants.
package engine

import (
	"fmt"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// CheckPermission verifies that the given participant exists, is active, and
// holds the required permission. Returns nil if the check passes.
func CheckPermission(participantID string, requiredPerm string, ps store.ParticipantStore) error {
	p, err := ps.Get(participantID)
	if err != nil {
		return fmt.Errorf("participant not found")
	}
	if p.Status != types.ParticipantActive {
		return fmt.Errorf("participant is suspended")
	}
	for _, perm := range p.Permissions {
		if perm == requiredPerm {
			return nil
		}
	}
	return fmt.Errorf("permission denied: requires %s", requiredPerm)
}
