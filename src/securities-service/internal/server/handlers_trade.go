// Package server — trade CRUD and correction HTTP handlers.
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

// handleTrades dispatches GET /api/v1/securities/trades (list all trades).
func (s *Server) handleTrades(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListTrades(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleListTrades handles GET /api/v1/securities/trades.
func (s *Server) handleListTrades(w http.ResponseWriter, r *http.Request) {
	trades, err := s.tradeStore.List()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	if trades == nil {
		trades = []types.SecurityTrade{}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  trades,
		"total": len(trades),
	})
}

// handleTrade dispatches requests under /api/v1/securities/trades/ by path suffix.
//
// Routes:
//
//	GET  /api/v1/securities/trades/{id}             → get trade by ID
//	GET  /api/v1/securities/trades/{id}/corrections → list corrections for this trade
//	POST /api/v1/securities/trades/{id}/bust        → bust trade
//	POST /api/v1/securities/trades/{id}/correct     → correct trade
//	POST /api/v1/securities/trades/{id}/reinstate   → reinstate busted trade
func (s *Server) handleTrade(w http.ResponseWriter, r *http.Request) {
	// Strip prefix: /api/v1/securities/trades/
	const prefix = "/api/v1/securities/trades/"
	suffix := strings.TrimPrefix(r.URL.Path, prefix)
	suffix = strings.TrimSuffix(suffix, "/")

	parts := strings.Split(suffix, "/")
	tradeID := parts[0]
	if tradeID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "trade id is required", nil)
		return
	}

	if len(parts) == 1 {
		// /trades/{id}
		if r.Method == http.MethodGet {
			s.handleGetTrade(w, r, tradeID)
		} else {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		}
		return
	}

	action := parts[1]
	switch action {
	case "corrections":
		if r.Method == http.MethodGet {
			s.handleListTradeCorrections(w, r, tradeID)
		} else {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		}
	case "bust":
		if r.Method == http.MethodPost {
			s.handleBustTrade(w, r, tradeID)
		} else {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		}
	case "correct":
		if r.Method == http.MethodPost {
			s.handleCorrectTrade(w, r, tradeID)
		} else {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		}
	case "reinstate":
		if r.Method == http.MethodPost {
			s.handleReinstateTrade(w, r, tradeID)
		} else {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		}
	case "give-up":
		if r.Method == http.MethodPost {
			s.handleGiveUpForTrade(w, r)
		} else {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		}
	default:
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("unknown trade action: %s", action), nil)
	}
}

// handleGetTrade handles GET /api/v1/securities/trades/{id}.
func (s *Server) handleGetTrade(w http.ResponseWriter, r *http.Request, tradeID string) {
	trade, err := s.tradeStore.Get(tradeID)
	if err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND",
				fmt.Sprintf("trade %s not found", tradeID), nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, trade)
}

// bustTradeRequest is the request body for POST /trades/{id}/bust.
type bustTradeRequest struct {
	Reason  string `json:"reason"`
	ActorID string `json:"actor_id"`
}

// handleBustTrade handles POST /api/v1/securities/trades/{id}/bust.
//
// Validation:
//   - reason is required
//   - trade must exist and not already be busted
func (s *Server) handleBustTrade(w http.ResponseWriter, r *http.Request, tradeID string) {
	if err := s.checkPermission(r, types.PermTradeBust); err != nil {
		s.writeError(w, http.StatusForbidden, "PERMISSION_DENIED", err.Error(), nil)
		return
	}
	var req bustTradeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}
	if req.Reason == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "reason is required", nil)
		return
	}

	trade, err := s.tradeStore.Get(tradeID)
	if err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND",
				fmt.Sprintf("trade %s not found", tradeID), nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	// Trade must be active (not already busted, settled, or failed).
	if trade.Status == types.TradeStatusBusted {
		s.writeError(w, http.StatusConflict, "INVALID_STATE",
			fmt.Sprintf("trade %s is already busted", tradeID), nil)
		return
	}
	if trade.Status == types.TradeStatusSettled {
		s.writeError(w, http.StatusConflict, "INVALID_STATE",
			fmt.Sprintf("trade %s is settled and cannot be busted", tradeID), nil)
		return
	}

	correctionID, err := newUUID()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate id", nil)
		return
	}

	correction := &types.TradeCorrection{
		ID:               correctionID,
		TradeID:          tradeID,
		Action:           "BUST",
		Reason:           req.Reason,
		OriginalPrice:    trade.Price,
		OriginalQuantity: trade.Quantity,
		ActorID:          req.ActorID,
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
	}

	if err := s.tradeCorrectionStore.Create(correction); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	// Mark trade as busted.
	trade.Status = types.TradeStatusBusted
	if err := s.tradeStore.UpdateStatus(tradeID, types.TradeStatusBusted); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	// Audit log: trade busted (best-effort).
	if s.auditStore != nil {
		entryID, _ := newUUID()
		_ = s.auditStore.Log(types.AuditEntry{
			ID:         entryID,
			EntityType: "TRADE",
			EntityID:   tradeID,
			Action:     "BUST",
			ActorID:    req.ActorID,
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
		})
	}

	s.writeJSON(w, http.StatusCreated, correction)
}

