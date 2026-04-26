// Package server — session management HTTP handlers.
package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/garudax-platform/securities-service/internal/middleware"
	"github.com/garudax-platform/securities-service/internal/types"
)

// handleSessions dispatches GET /api/v1/securities/sessions.
func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}
	s.handleListSessions(w, r)
}

// handleListSessions returns a map of instrumentID → current session.
func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	if s.sessionManager == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "session manager not configured", nil)
		return
	}

	sessions := s.sessionManager.GetAllSessions()
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"sessions": sessions,
	})
}

// handleSessionOrAdjustment is the wildcard dispatcher for /api/v1/securities/sessions/.
// It routes to transition, extend, or shorten based on the path suffix.
func (s *Server) handleSessionOrAdjustment(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	switch {
	case strings.HasSuffix(path, "/transition"):
		s.handleSessionTransition(w, r)
	case strings.HasSuffix(path, "/extend"):
		s.handleSessionExtend(w, r)
	case strings.HasSuffix(path, "/shorten"):
		s.handleSessionShorten(w, r)
	default:
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "unknown session sub-resource", nil)
	}
}

// handleSessionTransition dispatches POST /api/v1/securities/sessions/{instrumentID}/transition.
func (s *Server) handleSessionTransition(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}

	if s.sessionManager == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "session manager not configured", nil)
		return
	}

	// Extract tenant from context.
	tenantID, ok := middleware.TenantFromContext(r.Context())
	if !ok {
		s.writeError(w, http.StatusUnauthorized, "TENANT_REQUIRED", "X-GarudaX-Tenant header is required", nil)
		return
	}

	// Extract instrumentID from path: /api/v1/securities/sessions/{instrumentID}/transition
	path := strings.TrimSuffix(r.URL.Path, "/")
	if !strings.HasSuffix(path, "/transition") {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "expected /transition suffix", nil)
		return
	}
	path = strings.TrimSuffix(path, "/transition")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "instrument_id is required", nil)
		return
	}
	instrumentID := parts[len(parts)-1]
	if instrumentID == "" || instrumentID == "sessions" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "instrument_id is required", nil)
		return
	}

	// Parse request body.
	var req struct {
		Session string `json:"session"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}

	newSession := types.MarketSession(req.Session)

	// Validate the session value.
	validSessions := map[types.MarketSession]bool{
		types.SessionPreOpen:        true,
		types.SessionContinuous:     true,
		types.SessionClosingAuction: true,
		types.SessionClosed:         true,
	}
	if !validSessions[newSession] {
		s.writeError(w, http.StatusUnprocessableEntity, "INVALID_FIELD",
			"session must be one of PRE_OPEN, CONTINUOUS, CLOSING_AUCTION, CLOSED", nil)
		return
	}

	result, err := s.sessionManager.TransitionTo(instrumentID, tenantID.String(), newSession)
	if err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "INVALID_TRANSITION", err.Error(), nil)
		return
	}

	resp := map[string]interface{}{
		"instrument_id":   instrumentID,
		"current_session": newSession,
	}
	if result != nil {
		resp["auction_result"] = result
	}

	s.writeJSON(w, http.StatusOK, resp)
}

// handleSessionExtend dispatches POST /api/v1/securities/sessions/{instrument_id}/extend.
func (s *Server) handleSessionExtend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}
	s.handleSessionAdjustment(w, r, "EXTEND")
}

// handleSessionShorten dispatches POST /api/v1/securities/sessions/{instrument_id}/shorten.
func (s *Server) handleSessionShorten(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}
	s.handleSessionAdjustment(w, r, "SHORTEN")
}

// handleSessionAdjustment is the shared implementation for extend and shorten.
// action is "EXTEND" or "SHORTEN".
func (s *Server) handleSessionAdjustment(w http.ResponseWriter, r *http.Request, action string) {
	// Require tenant context.
	if _, ok := middleware.TenantFromContext(r.Context()); !ok {
		s.writeError(w, http.StatusUnauthorized, "TENANT_REQUIRED", "X-GarudaX-Tenant header is required", nil)
		return
	}

	// Extract instrument_id from path: /api/v1/securities/sessions/{instrument_id}/extend|shorten
	path := strings.TrimSuffix(r.URL.Path, "/")
	suffix := strings.ToLower(action) // "extend" or "shorten"
	if !strings.HasSuffix(path, "/"+suffix) {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "unexpected path suffix", nil)
		return
	}
	path = strings.TrimSuffix(path, "/"+suffix)
	parts := strings.Split(path, "/")
	instrumentID := parts[len(parts)-1]
	if instrumentID == "" || instrumentID == "sessions" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "instrument_id is required", nil)
		return
	}

	// Parse request body.
	var req struct {
		DurationMinutes int    `json:"duration_minutes"`
		Reason          string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}
	if req.DurationMinutes <= 0 {
		s.writeError(w, http.StatusBadRequest, "INVALID_FIELD", "duration_minutes must be a positive integer", nil)
		return
	}

	now := time.Now().UTC()
	ext := types.SessionExtension{
		InstrumentID:    instrumentID,
		Action:          action,
		DurationMinutes: req.DurationMinutes,
		Reason:          req.Reason,
		CreatedAt:       now.Format(time.RFC3339),
	}

	// Calculate new_end_time as now + duration_minutes.
	newEndTime := now.Add(time.Duration(req.DurationMinutes) * time.Minute)

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"instrument_id":    ext.InstrumentID,
		"action":           ext.Action,
		"duration_minutes": ext.DurationMinutes,
		"new_end_time":     newEndTime.Format(time.RFC3339),
		"reason":           ext.Reason,
		"created_at":       ext.CreatedAt,
	})
}
