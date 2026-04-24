// Package server — surveillance HTTP handlers.
package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// handleSurveillanceAlerts dispatches GET and POST for the alerts collection.
func (s *Server) handleSurveillanceAlerts(w http.ResponseWriter, r *http.Request) {
	if s.surveillanceStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "surveillance store not configured", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleListAlerts(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleSurveillanceAlert dispatches actions on a specific alert (PUT .../resolve).
func (s *Server) handleSurveillanceAlert(w http.ResponseWriter, r *http.Request) {
	if s.surveillanceStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "surveillance store not configured", nil)
		return
	}
	if strings.HasSuffix(strings.TrimSuffix(r.URL.Path, "/"), "/resolve") {
		if r.Method == http.MethodPut {
			s.handleResolveAlert(w, r)
		} else {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		}
		return
	}
	s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
}

// handleSurveillanceThresholds dispatches GET and PUT for thresholds on an instrument.
func (s *Server) handleSurveillanceThresholds(w http.ResponseWriter, r *http.Request) {
	if s.surveillanceStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "surveillance store not configured", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGetThresholds(w, r)
	case http.MethodPut:
		s.handleSetThreshold(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleListAlerts handles GET /api/v1/securities/surveillance/alerts.
// Optional query params: status, alert_type.
func (s *Server) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filters := store.SurveillanceAlertFilters{
		Status:    types.AlertStatus(q.Get("status")),
		AlertType: types.AlertType(q.Get("alert_type")),
	}
	alerts, err := s.surveillanceStore.ListAlerts(filters)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	if alerts == nil {
		alerts = []types.SurveillanceAlert{}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  alerts,
		"total": len(alerts),
	})
}

// resolveAlertRequest is the request body for PUT .../resolve.
type resolveAlertRequest struct {
	ResolvedBy string `json:"resolved_by"`
}

// handleResolveAlert handles PUT /api/v1/securities/surveillance/alerts/{id}/resolve.
func (s *Server) handleResolveAlert(w http.ResponseWriter, r *http.Request) {
	// Extract alert ID from path: .../alerts/{id}/resolve
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// path is api/v1/securities/surveillance/alerts/{id}/resolve
	if len(parts) < 2 {
		s.writeError(w, http.StatusBadRequest, "INVALID_PATH", "missing alert id", nil)
		return
	}
	alertID := parts[len(parts)-2] // second-to-last segment

	var req resolveAlertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.ResolvedBy = "system"
	}
	if req.ResolvedBy == "" {
		req.ResolvedBy = "system"
	}

	if err := s.surveillanceStore.ResolveAlert(alertID, req.ResolvedBy); err != nil {
		if err.Error() == "not found" || strings.Contains(err.Error(), "not found") {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND", "alert not found", nil)
			return
		}
		s.writeError(w, http.StatusBadRequest, "RESOLVE_FAILED", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "resolved"})
}

// setThresholdRequest is the request body for PUT .../thresholds/{instrumentID}.
type setThresholdRequest struct {
	AlertType types.AlertType `json:"alert_type"`
	Value     float64         `json:"value"`
}

// handleSetThreshold handles PUT /api/v1/securities/surveillance/thresholds/{instrumentID}.
func (s *Server) handleSetThreshold(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	instrumentID := parts[len(parts)-1]
	if instrumentID == "" {
		s.writeError(w, http.StatusBadRequest, "INVALID_PATH", "missing instrument id", nil)
		return
	}

	var req setThresholdRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}
	if req.AlertType == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "alert_type is required", nil)
		return
	}

	threshold := &types.SurveillanceThreshold{
		InstrumentID: instrumentID,
		AlertType:    req.AlertType,
		Value:        req.Value,
		UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	if err := s.surveillanceStore.SetThreshold(threshold); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, threshold)
}

// handleGetThresholds handles GET /api/v1/securities/surveillance/thresholds/{instrumentID}.
func (s *Server) handleGetThresholds(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	instrumentID := parts[len(parts)-1]
	if instrumentID == "" {
		s.writeError(w, http.StatusBadRequest, "INVALID_PATH", "missing instrument id", nil)
		return
	}

	thresholds, err := s.surveillanceStore.GetThresholds(instrumentID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	if thresholds == nil {
		thresholds = []types.SurveillanceThreshold{}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"instrument_id": instrumentID,
		"thresholds":    thresholds,
		"total":         len(thresholds),
	})
}
