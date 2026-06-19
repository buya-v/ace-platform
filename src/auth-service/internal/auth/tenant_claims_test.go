package auth

import (
	"testing"

	"github.com/garudax-platform/auth-service/internal/types"
)

// containsRole reports whether roles contains want.
func containsRole(roles []types.Role, want types.Role) bool {
	for _, r := range roles {
		if r == want {
			return true
		}
	}
	return false
}

func containsPerm(perms []types.Permission, want types.Permission) bool {
	for _, p := range perms {
		if p == want {
			return true
		}
	}
	return false
}

func containsPlatformRole(roles []types.PlatformRole, want types.PlatformRole) bool {
	for _, r := range roles {
		if r == want {
			return true
		}
	}
	return false
}

// TestLoginEmitsTenantScopedClaims verifies a multi-tenant user's token carries
// the full tenant_roles map, an active_tenant, and active-tenant permissions.
func TestLoginEmitsTenantScopedClaims(t *testing.T) {
	svc := newTestService()
	user, _ := svc.Register("multi@example.com", "password123", types.RoleTrader)

	if err := svc.AssignTenantRole("ace-commodities", user.ID, string(types.RoleTrader), "system"); err != nil {
		t.Fatalf("AssignTenantRole ace: %v", err)
	}
	if err := svc.AssignTenantRole("mse-equities", user.ID, string(types.RoleViewer), "system"); err != nil {
		t.Fatalf("AssignTenantRole mse: %v", err)
	}

	tokens, err := svc.LoginWithTenant("multi@example.com", "password123", "mse-equities")
	if err != nil {
		t.Fatalf("LoginWithTenant: %v", err)
	}

	claims, err := svc.ValidateToken(tokens.AccessToken)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}

	if claims.Iss != Issuer {
		t.Errorf("Iss = %q, want %q", claims.Iss, Issuer)
	}
	if claims.ActiveTenant != "mse-equities" {
		t.Errorf("ActiveTenant = %q, want mse-equities", claims.ActiveTenant)
	}
	if !containsRole(claims.TenantRoles["ace-commodities"], types.RoleTrader) {
		t.Errorf("expected trader role in ace-commodities, got %v", claims.TenantRoles["ace-commodities"])
	}
	if !containsRole(claims.TenantRoles["mse-equities"], types.RoleViewer) {
		t.Errorf("expected viewer role in mse-equities, got %v", claims.TenantRoles["mse-equities"])
	}
	// Active tenant is mse-equities (viewer) → read-only permissions only.
	if !containsPerm(claims.Permissions, types.PermMarketRead) {
		t.Errorf("expected market:read permission, got %v", claims.Permissions)
	}
	if containsPerm(claims.Permissions, types.PermTradeExecute) {
		t.Errorf("viewer in active tenant should NOT have trade:execute, got %v", claims.Permissions)
	}
	if claims.Role != types.RoleViewer {
		t.Errorf("primary Role = %q, want viewer (active tenant)", claims.Role)
	}
}

// TestLoginRejectsTenantWithoutAccess enforces that tenant context is never
// silently dropped: requesting a tenant the user has no role in fails.
func TestLoginRejectsTenantWithoutAccess(t *testing.T) {
	svc := newTestService()
	user, _ := svc.Register("scoped@example.com", "password123", types.RoleTrader)
	svc.AssignTenantRole("ace-commodities", user.ID, string(types.RoleTrader), "system")

	_, err := svc.LoginWithTenant("scoped@example.com", "password123", "mse-equities")
	if err != ErrTenantAccessDenied {
		t.Errorf("expected ErrTenantAccessDenied, got %v", err)
	}
}

// TestPlatformAdminBypassesTenantRoleCheck verifies a platform-admin may select
// any tenant even without an explicit per-tenant role, and the platform_roles
// claim is emitted.
func TestPlatformAdminBypassesTenantRoleCheck(t *testing.T) {
	svc := newTestService()
	user, _ := svc.Register("ops@example.com", "password123", types.RoleAdmin)
	svc.AssignTenantRole(types.PlatformScope, user.ID, string(types.PlatformRoleAdmin), "system")

	tokens, err := svc.LoginWithTenant("ops@example.com", "password123", "mse-equities")
	if err != nil {
		t.Fatalf("platform-admin login: %v", err)
	}
	claims, err := svc.ValidateToken(tokens.AccessToken)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if !containsPlatformRole(claims.PlatformRoles, types.PlatformRoleAdmin) {
		t.Errorf("expected platform-admin in platform_roles, got %v", claims.PlatformRoles)
	}
	if claims.ActiveTenant != "mse-equities" {
		t.Errorf("ActiveTenant = %q, want mse-equities", claims.ActiveTenant)
	}
	// Platform-scoped role must not leak into tenant_roles.
	if _, ok := claims.TenantRoles[types.PlatformScope]; ok {
		t.Errorf("platform scope should not appear in tenant_roles: %v", claims.TenantRoles)
	}
}

