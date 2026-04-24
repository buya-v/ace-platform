// Package server — instrument CRUD HTTP handlers.
package server

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// newUUID generates a random UUID v4 string using crypto/rand.
func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	// Set version 4 and variant bits (RFC 4122).
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// handleListInstruments handles GET /api/v1/securities/instruments.
//
// Query parameters:
//
//	asset_class    — filter by AssetClass enum value
//	trading_status — filter by TradingStatus enum value
//	exchange_code  — filter by exchange MIC code
//	search         — case-insensitive match against ticker or name
//	limit          — page size (default 50)
//	offset         — number of records to skip (default 0)
func (s *Server) handleListInstruments(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	filters := store.InstrumentFilters{
		AssetClass:    types.AssetClass(q.Get("asset_class")),
		TradingStatus: types.TradingStatus(q.Get("trading_status")),
		ExchangeCode:  q.Get("exchange_code"),
	}

	limit := 50
	if lStr := q.Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}
	offset := 0
	if oStr := q.Get("offset"); oStr != "" {
		if o, err := strconv.Atoi(oStr); err == nil && o >= 0 {
			offset = o
		}
	}
	search := strings.ToLower(q.Get("search"))

	all, err := s.instrumentStore.List(filters)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	// Apply in-memory search filter.
	if search != "" {
		filtered := all[:0]
		for _, inst := range all {
			if strings.Contains(strings.ToLower(inst.Ticker), search) ||
				strings.Contains(strings.ToLower(inst.Name), search) {
				filtered = append(filtered, inst)
			}
		}
		all = filtered
	}

	total := len(all)

	// Apply offset.
	if offset >= total {
		all = []types.Instrument{}
	} else {
		all = all[offset:]
	}

	// Apply limit.
	if len(all) > limit {
		all = all[:limit]
	}

	// Ensure JSON array is never null.
	if all == nil {
		all = []types.Instrument{}
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":   all,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// handleGetInstrument handles GET /api/v1/securities/instruments/{id}.
// Extracts the ID as the last path segment.
func (s *Server) handleGetInstrument(w http.ResponseWriter, r *http.Request) {
	id := extractLastSegment(r.URL.Path)
	if id == "" || id == "status" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "instrument id is required", nil)
		return
	}

	inst, err := s.instrumentStore.Get(id)
	if err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND",
				fmt.Sprintf("instrument %s not found", id), nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, inst)
}

// handleCreateInstrument handles POST /api/v1/securities/instruments.
//
// Validation rules:
//   - ticker, name, asset_class are required
//   - lot_size must be > 0
//   - tick_size must be > 0
//
// Side effects:
//   - Generates a UUID v4 for ID
//   - Sets CreatedAt and UpdatedAt to the current UTC time
//   - Defaults TradingStatus to ACTIVE if not provided
func (s *Server) handleCreateInstrument(w http.ResponseWriter, r *http.Request) {
	var inst types.Instrument
	if err := json.NewDecoder(r.Body).Decode(&inst); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}

	// Validate required fields.
	if inst.Ticker == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "ticker is required", nil)
		return
	}
	if inst.Name == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "name is required", nil)
		return
	}
	if inst.AssetClass == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "asset_class is required", nil)
		return
	}
	if inst.LotSize <= 0 {
		s.writeError(w, http.StatusBadRequest, "INVALID_FIELD", "lot_size must be greater than 0", nil)
		return
	}
	if inst.TickSize <= 0 {
		s.writeError(w, http.StatusBadRequest, "INVALID_FIELD", "tick_size must be greater than 0", nil)
		return
	}

	// Generate UUID and timestamps.
	id, err := newUUID()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate id", nil)
		return
	}
	inst.ID = id

	now := time.Now().UTC().Format(time.RFC3339)
	inst.CreatedAt = now
	inst.UpdatedAt = now

	// Default trading status.
	if inst.TradingStatus == "" {
		inst.TradingStatus = types.TradingStatusActive
	}

	if err := s.instrumentStore.Create(&inst); err != nil {
		s.writeError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusCreated, inst)
}

// handleUpdateInstrument handles PATCH /api/v1/securities/instruments/{id}.
// Only non-zero fields in the request body are applied (partial update semantics).
func (s *Server) handleUpdateInstrument(w http.ResponseWriter, r *http.Request) {
	id := extractLastSegment(r.URL.Path)
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "instrument id is required", nil)
		return
	}

	var partial store.InstrumentUpdate
	if err := json.NewDecoder(r.Body).Decode(&partial); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}

	if err := s.instrumentStore.Update(id, partial); err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND",
				fmt.Sprintf("instrument %s not found", id), nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	inst, err := s.instrumentStore.Get(id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, inst)
}

// setInstrumentStatusRequest is the request body for PUT .../instruments/{id}/status.
type setInstrumentStatusRequest struct {
	Status            types.TradingStatus `json:"status"`
	Reason            string              `json:"reason"`
	ResumeWithAuction bool                `json:"resume_with_auction"`
}

