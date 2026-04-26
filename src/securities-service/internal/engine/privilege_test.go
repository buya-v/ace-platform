// Package engine_test — tests for PrivilegeEngine (RBAC privilege checking).
package engine_test

import (
	"testing"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// privParticipant builds an ExchangeParticipant for privilege tests.
// roleID is stored in the Role field; pass "" for no role.
func privParticipant(id, roleID string, status types.ParticipantStatus, perms ...string) *types.ExchangeParticipant {
	if perms == nil {
		perms = []string{}
	}
	return &types.ExchangeParticipant{
		ID:          id,
		FirmID:      "FIRM-PRIV",
		Name:        id + "-name",
		Role:        roleID,
		Status:      status,
		Permissions: perms,
		CreatedAt:   "2026-04-26T00:00:00Z",
		UpdatedAt:   "2026-04-26T00:00:00Z",
	}
}

// privRole builds a types.Role with the given permissions.
func privRole(id, name string, perms ...string) *types.Role {
	if perms == nil {
		perms = []string{}
	}
	return &types.Role{
		ID:          id,
		Name:        name,
		Description: "privilege test role",
		Permissions: perms,
		CreatedAt:   "2026-04-26T00:00:00Z",
		UpdatedAt:   "2026-04-26T00:00:00Z",
	}
}

// seedParticipantStore creates an InMemoryParticipantStore seeded with the given participants.
func seedParticipantStore(t *testing.T, participants ...*types.ExchangeParticipant) store.ParticipantStore {
	t.Helper()
	ps := store.NewInMemoryParticipantStore()
	for _, p := range participants {
		if err := ps.Create(p); err != nil {
			t.Fatalf("seedParticipantStore: Create %s: %v", p.ID, err)
		}
	}
	return ps
}

// seedRoleStore creates an InMemoryRoleStore seeded with the given roles.
func seedRoleStore(t *testing.T, roles ...*types.Role) store.RoleStore {
	t.Helper()
	rs := store.NewInMemoryRoleStore()
	for _, r := range roles {
		if err := rs.Create(r); err != nil {
			t.Fatalf("seedRoleStore: Create %s: %v", r.ID, err)
		}
	}
	return rs
}

// ── TestResolvePermissions_FromParticipant ────────────────────────────────────

// TestResolvePermissions_FromParticipant verifies that a participant with direct
// permissions (no role) can pass a HasPermission check on those permissions.
func TestResolvePermissions_FromParticipant(t *testing.T) {
	ps := seedParticipantStore(t,
		privParticipant("P-DIRECT", "", types.ParticipantActive,
			types.PermTradeEquity, types.PermTradeBond),
	)
	pe := engine.NewPrivilegeEngine(ps, nil) // no roleStore

	if err := pe.HasPermission("P-DIRECT", types.PermTradeEquity); err != nil {
		t.Errorf("direct perm %s: expected nil, got %v", types.PermTradeEquity, err)
	}
	if err := pe.HasPermission("P-DIRECT", types.PermTradeBond); err != nil {
		t.Errorf("direct perm %s: expected nil, got %v", types.PermTradeBond, err)
	}
	// Permission not granted directly.
	if err := pe.HasPermission("P-DIRECT", types.PermMarketMaker); err == nil {
		t.Error("expected denial for MARKET_MAKER (not directly granted), got nil")
	}
}

// ── TestResolvePermissions_FromRole ──────────────────────────────────────────

// TestResolvePermissions_FromRole verifies that a participant with a RoleID but
// no direct permissions inherits the permissions from their role.
func TestResolvePermissions_FromRole(t *testing.T) {
	rs := seedRoleStore(t,
		privRole("ROLE-TRADER", "TRADER",
			types.PermTradeEquity, types.PermTradeBond, types.PermTradeETF),
	)
	ps := seedParticipantStore(t,
		// Participant has no direct permissions; role provides them.
		privParticipant("P-ROLE-ONLY", "ROLE-TRADER", types.ParticipantActive),
	)
	pe := engine.NewPrivilegeEngine(ps, rs)

	if err := pe.HasPermission("P-ROLE-ONLY", types.PermTradeEquity); err != nil {
		t.Errorf("role perm %s: expected nil, got %v", types.PermTradeEquity, err)
	}
	if err := pe.HasPermission("P-ROLE-ONLY", types.PermTradeETF); err != nil {
		t.Errorf("role perm %s: expected nil, got %v", types.PermTradeETF, err)
	}
	// Permission not in role.
	if err := pe.HasPermission("P-ROLE-ONLY", types.PermMarketMaker); err == nil {
		t.Error("expected denial for MARKET_MAKER (not in role), got nil")
	}
}

// ── TestResolvePermissions_Merged ─────────────────────────────────────────────

// TestResolvePermissions_Merged verifies that a participant with both direct
// permissions and a role can use permissions from either source without
// duplicating checks (the engine accepts on first match).
func TestResolvePermissions_Merged(t *testing.T) {
	rs := seedRoleStore(t,
		privRole("ROLE-BOND", "BOND-TRADER", types.PermTradeBond),
	)
	ps := seedParticipantStore(t,
		// Direct: TRADE_EQUITY. Role adds: TRADE_BOND.
		privParticipant("P-MERGED", "ROLE-BOND", types.ParticipantActive,
			types.PermTradeEquity),
	)
	pe := engine.NewPrivilegeEngine(ps, rs)

	// Direct permission.
	if err := pe.HasPermission("P-MERGED", types.PermTradeEquity); err != nil {
		t.Errorf("merged direct perm: expected nil, got %v", err)
	}
	// Role-based permission.
	if err := pe.HasPermission("P-MERGED", types.PermTradeBond); err != nil {
		t.Errorf("merged role perm: expected nil, got %v", err)
	}
	// Neither direct nor role.
	if err := pe.HasPermission("P-MERGED", types.PermMarketMaker); err == nil {
		t.Error("expected denial for MARKET_MAKER (in neither direct nor role), got nil")
	}
	// Calling HasPermission twice for the same perm is idempotent.
	if err := pe.HasPermission("P-MERGED", types.PermTradeEquity); err != nil {
		t.Errorf("repeated HasPermission call: expected nil, got %v", err)
	}
}

// ── TestHasPermission_AdminFull ───────────────────────────────────────────────

// TestHasPermission_AdminFull verifies that a participant assigned to an ADMIN
// role whose Permissions list contains every platform permission can pass
// HasPermission for each of those permissions.  This exercises the "full admin"
// pattern: all permissions are enumerated on the role, so the admin can perform
// any operation.
func TestHasPermission_AdminFull(t *testing.T) {
	allPerms := []string{
		types.PermTradeEquity,
		types.PermTradeBond,
		types.PermTradeETF,
		types.PermMarketMaker,
		types.PermSponsoredAccess,
		types.PermAdminAnnouncements,
		types.PermAdminForceLogout,
		types.PermAdminAuditView,
		types.PermAdminRoleManage,
		types.PermRefDataWrite,
	}

	rs := seedRoleStore(t, privRole("ROLE-ADMIN-FULL", "ADMIN", allPerms...))
	ps := seedParticipantStore(t,
		privParticipant("P-ADMIN", "ROLE-ADMIN-FULL", types.ParticipantActive),
	)
	pe := engine.NewPrivilegeEngine(ps, rs)

	for _, perm := range allPerms {
		if err := pe.HasPermission("P-ADMIN", perm); err != nil {
			t.Errorf("ADMIN should have %s via role, got: %v", perm, err)
		}
	}
}

// ── TestHasPermission_Allowed ─────────────────────────────────────────────────

// TestHasPermission_Allowed verifies that an ACTIVE participant holding the
// required permission passes without error.
func TestHasPermission_Allowed(t *testing.T) {
	ps := seedParticipantStore(t,
		privParticipant("P-ALLOWED", "", types.ParticipantActive,
			types.PermTradeEquity),
	)
	pe := engine.NewPrivilegeEngine(ps, nil)

	if err := pe.HasPermission("P-ALLOWED", types.PermTradeEquity); err != nil {
		t.Errorf("expected nil for held permission, got: %v", err)
	}
}

// ── TestHasPermission_Denied ──────────────────────────────────────────────────

// TestHasPermission_Denied verifies that an ACTIVE participant lacking the
// required permission receives a non-nil error describing the denial.
func TestHasPermission_Denied(t *testing.T) {
	ps := seedParticipantStore(t,
		privParticipant("P-DENIED", "", types.ParticipantActive,
			types.PermTradeEquity),
	)
	pe := engine.NewPrivilegeEngine(ps, nil)

	err := pe.HasPermission("P-DENIED", types.PermMarketMaker)
	if err == nil {
		t.Fatal("expected denial error for missing permission, got nil")
	}
	if err.Error() == "" {
		t.Error("denial error message must not be empty")
	}
}

// ── TestHasPermission_UnknownParticipant ──────────────────────────────────────

// TestHasPermission_UnknownParticipant verifies that querying a participant ID
// that does not exist in the store returns a non-nil error.
func TestHasPermission_UnknownParticipant(t *testing.T) {
	ps := seedParticipantStore(t) // empty store
	pe := engine.NewPrivilegeEngine(ps, nil)

	err := pe.HasPermission("UNKNOWN-ID", types.PermTradeEquity)
	if err == nil {
		t.Fatal("expected error for unknown participant, got nil")
	}
}

// ── Suspended participant ─────────────────────────────────────────────────────

// TestHasPermission_SuspendedParticipant verifies that a SUSPENDED participant
// is denied even if they hold the required permission.
func TestHasPermission_SuspendedParticipant(t *testing.T) {
	ps := seedParticipantStore(t,
		privParticipant("P-SUSP", "", types.ParticipantSuspended,
			types.PermTradeEquity),
	)
	pe := engine.NewPrivilegeEngine(ps, nil)

	if err := pe.HasPermission("P-SUSP", types.PermTradeEquity); err == nil {
		t.Fatal("expected denial for suspended participant, got nil")
	}
}

// ── roleStore nil with participant role set ───────────────────────────────────

// TestHasPermission_NilRoleStore verifies that when roleStore is nil, only
// direct participant permissions are considered (no panic on nil roleStore).
func TestHasPermission_NilRoleStore(t *testing.T) {
	ps := seedParticipantStore(t,
		// Participant has a roleID set but the engine has no roleStore.
		privParticipant("P-NO-RS", "ROLE-NONEXISTENT", types.ParticipantActive,
			types.PermTradeBond),
	)
	pe := engine.NewPrivilegeEngine(ps, nil)

	// Direct permission must still work.
	if err := pe.HasPermission("P-NO-RS", types.PermTradeBond); err != nil {
		t.Errorf("direct perm with nil roleStore: expected nil, got %v", err)
	}
	// Permission only in the non-existent role → denied.
	if err := pe.HasPermission("P-NO-RS", types.PermTradeEquity); err == nil {
		t.Error("expected denial when roleStore is nil and perm not direct, got nil")
	}
}
