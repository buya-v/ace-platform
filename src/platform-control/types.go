// Package main implements the GarudaX Platform Control Plane.
//
// The control plane is the platform-level (NOT tenant-scoped) authority that owns
// the tenant registry and exposes the platform-admin API used to create, suspend,
// modify, activate, and decommission tenant environments (venues).
//
// Platform invariant (CLAUDE.md): GarudaX is the platform, tenants are the venues,
// MSE is the flagship, and tenant ID is never optional. This service IS the platform,
// so its routes live under /platform/v1/* and carry no tenant middleware.
//
// The package is deliberately a single, zero-dependency Go module (stdlib only),
// matching the established service pattern in this codebase. The two core pieces are
// the TenantRegistry (registry.go) and the HTTP API (api.go).
package main

import "time"

// Tenant lifecycle states. These mirror the CHECK constraint on
// platform.tenants.status in infrastructure/db/migrations/V31__platform_control_schemas.sql.
const (
	StatusOnboarding     = "ONBOARDING"     // Tenant created, environment being provisioned, not yet trading.
	StatusActive         = "ACTIVE"         // Tenant live and accepting traffic.
	StatusSuspended      = "SUSPENDED"      // Tenant temporarily halted (e.g. regulatory hold); reversible.
	StatusDecommissioned = "DECOMMISSIONED" // Tenant permanently retired; terminal state.
)

// Governance tiers mirror the CHECK constraint on platform.tenants.governance_tier.
const (
	TierFlagship = "FLAGSHIP"
	TierStandard = "STANDARD"
	TierSandbox  = "SANDBOX"
)

// validStatuses is the set of recognised lifecycle states.
var validStatuses = map[string]bool{
	StatusOnboarding:     true,
	StatusActive:         true,
	StatusSuspended:      true,
	StatusDecommissioned: true,
}

// validTiers is the set of recognised governance tiers.
var validTiers = map[string]bool{
	TierFlagship: true,
	TierStandard: true,
	TierSandbox:  true,
}

// allowedTransitions is the lifecycle state machine. A transition from state S is
// permitted only if the target state appears in allowedTransitions[S].
//
//	ONBOARDING ──activate──▶ ACTIVE ──suspend──▶ SUSPENDED
//	     │                     │                     │
//	     └──────────┐          └──────────┐          └──activate──▶ ACTIVE
//	                ▼                      ▼                     │
//	          DECOMMISSIONED ◀────────────┴─────────────────────┘
//
// DECOMMISSIONED is terminal: it has no outgoing transitions.
var allowedTransitions = map[string]map[string]bool{
	StatusOnboarding: {
		StatusActive:         true, // go live
		StatusDecommissioned: true, // abandon onboarding
	},
	StatusActive: {
		StatusSuspended:      true, // temporary halt
		StatusDecommissioned: true, // retire
	},
	StatusSuspended: {
		StatusActive:         true, // resume
		StatusDecommissioned: true, // retire from suspension
	},
	StatusDecommissioned: {}, // terminal
}

// Tenant is the registry entry for a GarudaX venue. Fields correspond to the
// platform.tenants table (V31). Timestamps are RFC3339 strings for stable JSON.
type Tenant struct {
	ID                     string                 `json:"id"`
	DisplayName            string                 `json:"display_name"`
	Description            string                 `json:"description,omitempty"`
	Status                 string                 `json:"status"`
	Flagship               bool                   `json:"flagship"`
	GovernanceTier         string                 `json:"governance_tier"`
	AssetClasses           []string               `json:"asset_classes"`
	DefaultSettlementCycle string                 `json:"default_settlement_cycle"`
	PrimaryCurrency        string                 `json:"primary_currency"`
	Timezone               string                 `json:"timezone"`
	RegulatoryBody         string                 `json:"regulatory_body,omitempty"`
	OnboardingMetadata     map[string]interface{} `json:"onboarding_metadata"`
	ConfigVersion          int                    `json:"config_version"`
	CreatedAt              string                 `json:"created_at"`
	UpdatedAt              string                 `json:"updated_at"`
	ActivatedAt            string                 `json:"activated_at,omitempty"`
	SuspendedAt            string                 `json:"suspended_at,omitempty"`
	DecommissionedAt       string                 `json:"decommissioned_at,omitempty"`
}

// clone returns a deep-enough copy of the tenant so callers cannot mutate registry
// state through the returned pointer. The metadata map and asset-class slice are copied.
func (t *Tenant) clone() *Tenant {
	cp := *t
	if t.OnboardingMetadata != nil {
		cp.OnboardingMetadata = make(map[string]interface{}, len(t.OnboardingMetadata))
		for k, v := range t.OnboardingMetadata {
			cp.OnboardingMetadata[k] = v
		}
	}
	if t.AssetClasses != nil {
		// Preserve a non-nil empty slice (append to nil with no elems yields nil).
		cp.AssetClasses = make([]string, len(t.AssetClasses))
		copy(cp.AssetClasses, t.AssetClasses)
	}
	return &cp
}

// AuditEntry records a single control-plane action against a tenant. The registry
// keeps an append-only, in-memory audit trail; this is the application-side mirror
// of the platform.audit table (V31).
type AuditEntry struct {
	Sequence   int       `json:"sequence"`
	TenantID   string    `json:"tenant_id"`
	Action     string    `json:"action"` // e.g. "tenant.created", "tenant.suspended"
	FromStatus string    `json:"from_status,omitempty"`
	ToStatus   string    `json:"to_status,omitempty"`
	Actor      string    `json:"actor"`
	Detail     string    `json:"detail,omitempty"`
	At         time.Time `json:"at"`
}

// CreateTenantRequest is the body of POST /platform/v1/tenants.
type CreateTenantRequest struct {
	ID                     string                 `json:"id"`
	DisplayName            string                 `json:"display_name"`
	Description            string                 `json:"description"`
	Flagship               bool                   `json:"flagship"`
	GovernanceTier         string                 `json:"governance_tier"`
	AssetClasses           []string               `json:"asset_classes"`
	DefaultSettlementCycle string                 `json:"default_settlement_cycle"`
	PrimaryCurrency        string                 `json:"primary_currency"`
	Timezone               string                 `json:"timezone"`
	RegulatoryBody         string                 `json:"regulatory_body"`
	OnboardingMetadata     map[string]interface{} `json:"onboarding_metadata"`
}

// UpdateTenantRequest is the body of PATCH /platform/v1/tenants/{id}. All fields are
// optional pointers so an omitted field is left unchanged (distinct from "set to empty").
type UpdateTenantRequest struct {
	DisplayName    *string   `json:"display_name"`
	Description    *string   `json:"description"`
	GovernanceTier *string   `json:"governance_tier"`
	AssetClasses   *[]string `json:"asset_classes"`
	RegulatoryBody *string   `json:"regulatory_body"`
}

// StatusChangeRequest is the body of PUT /platform/v1/tenants/{id}/status.
type StatusChangeRequest struct {
	Status string `json:"status"`
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
}

// ErrorDetail carries a machine-readable code and human-readable message.
type ErrorDetail struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Details []string `json:"details,omitempty"`
}

// ErrorResponse is the standard error envelope returned by all endpoints.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}
