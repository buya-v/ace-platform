package types

import "time"

type Role string

const (
	RoleSuperAdmin        Role = "super_admin"
	RoleAdmin             Role = "admin"
	RoleTrader            Role = "trader"
	RoleViewer            Role = "viewer"
	RoleComplianceOfficer Role = "compliance_officer"
)

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
	HashedPassword string
	Role           Role
	FailedAttempts int
	LockedUntil    time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
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