// instrumentStatusResponse is the response body for status-change operations.
type instrumentStatusResponse struct {
	InstrumentID   string              `json:"instrument_id"`
	PreviousStatus types.TradingStatus `json:"previous_status"`
	CurrentStatus  types.TradingStatus `json:"current_status"`
	Reason         string              `json:"reason"`
	ChangedAt      string              `json:"changed_at"`
}

// validTradingStatuses is the set of accepted TradingStatus enum values.
var validTradingStatuses = map[types.TradingStatus]bool{
	types.TradingStatusActive:    true,
	types.TradingStatusHalted:    true,
	types.TradingStatusSuspended: true,
	types.TradingStatusDelisted:  true,
}

// handleUpdateInstrumentStatus handles PUT /api/v1/securities/instruments/{id}/status.
// Validates that the requested status is a valid TradingStatus enum value.
func (s *Server) handleUpdateInstrumentStatus(w http.ResponseWriter, r *http.Request) {
	// Path is .../instruments/{id}/status — id is the second-to-last segment.
	path := strings.TrimSuffix(r.URL.Path, "/")
	segments := strings.Split(path, "/")
	// Expect [..., "instruments", "{id}", "status"]
	if len(segments) < 2 || segments[len(segments)-1] != "status" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "instrument id is required", nil)
		return
	}
	id := segments[len(segments)-2]
	if id == "" || id == "instruments" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "instrument id is required", nil)
		return
	}

	var req setInstrumentStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}

	if req.Status == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "status is required", nil)
		return
	}
	if !validTradingStatuses[req.Status] {
		s.writeError(w, http.StatusBadRequest, "INVALID_FIELD",
			fmt.Sprintf("invalid status %q: must be one of ACTIVE, HALTED, SUSPENDED, DELISTED", req.Status), nil)
		return
	}

	// Fetch current state before updating.
	current, err := s.instrumentStore.Get(id)
	if err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND",
				fmt.Sprintf("instrument %s not found", id), nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	previousStatus := current.TradingStatus

	if err := s.instrumentStore.UpdateStatus(id, req.Status); err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND",
				fmt.Sprintf("instrument %s not found", id), nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	changedAt := time.Now().UTC().Format(time.RFC3339)
	resp := instrumentStatusResponse{
		InstrumentID:   id,
		PreviousStatus: previousStatus,
		CurrentStatus:  req.Status,
		Reason:         req.Reason,
		ChangedAt:      changedAt,
	}

	// Audit log: instrument status updated (best-effort).
	if s.auditStore != nil {
		entryID, _ := newUUID()
		_ = s.auditStore.Log(types.AuditEntry{
			ID:         entryID,
			EntityType: "INSTRUMENT",
			EntityID:   id,
			Action:     "UPDATE",
			ActorID:    "system",
			Timestamp:  changedAt,
			Detail:     string(req.Status),
		})
	}

	s.writeJSON(w, http.StatusOK, resp)
}

// handleDeleteInstrument handles DELETE /api/v1/securities/instruments/{id}.
//
// Soft-deletes an instrument by setting DeletionStatus to "FLAGGED" and
// DeletionDate to 30 days from now. The instrument record is NOT removed from
// the store — it remains readable and tradeable until the deletion date is
// processed by a scheduled job.
func (s *Server) handleDeleteInstrument(w http.ResponseWriter, r *http.Request) {
	id := extractLastSegment(r.URL.Path)
	if id == "" || id == "status" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "instrument id is required", nil)
		return
	}

	inst, err := s.instrumentStore.Get(id)
	if err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND",
				fmt.Sprintf("instrument %s not found", id), nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	// Flag for deletion 30 calendar days from now.
	deletionDate := time.Now().UTC().AddDate(0, 0, 30).Format("2006-01-02")
	partial := store.InstrumentUpdate{
		// Carry through existing values so the partial update doesn't zero them.
		Name:              inst.Name,
		TradingStatus:     inst.TradingStatus,
		LotSize:           inst.LotSize,
		TickSize:          inst.TickSize,
		OutstandingShares: inst.OutstandingShares,
		DeletionStatus:    "FLAGGED",
		DeletionDate:      deletionDate,
	}

	if err := s.instrumentStore.Update(id, partial); err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND",
				fmt.Sprintf("instrument %s not found", id), nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	updated, err := s.instrumentStore.Get(id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, updated)
}

// extractLastSegment returns the last non-empty path segment of rawPath.
// e.g. "/api/v1/securities/instruments/abc-123" → "abc-123"
func extractLastSegment(rawPath string) string {
	trimmed := strings.TrimSuffix(rawPath, "/")
	idx := strings.LastIndex(trimmed, "/")
	if idx < 0 {
		return trimmed
	}
	return trimmed[idx+1:]
}
