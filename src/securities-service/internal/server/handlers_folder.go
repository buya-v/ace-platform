package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// handleFolders dispatches:
//
//	GET  /api/v1/securities/folders
//	POST /api/v1/securities/folders
func (s *Server) handleFolders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListFolders(w, r)
	case http.MethodPost:
		s.handleCreateFolder(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleFolderItem dispatches:
//
//	GET    /api/v1/securities/folders/{id}
//	DELETE /api/v1/securities/folders/{id}
//	GET    /api/v1/securities/folders/{id}/children
func (s *Server) handleFolderItem(w http.ResponseWriter, r *http.Request) {
	suffix := strings.TrimPrefix(r.URL.Path, "/api/v1/securities/folders/")
	suffix = strings.TrimSuffix(suffix, "/")

	if strings.HasSuffix(suffix, "/children") {
		id := strings.TrimSuffix(suffix, "/children")
		if r.Method == http.MethodGet {
			s.handleListFolderChildren(w, r, id)
		} else {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		}
		return
	}

	id := suffix
	switch r.Method {
	case http.MethodGet:
		s.handleGetFolder(w, r, id)
	case http.MethodDelete:
		s.handleDeleteFolder(w, r, id)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

func (s *Server) handleListFolders(w http.ResponseWriter, r *http.Request) {
	if s.folderStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "folder store not configured", nil)
		return
	}
	folders, err := s.folderStore.List()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, folders)
}

func (s *Server) handleCreateFolder(w http.ResponseWriter, r *http.Request) {
	if s.folderStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "folder store not configured", nil)
		return
	}
	var folder types.Folder
	if err := json.NewDecoder(r.Body).Decode(&folder); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body", nil)
		return
	}
	if folder.Name == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "name is required", nil)
		return
	}
	if folder.ID == "" {
		folder.ID = fmt.Sprintf("fld-%d", time.Now().UnixNano())
	}
	if folder.CreatedAt == "" {
		folder.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if err := s.folderStore.Create(&folder); err != nil {
		s.writeError(w, http.StatusConflict, "ALREADY_EXISTS", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusCreated, folder)
}

func (s *Server) handleGetFolder(w http.ResponseWriter, r *http.Request, id string) {
	if s.folderStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "folder store not configured", nil)
		return
	}
	folder, err := s.folderStore.Get(id)
	if err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "folder not found", nil)
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, folder)
}

func (s *Server) handleDeleteFolder(w http.ResponseWriter, r *http.Request, id string) {
	if s.folderStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "folder store not configured", nil)
		return
	}
	if err := s.folderStore.Delete(id); err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "folder not found", nil)
		return
	} else if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListFolderChildren(w http.ResponseWriter, r *http.Request, parentID string) {
	if s.folderStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "folder store not configured", nil)
		return
	}
	children, err := s.folderStore.ListChildren(parentID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, children)
}
