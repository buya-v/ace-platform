// Package server — market and segment HTTP handlers.
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/garudax-platform/securities-service/internal/types"
)

// handleMarkets dispatches GET /api/v1/securities/markets (list)
// and POST /api/v1/securities/markets (create).
func (s *Server) handleMarkets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListMarkets(w, r)
	case http.MethodPost:
		s.handleCreateMarket(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleMarket dispatches GET /api/v1/securities/markets/{id}
// and PUT /api/v1/securities/markets/{id}/status.
func (s *Server) handleMarket(w http.ResponseWriter, r *http.Request) {
	// Detect the /status sub-resource.
	if strings.HasSuffix(strings.TrimSuffix(r.URL.Path, "/"), "/status") {
		if r.Method == http.MethodPut {
			s.handleUpdateMarketStatus(w, r)
		} else {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetMarket(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleListMarkets handles GET /api/v1/securities/markets.
func (s *Server) handleListMarkets(w http.ResponseWriter, r *http.Request) {
	markets, err := s.marketStore.List()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	if markets == nil {
		markets = []types.Market{}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  markets,
		"total": len(markets),
	})
}

// handleGetMarket handles GET /api/v1/securities/markets/{id}.
func (s *Server) handleGetMarket(w http.ResponseWriter, r *http.Request) {
	id := extractLastSegment(r.URL.Path)
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "market id is required", nil)
		return
	}

	market, err := s.marketStore.Get(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND",
			fmt.Sprintf("market %s not found", id), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, market)
}

// createMarketRequest is the request body for POST /api/v1/securities/markets.
type createMarketRequest struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Timezone string `json:"timezone"`
}

// handleCreateMarket handles POST /api/v1/securities/markets.
//
// Validation: id and name are required.
// Defaults: status = MARKET_ACTIVE, timezone = "UTC" if not provided.
func (s *Server) handleCreateMarket(w http.ResponseWriter, r *http.Request) {
	var req createMarketRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}

	if req.ID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "id is required", nil)
		return
	}
	if req.Name == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "name is required", nil)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	tz := req.Timezone
	if tz == "" {
		tz = "UTC"
	}

	market := &types.Market{
		ID:        req.ID,
		Name:      req.Name,
		Status:    types.MarketActive,
		Timezone:  tz,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.marketStore.Create(market); err != nil {
		s.writeError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusCreated, market)
}

// setMarketStatusRequest is the request body for PUT .../markets/{id}/status.
type setMarketStatusRequest struct {
	Status string `json:"status"`
}

// handleUpdateMarketStatus handles PUT /api/v1/securities/markets/{id}/status.
func (s *Server) handleUpdateMarketStatus(w http.ResponseWriter, r *http.Request) {
	// Path is .../markets/{id}/status — id is the second-to-last segment.
	path := strings.TrimSuffix(r.URL.Path, "/")
	segments := strings.Split(path, "/")
	if len(segments) < 2 || segments[len(segments)-1] != "status" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "market id is required", nil)
		return
	}
	id := segments[len(segments)-2]
	if id == "" || id == "markets" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "market id is required", nil)
		return
	}

	var req setMarketStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}

	if req.Status == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "status is required", nil)
		return
	}

	if err := s.marketStore.UpdateStatus(id, req.Status); err != nil {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND",
			fmt.Sprintf("market %s not found", id), nil)
		return
	}

	market, err := s.marketStore.Get(id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, market)
}

// handleSegments dispatches GET /api/v1/securities/segments (list, optional ?market_id= filter)
// and POST /api/v1/securities/segments (create).
func (s *Server) handleSegments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListSegments(w, r)
	case http.MethodPost:
		s.handleCreateSegment(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleListSegments handles GET /api/v1/securities/segments.
//
// Query parameters:
//
//	market_id — optional filter by market
func (s *Server) handleListSegments(w http.ResponseWriter, r *http.Request) {
	marketID := r.URL.Query().Get("market_id")

	segments, err := s.segmentStore.ListByMarket(marketID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	if segments == nil {
		segments = []types.Segment{}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  segments,
		"total": len(segments),
	})
}

// createSegmentRequest is the request body for POST /api/v1/securities/segments.
type createSegmentRequest struct {
	ID       string `json:"id"`
	MarketID string `json:"market_id"`
	Name     string `json:"name"`
}

// handleCreateSegment handles POST /api/v1/securities/segments.
//
// Validation: id, market_id, and name are required.
// Defaults: status = SEG_ACTIVE.
func (s *Server) handleCreateSegment(w http.ResponseWriter, r *http.Request) {
	var req createSegmentRequest
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

	now := time.Now().UTC().Format(time.RFC3339)
	seg := &types.Segment{
		ID:        req.ID,
		MarketID:  req.MarketID,
		Name:      req.Name,
		Status:    types.SegActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.segmentStore.Create(seg); err != nil {
		s.writeError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusCreated, seg)
}
