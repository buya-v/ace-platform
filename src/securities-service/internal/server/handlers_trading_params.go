// Package server — trading parameter set HTTP handlers.
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

// handleTradingParams dispatches:
//
//	POST /api/v1/securities/trading-params   → create
//	GET  /api/v1/securities/trading-params   → list
func (s *Server) handleTradingParams(w http.ResponseWriter, r *http.Request) {
	if s.tradingParamSetStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED",
			"trading parameter set store is not configured", nil)
		return
	}
	switch r.Method {
	case http.MethodPost:
		s.handleCreateTradingParamSet(w, r)
	case http.MethodGet:
		s.handleListTradingParamSets(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleTradingParamItem dispatches:
//
//	GET    /api/v1/securities/trading-params/{id}   → get by ID
//	PUT    /api/v1/securities/trading-params/{id}   → update
//	DELETE /api/v1/securities/trading-params/{id}   → delete
func (s *Server) handleTradingParamItem(w http.ResponseWriter, r *http.Request) {
	if s.tradingParamSetStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED",
			"trading parameter set store is not configured", nil)
		return
	}
	// Reject the instrument sub-route handled by handleTradingParamByInstrument.
	if strings.Contains(r.URL.Path, "/instrument/") {
		s.handleTradingParamByInstrument(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGetTradingParamSet(w, r)
	case http.MethodPut:
		s.handleUpdateTradingParamSet(w, r)
	case http.MethodDelete:
		s.handleDeleteTradingParamSet(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleTradingParamByInstrument handles:
//
//	GET /api/v1/securities/trading-params/instrument/{instrument_id}
func (s *Server) handleTradingParamByInstrument(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}
	if s.tradingParamSetStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED",
			"trading parameter set store is not configured", nil)
		return
	}
	instrumentID := extractLastSegment(r.URL.Path)
	if instrumentID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "instrument_id is required", nil)
		return
	}
	ps, err := s.tradingParamSetStore.GetByInstrument(instrumentID)
	if err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND",
				fmt.Sprintf("no trading parameter set found for instrument %s", instrumentID), nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, ps)
}

// handleCreateTradingParamSet handles POST /api/v1/securities/trading-params.
func (s *Server) handleCreateTradingParamSet(w http.ResponseWriter, r *http.Request) {
	var ps types.TradingParameterSet
	if err := json.NewDecoder(r.Body).Decode(&ps); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}
	if ps.InstrumentID == "" {
		s.writeError(w, http.StatusUnprocessableEntity, "MISSING_FIELD", "instrument_id is required", nil)
		return
	}
	id, err := newUUID()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate id", nil)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	ps.ID = id
	ps.CreatedAt = now
	ps.UpdatedAt = now

	if err := s.tradingParamSetStore.Create(&ps); err != nil {
		s.writeError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusCreated, ps)
}

// handleListTradingParamSets handles GET /api/v1/securities/trading-params.
func (s *Server) handleListTradingParamSets(w http.ResponseWriter, r *http.Request) {
	all, err := s.tradingParamSetStore.List()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	if all == nil {
		all = []types.TradingParameterSet{}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  all,
		"total": len(all),
	})
}

// handleGetTradingParamSet handles GET /api/v1/securities/trading-params/{id}.
func (s *Server) handleGetTradingParamSet(w http.ResponseWriter, r *http.Request) {
	id := extractLastSegment(r.URL.Path)
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "trading parameter set id is required", nil)
		return
	}
	ps, err := s.tradingParamSetStore.Get(id)
	if err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND",
				fmt.Sprintf("trading parameter set %s not found", id), nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, ps)
}

// handleUpdateTradingParamSet handles PUT /api/v1/securities/trading-params/{id}.
func (s *Server) handleUpdateTradingParamSet(w http.ResponseWriter, r *http.Request) {
	id := extractLastSegment(r.URL.Path)
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "trading parameter set id is required", nil)
		return
	}
	var ps types.TradingParameterSet
	if err := json.NewDecoder(r.Body).Decode(&ps); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}
	ps.ID = id
	ps.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	// Preserve CreatedAt from the stored record.
	existing, err := s.tradingParamSetStore.Get(id)
	if err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND",
				fmt.Sprintf("trading parameter set %s not found", id), nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	ps.CreatedAt = existing.CreatedAt

	if err := s.tradingParamSetStore.Update(&ps); err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND",
				fmt.Sprintf("trading parameter set %s not found", id), nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, ps)
}

// handleDeleteTradingParamSet handles DELETE /api/v1/securities/trading-params/{id}.
func (s *Server) handleDeleteTradingParamSet(w http.ResponseWriter, r *http.Request) {
	id := extractLastSegment(r.URL.Path)
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "trading parameter set id is required", nil)
		return
	}
	if err := s.tradingParamSetStore.Delete(id); err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND",
				fmt.Sprintf("trading parameter set %s not found", id), nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
