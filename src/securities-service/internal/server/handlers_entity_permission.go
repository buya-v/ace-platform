package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// handleEntityPermissions dispatches:
//
//	GET  /api/v1/securities/entity-permissions?role_id=...
//	PUT  /api/v1/securities/entity-permissions
func (s *Server) handleEntityPermissions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListEntityPermissions(w, r)
	case http.MethodPut:
		s.handleSetEntityPermission(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleEntityPermissionItem dispatches:
//
//	DELETE /api/v1/securities/entity-permissions/{role_id}/{entity_type}
func (s *Server) handleEntityPermissionItem(w http.ResponseWriter, r *http.Request) {
	// path suffix after /api/v1/securities/entity-permissions/
	suffix := strings.TrimPrefix(r.URL.Path, "/api/v1/securities/entity-permissions/")
	suffix = strings.TrimSuffix(suffix, "/")
	parts := strings.SplitN(suffix, "/", 2)
	if len(parts) != 2 {
		s.writeError(w, http.StatusBadRequest, "INVALID_PATH", "expected /{role_id}/{entity_type}", nil)
		return
	}
	roleID, entityType := parts[0], parts[1]

	if r.Method == http.MethodDelete {
		s.handleDeleteEntityPermission(w, r, roleID, entityType)
	} else {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

func (s *Server) handleListEntityPermissions(w http.ResponseWriter, r *http.Request) {
	if s.entityPermissionStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "entity permission store not configured", nil)
		return
	}
	roleID := r.URL.Query().Get("role_id")
	if roleID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_PARAM", "role_id query parameter required", nil)
		return
	}
	perms, err := s.entityPermissionStore.ListByRole(roleID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, perms)
}

func (s *Server) handleSetEntityPermission(w http.ResponseWriter, r *http.Request) {
	if s.entityPermissionStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "entity permission store not configured", nil)
		return
	}
	var ep types.EntityPermission
	if err := json.NewDecoder(r.Body).Decode(&ep); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body", nil)
		return
	}
	if ep.RoleID == "" || ep.EntityType == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "role_id and entity_type are required", nil)
		return
	}
	if ep.ID == "" {
		ep.ID = fmt.Sprintf("ep-%d", time.Now().UnixNano())
	}
	if err := s.entityPermissionStore.Set(&ep); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, ep)
}

func (s *Server) handleDeleteEntityPermission(w http.ResponseWriter, r *http.Request, roleID, entityType string) {
	if s.entityPermissionStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "entity permission store not configured", nil)
		return
	}
	if err := s.entityPermissionStore.Delete(roleID, entityType); err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "entity permission not found", nil)
		return
	} else if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
