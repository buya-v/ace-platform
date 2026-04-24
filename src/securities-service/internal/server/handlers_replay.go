// Package server — HTTP handlers for market replay endpoints.
package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// handleReplaySessions dispatches GET /api/v1/securities/replay/sessions (list)
// and POST /api/v1/securities/replay/sessions (create).
func (s *Server) handleReplaySessions(w http.ResponseWriter, r *http.Request) {
	if s.replayStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "replay store not configured", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleListReplaySessions(w, r)
	case http.MethodPost:
		s.handleCreateReplaySession(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleReplaySession dispatches for /api/v1/securities/replay/sessions/{id}
// and sub-resource /events.
func (s *Server) handleReplaySession(w http.ResponseWriter, r *http.Request) {
	if s.replayStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "replay store not configured", nil)
		return
	}
	path := strings.TrimSuffix(r.URL.Path, "/")
	if strings.HasSuffix(path, "/events") {
		switch r.Method {
		case http.MethodGet:
			s.handleGetReplayEvents(w, r)
		case http.MethodPost:
			s.handleAddReplayEvent(w, r)
		default:
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		}
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGetReplaySession(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

func (s *Server) handleListReplaySessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.replayStore.ListSessions()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  sessions,
		"total": len(sessions),
	})
}

func (s *Server) handleCreateReplaySession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID           string `json:"id"`
		InstrumentID string `json:"instrument_id"`
		StartTime    string `json:"start_time"`
		EndTime      string `json:"end_time"`
		Description  string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid JSON body", nil)
		return
	}
	if req.ID == "" || req.InstrumentID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELDS", "id and instrument_id are required", nil)
		return
	}
	sess := &types.ReplaySession{
		ID:           req.ID,
		InstrumentID: req.InstrumentID,
		StartTime:    req.StartTime,
		EndTime:      req.EndTime,
		Description:  req.Description,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	if err := s.replayStore.CreateSession(sess); err != nil {
		s.writeError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusCreated, sess)
}

func (s *Server) handleGetReplaySession(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/securities/replay/sessions/")
	id = strings.TrimSuffix(id, "/")
	sess, err := s.replayStore.GetSession(id)
	if err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "replay session not found", nil)
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, sess)
}

func (s *Server) handleGetReplayEvents(w http.ResponseWriter, r *http.Request) {
	// Path: /api/v1/securities/replay/sessions/{id}/events
	path := strings.TrimSuffix(r.URL.Path, "/")
	path = strings.TrimSuffix(path, "/events")
	id := strings.TrimPrefix(path, "/api/v1/securities/replay/sessions/")

	events, err := s.replayStore.GetEvents(id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	if events == nil {
		events = []types.ReplayEvent{}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"session_id": id,
		"events":     events,
		"total":      len(events),
	})
}

func (s *Server) handleAddReplayEvent(w http.ResponseWriter, r *http.Request) {
	// Path: /api/v1/securities/replay/sessions/{id}/events
	path := strings.TrimSuffix(r.URL.Path, "/")
	path = strings.TrimSuffix(path, "/events")
	sessionID := strings.TrimPrefix(path, "/api/v1/securities/replay/sessions/")

	var req struct {
		Sequence   int         `json:"sequence"`
		EventType  string      `json:"event_type"`
		Payload    interface{} `json:"payload"`
		OccurredAt string      `json:"occurred_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid JSON body", nil)
		return
	}
	if req.EventType == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELDS", "event_type is required", nil)
		return
	}
	event := &types.ReplayEvent{
		SessionID:  sessionID,
		Sequence:   req.Sequence,
		EventType:  req.EventType,
		Payload:    req.Payload,
		OccurredAt: req.OccurredAt,
	}
	if err := s.replayStore.AddEvent(event); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusCreated, event)
}
