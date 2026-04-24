// Package server — HTTP handlers for trading strategy endpoints.
package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// handleStrategies dispatches GET /api/v1/securities/strategies (list)
// and POST /api/v1/securities/strategies (create).
func (s *Server) handleStrategies(w http.ResponseWriter, r *http.Request) {
	if s.strategyStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "strategy store not configured", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleListStrategies(w, r)
	case http.MethodPost:
		s.handleCreateStrategy(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleStrategy dispatches GET/DELETE /api/v1/securities/strategies/{id}.
func (s *Server) handleStrategy(w http.ResponseWriter, r *http.Request) {
	if s.strategyStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "strategy store not configured", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGetStrategy(w, r)
	case http.MethodDelete:
		s.handleDeleteStrategy(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

func (s *Server) handleListStrategies(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	strategies, err := s.strategyStore.List(tenantID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  strategies,
		"total": len(strategies),
	})
}

func (s *Server) handleCreateStrategy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID           string               `json:"id"`
		Name         string               `json:"name"`
		StrategyType types.StrategyType   `json:"strategy_type"`
		Legs         []types.StrategyLeg  `json:"legs"`
		TenantID     string               `json:"tenant_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid JSON body", nil)
		return
	}
	if req.Name == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELDS", "name is required", nil)
		return
	}
	if len(req.Legs) < 2 {
		s.writeError(w, http.StatusBadRequest, "TOO_FEW_LEGS", "strategy must have at least 2 legs", nil)
		return
	}
	for i, leg := range req.Legs {
		if leg.InstrumentID == "" {
			s.writeError(w, http.StatusBadRequest, "INVALID_LEG", "each leg must have an instrument_id", []string{
				strings.Join([]string{"leg", string(rune('0'+i)), ": missing instrument_id"}, " "),
			})
			return
		}
		if leg.RatioQty <= 0 {
			s.writeError(w, http.StatusBadRequest, "INVALID_LEG", "each leg must have a positive ratio_qty", nil)
			return
		}
	}

	// Generate ID if not provided.
	id := req.ID
	if id == "" {
		id = "strat-" + time.Now().UTC().Format("20060102150405.000000000")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	strategy := &types.TradingStrategy{
		ID:           id,
		Name:         req.Name,
		StrategyType: req.StrategyType,
		Legs:         req.Legs,
		Status:       types.StrategyStatusActive,
		TenantID:     req.TenantID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if strategy.StrategyType == "" {
		strategy.StrategyType = types.StrategyTypeCustom
	}

	if err := s.strategyStore.Create(strategy); err != nil {
		s.writeError(w, http.StatusConflict, "ALREADY_EXISTS", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusCreated, strategy)
}

func (s *Server) handleGetStrategy(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/securities/strategies/")
	id = strings.TrimSuffix(id, "/")
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_ID", "strategy id is required", nil)
		return
	}
	strategy, err := s.strategyStore.Get(id)
	if err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "strategy not found", nil)
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, strategy)
}

func (s *Server) handleDeleteStrategy(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/securities/strategies/")
	id = strings.TrimSuffix(id, "/")
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_ID", "strategy id is required", nil)
		return
	}
	err := s.strategyStore.Delete(id)
	if err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "strategy not found", nil)
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
