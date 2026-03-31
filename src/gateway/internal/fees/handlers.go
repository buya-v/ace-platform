package fees

import (
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

	// Admin-only routes
	rt.Handle("GET", "/api/v1/admin/fees", h.ListAllSchedules)
	rt.Handle("POST", "/api/v1/admin/fees/rules", h.CreateFeeRule)
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
		RateBPS:           req.RateBPS,
		MinFee:            req.MinFee,
		MaxFee:            req.MaxFee,
		PerContractFee:    req.PerContractFee,
	}

	if err := h.store.CreateRule(r.Context(), rule); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create fee rule")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"data": rule,
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
