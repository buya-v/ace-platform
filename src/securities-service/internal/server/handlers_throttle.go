// Package server — per-firm throttle-config HTTP handlers.
package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// handleThrottleConfigs is the collection endpoint:
//
//	GET  /api/v1/securities/throttle-config  — list all configs
func (s *Server) handleThrottleConfigs(w http.ResponseWriter, r *http.Request) {
	if s.throttleConfigStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED",
			"throttle config store not available", nil)
		return
	}
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}
	configs, err := s.throttleConfigStore.List()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	if configs == nil {
		configs = []types.ThrottleConfig{}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  configs,
		"total": len(configs),
	})
}

// handleThrottleConfig is the per-firm endpoint:
//
//	GET    /api/v1/securities/throttle-config/{firm_id} — get config for firm
//	PUT    /api/v1/securities/throttle-config/{firm_id} — set config for firm
//	DELETE /api/v1/securities/throttle-config/{firm_id} — remove config for firm
func (s *Server) handleThrottleConfig(w http.ResponseWriter, r *http.Request) {
	if s.throttleConfigStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED",
			"throttle config store not available", nil)
		return
	}

	// Extract firm_id from the URL path.
	// Path: /api/v1/securities/throttle-config/{firm_id}
	path := strings.TrimSuffix(r.URL.Path, "/")
	firmID := path[strings.LastIndex(path, "/")+1:]
	if firmID == "" || firmID == "throttle-config" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "firm_id is required", nil)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetThrottleConfig(w, r, firmID)
	case http.MethodPut:
		s.handleSetThrottleConfig(w, r, firmID)
	case http.MethodDelete:
		s.handleDeleteThrottleConfig(w, r, firmID)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleGetThrottleConfig handles GET /api/v1/securities/throttle-config/{firm_id}.
func (s *Server) handleGetThrottleConfig(w http.ResponseWriter, r *http.Request, firmID string) {
	cfg, err := s.throttleConfigStore.Get(firmID)
	if err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND",
			"throttle config not found for firm "+firmID, nil)
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, cfg)
}

// putThrottleConfigRequest is the request body for PUT throttle-config.
type putThrottleConfigRequest struct {
	MaxOrdersPerSecond *int `json:"max_orders_per_second"`
	Enabled            *bool `json:"enabled"`
}

// handleSetThrottleConfig handles PUT /api/v1/securities/throttle-config/{firm_id}.
func (s *Server) handleSetThrottleConfig(w http.ResponseWriter, r *http.Request, firmID string) {
	var req putThrottleConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}

	// Fetch existing config or start fresh.
	existing, err := s.throttleConfigStore.Get(firmID)
	if err != nil && err != store.ErrNotFound {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	var cfg types.ThrottleConfig
	if existing != nil {
		cfg = *existing
	} else {
		cfg = types.ThrottleConfig{
			FirmID:             firmID,
			MaxOrdersPerSecond: 100, // default
			Enabled:            true,
		}
	}

	if req.MaxOrdersPerSecond != nil {
		if *req.MaxOrdersPerSecond <= 0 {
			s.writeError(w, http.StatusUnprocessableEntity, "INVALID_FIELD",
				"max_orders_per_second must be greater than 0", nil)
			return
		}
		cfg.MaxOrdersPerSecond = *req.MaxOrdersPerSecond
	}
	if req.Enabled != nil {
		cfg.Enabled = *req.Enabled
	}

	if err := s.throttleConfigStore.Set(&cfg); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	// Re-fetch to return the server-stamped UpdatedAt.
	saved, _ := s.throttleConfigStore.Get(firmID)
	s.writeJSON(w, http.StatusOK, saved)
}

// handleDeleteThrottleConfig handles DELETE /api/v1/securities/throttle-config/{firm_id}.
func (s *Server) handleDeleteThrottleConfig(w http.ResponseWriter, r *http.Request, firmID string) {
	if err := s.throttleConfigStore.Delete(firmID); err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND",
			"throttle config not found for firm "+firmID, nil)
		return
	} else if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
