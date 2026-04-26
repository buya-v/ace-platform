// Package engine — privilege checking for RBAC enforcement.
package engine

import (
	"fmt"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// PrivilegeEngine checks whether a participant holds a required permission.
// It consults both the participant's own Permissions slice (legacy coarse-grained)
// and any roles assigned via the RoleStore (fine-grained RBAC).
type PrivilegeEngine struct {
	participantStore store.ParticipantStore
	roleStore        store.RoleStore
}

// NewPrivilegeEngine constructs a PrivilegeEngine backed by the given stores.
// roleStore may be nil; in that case only participant-level permissions are checked.
func NewPrivilegeEngine(ps store.ParticipantStore, rs store.RoleStore) *PrivilegeEngine {
	return &PrivilegeEngine{
		participantStore: ps,
		roleStore:        rs,
	}
}

// HasPermission returns nil when participantID holds requiredPerm, or an error
// describing the denial.  The check fails fast on the first matching permission.
//
// Permission resolution order:
//  1. Participant must exist and be ACTIVE.
//  2. Check participant.Permissions (direct / legacy grants).
//  3. If roleStore is set, resolve all roles referenced in participant.Role
//     and check their permissions.
func (pe *PrivilegeEngine) HasPermission(participantID, requiredPerm string) error {
	p, err := pe.participantStore.Get(participantID)
	if err != nil {
		return fmt.Errorf("participant not found")
	}
	if p.Status != types.ParticipantActive {
		return fmt.Errorf("participant is suspended")
	}

	// 1. Direct permission on participant.
	for _, perm := range p.Permissions {
		if perm == requiredPerm {
			return nil
		}
	}

	// 2. Role-based permissions.
	if pe.roleStore != nil && p.Role != "" {
		role, rerr := pe.roleStore.Get(p.Role)
		if rerr == nil {
			for _, perm := range role.Permissions {
				if perm == requiredPerm {
					return nil
				}
			}
		}
	}

	return fmt.Errorf("permission denied: requires %s", requiredPerm)
}
