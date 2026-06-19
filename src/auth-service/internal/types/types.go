package types

import "time"

type Role string

const (
	RoleSuperAdmin        Role = "super_admin"
	RoleAdmin             Role = "admin"
	RoleTrader            Role = "trader"
	RoleViewer            Role = "viewer"
	RoleComplianceOfficer Role = "compliance_officer"
	// Tenant-scoped operational roles (platform-architecture.md §5.2).
	RoleClearingAdmin Role = "clearing_admin"
	RoleExchangeAdmin Role = "exchange_admin"
)

// PlatformRole is a platform-level (cross-tenant) role. It is NOT tenant-scoped;
// it grants abilities above any single venue (platform-architecture.md §5.2).
type PlatformRole string

const (
	// PlatformRoleAdmin can manage tenant lifecycle, run cross-tenant queries,
	// and change platform config. It is the only platform role currently defined.
	PlatformRoleAdmin PlatformRole = "platform-admin"
)

// PlatformScope is the sentinel tenant_id used in platform.tenant_user_roles for
// platform-level (cross-tenant) role assignments. Such assignments surface as
// platform_roles in the JWT rather than under tenant_roles.
const PlatformScope = "platform"

// DefaultTenant is the venue assumed for legacy single-tenant users that have no
// explicit platform.tenant_user_roles assignment. ace-commodities is the live
// production tenant (V29 seed); this preserves backward compatibility while the
// platform completes its multi-tenant retrofit.
const DefaultTenant = "ace-commodities"

type Permission string

const (
	PermUserManage       Permission = "user:manage"
	PermUserRead         Permission = "user:read"
	PermTradeExecute     Permission = "trade:execute"
	PermTradeRead        Permission = "trade:read"
	PermOrderCreate      Permission = "order:create"
	PermOrderCancel      Permission = "order:cancel"
	PermOrderRead        Permission = "order:read"
	PermMarketRead       Permission = "market:read"
	PermComplianceRead   Permission = "compliance:read"
	PermComplianceManage Permission = "compliance:manage"
	PermSystemAdmin      Permission = "system:admin"
	PermAPIKeyManage     Permission = "apikey:manage"
	// Operational permissions for tenant-scoped admin roles.
	PermSettlementManage Permission = "settlement:manage"
	PermInstrumentManage Permission = "instrument:manage"
	PermTradingHalt      Permission = "trading:halt"
)

var RolePermissions = map[Role][]Permission{
	RoleSuperAdmin: {
		PermUserManage, PermUserRead, PermTradeExecute, PermTradeRead,
		PermOrderCreate, PermOrderCancel, PermOrderRead, PermMarketRead,
		PermComplianceRead, PermComplianceManage, PermSystemAdmin, PermAPIKeyManage,
	},
	RoleAdmin: {
		PermUserManage, PermUserRead, PermTradeRead,
		PermOrderRead, PermMarketRead, PermComplianceRead, PermAPIKeyManage,
	},
	RoleTrader: {
		PermUserRead, PermTradeExecute, PermTradeRead,
		PermOrderCreate, PermOrderCancel, PermOrderRead, PermMarketRead, PermAPIKeyManage,
	},
	RoleViewer: {
		PermUserRead, PermTradeRead, PermOrderRead, PermMarketRead,
	},
	RoleComplianceOfficer: {
		PermUserRead, PermTradeRead, PermOrderRead, PermMarketRead,
		PermComplianceRead, PermComplianceManage,
	},
	RoleClearingAdmin: {
		PermUserRead, PermTradeRead, PermOrderRead, PermMarketRead,
		PermSettlementManage, PermAPIKeyManage,
	},
	RoleExchangeAdmin: {
		PermUserRead, PermTradeRead, PermOrderRead, PermMarketRead,
		PermInstrumentManage, PermTradingHalt, PermAPIKeyManage,
	},
}

// PermissionsForRoles returns the deduplicated union of permissions granted by
// the given roles, preserving first-seen order. Used to compute the
// active-tenant permission claim embedded in a JWT.
func PermissionsForRoles(roles []Role) []Permission {
	seen := make(map[Permission]bool)
	out := make([]Permission, 0)
	for _, r := range roles {
		for _, p := range RolePermissions[r] {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}

func HasPermission(role Role, perm Permission) bool {
	perms, ok := RolePermissions[role]
	if !ok {
		return false
	}
	for _, p := range perms {
		if p == perm {
			return true
		}
	}
	return false
}

func ValidRole(r string) bool {
	switch Role(r) {
	case RoleSuperAdmin, RoleAdmin, RoleTrader, RoleViewer, RoleComplianceOfficer:
		return true
	}
	return false
}

type User struct {
	ID             string
	Email          string
	Name           string
	HashedPassword string
	Role           Role
	FailedAttempts int
	LockedUntil    time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// TenantUserRole binds a platform-level user (auth.users) to a tenant-scoped —
// or, when TenantID == PlatformScope, a platform-scoped — role. It mirrors the
// platform.tenant_user_roles table introduced in migration V31 and is the source
// of truth for the per-tenant claims embedded in a JWT (platform-architecture.md §5).
type TenantUserRole struct {
	TenantID  string
	UserID    string
	Role      string
	GrantedBy string
	GrantedAt time.Time
	Revoked   bool
}

type Session struct {
	ID               string
	UserID           string
	RefreshTokenHash string
	ExpiresAt        time.Time
	CreatedAt        time.Time
	Revoked          bool
}

type APIKey struct {
	ID        string
	UserID    string
	Name      string
	KeyHash   string
	Prefix    string
	CreatedAt time.Time
	ExpiresAt time.Time
	Revoked   bool
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
}

type PKCEChallenge struct {
	CodeChallenge       string
	CodeChallengeMethod string
	AuthCode            string
	UserID              string
	RedirectURI         string
	ExpiresAt           time.Time
	Used                bool
}
