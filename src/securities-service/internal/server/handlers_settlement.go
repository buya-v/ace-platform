package server

import (
	"encoding/json"
	"net/http"

	"github.com/garudax-platform/securities-service/internal/types"
)

// handleSettlements dispatches GET /api/v1/securities/settlements.
func (s *Server) handleSettlements(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}
	s.handleListSettlements(w, r)
}

// handleListSettlements returns settlement obligations filtered by date and/or status.
func (s *Server) handleListSettlements(w http.ResponseWriter, r *http.Request) {
	if s.settlementEngine == nil {
		s.writeError(w, http.StatusServiceUnavailable, "SETTLEMENT_UNAVAILABLE", "settlement engine not configured", nil)
		return
	}

	date := r.URL.Query().Get("date")
	status := r.URL.Query().Get("status")

	if date == "" && status == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FILTER", "at least one of 'date' or 'status' query parameters is required", nil)
		return
	}

	if date != "" {
		obligations, err := s.settlementStore.ListByDate(date)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
			return
		}
		s.writeJSON(w, http.StatusOK, obligations)
		return
	}

	// Filter by status.
	obligations, err := s.settlementStore.ListByStatus(types.SettlementStatus(status))
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, obligations)
}

// settlementCycleRequest is the JSON body for triggering a settlement cycle.
type settlementCycleRequest struct {
	Date string `json:"date"`
}

// handleSettlementCycle dispatches POST /api/v1/securities/settlements/cycle.
func (s *Server) handleSettlementCycle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}
	if err := s.checkPermission(r, types.PermSettlementTrigger); err != nil {
		s.writeError(w, http.StatusForbidden, "PERMISSION_DENIED", err.Error(), nil)
		return
	}

	if s.settlementEngine == nil {
		s.writeError(w, http.StatusServiceUnavailable, "SETTLEMENT_UNAVAILABLE", "settlement engine not configured", nil)
		return
	}

	var req settlementCycleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}

	if req.Date == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_DATE", "date field is required", nil)
		return
	}

	result, err := s.settlementEngine.ProcessSettlementCycle(req.Date)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "SETTLEMENT_ERROR", err.Error(), nil)
		return
	}

	s.writeJSON(w, http.StatusOK, result)
}
