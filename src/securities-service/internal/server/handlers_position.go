// Package server — position HTTP handlers.
package server

import (
	"net/http"

	"github.com/garudax-platform/securities-service/internal/types"
)

// handlePositions dispatches GET /api/v1/securities/positions.
func (s *Server) handlePositions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListPositions(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleListPositions handles GET /api/v1/securities/positions.
//
// Returns all positions across all participants. An empty list is returned when
// positionStore is nil (service unavailable guard is intentionally omitted for
// positions since it already exists in the legacy handler_settlement.go path).
func (s *Server) handleListPositions(w http.ResponseWriter, r *http.Request) {
	if s.positionStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "position store not available", nil)
		return
	}

	// List("") returns all positions regardless of participant.
	positions, err := s.positionStore.List("")
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	if positions == nil {
		positions = []types.Position{}
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  positions,
		"total": len(positions),
	})
}
