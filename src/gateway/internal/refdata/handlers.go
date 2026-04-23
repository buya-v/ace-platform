package refdata

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/garudax-platform/gateway/internal/middleware"
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

// RegisterAdminRoutes registers admin-only write routes for reference data.
// All routes under /api/v1/admin/ require admin or exchange_admin role
// (enforced per handler via middleware.ClaimsFromContext).
func (h *Handlers) RegisterAdminRoutes(rt *router.Router) {
	rt.Handle("POST", "/api/v1/admin/instruments", h.CreateInstrument)
	rt.Handle("PUT", "/api/v1/admin/instruments/{id}", h.UpdateInstrument)
	rt.Handle("POST", "/api/v1/admin/commodities", h.CreateCommodity)
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
		// Try path parameter {id} from the router
		id = r.PathValue("id")
	}
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

// CreateInstrument handles POST /api/v1/admin/instruments.
// Creates a new tradeable instrument. Requires admin or exchange_admin role.
func (h *Handlers) CreateInstrument(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil || !claims.HasAnyRole("admin", "exchange_admin") {
		writeError(w, http.StatusForbidden, "PERMISSION_DENIED", "Admin role required")
		return
	}

	var input InstrumentInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Invalid request body")
		return
	}

	if input.CommodityID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Missing commodity_id")
		return
	}
	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Missing name")
		return
	}
	if input.Currency == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Missing currency")
		return
	}
	if input.ContractSize == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Missing contract_size")
		return
	}
	if input.TickSize == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Missing tick_size")
		return
	}

	// Auto-generate ID if not provided.
	if input.ID == "" {
		b := make([]byte, 8)
		rand.Read(b) //nolint:errcheck
		input.ID = "INST-" + hex.EncodeToString(b)
	}
	if input.SettlementType == "" {
		input.SettlementType = "physical"
	}

	inst, err := h.store.CreateInstrument(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create instrument")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"data": inst,
	})
}

// UpdateInstrument handles PUT /api/v1/admin/instruments/{id}.
// Applies a partial update to an existing instrument. Requires admin or exchange_admin role.
func (h *Handlers) UpdateInstrument(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil || !claims.HasAnyRole("admin", "exchange_admin") {
		writeError(w, http.StatusForbidden, "PERMISSION_DENIED", "Admin role required")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		id = r.URL.Query().Get("id")
	}
	if id == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Missing instrument id")
		return
	}

	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Invalid request body")
		return
	}

	if len(updates) == 0 {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "No fields to update")
		return
	}

	inst, err := h.store.UpdateInstrument(r.Context(), id, updates)
	if err != nil {
		if err.Error() == "instrument not found" {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Instrument not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update instrument")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": inst,
	})
}

// CreateCommodity handles POST /api/v1/admin/commodities.
// Creates a new commodity. Requires admin or exchange_admin role.
func (h *Handlers) CreateCommodity(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil || !claims.HasAnyRole("admin", "exchange_admin") {
		writeError(w, http.StatusForbidden, "PERMISSION_DENIED", "Admin role required")
		return
	}

	var input CommodityInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Invalid request body")
		return
	}

	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Missing name")
		return
	}
	if input.Category == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Missing category")
		return
	}
	if input.Unit == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Missing unit")
		return
	}

	// Auto-generate ID if not provided.
	if input.ID == "" {
		b := make([]byte, 6)
		rand.Read(b) //nolint:errcheck
		input.ID = "COMM-" + hex.EncodeToString(b)
	}

	commodity, err := h.store.CreateCommodity(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create commodity")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"data": commodity,
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
