// Package server — corporate actions HTTP handlers.
package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// handleCorporateActions dispatches GET and POST for /api/v1/securities/corporate-actions.
func (s *Server) handleCorporateActions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListCorporateActions(w, r)
	case http.MethodPost:
		s.handleAnnounceCorporateAction(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleCorporateAction dispatches GET and the /process sub-resource for
// /api/v1/securities/corporate-actions/{id}.
func (s *Server) handleCorporateAction(w http.ResponseWriter, r *http.Request) {
	// Detect the /process sub-resource.
	path := strings.TrimSuffix(r.URL.Path, "/")
	if strings.HasSuffix(path, "/process") {
		if r.Method == http.MethodPost {
			s.handleProcessCorporateAction(w, r)
		} else {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetCorporateAction(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleListCorporateActions handles GET /api/v1/securities/corporate-actions.
//
// Query parameters:
//
//	instrument_id — filter by instrument
//	action_type   — filter by CorporateActionType
func (s *Server) handleListCorporateActions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filters := store.CorporateActionFilters{
		InstrumentID: q.Get("instrument_id"),
		ActionType:   types.CorporateActionType(q.Get("action_type")),
	}

	actions, err := s.corporateActionStore.List(filters)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	if actions == nil {
		actions = []types.CorporateAction{}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"corporate_actions": actions,
		"count":             len(actions),
	})
}

// handleAnnounceCorporateAction handles POST /api/v1/securities/corporate-actions.
func (s *Server) handleAnnounceCorporateAction(w http.ResponseWriter, r *http.Request) {
	var req types.CorporateAction
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}

	// Validate required fields.
	if req.InstrumentID == "" {
		s.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "instrument_id is required", nil)
		return
	}
	if req.ActionType == "" {
		s.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "action_type is required", nil)
		return
	}

	// Generate ID and timestamps.
	id, err := newUUID()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate id", nil)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	req.ID = id
	req.Status = types.CAStatusAnnounced
	req.CreatedAt = now
	req.UpdatedAt = now
	if req.Details == nil {
		req.Details = map[string]interface{}{}
	}

	if err := s.corporateActionStore.Create(&req); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusCreated, req)
}

// handleGetCorporateAction handles GET /api/v1/securities/corporate-actions/{id}.
func (s *Server) handleGetCorporateAction(w http.ResponseWriter, r *http.Request) {
	id := extractID(r.URL.Path, "/api/v1/securities/corporate-actions/")
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "missing id", nil)
		return
	}

	ca, err := s.corporateActionStore.Get(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "corporate action not found", nil)
		return
	}
	s.writeJSON(w, http.StatusOK, ca)
}

