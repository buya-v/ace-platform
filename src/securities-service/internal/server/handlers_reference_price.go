// Package server — reference price HTTP handlers.
package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// handleReferencePrice dispatches:
//
//	GET  /api/v1/securities/instruments/{id}/reference-price
//	POST /api/v1/securities/instruments/{id}/reference-price
func (s *Server) handleReferencePrice(w http.ResponseWriter, r *http.Request) {
	if s.referencePriceStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "reference price store not available", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGetReferencePrice(w, r)
	case http.MethodPost:
		s.handleSetReferencePrice(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// instrumentIDFromReferencePricePath extracts the instrument ID from:
//
//	/api/v1/securities/instruments/{id}/reference-price
func instrumentIDFromReferencePricePath(rawPath string) string {
	// Strip trailing slash then the "/reference-price" suffix.
	p := strings.TrimSuffix(rawPath, "/")
	p = strings.TrimSuffix(p, "/reference-price")
	return extractLastSegment(p)
}

// handleGetReferencePrice handles GET /api/v1/securities/instruments/{id}/reference-price.
//
// Returns the current reference price. If the price is older than StaleThresholdMinutes,
// the response includes "stale": true.
func (s *Server) handleGetReferencePrice(w http.ResponseWriter, r *http.Request) {
	instrumentID := instrumentIDFromReferencePricePath(r.URL.Path)
	if instrumentID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "instrument_id is required", nil)
		return
	}

	rp, err := s.referencePriceStore.Get(instrumentID)
	if err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND",
			"no reference price set for instrument "+instrumentID, nil)
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	// Stale detection: compare age against StaleThresholdMinutes.
	stale := false
	if rp.StaleThresholdMinutes > 0 {
		setAt, parseErr := time.Parse(time.RFC3339, rp.SetAt)
		if parseErr == nil {
			age := time.Since(setAt)
			stale = age > time.Duration(rp.StaleThresholdMinutes)*time.Minute
		}
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"reference_price": rp,
		"stale":           stale,
	})
}

// setReferencePriceRequest is the POST body for setting a reference price.
type setReferencePriceRequest struct {
	Price                 float64 `json:"price"`
	SetBy                 string  `json:"set_by"`
	StaleThresholdMinutes int     `json:"stale_threshold_minutes"`
}

// handleSetReferencePrice handles POST /api/v1/securities/instruments/{id}/reference-price.
//
// Validation: price must be > 0.
// Side effect: if a circuit breaker exists for the instrument, its reference_price is updated.
func (s *Server) handleSetReferencePrice(w http.ResponseWriter, r *http.Request) {
	instrumentID := instrumentIDFromReferencePricePath(r.URL.Path)
	if instrumentID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "instrument_id is required", nil)
		return
	}

	var req setReferencePriceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}

	if req.Price <= 0 {
		s.writeError(w, http.StatusBadRequest, "INVALID_FIELD", "price must be greater than 0", nil)
		return
	}

	rp := &types.ReferencePrice{
		InstrumentID:          instrumentID,
		Price:                 req.Price,
		SetBy:                 req.SetBy,
		SetAt:                 time.Now().UTC().Format(time.RFC3339),
		StaleThresholdMinutes: req.StaleThresholdMinutes,
	}

	if err := s.referencePriceStore.Set(rp); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	// Update circuit breaker reference price if one exists for this instrument.
	if s.circuitBreakerStore != nil {
		cb, err := s.circuitBreakerStore.Get(instrumentID)
		if err == nil && cb != nil {
			cb.ReferencePrice = req.Price
			_ = s.circuitBreakerStore.Set(cb)
		}
	}

	s.writeJSON(w, http.StatusOK, rp)
}
