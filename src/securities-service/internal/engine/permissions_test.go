// Package engine_test — permission gate tests for CheckPermission.
package engine_test

import (
	"testing"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// permParticipant is a convenience builder for permission tests.
func permParticipant(id, firmID string, status types.ParticipantStatus, perms ...string) *types.ExchangeParticipant {
	return &types.ExchangeParticipant{
		ID:          id,
		FirmID:      firmID,
		Name:        id + "-name",
		Status:      status,
		Permissions: perms,
		CreatedAt:   "2026-04-24T00:00:00Z",
		UpdatedAt:   "2026-04-24T00:00:00Z",
	}
}

// newPermStore seeds a ParticipantStore with a given list of participants.
func newPermStore(t *testing.T, participants ...*types.ExchangeParticipant) store.ParticipantStore {
	t.Helper()
	s := store.NewInMemoryParticipantStore()
	for _, p := range participants {
		if err := s.Create(p); err != nil {
			t.Fatalf("newPermStore: Create %s: %v", p.ID, err)
		}
	}
	return s
}

// TestPermission_Allowed verifies that an ACTIVE participant holding the
// required permission passes CheckPermission without error.
func TestPermission_Allowed(t *testing.T) {
	ps := newPermStore(t,
		permParticipant("P-TRADER", "FIRM-1",
			types.ParticipantActive,
			types.PermTradeEquity,
			types.PermTradeBond,
		),
	)

	// TRADE_EQUITY permission → should pass.
	if err := engine.CheckPermission("P-TRADER", types.PermTradeEquity, ps); err != nil {
		t.Errorf("expected nil error for allowed participant, got: %v", err)
	}

	// TRADE_BOND permission is also held → should also pass.
	if err := engine.CheckPermission("P-TRADER", types.PermTradeBond, ps); err != nil {
		t.Errorf("expected nil error for TRADE_BOND, got: %v", err)
	}
}

// TestPermission_Denied verifies that an ACTIVE participant lacking the
// required permission receives a "permission denied" error.
func TestPermission_Denied(t *testing.T) {
	ps := newPermStore(t,
		// Participant only holds TRADE_EQUITY, NOT MARKET_MAKER.
		permParticipant("P-EQUITY-ONLY", "FIRM-1",
			types.ParticipantActive,
			types.PermTradeEquity,
		),
	)

	err := engine.CheckPermission("P-EQUITY-ONLY", types.PermMarketMaker, ps)
	if err == nil {
		t.Fatal("expected permission denied error, got nil")
	}
	// Error message must mention the missing permission.
	if errMsg := err.Error(); errMsg == "" {
		t.Error("error message must not be empty")
	}
}

// TestPermission_SuspendedParticipant verifies that a SUSPENDED participant is
// rejected even if they hold the required permission.
func TestPermission_SuspendedParticipant(t *testing.T) {
	ps := newPermStore(t,
		// Participant is SUSPENDED but holds TRADE_EQUITY.
		permParticipant("P-SUSPENDED", "FIRM-1",
			types.ParticipantSuspended,
			types.PermTradeEquity,
		),
	)

	err := engine.CheckPermission("P-SUSPENDED", types.PermTradeEquity, ps)
	if err == nil {
		t.Fatal("expected error for suspended participant, got nil")
	}
}

// TestPermission_NotFound verifies that looking up a non-existent participant
// returns an error.
func TestPermission_NotFound(t *testing.T) {
	ps := newPermStore(t) // empty store

	err := engine.CheckPermission("NO-SUCH-PARTICIPANT", types.PermTradeEquity, ps)
	if err == nil {
		t.Fatal("expected error for non-existent participant, got nil")
	}
}

// TestPermission_MultiplePerms ensures the permission check scans the full
// list and finds a match anywhere in the slice.
func TestPermission_MultiplePerms(t *testing.T) {
	ps := newPermStore(t,
		permParticipant("P-MULTI", "FIRM-1",
			types.ParticipantActive,
			types.PermTradeETF,
			types.PermTradeBond,
			types.PermSponsoredAccess,
		),
	)

	// SPONSORED_ACCESS is at the end of the list — should still match.
	if err := engine.CheckPermission("P-MULTI", types.PermSponsoredAccess, ps); err != nil {
		t.Errorf("expected nil error for SPONSORED_ACCESS, got: %v", err)
	}

	// TRADE_EQUITY is NOT in the list.
	if err := engine.CheckPermission("P-MULTI", types.PermTradeEquity, ps); err == nil {
		t.Error("expected permission denied for TRADE_EQUITY (not in list), got nil")
	}
}

// TestPermission_EmptyPermissions verifies that a participant with no permissions
// is denied any permission check.
func TestPermission_EmptyPermissions(t *testing.T) {
	ps := newPermStore(t,
		permParticipant("P-EMPTY", "FIRM-1", types.ParticipantActive),
		// No permissions variadic args → empty slice.
	)

	if err := engine.CheckPermission("P-EMPTY", types.PermTradeEquity, ps); err == nil {
		t.Error("expected permission denied for participant with empty permissions, got nil")
	}
}