// correctTradeRequest is the request body for POST /trades/{id}/correct.
type correctTradeRequest struct {
	CorrectedPrice    float64 `json:"corrected_price"`
	CorrectedQuantity int     `json:"corrected_quantity"`
	Reason            string  `json:"reason"`
	ActorID           string  `json:"actor_id"`
}

// handleCorrectTrade handles POST /api/v1/securities/trades/{id}/correct.
//
// Validation:
//   - corrected_price or corrected_quantity must be provided (non-zero)
//   - reason is required
//   - trade must exist
func (s *Server) handleCorrectTrade(w http.ResponseWriter, r *http.Request, tradeID string) {
	var req correctTradeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}
	if req.Reason == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "reason is required", nil)
		return
	}
	if req.CorrectedPrice == 0 && req.CorrectedQuantity == 0 {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD",
			"at least one of corrected_price or corrected_quantity is required", nil)
		return
	}

	trade, err := s.tradeStore.Get(tradeID)
	if err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND",
				fmt.Sprintf("trade %s not found", tradeID), nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	correctionID, err := newUUID()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate id", nil)
		return
	}

	correction := &types.TradeCorrection{
		ID:                correctionID,
		TradeID:           tradeID,
		Action:            "CORRECT",
		Reason:            req.Reason,
		OriginalPrice:     trade.Price,
		OriginalQuantity:  trade.Quantity,
		CorrectedPrice:    req.CorrectedPrice,
		CorrectedQuantity: req.CorrectedQuantity,
		ActorID:           req.ActorID,
		Timestamp:         time.Now().UTC().Format(time.RFC3339),
	}

	if err := s.tradeCorrectionStore.Create(correction); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	s.writeJSON(w, http.StatusCreated, correction)
}

// reinstateTradeRequest is the request body for POST /trades/{id}/reinstate.
type reinstateTradeRequest struct {
	Reason  string `json:"reason"`
	ActorID string `json:"actor_id"`
}

// handleReinstateTrade handles POST /api/v1/securities/trades/{id}/reinstate.
//
// Validation:
//   - reason is required
//   - trade must exist and be in BUSTED state
func (s *Server) handleReinstateTrade(w http.ResponseWriter, r *http.Request, tradeID string) {
	var req reinstateTradeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}
	if req.Reason == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "reason is required", nil)
		return
	}

	trade, err := s.tradeStore.Get(tradeID)
	if err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND",
				fmt.Sprintf("trade %s not found", tradeID), nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	if trade.Status != types.TradeStatusBusted {
		s.writeError(w, http.StatusConflict, "INVALID_STATE",
			fmt.Sprintf("trade %s must be BUSTED to reinstate; current status: %s", tradeID, trade.Status), nil)
		return
	}

	correctionID, err := newUUID()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate id", nil)
		return
	}

	correction := &types.TradeCorrection{
		ID:               correctionID,
		TradeID:          tradeID,
		Action:           "REINSTATE",
		Reason:           req.Reason,
		OriginalPrice:    trade.Price,
		OriginalQuantity: trade.Quantity,
		ActorID:          req.ActorID,
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
	}

	if err := s.tradeCorrectionStore.Create(correction); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	// Restore trade to confirmed status.
	if err := s.tradeStore.UpdateStatus(tradeID, types.TradeStatusConfirmed); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	s.writeJSON(w, http.StatusCreated, correction)
}

// handleListTradeCorrections handles GET /api/v1/securities/trades/{id}/corrections.
func (s *Server) handleListTradeCorrections(w http.ResponseWriter, r *http.Request, tradeID string) {
	// Verify trade exists.
	if _, err := s.tradeStore.Get(tradeID); err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND",
				fmt.Sprintf("trade %s not found", tradeID), nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	corrections, err := s.tradeCorrectionStore.ListByTrade(tradeID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	if corrections == nil {
		corrections = []types.TradeCorrection{}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  corrections,
		"total": len(corrections),
	})
}
