package server

import (
	"net/http"
	"strings"

	"github.com/garudax-platform/securities-service/internal/store"
)

// handleWarnings dispatches:
//
//	GET /api/v1/securities/warnings?acknowledged=false
func (s *Server) handleWarnings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}
	s.handleListWarnings(w, r)
}

// handleWarningItem dispatches:
//
//	POST /api/v1/securities/warnings/{id}/acknowledge
func (s *Server) handleWarningItem(w http.ResponseWriter, r *http.Request) {
	suffix := strings.TrimPrefix(r.URL.Path, "/api/v1/securities/warnings/")
	suffix = strings.TrimSuffix(suffix, "/")

	if strings.HasSuffix(suffix, "/acknowledge") {
		id := strings.TrimSuffix(suffix, "/acknowledge")
		if r.Method == http.MethodPost {
			s.handleAcknowledgeWarning(w, r, id)
		} else {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		}
		return
	}

	s.writeError(w, http.StatusNotFound, "NOT_FOUND", "not found", nil)
}

func (s *Server) handleListWarnings(w http.ResponseWriter, r *http.Request) {
	if s.warningStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "warning store not configured", nil)
		return
	}
	// Default to listing unacknowledged warnings; pass ?acknowledged=true for acknowledged ones.
	acknowledged := false
	if r.URL.Query().Get("acknowledged") == "true" {
		acknowledged = true
	}
	warnings, err := s.warningStore.List(acknowledged)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, warnings)
}

func (s *Server) handleAcknowledgeWarning(w http.ResponseWriter, r *http.Request, id string) {
	if s.warningStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "warning store not configured", nil)
		return
	}
	// The acknowledging user is taken from the X-User-ID header (or defaults to "system").
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "system"
	}
	if err := s.warningStore.Acknowledge(id, userID); err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "warning not found", nil)
		return
	} else if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
