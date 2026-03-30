package auth

import (
	"testing"

	"github.com/garudax-platform/auth-service/internal/types"
)

func TestHasPermission(t *testing.T) {
	tests := []struct {
		role types.Role
		perm types.Permission
		want bool
	}{
		{types.RoleSuperAdmin, types.PermSystemAdmin, true},
		{types.RoleSuperAdmin, types.PermTradeExecute, true},
		{types.RoleAdmin, types.PermUserManage, true},
		{types.RoleAdmin, types.PermTradeExecute, false},
		{types.RoleTrader, types.PermTradeExecute, true},
		{types.RoleTrader, types.PermUserManage, false},
		{types.RoleTrader, types.PermOrderCreate, true},
		{types.RoleViewer, types.PermTradeRead, true},
		{types.RoleViewer, types.PermTradeExecute, false},
		{types.RoleViewer, types.PermOrderCreate, false},
		{types.RoleComplianceOfficer, types.PermComplianceManage, true},
		{types.RoleComplianceOfficer, types.PermTradeExecute, false},
		{types.Role("invalid"), types.PermTradeRead, false},
	}

	for _, tt := range tests {
		got := types.HasPermission(tt.role, tt.perm)
		if got != tt.want {
			t.Errorf("HasPermission(%q, %q) = %v, want %v", tt.role, tt.perm, got, tt.want)
		}
	}
}

func TestValidRole(t *testing.T) {
	valid := []string{"super_admin", "admin", "trader", "viewer", "compliance_officer"}
	for _, r := range valid {
		if !types.ValidRole(r) {
			t.Errorf("ValidRole(%q) = false, want true", r)
		}
	}

	invalid := []string{"", "hacker", "root", "ADMIN"}
	for _, r := range invalid {
		if types.ValidRole(r) {
			t.Errorf("ValidRole(%q) = true, want false", r)
		}
	}
}

func TestAllRolesHavePermissions(t *testing.T) {
	roles := []types.Role{types.RoleSuperAdmin, types.RoleAdmin, types.RoleTrader, types.RoleViewer, types.RoleComplianceOfficer}
	for _, r := range roles {
		perms, ok := types.RolePermissions[r]
		if !ok {
			t.Errorf("Role %q missing from RolePermissions map", r)
			continue
		}
		if len(perms) == 0 {
			t.Errorf("Role %q has no permissions", r)
		}
	}
}

func TestSuperAdminHasAllPermissions(t *testing.T) {
	allPerms := []types.Permission{
		types.PermUserManage, types.PermUserRead, types.PermTradeExecute, types.PermTradeRead,
		types.PermOrderCreate, types.PermOrderCancel, types.PermOrderRead, types.PermMarketRead,
		types.PermComplianceRead, types.PermComplianceManage, types.PermSystemAdmin, types.PermAPIKeyManage,
	}
	for _, p := range allPerms {
		if !types.HasPermission(types.RoleSuperAdmin, p) {
			t.Errorf("SuperAdmin should have permission %q", p)
		}
	}
}
