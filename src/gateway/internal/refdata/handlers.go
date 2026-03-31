package refdata

import (
	"encoding/json"
	"net/http"

	"github.com/garudax-platform/gateway/internal/router"
)

// Handlers provides HTTP handlers for reference data endpoints.
type Handlers struct {
	store Store
}

// NewHandlers creates reference data handlers backed by the given store.
func NewHandlers(store Store) *Handlers {
	return &Handlers{store: store}
}

// RegisterRoutes registers reference data routes on the router.
// These are PUBLIC (no auth) routes for commodity and instrument reference data.
// NOTE: Instrument detail route uses {id} param. The existing /api/v1/instruments/list
// route is registered before this and will match first due to router ordering.
func (h *Handlers) RegisterRoutes(rt *router.Router) {
	rt.Handle("GET", "/api/v1/commodities", h.ListCommodities)
	rt.Handle("GET", "/api/v1/instruments", h.ListInstruments)
	rt.Handle("GET", "/api/v1/instruments/{id}", h.GetInstrument)
}

// ListCommodities handles GET /api/v1/commodities.
func (h *Handlers) ListCommodities(w http.ResponseWriter, r *http.Request) {
	commodities, err := h.store.ListCommodities(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch commodities")
		return
	}

	if commodities == nil {
		commodities = []Commodity{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": commodities,
	})
}

// ListInstruments handles GET /api/v1/instruments with optional ?status= filter.
func (h *Handlers) ListInstruments(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")

	instruments, err := h.store.ListInstruments(r.Context(), status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch instruments")
		return
	}

	if instruments == nil {
		instruments = []Instrument{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": instruments,
	})
}

// GetInstrument handles GET /api/v1/instruments/{id}.
func (h *Handlers) GetInstrument(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Missing instrument id")
		return
	}

	detail, err := h.store.GetInstrument(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch instrument")
		return
	}

	if detail == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Instrument not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": detail,
	})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}
