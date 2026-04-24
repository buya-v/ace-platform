// Package server — trade give-up HTTP handlers (P4a).
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// handleGiveUpForTrade handles POST /api/v1/securities/trades/{id}/give-up.
// It extracts the trade ID from the path (penultimate segment before "give-up").
func (s *Server) handleGiveUpForTrade(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}
	if s.giveUpStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_AVAILABLE", "give-up store not configured", nil)
		return
	}

	tradeIDStr := extractPenultimateSegment(r.URL.Path)
	if tradeIDStr == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "trade id is required", nil)
		return
	}
	tradeID, err := strconv.Atoi(tradeIDStr)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_FIELD", "trade id must be an integer", nil)
		return
	}

	var body struct {
		ToFirmID int `json:"to_firm_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ToFirmID == 0 {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "to_firm_id is required", nil)
		return
	}

	req := &types.GiveUpRequest{
		TradeID:  tradeID,
		ToFirmID: body.ToFirmID,
	}
	if err := s.giveUpStore.Create(req); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusCreated, req)
}

// handleGiveUps dispatches GET /api/v1/securities/give-ups.
func (s *Server) handleGiveUps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}
	if s.giveUpStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_AVAILABLE", "give-up store not configured", nil)
		return
	}

	firmID := r.URL.Query().Get("firm_id")
	giveUps, err := s.giveUpStore.List(firmID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	if giveUps == nil {
		giveUps = []types.GiveUpRequest{}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"data": giveUps, "total": len(giveUps)})
}

// handleGiveUpAction dispatches sub-resource actions on /api/v1/securities/give-ups/{id}/*.
func (s *Server) handleGiveUpAction(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case hasSuffix(path, "/accept"):
		if r.Method != http.MethodPost {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
			return
		}
		s.handleAcceptGiveUp(w, r)
	case hasSuffix(path, "/reject"):
		if r.Method != http.MethodPost {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
			return
		}
		s.handleRejectGiveUp(w, r)
	default:
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "endpoint not found", nil)
	}
}

// handleAcceptGiveUp handles POST /api/v1/securities/give-ups/{id}/accept.
func (s *Server) handleAcceptGiveUp(w http.ResponseWriter, r *http.Request) {
	if s.giveUpStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_AVAILABLE", "give-up store not configured", nil)
		return
	}

	id := extractPenultimateSegment(r.URL.Path)
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "give-up id is required", nil)
		return
	}

	if err := s.giveUpStore.Accept(id); err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("give-up %s not found", id), nil)
			return
		}
		s.writeError(w, http.StatusConflict, "INVALID_STATE", err.Error(), nil)
		return
	}

	req, err := s.giveUpStore.Get(id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, req)
}

// handleRejectGiveUp handles POST /api/v1/securities/give-ups/{id}/reject.
// Body: { "reason": "..." }
func (s *Server) handleRejectGiveUp(w http.ResponseWriter, r *http.Request) {
	if s.giveUpStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_AVAILABLE", "give-up store not configured", nil)
		return
	}

	id := extractPenultimateSegment(r.URL.Path)
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "give-up id is required", nil)
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	// Reason is optional.
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
	}

	if err := s.giveUpStore.Reject(id, body.Reason); err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("give-up %s not found", id), nil)
			return
		}
		s.writeError(w, http.StatusConflict, "INVALID_STATE", err.Error(), nil)
		return
	}

	req, err := s.giveUpStore.Get(id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, req)
}
