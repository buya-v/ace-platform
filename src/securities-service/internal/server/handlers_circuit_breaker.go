// Package server — circuit breaker configuration HTTP handlers.
package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/garudax-platform/securities-service/internal/types"
)

// handleCircuitBreakers dispatches GET /api/v1/securities/circuit-breakers (list all).
func (s *Server) handleCircuitBreakers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListCircuitBreakers(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleCircuitBreaker dispatches GET, PUT, DELETE for
// /api/v1/securities/circuit-breakers/{instrument_id}.
func (s *Server) handleCircuitBreaker(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetCircuitBreaker(w, r)
	case http.MethodPut:
		s.handleSetCircuitBreaker(w, r)
	case http.MethodDelete:
		s.handleDeleteCircuitBreaker(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleListCircuitBreakers handles GET /api/v1/securities/circuit-breakers.
// Returns all configured circuit breaker configs.
func (s *Server) handleListCircuitBreakers(w http.ResponseWriter, r *http.Request) {
	cbs, err := s.circuitBreakerStore.List()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	if cbs == nil {
		cbs = []types.CircuitBreaker{}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  cbs,
		"total": len(cbs),
	})
}

// handleGetCircuitBreaker handles GET /api/v1/securities/circuit-breakers/{instrument_id}.
// Returns 404 if no circuit breaker is configured for the instrument.
func (s *Server) handleGetCircuitBreaker(w http.ResponseWriter, r *http.Request) {
	instrumentID := extractLastSegment(r.URL.Path)
	if instrumentID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "instrument_id is required", nil)
		return
	}

	cb, err := s.circuitBreakerStore.Get(instrumentID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	if cb == nil {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND",
			fmt.Sprintf("no circuit breaker configured for instrument %s", instrumentID), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, cb)
}

// setCircuitBreakerRequest is the request body for PUT .../circuit-breakers/{instrument_id}.
type setCircuitBreakerRequest struct {
	ReferencePrice  float64 `json:"reference_price"`
	StaticUpperPct  float64 `json:"static_upper_pct"`
	StaticLowerPct  float64 `json:"static_lower_pct"`
	DynamicUpperPct float64 `json:"dynamic_upper_pct"`
	DynamicLowerPct float64 `json:"dynamic_lower_pct"`
	CooldownMinutes int     `json:"cooldown_minutes"`
}

// handleSetCircuitBreaker handles PUT /api/v1/securities/circuit-breakers/{instrument_id}.
//
// Validation:
//   - instrument_id is required (from path)
//   - reference_price must be > 0
//   - at least one percentage limit (static_upper_pct, static_lower_pct,
//     dynamic_upper_pct, or dynamic_lower_pct) must be > 0
func (s *Server) handleSetCircuitBreaker(w http.ResponseWriter, r *http.Request) {
	instrumentID := extractLastSegment(r.URL.Path)
	if instrumentID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "instrument_id is required", nil)
		return
	}

	var req setCircuitBreakerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}

	if req.ReferencePrice <= 0 {
		s.writeError(w, http.StatusBadRequest, "INVALID_FIELD",
			"reference_price must be greater than 0", nil)
		return
	}

	if req.StaticUpperPct <= 0 && req.StaticLowerPct <= 0 &&
		req.DynamicUpperPct <= 0 && req.DynamicLowerPct <= 0 {
		s.writeError(w, http.StatusBadRequest, "INVALID_FIELD",
			"at least one percentage limit (static_upper_pct, static_lower_pct, dynamic_upper_pct, dynamic_lower_pct) must be greater than 0",
			nil)
		return
	}

	cb := &types.CircuitBreaker{
		InstrumentID:    instrumentID,
		ReferencePrice:  req.ReferencePrice,
		StaticUpperPct:  req.StaticUpperPct,
		StaticLowerPct:  req.StaticLowerPct,
		DynamicUpperPct: req.DynamicUpperPct,
		DynamicLowerPct: req.DynamicLowerPct,
		CooldownMinutes: req.CooldownMinutes,
		Status:          types.CBActive,
	}

	if err := s.circuitBreakerStore.Set(cb); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	s.writeJSON(w, http.StatusOK, cb)
}

// handleDeleteCircuitBreaker handles DELETE /api/v1/securities/circuit-breakers/{instrument_id}.
// Removes the circuit breaker config. Returns 204 No Content on success.
func (s *Server) handleDeleteCircuitBreaker(w http.ResponseWriter, r *http.Request) {
	instrumentID := extractLastSegment(r.URL.Path)
	if instrumentID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "instrument_id is required", nil)
		return
	}

	// Verify the circuit breaker exists before deleting.
	cb, err := s.circuitBreakerStore.Get(instrumentID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	if cb == nil {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND",
			fmt.Sprintf("no circuit breaker configured for instrument %s", instrumentID), nil)
		return
	}

	if err := s.circuitBreakerStore.Delete(instrumentID); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
