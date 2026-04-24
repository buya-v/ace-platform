// Package server — tick table HTTP handlers.
package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// handleTickTables dispatches GET /api/v1/securities/tick-tables (list all tick tables).
func (s *Server) handleTickTables(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListTickTables(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleListTickTables handles GET /api/v1/securities/tick-tables.
func (s *Server) handleListTickTables(w http.ResponseWriter, r *http.Request) {
	tables, err := s.tickTableStore.List()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	if tables == nil {
		tables = []types.TickTable{}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  tables,
		"total": len(tables),
	})
}

// handleTickTable dispatches GET/PUT/DELETE /api/v1/securities/tick-tables/{instrument_id}.
func (s *Server) handleTickTable(w http.ResponseWriter, r *http.Request) {
	instrumentID := extractLastSegment(r.URL.Path)
	if instrumentID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "instrument_id is required", nil)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetTickTable(w, r, instrumentID)
	case http.MethodPut:
		s.handleSetTickTable(w, r, instrumentID)
	case http.MethodDelete:
		s.handleDeleteTickTable(w, r, instrumentID)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleGetTickTable handles GET /api/v1/securities/tick-tables/{instrument_id}.
func (s *Server) handleGetTickTable(w http.ResponseWriter, r *http.Request, instrumentID string) {
	table, err := s.tickTableStore.Get(instrumentID)
	if err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND",
				fmt.Sprintf("tick table for instrument %s not found", instrumentID), nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, table)
}

// setTickTableRequest is the request body for PUT /tick-tables/{instrument_id}.
type setTickTableRequest struct {
	Tiers []types.TickTier `json:"tiers"`
}

// handleSetTickTable handles PUT /api/v1/securities/tick-tables/{instrument_id}.
//
// Validation:
//   - tiers array must be non-empty
//   - each tier: min_price < max_price, tick_size > 0
func (s *Server) handleSetTickTable(w http.ResponseWriter, r *http.Request, instrumentID string) {
	var req setTickTableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}

	if len(req.Tiers) == 0 {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "tiers array is required and must not be empty", nil)
		return
	}

	for i, tier := range req.Tiers {
		if tier.MinPrice >= tier.MaxPrice {
			s.writeError(w, http.StatusBadRequest, "INVALID_FIELD",
				fmt.Sprintf("tier[%d]: min_price (%.6g) must be less than max_price (%.6g)", i, tier.MinPrice, tier.MaxPrice), nil)
			return
		}
		if tier.TickSize <= 0 {
			s.writeError(w, http.StatusBadRequest, "INVALID_FIELD",
				fmt.Sprintf("tier[%d]: tick_size must be greater than 0", i), nil)
			return
		}
	}

	table := &types.TickTable{
		InstrumentID: instrumentID,
		Tiers:        req.Tiers,
	}

	if err := s.tickTableStore.Set(table); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	s.writeJSON(w, http.StatusOK, table)
}

// handleDeleteTickTable handles DELETE /api/v1/securities/tick-tables/{instrument_id}.
func (s *Server) handleDeleteTickTable(w http.ResponseWriter, r *http.Request, instrumentID string) {
	// Verify existence before deleting.
	if _, err := s.tickTableStore.Get(instrumentID); err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND",
				fmt.Sprintf("tick table for instrument %s not found", instrumentID), nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	if err := s.tickTableStore.Delete(instrumentID); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
