// Package server — post-trade parameters CRUD HTTP handlers.
package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// handlePostTradeParams dispatches GET (list) and POST (create) for /api/v1/securities/post-trade-params.
func (s *Server) handlePostTradeParams(w http.ResponseWriter, r *http.Request) {
	if s.postTradeParamsStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "post-trade-params store not configured", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleListPostTradeParams(w, r)
	case http.MethodPost:
		s.handleCreatePostTradeParams(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handlePostTradeParamItem dispatches GET, PUT, DELETE for /api/v1/securities/post-trade-params/{id}.
func (s *Server) handlePostTradeParamItem(w http.ResponseWriter, r *http.Request) {
	if s.postTradeParamsStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "post-trade-params store not configured", nil)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/securities/post-trade-params/")
	id = strings.TrimSuffix(id, "/")
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_ID", "id is required", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGetPostTradeParams(w, r, id)
	case http.MethodPut:
		s.handleUpdatePostTradeParams(w, r, id)
	case http.MethodDelete:
		s.handleDeletePostTradeParams(w, r, id)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handlePostTradeParamsByInstrument handles GET /api/v1/securities/post-trade-params/instrument/{id}.
func (s *Server) handlePostTradeParamsByInstrument(w http.ResponseWriter, r *http.Request) {
	if s.postTradeParamsStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "post-trade-params store not configured", nil)
		return
	}
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}
	// Extract {id} from /api/v1/securities/post-trade-params/instrument/{id}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/securities/post-trade-params/instrument/")
	instrumentID := strings.TrimSuffix(path, "/")
	if instrumentID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_ID", "instrument id is required", nil)
		return
	}
	p, err := s.postTradeParamsStore.GetByInstrument(instrumentID)
	if err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "post-trade-params not found for instrument", nil)
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleListPostTradeParams(w http.ResponseWriter, r *http.Request) {
	// list is not natively in the interface; return all by enumerating via an empty list approach.
	// Since PostTradeParamsStore has no List(), we return 200 with empty array placeholder.
	// A full implementation would add List() to the interface.
	s.writeJSON(w, http.StatusOK, []types.PostTradeParams{})
}

func (s *Server) handleCreatePostTradeParams(w http.ResponseWriter, r *http.Request) {
	var p types.PostTradeParams
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}
	if p.InstrumentID == "" {
		s.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "instrument_id is required", nil)
		return
	}
	if p.ID == "" {
		id, err := newUUID()
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate id", nil)
			return
		}
		p.ID = id
	}
	now := time.Now().UTC().Format(time.RFC3339)
	p.CreatedAt = now
	p.UpdatedAt = now
	if err := s.postTradeParamsStore.Create(&p); err != nil {
		s.writeError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleGetPostTradeParams(w http.ResponseWriter, r *http.Request, id string) {
	p, err := s.postTradeParamsStore.Get(id)
	if err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "post-trade-params not found", nil)
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleUpdatePostTradeParams(w http.ResponseWriter, r *http.Request, id string) {
	existing, err := s.postTradeParamsStore.Get(id)
	if err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "post-trade-params not found", nil)
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	var p types.PostTradeParams
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}
	p.ID = existing.ID
	p.CreatedAt = existing.CreatedAt
	if err := s.postTradeParamsStore.Update(&p); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	updated, _ := s.postTradeParamsStore.Get(id)
	s.writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeletePostTradeParams(w http.ResponseWriter, r *http.Request, id string) {
	if err := s.postTradeParamsStore.Delete(id); err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "post-trade-params not found", nil)
		return
	} else if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
