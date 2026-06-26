package fees

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/garudax-platform/gateway/internal/middleware"
	"github.com/garudax-platform/gateway/internal/router"
)

// Handlers provides HTTP handlers for fee management endpoints.
type Handlers struct {
	store Store
}

// NewHandlers creates fee handlers backed by the given store.
func NewHandlers(store Store) *Handlers {
	return &Handlers{store: store}
}

// RegisterRoutes registers fee routes on the router.
func (h *Handlers) RegisterRoutes(rt *router.Router) {
	// Public (authenticated) routes
	rt.Handle("GET", "/api/v1/fees/schedule", h.ListActiveSchedules)
	rt.Handle("GET", "/api/v1/fees/my-fees", h.GetMyFees)

	// Admin-only read routes
	rt.Handle("GET", "/api/v1/admin/fees", h.ListAllSchedules)
	rt.Handle("POST", "/api/v1/admin/fees/rules", h.CreateFeeRule)
}

// RegisterAdminRoutes registers additional admin write routes for fee management.
func (h *Handlers) RegisterAdminRoutes(rt *router.Router) {
	rt.Handle("POST", "/api/v1/admin/fees/schedules", h.CreateSchedule)
	rt.Handle("PUT", "/api/v1/admin/fees/rules/{id}", h.UpdateFeeRule)
	rt.Handle("PUT", "/api/v1/admin/fees/tiers/{participant_id}", h.SetParticipantTier)
}

// ListActiveSchedules handles GET /api/v1/fees/schedule.
// Returns active fee schedules with their rules.
func (h *Handlers) ListActiveSchedules(w http.ResponseWriter, r *http.Request) {
	schedules, err := h.store.ListActiveSchedules(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch fee schedules")
		return
	}

	if schedules == nil {
		schedules = []FeeSchedule{}
	}

	// Attach rules to each schedule
	for i := range schedules {
		rules, err := h.store.GetRulesForSchedule(r.Context(), schedules[i].ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch fee rules")
			return
		}
		if rules == nil {
			rules = []FeeRule{}
		}
		schedules[i].Rules = rules
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": schedules,
	})
}

// GetMyFees handles GET /api/v1/fees/my-fees?from=&to=.
// Returns the authenticated participant's fee transactions.
func (h *Handlers) GetMyFees(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication required")
		return
	}

	participantID := claims.ParticipantID
	if participantID == "" {
		participantID = claims.Sub
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	txns, err := h.store.ListFeeTransactions(r.Context(), participantID, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch fee transactions")
		return
	}

	if txns == nil {
		txns = []FeeTransaction{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": txns,
	})
}

// ListAllSchedules handles GET /api/v1/admin/fees.
// Returns all fee schedules (admin only - auth checked by middleware).
func (h *Handlers) ListAllSchedules(w http.ResponseWriter, r *http.Request) {
	schedules, err := h.store.ListAllSchedules(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch fee schedules")
		return
	}

	if schedules == nil {
		schedules = []FeeSchedule{}
	}

	for i := range schedules {
		rules, err := h.store.GetRulesForSchedule(r.Context(), schedules[i].ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch fee rules")
			return
		}
		if rules == nil {
			rules = []FeeRule{}
		}
		schedules[i].Rules = rules
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": schedules,
	})
}

// CreateFeeRuleRequest is the payload for creating a new fee rule.
type CreateFeeRuleRequest struct {
	ID                string   `json:"id"`
	ScheduleID        string   `json:"schedule_id"`
	FeeType           string   `json:"fee_type"`
	InstrumentPattern string   `json:"instrument_pattern"`
	ParticipantTier   string   `json:"participant_tier"`
	RateBPS           float64  `json:"rate_bps"`
	MinFee            float64  `json:"min_fee"`
	MaxFee            *float64 `json:"max_fee,omitempty"`
	PerContractFee    float64  `json:"per_contract_fee"`
}

