// Package server — firm and participant HTTP handlers.
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

// ── Firms ─────────────────────────────────────────────────────────────────────

// handleFirms dispatches GET /api/v1/securities/firms (list)
// and POST /api/v1/securities/firms (create).
func (s *Server) handleFirms(w http.ResponseWriter, r *http.Request) {
	if s.firmStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "firm store not configured", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleListFirms(w, r)
	case http.MethodPost:
		s.handleCreateFirm(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleFirm dispatches GET /api/v1/securities/firms/{id}
// and PUT /api/v1/securities/firms/{id}/status.
func (s *Server) handleFirm(w http.ResponseWriter, r *http.Request) {
	if s.firmStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "firm store not configured", nil)
		return
	}
	// Detect /status sub-resource.
	if strings.HasSuffix(strings.TrimSuffix(r.URL.Path, "/"), "/status") {
		if r.Method == http.MethodPut {
			s.handleUpdateFirmStatus(w, r)
		} else {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		}
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGetFirm(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleListFirms handles GET /api/v1/securities/firms.
func (s *Server) handleListFirms(w http.ResponseWriter, r *http.Request) {
	firms, err := s.firmStore.List()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	if firms == nil {
		firms = []types.Firm{}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  firms,
		"total": len(firms),
	})
}

// handleGetFirm handles GET /api/v1/securities/firms/{id}.
func (s *Server) handleGetFirm(w http.ResponseWriter, r *http.Request) {
	id := extractLastSegment(r.URL.Path)
	if id == "" || id == "firms" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "firm id is required", nil)
		return
	}
	firm, err := s.firmStore.Get(id)
	if err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND",
				fmt.Sprintf("firm %s not found", id), nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, firm)
}

// handleCreateFirm handles POST /api/v1/securities/firms.
// Required fields: id, name.
func (s *Server) handleCreateFirm(w http.ResponseWriter, r *http.Request) {
	var firm types.Firm
	if err := json.NewDecoder(r.Body).Decode(&firm); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}
	if firm.ID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "id is required", nil)
		return
	}
	if firm.Name == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "name is required", nil)
		return
	}
	if firm.Status == "" {
		firm.Status = types.FirmActive
	}
	now := time.Now().UTC().Format(time.RFC3339)
	firm.CreatedAt = now
	firm.UpdatedAt = now

	if err := s.firmStore.Create(&firm); err != nil {
		s.writeError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusCreated, firm)
}

// setFirmStatusRequest is the request body for PUT .../firms/{id}/status.
type setFirmStatusRequest struct {
	Status types.FirmStatus `json:"status"`
}

// validFirmStatuses is the set of accepted FirmStatus enum values.
var validFirmStatuses = map[types.FirmStatus]bool{
	types.FirmActive:    true,
	types.FirmSuspended: true,
	types.FirmDeactivated: true,
}

// handleUpdateFirmStatus handles PUT /api/v1/securities/firms/{id}/status.
func (s *Server) handleUpdateFirmStatus(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	segments := strings.Split(path, "/")
	if len(segments) < 2 || segments[len(segments)-1] != "status" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "firm id is required", nil)
		return
	}
	id := segments[len(segments)-2]
	if id == "" || id == "firms" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "firm id is required", nil)
		return
	}

	var req setFirmStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}
	if req.Status == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "status is required", nil)
		return
	}
	if !validFirmStatuses[req.Status] {
		s.writeError(w, http.StatusBadRequest, "INVALID_FIELD",
			fmt.Sprintf("invalid status %q: must be one of FIRM_ACTIVE, FIRM_SUSPENDED, FIRM_REVOKED", req.Status), nil)
		return
	}

	if err := s.firmStore.UpdateStatus(id, req.Status); err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND",
				fmt.Sprintf("firm %s not found", id), nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	firm, err := s.firmStore.Get(id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, firm)
}

// ── Participants ──────────────────────────────────────────────────────────────

// handleParticipants dispatches GET /api/v1/securities/participants (list, ?firm_id= filter)
// and POST /api/v1/securities/participants (create).
func (s *Server) handleParticipants(w http.ResponseWriter, r *http.Request) {
	if s.participantStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "participant store not configured", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleListParticipants(w, r)
	case http.MethodPost:
		s.handleCreateParticipant(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleParticipant dispatches GET /api/v1/securities/participants/{id}
// and PUT /api/v1/securities/participants/{id}/permissions.
func (s *Server) handleParticipant(w http.ResponseWriter, r *http.Request) {
	if s.participantStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "participant store not configured", nil)
		return
	}
	// Detect /permissions sub-resource.
	if strings.HasSuffix(strings.TrimSuffix(r.URL.Path, "/"), "/permissions") {
		if r.Method == http.MethodPut {
			s.handleUpdateParticipantPermissions(w, r)
		} else {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		}
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGetParticipant(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleListParticipants handles GET /api/v1/securities/participants.
// Supports ?firm_id= query parameter to filter by firm.
func (s *Server) handleListParticipants(w http.ResponseWriter, r *http.Request) {
	filters := store.ParticipantFilters{
		FirmID: r.URL.Query().Get("firm_id"),
	}
	participants, err := s.participantStore.List(filters)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	if participants == nil {
		participants = []types.ExchangeParticipant{}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  participants,
		"total": len(participants),
	})
}

// handleGetParticipant handles GET /api/v1/securities/participants/{id}.
func (s *Server) handleGetParticipant(w http.ResponseWriter, r *http.Request) {
	id := extractLastSegment(r.URL.Path)
	if id == "" || id == "participants" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "participant id is required", nil)
		return
	}
	p, err := s.participantStore.Get(id)
	if err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND",
				fmt.Sprintf("participant %s not found", id), nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, p)
}

// handleCreateParticipant handles POST /api/v1/securities/participants.
// Required fields: id, firm_id, name, permissions (array).
func (s *Server) handleCreateParticipant(w http.ResponseWriter, r *http.Request) {
	var p types.ExchangeParticipant
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}
	if p.ID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "id is required", nil)
		return
	}
	if p.FirmID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "firm_id is required", nil)
		return
	}
	if p.Name == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "name is required", nil)
		return
	}
	if p.Permissions == nil {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "permissions array is required", nil)
		return
	}
	if p.Status == "" {
		p.Status = types.ParticipantActive
	}
	now := time.Now().UTC().Format(time.RFC3339)
	p.CreatedAt = now
	p.UpdatedAt = now

	if err := s.participantStore.Create(&p); err != nil {
		s.writeError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusCreated, p)
}

// updatePermissionsRequest is the request body for PUT .../participants/{id}/permissions.
type updatePermissionsRequest struct {
	Permissions []string `json:"permissions"`
}

// handleUpdateParticipantPermissions handles PUT /api/v1/securities/participants/{id}/permissions.
func (s *Server) handleUpdateParticipantPermissions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	segments := strings.Split(path, "/")
	if len(segments) < 2 || segments[len(segments)-1] != "permissions" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "participant id is required", nil)
		return
	}
	id := segments[len(segments)-2]
	if id == "" || id == "participants" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "participant id is required", nil)
		return
	}

	var req updatePermissionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}
	if req.Permissions == nil {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "permissions array is required", nil)
		return
	}

	if err := s.participantStore.UpdatePermissions(id, req.Permissions); err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND",
				fmt.Sprintf("participant %s not found", id), nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	p, err := s.participantStore.Get(id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, p)
}
