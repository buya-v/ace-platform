// Package server — off-book trade HTTP handlers.
package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/garudax-platform/securities-service/internal/types"
)

// handleOffBookTrades dispatches GET and POST for the off-book trades collection.
func (s *Server) handleOffBookTrades(w http.ResponseWriter, r *http.Request) {
	if s.offBookTradeStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "off-book trade store not configured", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleListOffBookTrades(w, r)
	case http.MethodPost:
		s.handleSubmitOffBookTrade(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleOffBookTrade dispatches actions on a specific off-book trade.
func (s *Server) handleOffBookTrade(w http.ResponseWriter, r *http.Request) {
	if s.offBookTradeStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "off-book trade store not configured", nil)
		return
	}
	path := strings.TrimSuffix(r.URL.Path, "/")
	switch {
	case strings.HasSuffix(path, "/status"):
		if r.Method == http.MethodPut {
			s.handleUpdateOffBookStatus(w, r)
		} else {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		}
	case strings.HasSuffix(path, "/confirm"):
		if r.Method == http.MethodPost {
			s.handleConfirmOffBookTrade(w, r)
		} else {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		}
	case strings.HasSuffix(path, "/reject"):
		if r.Method == http.MethodPost {
			s.handleRejectOffBookTrade(w, r)
		} else {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		}
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleListOffBookTrades handles GET /api/v1/securities/off-book-trades.
func (s *Server) handleListOffBookTrades(w http.ResponseWriter, r *http.Request) {
	trades, err := s.offBookTradeStore.List()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	if trades == nil {
		trades = []types.OffBookTrade{}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  trades,
		"total": len(trades),
	})
}

// submitOffBookTradeRequest is the request body for POST /api/v1/securities/off-book-trades.
type submitOffBookTradeRequest struct {
	InstrumentID    string  `json:"instrument_id"`
	BuyParticipant  string  `json:"buy_participant"`
	SellParticipant string  `json:"sell_participant"`
	Price           float64 `json:"price"`
	Quantity        int     `json:"quantity"`
	Notes           string  `json:"notes"`
}

// handleSubmitOffBookTrade handles POST /api/v1/securities/off-book-trades.
func (s *Server) handleSubmitOffBookTrade(w http.ResponseWriter, r *http.Request) {
	var req submitOffBookTradeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}
	if req.InstrumentID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "instrument_id is required", nil)
		return
	}
	if req.BuyParticipant == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "buy_participant is required", nil)
		return
	}
	if req.SellParticipant == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "sell_participant is required", nil)
		return
	}
	if req.Quantity <= 0 {
		s.writeError(w, http.StatusBadRequest, "INVALID_FIELD", "quantity must be positive", nil)
		return
	}

	id, err := newUUID()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate id", nil)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	trade := &types.OffBookTrade{
		ID:              id,
		InstrumentID:    req.InstrumentID,
		BuyParticipant:  req.BuyParticipant,
		SellParticipant: req.SellParticipant,
		Price:           req.Price,
		Quantity:        req.Quantity,
		TradeDate:       now[:10],
		Status:          types.OffBookReported,
		Notes:           req.Notes,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.offBookTradeStore.Create(trade); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusCreated, trade)
}

// updateOffBookStatusRequest is the request body for PUT .../off-book-trades/{id}/status.
type updateOffBookStatusRequest struct {
	Status types.OffBookStatus `json:"status"`
}

// handleUpdateOffBookStatus handles PUT /api/v1/securities/off-book-trades/{id}/status.
func (s *Server) handleUpdateOffBookStatus(w http.ResponseWriter, r *http.Request) {
	// Extract trade ID from path: .../off-book-trades/{id}/status
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	tradeID := parts[len(parts)-2]
	if tradeID == "" {
		s.writeError(w, http.StatusBadRequest, "INVALID_PATH", "missing trade id", nil)
		return
	}

	var req updateOffBookStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}
	if req.Status == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "status is required", nil)
		return
	}

	if err := s.offBookTradeStore.UpdateStatus(tradeID, req.Status); err != nil {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "off-book trade not found", nil)
		return
	}
	trade, _ := s.offBookTradeStore.Get(tradeID)
	s.writeJSON(w, http.StatusOK, trade)
}

// confirmOffBookTradeRequest is the request body for POST .../off-book-trades/{id}/confirm.
type confirmOffBookTradeRequest struct {
	ConfirmedBy string `json:"confirmed_by"`
}

// handleConfirmOffBookTrade handles POST /api/v1/securities/trades/off-book/{id}/confirm.
// Sets Status=CONFIRMED, records ConfirmedBy.
func (s *Server) handleConfirmOffBookTrade(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	tradeID := parts[len(parts)-2]
	if tradeID == "" {
		s.writeError(w, http.StatusBadRequest, "INVALID_PATH", "missing trade id", nil)
		return
	}

	var req confirmOffBookTradeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}
	if req.ConfirmedBy == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "confirmed_by is required", nil)
		return
	}

	if err := s.offBookTradeStore.Confirm(tradeID, req.ConfirmedBy); err != nil {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "off-book trade not found", nil)
		return
	}
	trade, _ := s.offBookTradeStore.Get(tradeID)
	s.writeJSON(w, http.StatusOK, trade)
}

// rejectOffBookTradeRequest is the request body for POST .../off-book-trades/{id}/reject.
type rejectOffBookTradeRequest struct {
	RejectedBy      string `json:"rejected_by"`
	RejectionReason string `json:"rejection_reason"`
}

// handleRejectOffBookTrade handles POST /api/v1/securities/trades/off-book/{id}/reject.
// Sets Status=REJECTED, records RejectedBy and RejectionReason (required).
func (s *Server) handleRejectOffBookTrade(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	tradeID := parts[len(parts)-2]
	if tradeID == "" {
		s.writeError(w, http.StatusBadRequest, "INVALID_PATH", "missing trade id", nil)
		return
	}

	var req rejectOffBookTradeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}
	if req.RejectedBy == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "rejected_by is required", nil)
		return
	}
	if req.RejectionReason == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "rejection_reason is required", nil)
		return
	}

	if err := s.offBookTradeStore.Reject(tradeID, req.RejectedBy, req.RejectionReason); err != nil {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "off-book trade not found", nil)
		return
	}
	trade, _ := s.offBookTradeStore.Get(tradeID)
	s.writeJSON(w, http.StatusOK, trade)
}
