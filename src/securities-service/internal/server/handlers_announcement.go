// Package server — announcement HTTP handlers.
package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/garudax-platform/securities-service/internal/middleware"
	"github.com/garudax-platform/securities-service/internal/types"
)

// handleAnnouncements dispatches GET /api/v1/securities/announcements (list by tenant)
// and POST /api/v1/securities/announcements (create).
func (s *Server) handleAnnouncements(w http.ResponseWriter, r *http.Request) {
	if s.announcementStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "announcement store not configured", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleListAnnouncements(w, r)
	case http.MethodPost:
		s.handleCreateAnnouncement(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleListAnnouncements handles GET /api/v1/securities/announcements.
// Returns all announcements scoped to the request tenant.
func (s *Server) handleListAnnouncements(w http.ResponseWriter, r *http.Request) {
	// Try tenant from context first; fall back to X-GarudaX-Tenant header.
	tenantID := ""
	if t, ok := middleware.TenantFromContext(r.Context()); ok {
		tenantID = t.String()
	} else {
		tenantID = r.Header.Get("X-GarudaX-Tenant")
	}

	announcements, err := s.announcementStore.ListByTenant(tenantID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	if announcements == nil {
		announcements = []types.Announcement{}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":      announcements,
		"total":     len(announcements),
		"tenant_id": tenantID,
	})
}

// createAnnouncementRequest is the request body for POST /api/v1/securities/announcements.
type createAnnouncementRequest struct {
	Title    string                      `json:"title"`
	Body     string                      `json:"body"`
	Audience types.AnnouncementAudience  `json:"audience"`
}

// handleCreateAnnouncement handles POST /api/v1/securities/announcements.
// Required: title, body. Audience defaults to PUBLIC.
func (s *Server) handleCreateAnnouncement(w http.ResponseWriter, r *http.Request) {
	// Resolve tenant.
	tenantID := ""
	if t, ok := middleware.TenantFromContext(r.Context()); ok {
		tenantID = t.String()
	} else {
		tenantID = r.Header.Get("X-GarudaX-Tenant")
	}

	var req createAnnouncementRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}

	if req.Title == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "title is required", nil)
		return
	}
	if req.Body == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "body is required", nil)
		return
	}

	// Default audience to PUBLIC.
	if req.Audience == "" {
		req.Audience = types.AudiencePublic
	}

	id, err := newUUID()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate id", nil)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	a := types.Announcement{
		ID:        id,
		TenantID:  tenantID,
		Title:     req.Title,
		Body:      req.Body,
		Audience:  req.Audience,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.announcementStore.Create(&a); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusCreated, a)
}