// CreateFeeRule handles POST /api/v1/admin/fees/rules.
// Creates a new fee rule (admin only).
func (h *Handlers) CreateFeeRule(w http.ResponseWriter, r *http.Request) {
	var req CreateFeeRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Invalid request body")
		return
	}

	if req.ID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Missing rule id")
		return
	}
	if req.ScheduleID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Missing schedule_id")
		return
	}
	if req.FeeType == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Missing fee_type")
		return
	}
	if !isValidFeeType(req.FeeType) {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "fee_type must be one of: trading, clearing, data, membership")
		return
	}
	if req.RateBPS < 0 {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "rate_bps must be non-negative")
		return
	}

	if req.InstrumentPattern == "" {
		req.InstrumentPattern = "*"
	}
	if req.ParticipantTier == "" {
		req.ParticipantTier = "*"
	}

	rule := FeeRule{
		ID:                req.ID,
		ScheduleID:        req.ScheduleID,
		FeeType:           req.FeeType,
		InstrumentPattern: req.InstrumentPattern,
		ParticipantTier:   req.ParticipantTier,
		RateBPS:           decFromFloat(req.RateBPS),
		MinFee:            decFromFloat(req.MinFee),
		PerContractFee:    decFromFloat(req.PerContractFee),
	}
	if req.MaxFee != nil {
		m := decFromFloat(*req.MaxFee)
		rule.MaxFee = &m
	}

	if err := h.store.CreateRule(r.Context(), rule); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create fee rule")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"data": rule,
	})
}

// CreateSchedule handles POST /api/v1/admin/fees/schedules.
// Creates a new fee schedule. Requires admin or exchange_admin role.
func (h *Handlers) CreateSchedule(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil || !claims.HasAnyRole("admin", "exchange_admin") {
		writeError(w, http.StatusForbidden, "PERMISSION_DENIED", "Admin role required")
		return
	}

	var input FeeScheduleInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Invalid request body")
		return
	}

	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Missing name")
		return
	}

	if input.ID == "" {
		b := make([]byte, 8)
		rand.Read(b) //nolint:errcheck
		input.ID = "SCHED-" + hex.EncodeToString(b)
	}
	if input.Status == "" {
		input.Status = "ACTIVE"
	}

	schedule, err := h.store.CreateSchedule(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create fee schedule")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"data": schedule,
	})
}

// UpdateFeeRule handles PUT /api/v1/admin/fees/rules/{id}.
// Applies a partial update to an existing fee rule. Requires admin or exchange_admin role.
func (h *Handlers) UpdateFeeRule(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil || !claims.HasAnyRole("admin", "exchange_admin") {
		writeError(w, http.StatusForbidden, "PERMISSION_DENIED", "Admin role required")
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Missing rule id")
		return
	}

	var updates FeeRuleUpdate
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Invalid request body")
		return
	}

	rule, err := h.store.UpdateRule(r.Context(), id, updates)
	if err != nil {
		if err.Error() == "fee rule not found" {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Fee rule not found")
			return
		}
		if err.Error() == "no updatable fields provided" {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "No fields to update")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update fee rule")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": rule,
	})
}

// SetParticipantTier handles PUT /api/v1/admin/fees/tiers/{participant_id}.
// Upserts a participant's fee tier. Requires admin or exchange_admin role.
func (h *Handlers) SetParticipantTier(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil || !claims.HasAnyRole("admin", "exchange_admin") {
		writeError(w, http.StatusForbidden, "PERMISSION_DENIED", "Admin role required")
		return
	}

	participantID := r.URL.Query().Get("participant_id")
	if participantID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Missing participant_id")
		return
	}

	var body struct {
		Tier string `json:"tier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Invalid request body")
		return
	}
	if body.Tier == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Missing tier")
		return
	}

	if err := h.store.SetParticipantTier(r.Context(), participantID, body.Tier); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to set participant tier")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]string{
			"participant_id": participantID,
			"tier":           body.Tier,
		},
	})
}

func isValidFeeType(ft string) bool {
	switch ft {
	case "trading", "clearing", "data", "membership":
		return true
	}
	return false
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
