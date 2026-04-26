// Package server — IP restriction HTTP handlers.
package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// handleIPRestriction dispatches GET/PUT/DELETE /ip-restrictions/{participant_id}.
func (s *Server) handleIPRestriction(w http.ResponseWriter, r *http.Request) {
	if s.ipRestrictionStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "ip restriction store not configured", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGetIPRestriction(w, r)
	case http.MethodPut:
		s.handleSetIPRestriction(w, r)
	case http.MethodDelete:
		s.handleDeleteIPRestriction(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// participantIDFromIPPath extracts the participant_id path segment from
// /ip-restrictions/{participant_id}.
func participantIDFromIPPath(path string) string {
	id := strings.TrimPrefix(path, "/ip-restrictions/")
	return strings.TrimSuffix(id, "/")
}

// handleGetIPRestriction handles GET /ip-restrictions/{participant_id}.
func (s *Server) handleGetIPRestriction(w http.ResponseWriter, r *http.Request) {
	participantID := participantIDFromIPPath(r.URL.Path)
	if participantID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_ID", "participant_id is required", nil)
		return
	}
	restriction, err := s.ipRestrictionStore.Get(participantID)
	if err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "ip restriction not found", nil)
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, restriction)
}

// setIPRestrictionRequest is the request body for PUT /ip-restrictions/{participant_id}.
type setIPRestrictionRequest struct {
	AllowedIPs []string `json:"allowed_ips"`
	Enabled    bool     `json:"enabled"`
}

// handleSetIPRestriction handles PUT /ip-restrictions/{participant_id}.
func (s *Server) handleSetIPRestriction(w http.ResponseWriter, r *http.Request) {
	participantID := participantIDFromIPPath(r.URL.Path)
	if participantID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_ID", "participant_id is required", nil)
		return
	}
	var req setIPRestrictionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}
	if req.AllowedIPs == nil {
		req.AllowedIPs = []string{}
	}
	restriction := types.IPRestriction{
		ParticipantID: participantID,
		AllowedIPs:    req.AllowedIPs,
		Enabled:       req.Enabled,
	}
	if err := s.ipRestrictionStore.Set(&restriction); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, restriction)
}

// handleDeleteIPRestriction handles DELETE /ip-restrictions/{participant_id}.
func (s *Server) handleDeleteIPRestriction(w http.ResponseWriter, r *http.Request) {
	participantID := participantIDFromIPPath(r.URL.Path)
	if participantID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_ID", "participant_id is required", nil)
		return
	}
	if err := s.ipRestrictionStore.Delete(participantID); err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "ip restriction not found", nil)
		return
	} else if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
