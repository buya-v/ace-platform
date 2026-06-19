package main

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"sync"
	"time"
)

// Sentinel errors returned by the registry. The API layer maps these to HTTP codes.
var (
	// ErrNotFound is returned when a tenant ID does not exist in the registry.
	ErrNotFound = errors.New("tenant not found")
	// ErrAlreadyExists is returned when creating a tenant whose ID is already taken.
	ErrAlreadyExists = errors.New("tenant already exists")
	// ErrValidation is returned when a request fails field validation.
	ErrValidation = errors.New("validation failed")
	// ErrInvalidTransition is returned when a lifecycle transition is not permitted.
	ErrInvalidTransition = errors.New("invalid status transition")
	// ErrFlagshipConflict is returned when an operation would create a second flagship tenant.
	ErrFlagshipConflict = errors.New("a flagship tenant already exists")
	// ErrTerminal is returned when modifying a tenant that has been decommissioned.
	ErrTerminal = errors.New("tenant is decommissioned and immutable")
)

// slugRE matches valid tenant IDs: lowercase alphanumeric and hyphens, 2-64 chars,
// not starting or ending with a hyphen.
var slugRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$`)

// ValidationError aggregates per-field validation problems. It wraps ErrValidation
// so callers can match with errors.Is.
type ValidationError struct {
	Fields []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %v", ErrValidation.Error(), e.Fields)
}

// Unwrap allows errors.Is(err, ErrValidation) to succeed.
func (e *ValidationError) Unwrap() error { return ErrValidation }

// nowFunc is the clock used for timestamps. It is a package variable so tests can
// pin time deterministically.
var nowFunc = time.Now

// TenantRegistry is the in-memory, thread-safe authority for the tenant registry.
// It enforces the lifecycle state machine, the single-flagship invariant, and keeps
// an append-only audit trail of every mutating action.
//
// The store is in-memory by design (matching the zero-dependency service pattern in
// this codebase); a DB-backed implementation can be slotted behind the same methods
// without changing the API layer.
type TenantRegistry struct {
	mu       sync.RWMutex
	tenants  map[string]*Tenant
	audit    []AuditEntry
	auditSeq int
}

// NewTenantRegistry returns an empty registry.
func NewTenantRegistry() *TenantRegistry {
	return &TenantRegistry{tenants: make(map[string]*Tenant)}
}

// NewSeededRegistry returns a registry pre-populated with the two known GarudaX
// tenants from V31__platform_control_schemas.sql: ace-commodities (ACTIVE) and
// mse-equities (ONBOARDING, flagship).
func NewSeededRegistry() *TenantRegistry {
	r := NewTenantRegistry()
	now := nowFunc().UTC().Format(time.RFC3339)
	r.tenants["ace-commodities"] = &Tenant{
		ID:                     "ace-commodities",
		DisplayName:            "ACE Commodity Exchange",
		Status:                 StatusActive,
		Flagship:               false,
		GovernanceTier:         TierStandard,
		AssetClasses:           []string{"COMMODITY"},
		DefaultSettlementCycle: "T+0",
		PrimaryCurrency:        "MNT",
		Timezone:               "Asia/Ulaanbaatar",
		RegulatoryBody:         "MCGA",
		OnboardingMetadata:     map[string]interface{}{},
		ConfigVersion:          1,
		CreatedAt:              now,
		UpdatedAt:              now,
		ActivatedAt:            now,
	}
	r.tenants["mse-equities"] = &Tenant{
		ID:                     "mse-equities",
		DisplayName:            "Mongolian Stock Exchange",
		Description:            "Flagship tenant — platform decisions defer to MSE requirements.",
		Status:                 StatusOnboarding,
		Flagship:               true,
		GovernanceTier:         TierFlagship,
		AssetClasses:           []string{"EQUITY", "BOND", "ETF"},
		DefaultSettlementCycle: "T+2",
		PrimaryCurrency:        "MNT",
		Timezone:               "Asia/Ulaanbaatar",
		RegulatoryBody:         "FRC",
		OnboardingMetadata:     map[string]interface{}{},
		ConfigVersion:          1,
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	r.appendAudit("ace-commodities", "tenant.seeded", "", StatusActive, "system", "seeded from V31")
	r.appendAudit("mse-equities", "tenant.seeded", "", StatusOnboarding, "system", "seeded from V31")
	return r
}

// List returns all tenants sorted by ID. If statusFilter is non-empty only tenants
// in that status are returned.
func (r *TenantRegistry) List(statusFilter string) []Tenant {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Tenant, 0, len(r.tenants))
	for _, t := range r.tenants {
		if statusFilter != "" && t.Status != statusFilter {
			continue
		}
		out = append(out, *t.clone())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Get returns a copy of the tenant with the given ID, or ErrNotFound.
func (r *TenantRegistry) Get(id string) (*Tenant, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tenants[id]
	if !ok {
		return nil, ErrNotFound
	}
	return t.clone(), nil
}

// Create validates and inserts a new tenant. New tenants always start in ONBOARDING.
// It enforces the single-flagship invariant and applies sensible defaults.
func (r *TenantRegistry) Create(req CreateTenantRequest) (*Tenant, error) {
	if verr := validateCreate(req); verr != nil {
		return nil, verr
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tenants[req.ID]; exists {
		return nil, ErrAlreadyExists
	}
	if req.Flagship {
		if existing := r.flagshipLocked(); existing != "" {
			return nil, fmt.Errorf("%w: held by %q", ErrFlagshipConflict, existing)
		}
	}

	now := nowFunc().UTC().Format(time.RFC3339)
	t := &Tenant{
		ID:                     req.ID,
		DisplayName:            req.DisplayName,
		Description:            req.Description,
		Status:                 StatusOnboarding,
		Flagship:               req.Flagship,
		GovernanceTier:         defaultStr(req.GovernanceTier, TierStandard),
		AssetClasses:           defaultSlice(req.AssetClasses),
		DefaultSettlementCycle: defaultStr(req.DefaultSettlementCycle, "T+0"),
		PrimaryCurrency:        defaultStr(req.PrimaryCurrency, "MNT"),
		Timezone:               defaultStr(req.Timezone, "Asia/Ulaanbaatar"),
		RegulatoryBody:         req.RegulatoryBody,
		OnboardingMetadata:     defaultMap(req.OnboardingMetadata),
		ConfigVersion:          1,
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	r.tenants[t.ID] = t
	r.appendAudit(t.ID, "tenant.created", "", StatusOnboarding, "platform-admin", "")
	return t.clone(), nil
}

// Update applies a partial modification to a tenant's descriptive fields. It does not
// change lifecycle status (use TransitionStatus for that). Decommissioned tenants are
// immutable. Modifying the governance tier or asset classes bumps config_version.
func (r *TenantRegistry) Update(id string, req UpdateTenantRequest) (*Tenant, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	t, ok := r.tenants[id]
	if !ok {
		return nil, ErrNotFound
	}
	if t.Status == StatusDecommissioned {
		return nil, ErrTerminal
	}

	var fields []string
	if req.GovernanceTier != nil && !validTiers[*req.GovernanceTier] {
		fields = append(fields, "governance_tier must be one of FLAGSHIP, STANDARD, SANDBOX")
	}
	if req.DisplayName != nil && *req.DisplayName == "" {
		fields = append(fields, "display_name cannot be set to empty")
	}
	if len(fields) > 0 {
		return nil, &ValidationError{Fields: fields}
	}

	bumpConfig := false
	if req.DisplayName != nil {
		t.DisplayName = *req.DisplayName
	}
	if req.Description != nil {
		t.Description = *req.Description
	}
	if req.RegulatoryBody != nil {
		t.RegulatoryBody = *req.RegulatoryBody
	}
	if req.GovernanceTier != nil && *req.GovernanceTier != t.GovernanceTier {
		t.GovernanceTier = *req.GovernanceTier
		bumpConfig = true
	}
	if req.AssetClasses != nil {
		t.AssetClasses = append([]string(nil), (*req.AssetClasses)...)
		bumpConfig = true
	}
	if bumpConfig {
		t.ConfigVersion++
	}
	t.UpdatedAt = nowFunc().UTC().Format(time.RFC3339)
	r.appendAudit(t.ID, "tenant.modified", t.Status, t.Status, "platform-admin", "")
	return t.clone(), nil
}

// TransitionStatus moves a tenant to a new lifecycle state if the transition is
// permitted by the state machine. It is the single entry point for activate, suspend,
// resume, and decommission. actor and reason are recorded in the audit trail.
func (r *TenantRegistry) TransitionStatus(id, target, actor, reason string) (*Tenant, error) {
	if !validStatuses[target] {
		return nil, &ValidationError{Fields: []string{
			"status must be one of ONBOARDING, ACTIVE, SUSPENDED, DECOMMISSIONED",
		}}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	t, ok := r.tenants[id]
	if !ok {
		return nil, ErrNotFound
	}

	from := t.Status
	if from == target {
		return nil, fmt.Errorf("%w: tenant %q is already %s", ErrInvalidTransition, id, target)
	}
	if !allowedTransitions[from][target] {
		return nil, fmt.Errorf("%w: %s → %s is not permitted", ErrInvalidTransition, from, target)
	}

	now := nowFunc().UTC().Format(time.RFC3339)
	t.Status = target
	t.UpdatedAt = now
	switch target {
	case StatusActive:
		t.ActivatedAt = now
	case StatusSuspended:
		t.SuspendedAt = now
	case StatusDecommissioned:
		t.DecommissionedAt = now
	}
	if actor == "" {
		actor = "platform-admin"
	}
	r.appendAudit(t.ID, actionForTransition(target), from, target, actor, reason)
	return t.clone(), nil
}

// Audit returns the audit entries for a single tenant (or all tenants when id is
// empty), most recent last. Returns ErrNotFound when a specific, unknown id is given.
func (r *TenantRegistry) Audit(id string) ([]AuditEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if id != "" {
		if _, ok := r.tenants[id]; !ok {
			return nil, ErrNotFound
		}
	}
	out := make([]AuditEntry, 0, len(r.audit))
	for _, e := range r.audit {
		if id == "" || e.TenantID == id {
			out = append(out, e)
		}
	}
	return out, nil
}

// flagshipLocked returns the ID of the current flagship tenant, or "" if none.
// Caller must hold the lock.
func (r *TenantRegistry) flagshipLocked() string {
	for _, t := range r.tenants {
		if t.Flagship && t.Status != StatusDecommissioned {
			return t.ID
		}
	}
	return ""
}

// appendAudit records an action. Caller must hold the write lock.
func (r *TenantRegistry) appendAudit(tenantID, action, from, to, actor, detail string) {
	r.auditSeq++
	r.audit = append(r.audit, AuditEntry{
		Sequence:   r.auditSeq,
		TenantID:   tenantID,
		Action:     action,
		FromStatus: from,
		ToStatus:   to,
		Actor:      actor,
		Detail:     detail,
		At:         nowFunc().UTC(),
	})
}

// --- helpers ---

func actionForTransition(target string) string {
	switch target {
	case StatusActive:
		return "tenant.activated"
	case StatusSuspended:
		return "tenant.suspended"
	case StatusDecommissioned:
		return "tenant.decommissioned"
	default:
		return "tenant.status_changed"
	}
}

func validateCreate(req CreateTenantRequest) *ValidationError {
	var fields []string
	switch {
	case req.ID == "":
		fields = append(fields, "id is required")
	case !slugRE.MatchString(req.ID):
		fields = append(fields, "id must be a lowercase slug [a-z0-9-], 2-64 chars, no leading/trailing hyphen")
	}
	if req.DisplayName == "" {
		fields = append(fields, "display_name is required")
	}
	if req.GovernanceTier != "" && !validTiers[req.GovernanceTier] {
		fields = append(fields, "governance_tier must be one of FLAGSHIP, STANDARD, SANDBOX")
	}
	if req.DefaultSettlementCycle != "" && !validSettlementCycle(req.DefaultSettlementCycle) {
		fields = append(fields, "default_settlement_cycle must be one of T+0, T+1, T+2, T+3")
	}
	if len(fields) > 0 {
		return &ValidationError{Fields: fields}
	}
	return nil
}

func validSettlementCycle(c string) bool {
	switch c {
	case "T+0", "T+1", "T+2", "T+3":
		return true
	default:
		return false
	}
}

func defaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func defaultSlice(v []string) []string {
	if v == nil {
		return []string{}
	}
	return append([]string(nil), v...)
}

func defaultMap(v map[string]interface{}) map[string]interface{} {
	if v == nil {
		return map[string]interface{}{}
	}
	return v
}
