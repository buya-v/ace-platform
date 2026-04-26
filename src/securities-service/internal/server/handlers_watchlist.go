// Package server — watch list HTTP handlers.
package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// handleWatchLists dispatches GET /watchlists and POST /watchlists.
func (s *Server) handleWatchLists(w http.ResponseWriter, r *http.Request) {
	if s.watchListStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "watch list store not configured", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleListWatchLists(w, r)
	case http.MethodPost:
		s.handleCreateWatchList(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleWatchList dispatches GET/PUT/DELETE /watchlists/{id}.
func (s *Server) handleWatchList(w http.ResponseWriter, r *http.Request) {
	if s.watchListStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "watch list store not configured", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGetWatchList(w, r)
	case http.MethodPut:
		s.handleUpdateWatchList(w, r)
	case http.MethodDelete:
		s.handleDeleteWatchList(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleListWatchLists handles GET /watchlists.
// Optional query param: owner_id — returns all watch lists; if owner_id is
// provided, filters to lists owned by that user.
func (s *Server) handleListWatchLists(w http.ResponseWriter, r *http.Request) {
	ownerID := r.URL.Query().Get("owner_id")
	lists, err := s.watchListStore.ListByOwner(ownerID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	if lists == nil {
		lists = []types.WatchList{}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  lists,
		"total": len(lists),
	})
}

// createWatchListRequest is the request body for POST /watchlists.
type createWatchListRequest struct {
	Name          string   `json:"name"`
	OwnerID       string   `json:"owner_id"`
	InstrumentIDs []string `json:"instrument_ids"`
	ClientIDs     []string `json:"client_ids"`
	FirmIDs       []string `json:"firm_ids"`
}

// handleCreateWatchList handles POST /watchlists.
func (s *Server) handleCreateWatchList(w http.ResponseWriter, r *http.Request) {
	var req createWatchListRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}
	if req.Name == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "name is required", nil)
		return
	}
	if req.OwnerID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "owner_id is required", nil)
		return
	}
	id, err := newUUID()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate id", nil)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	wl := types.WatchList{
		ID:            id,
		Name:          req.Name,
		OwnerID:       req.OwnerID,
		InstrumentIDs: req.InstrumentIDs,
		ClientIDs:     req.ClientIDs,
		FirmIDs:       req.FirmIDs,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if wl.InstrumentIDs == nil {
		wl.InstrumentIDs = []string{}
	}
	if wl.ClientIDs == nil {
		wl.ClientIDs = []string{}
	}
	if wl.FirmIDs == nil {
		wl.FirmIDs = []string{}
	}
	if err := s.watchListStore.Create(&wl); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusCreated, wl)
}

// handleGetWatchList handles GET /watchlists/{id}.
func (s *Server) handleGetWatchList(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/watchlists/")
	id = strings.TrimSuffix(id, "/")
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_ID", "watch list id is required", nil)
		return
	}
	wl, err := s.watchListStore.Get(id)
	if err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "watch list not found", nil)
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, wl)
}

// updateWatchListRequest is the request body for PUT /watchlists/{id}.
type updateWatchListRequest struct {
	Name          string   `json:"name"`
	InstrumentIDs []string `json:"instrument_ids"`
	ClientIDs     []string `json:"client_ids"`
	FirmIDs       []string `json:"firm_ids"`
}

// handleUpdateWatchList handles PUT /watchlists/{id}.
func (s *Server) handleUpdateWatchList(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/watchlists/")
	id = strings.TrimSuffix(id, "/")
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_ID", "watch list id is required", nil)
		return
	}
	existing, err := s.watchListStore.Get(id)
	if err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "watch list not found", nil)
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	var req updateWatchListRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}
	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.InstrumentIDs != nil {
		existing.InstrumentIDs = req.InstrumentIDs
	}
	if req.ClientIDs != nil {
		existing.ClientIDs = req.ClientIDs
	}
	if req.FirmIDs != nil {
		existing.FirmIDs = req.FirmIDs
	}
	existing.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := s.watchListStore.Update(existing); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, existing)
}

// handleDeleteWatchList handles DELETE /watchlists/{id}.
func (s *Server) handleDeleteWatchList(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/watchlists/")
	id = strings.TrimSuffix(id, "/")
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_ID", "watch list id is required", nil)
		return
	}
	if err := s.watchListStore.Delete(id); err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "watch list not found", nil)
		return
	} else if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