// TestLegacyUserFallsBackToDefaultTenant verifies single-tenant users with no
// explicit assignments still get a working, tenant-scoped token.
func TestLegacyUserFallsBackToDefaultTenant(t *testing.T) {
	svc := newTestService()
	svc.Register("legacy@example.com", "password123", types.RoleTrader)

	tokens, err := svc.Login("legacy@example.com", "password123")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	claims, err := svc.ValidateToken(tokens.AccessToken)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.ActiveTenant != types.DefaultTenant {
		t.Errorf("ActiveTenant = %q, want %q", claims.ActiveTenant, types.DefaultTenant)
	}
	if !containsRole(claims.TenantRoles[types.DefaultTenant], types.RoleTrader) {
		t.Errorf("expected trader in default tenant, got %v", claims.TenantRoles)
	}
	if !containsPerm(claims.Permissions, types.PermTradeExecute) {
		t.Errorf("expected trade:execute for trader, got %v", claims.Permissions)
	}
}

// TestResolveActiveTenantPrefersDefault checks deterministic active-tenant choice.
func TestResolveActiveTenantPrefersDefault(t *testing.T) {
	roles := map[string][]types.Role{
		"mse-equities":      {types.RoleViewer},
		types.DefaultTenant: {types.RoleTrader},
		"zeta-sandbox":      {types.RoleViewer},
	}
	if got := resolveActiveTenant(roles); got != types.DefaultTenant {
		t.Errorf("resolveActiveTenant = %q, want %q", got, types.DefaultTenant)
	}

	noDefault := map[string][]types.Role{
		"mse-equities": {types.RoleViewer},
		"alpha-venue":  {types.RoleViewer},
	}
	if got := resolveActiveTenant(noDefault); got != "alpha-venue" {
		t.Errorf("resolveActiveTenant = %q, want alpha-venue (alphabetically first)", got)
	}

	if got := resolveActiveTenant(map[string][]types.Role{}); got != "" {
		t.Errorf("resolveActiveTenant(empty) = %q, want \"\"", got)
	}
}

// TestPermissionsForRolesDedup verifies the union dedups overlapping permissions.
func TestPermissionsForRolesDedup(t *testing.T) {
	perms := types.PermissionsForRoles([]types.Role{types.RoleTrader, types.RoleViewer})
	seen := map[types.Permission]int{}
	for _, p := range perms {
		seen[p]++
	}
	for p, n := range seen {
		if n != 1 {
			t.Errorf("permission %q appears %d times, want 1", p, n)
		}
	}
	if !containsPerm(perms, types.PermTradeExecute) {
		t.Error("expected trade:execute from trader role")
	}
	// Empty role set → empty (non-nil) slice.
	if got := types.PermissionsForRoles(nil); len(got) != 0 {
		t.Errorf("PermissionsForRoles(nil) = %v, want empty", got)
	}
}

// TestAssignTenantRoleIdempotent verifies re-granting the same role does not
// duplicate the assignment.
func TestAssignTenantRoleIdempotent(t *testing.T) {
	svc := newTestService()
	user, _ := svc.Register("idem@example.com", "password123", types.RoleViewer)

	svc.AssignTenantRole("ace-commodities", user.ID, string(types.RoleTrader), "system")
	svc.AssignTenantRole("ace-commodities", user.ID, string(types.RoleTrader), "system")

	tokens, _ := svc.LoginWithTenant("idem@example.com", "password123", "ace-commodities")
	claims, _ := svc.ValidateToken(tokens.AccessToken)
	if got := len(claims.TenantRoles["ace-commodities"]); got != 1 {
		t.Errorf("ace-commodities role count = %d, want 1", got)
	}
}
