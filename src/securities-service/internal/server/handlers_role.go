// Package server — RBAC role CRUD HTTP handlers.
package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// handleRoles dispatches:
//
//	POST /api/v1/securities/roles — create a new role
//	GET  /api/v1/securities/roles — list all roles
func (s *Server) handleRoles(w http.ResponseWriter, r *http.Request) {
	if s.roleStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "role store not configured", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleListRoles(w, r)
	case http.MethodPost:
		s.handleCreateRole(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleRole dispatches:
//
//	GET    /api/v1/securities/roles/{id} — get a single role
//	PUT    /api/v1/securities/roles/{id} — update a role
//	DELETE /api/v1/securities/roles/{id} — delete a role
func (s *Server) handleRole(w http.ResponseWriter, r *http.Request) {
	if s.roleStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "role store not configured", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGetRole(w, r)
	case http.MethodPut:
		s.handleUpdateRole(w, r)
	case http.MethodDelete:
		s.handleDeleteRole(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// ── Request / response shapes ─────────────────────────────────────────────────

type createRoleRequest struct {
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
	Description string   `json:"description"`
}

type updateRoleRequest struct {
	Name        *string  `json:"name"`
	Permissions []string `json:"permissions"`
	Description *string  `json:"description"`
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// handleCreateRole handles POST /api/v1/securities/roles.
// Body: { name, permissions[], description? }
func (s *Server) handleCreateRole(w http.ResponseWriter, r *http.Request) {
	var req createRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}
	if req.Name == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "name is required", nil)
		return
	}

	id, err := newUUID()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate ID", nil)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	perms := req.Permissions
	if perms == nil {
		perms = []string{}
	}

	role := &types.Role{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		Permissions: perms,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.roleStore.Create(role); err != nil {
		s.writeError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
		return
	}

	s.writeJSON(w, http.StatusCreated, role)
}

// handleListRoles handles GET /api/v1/securities/roles.
func (s *Server) handleListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := s.roleStore.List()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, roles)
}

// handleGetRole handles GET /api/v1/securities/roles/{id}.
func (s *Server) handleGetRole(w http.ResponseWriter, r *http.Request) {
	id := extractRoleID(r.URL.Path)
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "role id is required", nil)
		return
	}

	role, err := s.roleStore.Get(id)
	if err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "role not found", nil)
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	s.writeJSON(w, http.StatusOK, role)
}

// handleUpdateRole handles PUT /api/v1/securities/roles/{id}.
// Body: { name?, permissions?, description? }
func (s *Server) handleUpdateRole(w http.ResponseWriter, r *http.Request) {
	id := extractRoleID(r.URL.Path)
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "role id is required", nil)
		return
	}

	var req updateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}

	name := ""
	if req.Name != nil {
		name = *req.Name
	}
	description := ""
	if req.Description != nil {
		description = *req.Description
	}

	if err := s.roleStore.Update(id, name, description, req.Permissions); err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND", "role not found", nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	role, err := s.roleStore.Get(id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	s.writeJSON(w, http.StatusOK, role)
}

// handleDeleteRole handles DELETE /api/v1/securities/roles/{id}.
func (s *Server) handleDeleteRole(w http.ResponseWriter, r *http.Request) {
	id := extractRoleID(r.URL.Path)
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "role id is required", nil)
		return
	}

	if err := s.roleStore.Delete(id); err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND", "role not found", nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// extractRoleID extracts the role ID from a path like /api/v1/securities/roles/{id}.
func extractRoleID(path string) string {
	// Trim trailing slash and return the last segment.
	path = strings.TrimSuffix(path, "/")
	idx := strings.LastIndex(path, "/")
	if idx < 0 {
		return ""
	}
	seg := path[idx+1:]
	// Reject if the last segment is the collection name itself.
	if seg == "roles" {
		return ""
	}
	return seg
}
