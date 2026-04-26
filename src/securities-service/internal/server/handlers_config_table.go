// Package server — config table CRUD HTTP handlers.
package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// handleConfigTables dispatches GET (list) and POST (create) for /api/v1/securities/config-tables.
func (s *Server) handleConfigTables(w http.ResponseWriter, r *http.Request) {
	if s.configTableStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "config table store not configured", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleListConfigTables(w, r)
	case http.MethodPost:
		s.handleCreateConfigTable(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleConfigTableItem dispatches GET, PUT, DELETE for /api/v1/securities/config-tables/{id}.
func (s *Server) handleConfigTableItem(w http.ResponseWriter, r *http.Request) {
	if s.configTableStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "config table store not configured", nil)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/securities/config-tables/")
	id = strings.TrimSuffix(id, "/")
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_ID", "id is required", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGetConfigTable(w, r, id)
	case http.MethodPut:
		s.handleUpdateConfigTable(w, r, id)
	case http.MethodDelete:
		s.handleDeleteConfigTable(w, r, id)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

func (s *Server) handleListConfigTables(w http.ResponseWriter, r *http.Request) {
	tableType := types.ConfigTableType(r.URL.Query().Get("table_type"))
	tables, err := s.configTableStore.ListByType(tableType)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, tables)
}

func (s *Server) handleCreateConfigTable(w http.ResponseWriter, r *http.Request) {
	var t types.ConfigTable
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}
	if t.Name == "" {
		s.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name is required", nil)
		return
	}
	if t.TableType == "" {
		s.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "table_type is required", nil)
		return
	}
	if t.ID == "" {
		id, err := newUUID()
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate id", nil)
			return
		}
		t.ID = id
	}
	now := time.Now().UTC().Format(time.RFC3339)
	t.CreatedAt = now
	t.UpdatedAt = now
	if t.Rows == nil {
		t.Rows = []map[string]interface{}{}
	}
	if err := s.configTableStore.Create(&t); err != nil {
		s.writeError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handleGetConfigTable(w http.ResponseWriter, r *http.Request, id string) {
	t, err := s.configTableStore.Get(id)
	if err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "config table not found", nil)
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleUpdateConfigTable(w http.ResponseWriter, r *http.Request, id string) {
	existing, err := s.configTableStore.Get(id)
	if err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "config table not found", nil)
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	var t types.ConfigTable
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}
	t.ID = existing.ID
	t.CreatedAt = existing.CreatedAt
	if t.Rows == nil {
		t.Rows = []map[string]interface{}{}
	}
	if err := s.configTableStore.Update(&t); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	updated, _ := s.configTableStore.Get(id)
	s.writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteConfigTable(w http.ResponseWriter, r *http.Request, id string) {
	if err := s.configTableStore.Delete(id); err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "config table not found", nil)
		return
	} else if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