// handleProcessCorporateAction handles POST /api/v1/securities/corporate-actions/{id}/process.
//
// For CA_DIVIDEND: looks up positions for the instrument, creates entitlements
// (quantity * dividend_amount from Details["dividend_amount"]).
//
// For CA_STOCK_SPLIT: adjusts position quantities (quantity * split_ratio from
// Details["split_ratio"]).
func (s *Server) handleProcessCorporateAction(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path: /api/v1/securities/corporate-actions/{id}/process
	path := strings.TrimSuffix(r.URL.Path, "/")
	withoutProcess := strings.TrimSuffix(path, "/process")
	id := extractID(withoutProcess+"/", "/api/v1/securities/corporate-actions/")
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "missing id", nil)
		return
	}

	ca, err := s.corporateActionStore.Get(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "corporate action not found", nil)
		return
	}

	if ca.Status != types.CAStatusAnnounced {
		s.writeError(w, http.StatusConflict, "INVALID_STATE",
			"corporate action can only be processed from ANNOUNCED state", nil)
		return
	}

	// Mark as PROCESSING.
	if err := s.corporateActionStore.UpdateStatus(id, types.CAStatusProcessing); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	result := map[string]interface{}{"corporate_action_id": id, "processed_at": now}

	switch ca.ActionType {
	case types.CA_DIVIDEND:
		entitlements, procErr := s.processDividend(ca, now)
		if procErr != nil {
			_ = s.corporateActionStore.UpdateStatus(id, types.CAStatusAnnounced) // rollback status
			s.writeError(w, http.StatusInternalServerError, "PROCESSING_ERROR", procErr.Error(), nil)
			return
		}
		_ = s.corporateActionStore.UpdateStatus(id, types.CAStatusCompleted)
		result["action_type"] = string(types.CA_DIVIDEND)
		result["entitlements_created"] = len(entitlements)

	case types.CA_STOCK_SPLIT:
		count, procErr := s.processStockSplit(ca)
		if procErr != nil {
			_ = s.corporateActionStore.UpdateStatus(id, types.CAStatusAnnounced) // rollback status
			s.writeError(w, http.StatusInternalServerError, "PROCESSING_ERROR", procErr.Error(), nil)
			return
		}
		_ = s.corporateActionStore.UpdateStatus(id, types.CAStatusCompleted)
		result["action_type"] = string(types.CA_STOCK_SPLIT)
		result["positions_adjusted"] = count

	default:
		// For CA_RIGHTS_ISSUE and CA_MERGER: mark completed (manual processing).
		_ = s.corporateActionStore.UpdateStatus(id, types.CAStatusCompleted)
		result["action_type"] = string(ca.ActionType)
		result["note"] = "manual processing required"
	}

	s.writeJSON(w, http.StatusOK, result)
}

// processDividend creates entitlements for all positions in the instrument.
// dividend_amount must be present in ca.Details["dividend_amount"].
func (s *Server) processDividend(ca *types.CorporateAction, now string) ([]types.Entitlement, error) {
	dividendAmount := 0.0
	if v, ok := ca.Details["dividend_amount"]; ok {
		switch val := v.(type) {
		case float64:
			dividendAmount = val
		case int:
			dividendAmount = float64(val)
		}
	}

	// List all positions for this instrument.
	positions, err := s.positionStore.List("")
	if err != nil {
		return nil, err
	}

	entitlements := make([]types.Entitlement, 0)
	for _, pos := range positions {
		if pos.InstrumentID != ca.InstrumentID || pos.Quantity <= 0 {
			continue
		}

		id, err := newUUID()
		if err != nil {
			return nil, err
		}

		e := types.Entitlement{
			ID:                id,
			CorporateActionID: ca.ID,
			ParticipantID:     pos.ParticipantID,
			InstrumentID:      ca.InstrumentID,
			Quantity:          pos.Quantity,
			EntitlementValue:  float64(pos.Quantity) * dividendAmount,
			Status:            types.EntitlementStatusPending,
			CreatedAt:         now,
		}
		if err := s.entitlementStore.Create(&e); err != nil {
			return nil, err
		}
		entitlements = append(entitlements, e)
	}
	return entitlements, nil
}

// processStockSplit adjusts all positions for the instrument by the split ratio.
// split_ratio must be present in ca.Details["split_ratio"].
func (s *Server) processStockSplit(ca *types.CorporateAction) (int, error) {
	splitRatio := 1.0
	if v, ok := ca.Details["split_ratio"]; ok {
		switch val := v.(type) {
		case float64:
			splitRatio = val
		case int:
			splitRatio = float64(val)
		}
	}

	positions, err := s.positionStore.List("")
	if err != nil {
		return 0, err
	}

	count := 0
	for _, pos := range positions {
		if pos.InstrumentID != ca.InstrumentID {
			continue
		}
		pos.Quantity = int(float64(pos.Quantity) * splitRatio)
		if err := s.positionStore.Update(&pos); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// extractID extracts the path segment after the given prefix.
// e.g. extractID("/api/v1/securities/corporate-actions/abc-123/", "/api/v1/securities/corporate-actions/") == "abc-123"
func extractID(path, prefix string) string {
	after := strings.TrimPrefix(path, prefix)
	// Strip any trailing sub-paths (e.g. "/process").
	parts := strings.SplitN(after, "/", 2)
	return parts[0]
}
