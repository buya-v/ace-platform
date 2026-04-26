// Package server — HTTP handlers for surveillance investigation endpoints.
package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// handleInvestigations dispatches GET /api/v1/securities/investigations (list)
// and POST /api/v1/securities/investigations (create).
func (s *Server) handleInvestigations(w http.ResponseWriter, r *http.Request) {
	if s.investigationStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "investigation store not configured", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleListInvestigations(w, r)
	case http.MethodPost:
		s.handleCreateInvestigation(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleInvestigation dispatches GET/POST/PUT for /api/v1/securities/investigations/{id}
// and sub-resources /close and /evidence.
func (s *Server) handleInvestigation(w http.ResponseWriter, r *http.Request) {
	if s.investigationStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "investigation store not configured", nil)
		return
	}
	path := strings.TrimSuffix(r.URL.Path, "/")
	if strings.HasSuffix(path, "/close") {
		if r.Method == http.MethodPost {
			s.handleCloseInvestigation(w, r)
		} else {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		}
		return
	}
	if strings.HasSuffix(path, "/evidence") {
		if r.Method == http.MethodPost {
			s.handleAddEvidence(w, r)
		} else {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		}
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGetInvestigation(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

func (s *Server) handleListInvestigations(w http.ResponseWriter, r *http.Request) {
	filters := store.InvestigationFilters{}
	if st := r.URL.Query().Get("status"); st != "" {
		filters.Status = types.InvestigationStatus(st)
	}
	invs, err := s.investigationStore.List(filters)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  invs,
		"total": len(invs),
	})
}

func (s *Server) handleCreateInvestigation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID           string `json:"id"`
		AlertID      string `json:"alert_id"`
		Subject      string `json:"subject"`
		InstrumentID string `json:"instrument_id"`
		AssignedTo   string `json:"assigned_to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid JSON body", nil)
		return
	}
	if req.ID == "" || req.Subject == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELDS", "id and subject are required", nil)
		return
	}
	inv := &types.Investigation{
		ID:           req.ID,
		AlertID:      req.AlertID,
		Subject:      req.Subject,
		InstrumentID: req.InstrumentID,
		Status:       types.InvestigationOpen,
		AssignedTo:   req.AssignedTo,
		Evidence:     []string{},
		OpenedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	if err := s.investigationStore.Create(inv); err != nil {
		s.writeError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
		return
	}
	// If an alert_id was provided, transition that alert to INVESTIGATING.
	if req.AlertID != "" && s.surveillanceStore != nil {
		_ = s.surveillanceStore.UpdateAlertStatus(req.AlertID, types.AlertStatusInvestigating)
	}
	s.writeJSON(w, http.StatusCreated, inv)
}

func (s *Server) handleGetInvestigation(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/securities/investigations/")
	id = strings.TrimSuffix(id, "/")
	inv, err := s.investigationStore.Get(id)
	if err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "investigation not found", nil)
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, inv)
}

func (s *Server) handleCloseInvestigation(w http.ResponseWriter, r *http.Request) {
	// Extract ID: path is /api/v1/securities/investigations/{id}/close
	path := strings.TrimSuffix(r.URL.Path, "/")
	path = strings.TrimSuffix(path, "/close")
	id := strings.TrimPrefix(path, "/api/v1/securities/investigations/")

	var req struct {
		Findings string `json:"findings"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid JSON body", nil)
		return
	}

	if err := s.investigationStore.Close(id, req.Findings); err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "investigation not found", nil)
		return
	} else if err != nil {
		s.writeError(w, http.StatusBadRequest, "CLOSE_FAILED", err.Error(), nil)
		return
	}

	inv, _ := s.investigationStore.Get(id)
	s.writeJSON(w, http.StatusOK, inv)
}

func (s *Server) handleAddEvidence(w http.ResponseWriter, r *http.Request) {
	// Extract ID: path is /api/v1/securities/investigations/{id}/evidence
	path := strings.TrimSuffix(r.URL.Path, "/")
	path = strings.TrimSuffix(path, "/evidence")
	id := strings.TrimPrefix(path, "/api/v1/securities/investigations/")

	var req struct {
		Evidence string `json:"evidence"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid JSON body", nil)
		return
	}
	if req.Evidence == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELDS", "evidence is required", nil)
		return
	}

	if err := s.investigationStore.AddEvidence(id, req.Evidence); err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "investigation not found", nil)
		return
	} else if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	inv, _ := s.investigationStore.Get(id)
	s.writeJSON(w, http.StatusOK, inv)
}
