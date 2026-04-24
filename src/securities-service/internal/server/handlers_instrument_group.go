// Package server — instrument group HTTP handlers.
package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/garudax-platform/securities-service/internal/types"
)

// handleInstrumentGroups dispatches GET and POST for the groups collection.
func (s *Server) handleInstrumentGroups(w http.ResponseWriter, r *http.Request) {
	if s.instrumentGroupStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "instrument group store not configured", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleListGroups(w, r)
	case http.MethodPost:
		s.handleCreateGroup(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleInstrumentGroup dispatches GET/DELETE for a specific group or manages its instruments.
func (s *Server) handleInstrumentGroup(w http.ResponseWriter, r *http.Request) {
	if s.instrumentGroupStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "instrument group store not configured", nil)
		return
	}
	// Check for /instruments sub-resource: .../groups/{id}/instruments or .../groups/{id}/instruments/{instrID}
	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	// parts: [..., "groups", groupID, "instruments", instrID?]
	instrIdx := -1
	for i, p := range parts {
		if p == "instruments" {
			instrIdx = i
			break
		}
	}
	if instrIdx >= 0 {
		groupID := parts[instrIdx-1]
		if instrIdx+1 < len(parts) {
			// .../instruments/{instrID}
			instrID := parts[instrIdx+1]
			switch r.Method {
			case http.MethodDelete:
				s.handleRemoveInstrumentFromGroup(w, r, groupID, instrID)
			default:
				s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
			}
		} else {
			// .../instruments
			switch r.Method {
			case http.MethodPost:
				s.handleAddInstrumentToGroup(w, r, groupID)
			default:
				s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
			}
		}
		return
	}

	// Plain group ID dispatch
	groupID := parts[len(parts)-1]
	switch r.Method {
	case http.MethodGet:
		s.handleGetGroup(w, r, groupID)
	case http.MethodDelete:
		s.handleDeleteGroup(w, r, groupID)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleListGroups handles GET /api/v1/securities/instrument-groups.
func (s *Server) handleListGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.instrumentGroupStore.List()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	if groups == nil {
		groups = []types.InstrumentGroup{}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  groups,
		"total": len(groups),
	})
}

// createGroupRequest is the request body for POST /api/v1/securities/instrument-groups.
type createGroupRequest struct {
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	GroupType     types.GroupType `json:"group_type"`
	InstrumentIDs []string        `json:"instrument_ids"`
}

// handleCreateGroup handles POST /api/v1/securities/instrument-groups.
func (s *Server) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	var req createGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}
	if req.Name == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "name is required", nil)
		return
	}
	if req.GroupType == "" {
		req.GroupType = types.GroupTypeManual
	}

	id, err := newUUID()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate id", nil)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if req.InstrumentIDs == nil {
		req.InstrumentIDs = []string{}
	}
	group := &types.InstrumentGroup{
		ID:            id,
		Name:          req.Name,
		Description:   req.Description,
		GroupType:     req.GroupType,
		InstrumentIDs: req.InstrumentIDs,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.instrumentGroupStore.Create(group); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusCreated, group)
}

// handleGetGroup handles GET /api/v1/securities/instrument-groups/{id}.
func (s *Server) handleGetGroup(w http.ResponseWriter, r *http.Request, groupID string) {
	group, err := s.instrumentGroupStore.Get(groupID)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "instrument group not found", nil)
		return
	}
	s.writeJSON(w, http.StatusOK, group)
}

// handleDeleteGroup handles DELETE /api/v1/securities/instrument-groups/{id}.
func (s *Server) handleDeleteGroup(w http.ResponseWriter, r *http.Request, groupID string) {
	if err := s.instrumentGroupStore.Delete(groupID); err != nil {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "instrument group not found", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// addInstrumentRequest is the request body for POST .../groups/{id}/instruments.
type addInstrumentRequest struct {
	InstrumentID string `json:"instrument_id"`
}

// handleAddInstrumentToGroup handles POST /api/v1/securities/instrument-groups/{id}/instruments.
func (s *Server) handleAddInstrumentToGroup(w http.ResponseWriter, r *http.Request, groupID string) {
	var req addInstrumentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}
	if req.InstrumentID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "instrument_id is required", nil)
		return
	}
	if err := s.instrumentGroupStore.AddInstrument(groupID, req.InstrumentID); err != nil {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "instrument group not found", nil)
		return
	}
	group, _ := s.instrumentGroupStore.Get(groupID)
	s.writeJSON(w, http.StatusOK, group)
}

// handleRemoveInstrumentFromGroup handles DELETE .../groups/{id}/instruments/{instrID}.
func (s *Server) handleRemoveInstrumentFromGroup(w http.ResponseWriter, r *http.Request, groupID, instrID string) {
	if err := s.instrumentGroupStore.RemoveInstrument(groupID, instrID); err != nil {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "instrument group not found", nil)
		return
	}
	group, _ := s.instrumentGroupStore.Get(groupID)
	s.writeJSON(w, http.StatusOK, group)
}
