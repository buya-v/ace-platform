// Package server — audit trail HTTP handler.
package server

import (
	"net/http"

	"github.com/garudax-platform/securities-service/internal/types"
)

// handleAuditTrail handles GET /api/v1/securities/audit-trail.
//
// Query parameters:
//
//	entity_type — filter by entity type (ORDER, TRADE, INSTRUMENT, PARTICIPANT)
//	entity_id   — filter by entity ID
//	actor_id    — filter by actor ID
//	start_date  — filter entries on or after this RFC3339 timestamp
//	end_date    — filter entries on or before this RFC3339 timestamp
func (s *Server) handleAuditTrail(w http.ResponseWriter, r *http.Request) {
	if s.auditStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "audit store not configured", nil)
		return
	}
	if r.Method != "GET" {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}

	q := r.URL.Query()
	filters := types.AuditFilters{
		EntityType: q.Get("entity_type"),
		EntityID:   q.Get("entity_id"),
		ActorID:    q.Get("actor_id"),
		StartDate:  q.Get("start_date"),
		EndDate:    q.Get("end_date"),
	}

	entries, err := s.auditStore.List(filters)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	if entries == nil {
		entries = []types.AuditEntry{}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  entries,
		"total": len(entries),
	})
}
