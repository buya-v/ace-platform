// Package server — client entity CRUD HTTP handlers.
package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// handleClients dispatches GET (list) and POST (create) for /api/v1/securities/clients.
func (s *Server) handleClients(w http.ResponseWriter, r *http.Request) {
	if s.clientStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "client store not configured", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleListClients(w, r)
	case http.MethodPost:
		s.handleCreateClient(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleClientItem dispatches GET and DELETE for /api/v1/securities/clients/{id}.
func (s *Server) handleClientItem(w http.ResponseWriter, r *http.Request) {
	if s.clientStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "client store not configured", nil)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/securities/clients/")
	id = strings.TrimSuffix(id, "/")
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_ID", "id is required", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGetClient(w, r, id)
	case http.MethodDelete:
		s.handleDeleteClient(w, r, id)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

func (s *Server) handleListClients(w http.ResponseWriter, r *http.Request) {
	firmID := r.URL.Query().Get("firm_id")
	clients, err := s.clientStore.ListByFirm(firmID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, clients)
}

func (s *Server) handleCreateClient(w http.ResponseWriter, r *http.Request) {
	var c types.Client
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}
	if c.FirmID == "" {
		s.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "firm_id is required", nil)
		return
	}
	if c.Name == "" {
		s.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name is required", nil)
		return
	}
	if c.ClientType == "" {
		s.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "client_type is required", nil)
		return
	}
	if c.ID == "" {
		id, err := newUUID()
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate id", nil)
			return
		}
		c.ID = id
	}
	c.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := s.clientStore.Create(&c); err != nil {
		s.writeError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusCreated, c)
}

func (s *Server) handleGetClient(w http.ResponseWriter, r *http.Request, id string) {
	c, err := s.clientStore.Get(id)
	if err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "client not found", nil)
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleDeleteClient(w http.ResponseWriter, r *http.Request, id string) {
	if err := s.clientStore.Delete(id); err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "client not found", nil)
		return
	} else if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
