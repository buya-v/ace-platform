// Package server — trading cycle CRUD HTTP handlers.
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

// handleTradingCycles dispatches GET (list) and POST (create) for /api/v1/securities/trading-cycles.
func (s *Server) handleTradingCycles(w http.ResponseWriter, r *http.Request) {
	if s.tradingCycleStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "trading cycle store not configured", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleListTradingCycles(w, r)
	case http.MethodPost:
		s.handleCreateTradingCycle(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleTradingCycle dispatches GET (get by id) and DELETE for /api/v1/securities/trading-cycles/{id}.
func (s *Server) handleTradingCycle(w http.ResponseWriter, r *http.Request) {
	if s.tradingCycleStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "trading cycle store not configured", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGetTradingCycle(w, r)
	case http.MethodDelete:
		s.handleDeleteTradingCycle(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleListTradingCycles handles GET /api/v1/securities/trading-cycles.
//
// Optional query param: market_id — filter by market.
func (s *Server) handleListTradingCycles(w http.ResponseWriter, r *http.Request) {
	marketID := r.URL.Query().Get("market_id")
	cycles, err := s.tradingCycleStore.ListByMarket(marketID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	if cycles == nil {
		cycles = []types.TradingCycle{}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  cycles,
		"total": len(cycles),
	})
}

// createTradingCycleRequest is the request body for POST /api/v1/securities/trading-cycles.
type createTradingCycleRequest struct {
	ID              string   `json:"id"`
	MarketID        string   `json:"market_id"`
	Name            string   `json:"name"`
	SessionSequence []string `json:"session_sequence"`
	IsDefault       bool     `json:"is_default"`
}

// handleCreateTradingCycle handles POST /api/v1/securities/trading-cycles.
//
// Validation: id, market_id, name, and session_sequence (non-empty) are required.
func (s *Server) handleCreateTradingCycle(w http.ResponseWriter, r *http.Request) {
	var req createTradingCycleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}
	if req.ID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "id is required", nil)
		return
	}
	if req.MarketID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "market_id is required", nil)
		return
	}
	if req.Name == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "name is required", nil)
		return
	}
	if len(req.SessionSequence) == 0 {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "session_sequence must contain at least one phase", nil)
		return
	}

	cycle := &types.TradingCycle{
		ID:              req.ID,
		MarketID:        req.MarketID,
		Name:            req.Name,
		SessionSequence: req.SessionSequence,
		IsDefault:       req.IsDefault,
		CreatedAt:       time.Now().UTC().Format(time.RFC3339),
	}

	if err := s.tradingCycleStore.Create(cycle); err != nil {
		s.writeError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusCreated, cycle)
}

// handleGetTradingCycle handles GET /api/v1/securities/trading-cycles/{id}.
func (s *Server) handleGetTradingCycle(w http.ResponseWriter, r *http.Request) {
	id := extractTradingCycleID(r.URL.Path)
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "trading cycle id is required", nil)
		return
	}
	cycle, err := s.tradingCycleStore.Get(id)
	if err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND",
				fmt.Sprintf("trading cycle %s not found", id), nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, cycle)
}

// handleDeleteTradingCycle handles DELETE /api/v1/securities/trading-cycles/{id}.
func (s *Server) handleDeleteTradingCycle(w http.ResponseWriter, r *http.Request) {
	id := extractTradingCycleID(r.URL.Path)
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "trading cycle id is required", nil)
		return
	}
	if err := s.tradingCycleStore.Delete(id); err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND",
				fmt.Sprintf("trading cycle %s not found", id), nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// extractTradingCycleID extracts the last path segment from a URL path of the form
// /api/v1/securities/trading-cycles/{id}.
func extractTradingCycleID(path string) string {
	path = strings.TrimSuffix(path, "/")
	segs := strings.Split(path, "/")
	if len(segs) == 0 {
		return ""
	}
	id := segs[len(segs)-1]
	if id == "trading-cycles" {
		return ""
	}
	return id
}
