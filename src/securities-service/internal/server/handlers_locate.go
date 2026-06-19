// Package server — locate request HTTP handlers (P4a).
package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// handleLocates dispatches:
//   - POST /api/v1/securities/locates   — request a locate
//   - GET  /api/v1/securities/locates   — list locates (?firm_id=)
func (s *Server) handleLocates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleRequestLocate(w, r)
	case http.MethodGet:
		s.handleListLocates(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleLocateAction dispatches sub-resource actions on /api/v1/securities/locates/{id}/approve.
func (s *Server) handleLocateAction(w http.ResponseWriter, r *http.Request) {
	// Expect path: /api/v1/securities/locates/{id}/approve
	path := r.URL.Path
	// Determine which action is requested.
	if hasSuffix(path, "/approve") {
		if r.Method != http.MethodPost {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
			return
		}
		s.handleApproveLocate(w, r)
		return
	}
	s.writeError(w, http.StatusNotFound, "NOT_FOUND", "endpoint not found", nil)
}

// handleRequestLocate handles POST /api/v1/securities/locates.
func (s *Server) handleRequestLocate(w http.ResponseWriter, r *http.Request) {
	if s.locateStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_AVAILABLE", "locate store not configured", nil)
		return
	}

	var req types.LocateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}

	// Validate and persist via the locate workflow engine. The engine applies a
	// default 24h expiry, enforces required fields, and creates the request.
	le := engine.NewLocateEngine(s.locateStore)
	if err := le.Request(&req); err != nil {
		if ssErr, ok := err.(*engine.ShortSellError); ok {
			s.writeError(w, ssErr.HTTPStatus(), ssErr.Code, ssErr.Message, nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	s.writeJSON(w, http.StatusCreated, req)
}

// handleListLocates handles GET /api/v1/securities/locates.
func (s *Server) handleListLocates(w http.ResponseWriter, r *http.Request) {
	if s.locateStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_AVAILABLE", "locate store not configured", nil)
		return
	}

	firmID := r.URL.Query().Get("firm_id")
	locates, err := s.locateStore.List(firmID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	if locates == nil {
		locates = []types.LocateRequest{}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"data": locates, "total": len(locates)})
}

// handleApproveLocate handles POST /api/v1/securities/locates/{id}/approve.
func (s *Server) handleApproveLocate(w http.ResponseWriter, r *http.Request) {
	if s.locateStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_AVAILABLE", "locate store not configured", nil)
		return
	}

	// Extract {id} from path: /api/v1/securities/locates/{id}/approve
	id := extractPenultimateSegment(r.URL.Path)
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "locate id is required", nil)
		return
	}

	var body struct {
		LenderFirmID string `json:"lender_firm_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		body.LenderFirmID = ""
	}

	if err := s.locateStore.Approve(id, body.LenderFirmID); err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("locate %s not found", id), nil)
			return
		}
		s.writeError(w, http.StatusConflict, "INVALID_STATE", err.Error(), nil)
		return
	}

	locate, err := s.locateStore.Get(id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, locate)
}

// hasSuffix returns true if path ends with suffix (ignoring trailing slash).
func hasSuffix(path, suffix string) bool {
	trimmed := path
	if len(trimmed) > 0 && trimmed[len(trimmed)-1] == '/' {
		trimmed = trimmed[:len(trimmed)-1]
	}
	return len(trimmed) >= len(suffix) && trimmed[len(trimmed)-len(suffix):] == suffix
}

// extractPenultimateSegment returns the second-to-last non-empty path segment.
// e.g. /api/v1/securities/locates/42/approve → "42"
func extractPenultimateSegment(rawPath string) string {
	parts := splitPath(rawPath)
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-2]
}

// splitPath splits a URL path on "/" and filters empty segments.
func splitPath(rawPath string) []string {
	var parts []string
	start := 0
	for i := 0; i <= len(rawPath); i++ {
		if i == len(rawPath) || rawPath[i] == '/' {
			seg := rawPath[start:i]
			if seg != "" {
				parts = append(parts, seg)
			}
			start = i + 1
		}
	}
	return parts
}
