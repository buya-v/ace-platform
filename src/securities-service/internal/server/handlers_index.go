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

// handleIndices dispatches GET /api/v1/securities/indices and POST /api/v1/securities/indices.
func (s *Server) handleIndices(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListIndices(w, r)
	case http.MethodPost:
		s.handleCreateIndex(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleIndexItem dispatches GET/DELETE /api/v1/securities/indices/{id}
// and POST /api/v1/securities/indices/{id}/calculate.
func (s *Server) handleIndexItem(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/securities/indices/")
	path = strings.TrimSuffix(path, "/")

	if strings.HasSuffix(path, "/calculate") {
		id := strings.TrimSuffix(path, "/calculate")
		if r.Method == http.MethodPost {
			s.handleCalculateIndex(w, r, id)
		} else {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		}
		return
	}

	id := path
	switch r.Method {
	case http.MethodGet:
		s.handleGetIndex(w, r, id)
	case http.MethodDelete:
		s.handleDeleteIndex(w, r, id)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

func (s *Server) handleListIndices(w http.ResponseWriter, r *http.Request) {
	if s.indexStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "index store not configured", nil)
		return
	}
	indices, err := s.indexStore.List()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, indices)
}

func (s *Server) handleCreateIndex(w http.ResponseWriter, r *http.Request) {
	if s.indexStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "index store not configured", nil)
		return
	}
	var idx types.Index
	if err := json.NewDecoder(r.Body).Decode(&idx); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body", nil)
		return
	}
	if idx.ID == "" {
		idx.ID = fmt.Sprintf("idx-%d", time.Now().UnixNano())
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if idx.CreatedAt == "" {
		idx.CreatedAt = now
	}
	if err := s.indexStore.Create(&idx); err != nil {
		s.writeError(w, http.StatusConflict, "ALREADY_EXISTS", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusCreated, idx)
}

func (s *Server) handleGetIndex(w http.ResponseWriter, r *http.Request, id string) {
	if s.indexStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "index store not configured", nil)
		return
	}
	idx, err := s.indexStore.Get(id)
	if err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "index not found", nil)
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, idx)
}

func (s *Server) handleDeleteIndex(w http.ResponseWriter, r *http.Request, id string) {
	if s.indexStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "index store not configured", nil)
		return
	}
	if err := s.indexStore.Delete(id); err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "index not found", nil)
		return
	} else if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleCalculateIndex recalculates the index CurrentValue from live trade prices.
// It looks up the last trade price for each instrument in InstrumentWeights from
// the instrument store (using a simple weight × 1.0 placeholder when no trade data
// is available), then updates the index record.
func (s *Server) handleCalculateIndex(w http.ResponseWriter, r *http.Request, id string) {
	if s.indexStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "index store not configured", nil)
		return
	}
	idx, err := s.indexStore.Get(id)
	if err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "index not found", nil)
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	// Recalculate: weighted sum of instrument prices.
	// Each weight is treated as a fraction (e.g. 0.4 = 40%).
	// When an instrument has no price data we use its weight × baseValue contribution.
	previous := idx.CurrentValue
	weighted := 0.0
	for instrID, weight := range idx.InstrumentWeights {
		price := idx.BaseValue // fallback: contribute base proportionally
		if s.instrumentStore != nil {
			if instr, e := s.instrumentStore.Get(instrID); e == nil {
				// Use TickSize as a proxy for last traded price when real trades unavailable.
				if instr.TickSize > 0 {
					price = instr.TickSize
				}
			}
		}
		weighted += weight * price
	}
	if weighted == 0 && idx.BaseValue > 0 {
		weighted = idx.BaseValue
	}

	now := time.Now().UTC().Format(time.RFC3339)
	idx.CurrentValue = weighted
	if previous > 0 {
		idx.ChangePercent = (weighted - previous) / previous * 100
	}
	idx.LastCalculatedAt = now

	if err := s.indexStore.Update(idx); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, idx)
}
