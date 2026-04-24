// Package server — Request For Quote (RFQ) HTTP handlers (P4a).
package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// handleRFQs dispatches:
//   - POST /api/v1/securities/rfq   — submit an RFQ
//   - GET  /api/v1/securities/rfq   — list RFQs
func (s *Server) handleRFQs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleSubmitRFQ(w, r)
	case http.MethodGet:
		s.handleListRFQs(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleRFQAction dispatches sub-resource actions on /api/v1/securities/rfq/{id}/*.
func (s *Server) handleRFQAction(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case hasSuffix(path, "/respond"):
		if r.Method != http.MethodPost {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
			return
		}
		s.handleRespondRFQ(w, r)
	case hasSuffix(path, "/cancel"):
		if r.Method != http.MethodPost {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
			return
		}
		s.handleCancelRFQ(w, r)
	default:
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "endpoint not found", nil)
	}
}

// handleSubmitRFQ handles POST /api/v1/securities/rfq.
func (s *Server) handleSubmitRFQ(w http.ResponseWriter, r *http.Request) {
	if s.rfqStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_AVAILABLE", "rfq store not configured", nil)
		return
	}

	var rfq types.RequestForQuote
	if err := json.NewDecoder(r.Body).Decode(&rfq); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}

	if rfq.InstrumentID == 0 {
		s.writeError(w, http.StatusUnprocessableEntity, "MISSING_FIELD", "instrument_id is required", nil)
		return
	}
	if rfq.Quantity <= 0 {
		s.writeError(w, http.StatusUnprocessableEntity, "INVALID_FIELD", "quantity must be greater than 0", nil)
		return
	}
	if rfq.Side != "BUY" && rfq.Side != "SELL" {
		s.writeError(w, http.StatusUnprocessableEntity, "INVALID_FIELD", "side must be BUY or SELL", nil)
		return
	}

	if err := s.rfqStore.Create(&rfq); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	s.writeJSON(w, http.StatusCreated, rfq)
}

// handleListRFQs handles GET /api/v1/securities/rfq.
// Query params: instrument_id, status
func (s *Server) handleListRFQs(w http.ResponseWriter, r *http.Request) {
	if s.rfqStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_AVAILABLE", "rfq store not configured", nil)
		return
	}

	q := r.URL.Query()
	rfqs, err := s.rfqStore.List(q.Get("instrument_id"), q.Get("status"))
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	if rfqs == nil {
		rfqs = []types.RequestForQuote{}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"data": rfqs, "total": len(rfqs)})
}

// handleRespondRFQ handles POST /api/v1/securities/rfq/{id}/respond.
// Body: { "quote_id": "..." }
func (s *Server) handleRespondRFQ(w http.ResponseWriter, r *http.Request) {
	if s.rfqStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_AVAILABLE", "rfq store not configured", nil)
		return
	}

	id := extractPenultimateSegment(r.URL.Path)
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "rfq id is required", nil)
		return
	}

	var body struct {
		QuoteID string `json:"quote_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.QuoteID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "quote_id is required", nil)
		return
	}

	if err := s.rfqStore.Respond(id, body.QuoteID); err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("rfq %s not found", id), nil)
			return
		}
		s.writeError(w, http.StatusConflict, "INVALID_STATE", err.Error(), nil)
		return
	}

	rfq, err := s.rfqStore.Get(id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, rfq)
}

// handleCancelRFQ handles POST /api/v1/securities/rfq/{id}/cancel.
func (s *Server) handleCancelRFQ(w http.ResponseWriter, r *http.Request) {
	if s.rfqStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_AVAILABLE", "rfq store not configured", nil)
		return
	}

	id := extractPenultimateSegment(r.URL.Path)
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "rfq id is required", nil)
		return
	}

	if err := s.rfqStore.Cancel(id); err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("rfq %s not found", id), nil)
			return
		}
		s.writeError(w, http.StatusConflict, "INVALID_STATE", err.Error(), nil)
		return
	}

	rfq, err := s.rfqStore.Get(id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, rfq)
}
