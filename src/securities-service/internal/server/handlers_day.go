// Package server — trading day lifecycle HTTP handlers.
package server

import (
	"net/http"
)

// handleDayStatus handles GET /api/v1/securities/day/status.
// Returns the current day state.
func (s *Server) handleDayStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}
	if s.dayManager == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "day manager not configured", nil)
		return
	}
	state := s.dayManager.GetState()
	s.writeJSON(w, http.StatusOK, map[string]string{
		"state": string(state),
	})
}

// handleDayStart handles POST /api/v1/securities/day/start.
// Transitions DAY_CLOSED → DAY_PRE_OPEN.
func (s *Server) handleDayStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}
	if s.dayManager == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "day manager not configured", nil)
		return
	}
	if err := s.dayManager.StartDay(); err != nil {
		s.writeError(w, http.StatusConflict, "INVALID_TRANSITION", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{
		"state": string(s.dayManager.GetState()),
	})
}

// handleDayTrading handles POST /api/v1/securities/day/trading.
// Transitions DAY_PRE_OPEN → DAY_TRADING.
func (s *Server) handleDayTrading(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}
	if s.dayManager == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "day manager not configured", nil)
		return
	}
	if err := s.dayManager.StartTrading(); err != nil {
		s.writeError(w, http.StatusConflict, "INVALID_TRANSITION", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{
		"state": string(s.dayManager.GetState()),
	})
}

// handleDayEndTrading handles POST /api/v1/securities/day/end-trading.
// Transitions DAY_TRADING → DAY_POST_CLOSE.
func (s *Server) handleDayEndTrading(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}
	if s.dayManager == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "day manager not configured", nil)
		return
	}
	if err := s.dayManager.EndTrading(); err != nil {
		s.writeError(w, http.StatusConflict, "INVALID_TRANSITION", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{
		"state": string(s.dayManager.GetState()),
	})
}

// handleDayEnd handles POST /api/v1/securities/day/end.
// Transitions DAY_POST_CLOSE → DAY_CLOSED.
func (s *Server) handleDayEnd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}
	if s.dayManager == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "day manager not configured", nil)
		return
	}
	if err := s.dayManager.EndDay(); err != nil {
		s.writeError(w, http.StatusConflict, "INVALID_TRANSITION", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{
		"state": string(s.dayManager.GetState()),
	})
}
