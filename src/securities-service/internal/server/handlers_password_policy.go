// Package server — password policy HTTP handlers.
package server

import (
	"encoding/json"
	"net/http"

	"github.com/garudax-platform/securities-service/internal/middleware"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// handlePasswordPolicy dispatches GET/PUT /password-policy.
func (s *Server) handlePasswordPolicy(w http.ResponseWriter, r *http.Request) {
	if s.passwordPolicyStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "password policy store not configured", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGetPasswordPolicy(w, r)
	case http.MethodPut:
		s.handleSetPasswordPolicy(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleGetPasswordPolicy handles GET /password-policy.
// Returns the password policy for the current tenant.
func (s *Server) handleGetPasswordPolicy(w http.ResponseWriter, r *http.Request) {
	tenantID := resolveTenantID(r)
	policy, err := s.passwordPolicyStore.Get(tenantID)
	if err == store.ErrNotFound {
		// Return a sensible default when no policy has been configured yet.
		policy = defaultPasswordPolicy(tenantID)
	} else if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, policy)
}

// setPasswordPolicyRequest is the request body for PUT /password-policy.
type setPasswordPolicyRequest struct {
	MinLength        int  `json:"min_length"`
	RequireUppercase bool `json:"require_uppercase"`
	RequireLowercase bool `json:"require_lowercase"`
	RequireDigit     bool `json:"require_digit"`
	RequireSpecial   bool `json:"require_special"`
	MaxAgeDays       int  `json:"max_age_days"`
}

// handleSetPasswordPolicy handles PUT /password-policy.
// Sets or replaces the password policy for the current tenant.
func (s *Server) handleSetPasswordPolicy(w http.ResponseWriter, r *http.Request) {
	tenantID := resolveTenantID(r)
	var req setPasswordPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}
	if req.MinLength < 1 {
		req.MinLength = 8 // sensible minimum
	}
	policy := types.PasswordPolicy{
		TenantID:         tenantID,
		MinLength:        req.MinLength,
		RequireUppercase: req.RequireUppercase,
		RequireLowercase: req.RequireLowercase,
		RequireDigit:     req.RequireDigit,
		RequireSpecial:   req.RequireSpecial,
		MaxAgeDays:       req.MaxAgeDays,
	}
	if err := s.passwordPolicyStore.Set(&policy); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, policy)
}

// resolveTenantID extracts the tenant ID from the request context or header.
func resolveTenantID(r *http.Request) string {
	if t, ok := middleware.TenantFromContext(r.Context()); ok {
		return t.String()
	}
	return r.Header.Get("X-GarudaX-Tenant")
}

// defaultPasswordPolicy returns a baseline policy when none has been configured.
func defaultPasswordPolicy(tenantID string) *types.PasswordPolicy {
	return &types.PasswordPolicy{
		TenantID:         tenantID,
		MinLength:        8,
		RequireUppercase: true,
		RequireLowercase: true,
		RequireDigit:     true,
		RequireSpecial:   false,
		MaxAgeDays:       90,
	}
}
