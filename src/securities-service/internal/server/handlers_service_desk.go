// Package server — service-desk HTTP handlers (P3b).
//
// Service-desk endpoints allow exchange operators to act on behalf of participants,
// providing a privileged path for order submission and cancellation that bypasses
// the normal participant-facing constraints.
package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/garudax-platform/securities-service/internal/types"
)

// serviceDeskSubmitOrderRequest is the body for
// POST /api/v1/securities/service-desk/orders.
type serviceDeskSubmitOrderRequest struct {
	ParticipantID string            `json:"participant_id"`
	InstrumentID  string            `json:"instrument_id"`
	Side          types.OrderSide   `json:"side"`
	OrderType     types.OrderType   `json:"order_type"`
	Quantity      int               `json:"quantity"`
	Price         types.Decimal     `json:"price"`
	TimeInForce   types.TimeInForce `json:"time_in_force"`
}

// handleServiceDeskSubmitOrder handles POST /api/v1/securities/service-desk/orders.
//
// Submits an order on behalf of the specified participant. The operator must supply
// participant_id in the request body instead of deriving it from a JWT claim.
func (s *Server) handleServiceDeskSubmitOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}

	var req serviceDeskSubmitOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}

	if req.ParticipantID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "participant_id is required", nil)
		return
	}
	if req.InstrumentID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "instrument_id is required", nil)
		return
	}
	if req.Side == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "side is required", nil)
		return
	}
	if req.OrderType == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "order_type is required", nil)
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

	tif := req.TimeInForce
	if tif == "" {
		tif = types.TimeInForceDAY
	}

	now := time.Now().UTC().Format(time.RFC3339)
	order := &types.SecurityOrder{
		ID:            id,
		InstrumentID:  req.InstrumentID,
		ParticipantID: req.ParticipantID,
		Side:          req.Side,
		OrderType:     req.OrderType,
		Quantity:      req.Quantity,
		Price:         req.Price,
		TimeInForce:   tif,
		Status:        types.OrderStatusPending,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.orderStore.Submit(order); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusCreated, order)
}

// serviceDeskCancelOrderRequest is the body for
// POST /api/v1/securities/service-desk/cancel-order.
type serviceDeskCancelOrderRequest struct {
	OrderID string `json:"order_id"`
	Reason  string `json:"reason"`
}

// handleServiceDeskCancelOrder handles POST /api/v1/securities/service-desk/cancel-order.
//
// Cancels an order on behalf of its owning participant. The reason is recorded for
// the audit trail but is not persisted on the order itself (audit integration is a
// future concern; here we validate and cancel the order in the store).
func (s *Server) handleServiceDeskCancelOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}

	var req serviceDeskCancelOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}

	if req.OrderID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "order_id is required", nil)
		return
	}
	if req.Reason == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "reason is required", nil)
		return
	}

	if err := s.orderStore.Cancel(req.OrderID); err != nil {
		s.writeError(w, http.StatusBadRequest, "CANCEL_FAILED", err.Error(), nil)
		return
	}

	order, err := s.orderStore.Get(req.OrderID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, order)
}
