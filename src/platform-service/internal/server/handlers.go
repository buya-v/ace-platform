// Package server — tenant CRUD HTTP handlers.
package server

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/garudax-platform/platform-service/internal/types"
)

// handleTenantConfig handles GET /platform/v1/tenants/{id}/config.
// Returns the venue configuration for the given tenant ID, or 404 if not found.
func (s *Server) handleTenantConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}

	// Path: /platform/v1/tenants/{id}/config — strip the /config suffix to get the tenant ID.
	path := strings.TrimSuffix(strings.TrimSuffix(r.URL.Path, "/"), "/config")
	id := extractLastSegment(path)

	cfg, err := s.configLoader.LoadConfig(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "CONFIG_NOT_FOUND", "config not found for tenant: "+id, nil)
		return
	}
	s.writeJSON(w, http.StatusOK, cfg)
}

// slugRE matches valid tenant IDs: lowercase alphanumeric and hyphens.
var slugRE = regexp.MustCompile(`^[a-z0-9-]+$`)

// validStatuses is the set of allowed tenant status values.
var validStatuses = map[string]bool{
	types.TenantStatusActive:         true,
	types.TenantStatusSuspended:      true,
	types.TenantStatusOnboarding:     true,
	types.TenantStatusDecommissioned: true,
}

// newUUID generates a random UUID v4 string using crypto/rand.
func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	// Set version 4 and variant bits (RFC 4122).
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// extractLastSegment returns the last non-empty path segment of rawPath.
// e.g. "/platform/v1/tenants/ace-commodities" → "ace-commodities"
func extractLastSegment(rawPath string) string {
	trimmed := strings.TrimSuffix(rawPath, "/")
	idx := strings.LastIndex(trimmed, "/")
	if idx < 0 {
		return trimmed
	}
	return trimmed[idx+1:]
}

// handleTenants dispatches GET /platform/v1/tenants (list)
// and POST /platform/v1/tenants (create).
func (s *Server) handleTenants(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListTenants(w, r)
	case http.MethodPost:
		s.handleCreateTenant(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleTenant dispatches requests to /platform/v1/tenants/{id} and sub-resources.
func (s *Server) handleTenant(w http.ResponseWriter, r *http.Request) {
	// Detect the /status sub-resource.
	if strings.HasSuffix(strings.TrimSuffix(r.URL.Path, "/"), "/status") {
		if r.Method == http.MethodPut {
			s.handleUpdateTenantStatus(w, r)
		} else {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetTenant(w, r)
	case http.MethodPatch:
		s.handleUpdateTenant(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleListTenants handles GET /platform/v1/tenants.
func (s *Server) handleListTenants(w http.ResponseWriter, r *http.Request) {
	tenants, err := s.tenantStore.List()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, tenants)
}

// handleGetTenant handles GET /platform/v1/tenants/{id}.
func (s *Server) handleGetTenant(w http.ResponseWriter, r *http.Request) {
	id := extractLastSegment(r.URL.Path)
	t, err := s.tenantStore.Get(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "TENANT_NOT_FOUND", "tenant not found: "+id, nil)
		return
	}
	s.writeJSON(w, http.StatusOK, t)
}

// handleCreateTenant handles POST /platform/v1/tenants.
//
// Required fields: id (lowercase slug [a-z0-9-]), name.
// Defaults:        status=ONBOARDING, flagship=false, governance_tier=STANDARD.
func (s *Server) handleCreateTenant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID                 string                 `json:"id"`
		Name               string                 `json:"name"`
		GovernanceTier     string                 `json:"governance_tier"`
		Flagship           bool                   `json:"flagship"`
		OnboardingMetadata map[string]interface{} `json:"onboarding_metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}

	// Validate required fields.
	var details []string
	if req.ID == "" {
		details = append(details, "id is required")
	} else if !slugRE.MatchString(req.ID) {
		details = append(details, "id must be a lowercase slug matching [a-z0-9-]")
	}
	if req.Name == "" {
		details = append(details, "name is required")
	}
	if len(details) > 0 {
		s.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "request validation failed", details)
		return
	}

	// Apply defaults.
	governanceTier := req.GovernanceTier
	if governanceTier == "" {
		governanceTier = "STANDARD"
	}
	onboardingMetadata := req.OnboardingMetadata
	if onboardingMetadata == nil {
		onboardingMetadata = map[string]interface{}{}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	t := &types.Tenant{
		ID:                 req.ID,
		Name:               req.Name,
		Status:             types.TenantStatusOnboarding,
		Flagship:           req.Flagship,
		GovernanceTier:     governanceTier,
		OnboardingMetadata: onboardingMetadata,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := s.tenantStore.Create(t); err != nil {
		s.writeError(w, http.StatusConflict, "TENANT_ALREADY_EXISTS", err.Error(), nil)
		return
	}

	// Provision schemas and topic prefixes for the new tenant.
	provResult, err := s.provisioner.ProvisionTenant(t)
	if err != nil {
		// Provisioning failure is non-fatal for MVP — log and return a partial result.
		s.writeJSON(w, http.StatusCreated, map[string]interface{}{
			"tenant": t,
			"provisioning": map[string]interface{}{
				"status": "FAILED",
				"error":  err.Error(),
			},
		})
		return
	}

	s.writeJSON(w, http.StatusCreated, map[string]interface{}{
		"tenant":       t,
		"provisioning": provResult,
	})
}

// handleUpdateTenant handles PATCH /platform/v1/tenants/{id}.
// Updatable fields: name, governance_tier.
func (s *Server) handleUpdateTenant(w http.ResponseWriter, r *http.Request) {
	id := extractLastSegment(r.URL.Path)

	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}

	// Restrict to allowed fields.
	updates := map[string]interface{}{}
	if v, ok := req["name"]; ok {
		updates["name"] = v
	}
	if v, ok := req["governance_tier"]; ok {
		updates["governance_tier"] = v
	}

	if err := s.tenantStore.Update(id, updates); err != nil {
		s.writeError(w, http.StatusNotFound, "TENANT_NOT_FOUND", "tenant not found: "+id, nil)
		return
	}

	t, err := s.tenantStore.Get(id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, t)
}

// handleUpdateTenantStatus handles PUT /platform/v1/tenants/{id}/status.
// Body: {"status": "<ACTIVE|SUSPENDED|ONBOARDING|DECOMMISSIONED>"}
func (s *Server) handleUpdateTenantStatus(w http.ResponseWriter, r *http.Request) {
	// Path is /platform/v1/tenants/{id}/status — extract the tenant ID.
	path := strings.TrimSuffix(r.URL.Path, "/")
	path = strings.TrimSuffix(path, "/status")
	id := extractLastSegment(path)

	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}

	if !validStatuses[req.Status] {
		s.writeError(w, http.StatusBadRequest, "INVALID_STATUS",
			fmt.Sprintf("invalid status %q; must be one of ACTIVE, SUSPENDED, ONBOARDING, DECOMMISSIONED", req.Status), nil)
		return
	}

	if err := s.tenantStore.UpdateStatus(id, req.Status); err != nil {
		s.writeError(w, http.StatusNotFound, "TENANT_NOT_FOUND", "tenant not found: "+id, nil)
		return
	}

	t, err := s.tenantStore.Get(id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, t)
}
