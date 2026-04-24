// Package server — pending change (maker/checker) HTTP handlers.
package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// handlePendingChanges dispatches:
//
//	POST /api/v1/securities/pending-changes — submit a pending change
//	GET  /api/v1/securities/pending-changes — list pending changes (optional ?status=)
func (s *Server) handlePendingChanges(w http.ResponseWriter, r *http.Request) {
	if s.pendingChangeStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "pending change store not available", nil)
		return
	}
	switch r.Method {
	case http.MethodPost:
		s.handleSubmitPendingChange(w, r)
	case http.MethodGet:
		s.handleListPendingChanges(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handlePendingChange dispatches:
//
//	POST /api/v1/securities/pending-changes/{id}/approve
//	POST /api/v1/securities/pending-changes/{id}/reject
func (s *Server) handlePendingChange(w http.ResponseWriter, r *http.Request) {
	if s.pendingChangeStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "pending change store not available", nil)
		return
	}
	path := strings.TrimSuffix(r.URL.Path, "/")
	if strings.HasSuffix(path, "/approve") {
		if r.Method != http.MethodPost {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
			return
		}
		s.handleApprovePendingChange(w, r)
		return
	}
	if strings.HasSuffix(path, "/reject") {
		if r.Method != http.MethodPost {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
			return
		}
		s.handleRejectPendingChange(w, r)
		return
	}
	s.writeError(w, http.StatusNotFound, "NOT_FOUND", "not found", nil)
}

// submitPendingChangeRequest is the POST body for submitting a pending change.
type submitPendingChangeRequest struct {
	EntityType string                 `json:"entity_type"`
	EntityID   string                 `json:"entity_id"`
	ChangeType string                 `json:"change_type"`
	Payload    map[string]interface{} `json:"payload"`
	SubmittedBy string               `json:"submitted_by"`
}

// handleSubmitPendingChange handles POST /api/v1/securities/pending-changes.
//
// Validation: entity_type, change_type, and payload are required.
func (s *Server) handleSubmitPendingChange(w http.ResponseWriter, r *http.Request) {
	var req submitPendingChangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}

	var missing []string
	if req.EntityType == "" {
		missing = append(missing, "entity_type")
	}
	if req.ChangeType == "" {
		missing = append(missing, "change_type")
	}
	if len(req.Payload) == 0 {
		missing = append(missing, "payload")
	}
	if len(missing) > 0 {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD",
			"required fields missing: "+strings.Join(missing, ", "), missing)
		return
	}

	validChangeTypes := map[string]bool{"CREATE": true, "UPDATE": true, "DELETE": true}
	if !validChangeTypes[req.ChangeType] {
		s.writeError(w, http.StatusBadRequest, "INVALID_FIELD",
			"change_type must be CREATE, UPDATE, or DELETE", nil)
		return
	}

	id, err := newUUID()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate ID", nil)
		return
	}

	change := &types.PendingChange{
		ID:          id,
		EntityType:  req.EntityType,
		EntityID:    req.EntityID,
		ChangeType:  req.ChangeType,
		Payload:     req.Payload,
		SubmittedBy: req.SubmittedBy,
		Status:      "PENDING_APPROVAL",
		SubmittedAt: time.Now().UTC().Format(time.RFC3339),
	}

	if err := s.pendingChangeStore.Create(change); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	s.writeJSON(w, http.StatusCreated, change)
}

// handleListPendingChanges handles GET /api/v1/securities/pending-changes.
// Optional query parameter: ?status=PENDING_APPROVAL|APPROVED|REJECTED
func (s *Server) handleListPendingChanges(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")

	changes, err := s.pendingChangeStore.ListByStatus(status)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	if changes == nil {
		changes = []types.PendingChange{}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  changes,
		"total": len(changes),
	})
}

// approvePendingChangeRequest is the POST body for approving a pending change.
type approvePendingChangeRequest struct {
	ReviewerID string `json:"reviewer_id"`
}

// handleApprovePendingChange handles POST /api/v1/securities/pending-changes/{id}/approve.
//
// Business rule: reviewer must not be the same as the submitter (four-eyes principle).
func (s *Server) handleApprovePendingChange(w http.ResponseWriter, r *http.Request) {
	// Extract the change ID: path is .../pending-changes/{id}/approve
	path := strings.TrimSuffix(r.URL.Path, "/")
	path = strings.TrimSuffix(path, "/approve")
	id := extractLastSegment(path)
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "change id is required", nil)
		return
	}

	var req approvePendingChangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}

	change, err := s.pendingChangeStore.Get(id)
	if err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "pending change not found", nil)
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	// Four-eyes principle: reviewer != submitter
	if req.ReviewerID != "" && req.ReviewerID == change.SubmittedBy {
		s.writeError(w, http.StatusForbidden, "FORBIDDEN",
			"reviewer cannot be the same as the submitter", nil)
		return
	}

	if err := s.pendingChangeStore.Approve(id, req.ReviewerID); err != nil {
		s.writeError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
		return
	}

	approved, _ := s.pendingChangeStore.Get(id)
	s.writeJSON(w, http.StatusOK, approved)
}

// rejectPendingChangeRequest is the POST body for rejecting a pending change.
type rejectPendingChangeRequest struct {
	ReviewerID string `json:"reviewer_id"`
	Comment    string `json:"comment"`
}

// handleRejectPendingChange handles POST /api/v1/securities/pending-changes/{id}/reject.
//
// Validation: comment is required.
func (s *Server) handleRejectPendingChange(w http.ResponseWriter, r *http.Request) {
	// Extract the change ID: path is .../pending-changes/{id}/reject
	path := strings.TrimSuffix(r.URL.Path, "/")
	path = strings.TrimSuffix(path, "/reject")
	id := extractLastSegment(path)
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "change id is required", nil)
		return
	}

	var req rejectPendingChangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}

	if strings.TrimSpace(req.Comment) == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "comment is required for rejection", nil)
		return
	}

	_, err := s.pendingChangeStore.Get(id)
	if err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "pending change not found", nil)
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	if err := s.pendingChangeStore.Reject(id, req.ReviewerID, req.Comment); err != nil {
		s.writeError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
		return
	}

	rejected, _ := s.pendingChangeStore.Get(id)
	s.writeJSON(w, http.StatusOK, rejected)
}
