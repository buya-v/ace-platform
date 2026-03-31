package reporting

import (
	"encoding/json"
	"net/http"

	"github.com/garudax-platform/gateway/internal/middleware"
	"github.com/garudax-platform/gateway/internal/router"
)

// Handlers provides HTTP handlers for reporting endpoints.
type Handlers struct {
	store Store
}

// NewHandlers creates reporting handlers backed by the given store.
func NewHandlers(store Store) *Handlers {
	return &Handlers{store: store}
}

// RegisterRoutes registers reporting routes on the router.
func (h *Handlers) RegisterRoutes(rt *router.Router) {
	// Authenticated participant routes
	rt.Handle("GET", "/api/v1/reports/settlement-statement", h.GetSettlementStatement)
	rt.Handle("GET", "/api/v1/reports/trade-summary", h.GetTradeSummary)

	// Admin-only routes
	rt.Handle("GET", "/api/v1/admin/reports/market-summary", h.GetMarketSummary)
	rt.Handle("GET", "/api/v1/admin/reports/large-traders", h.GetLargeTraders)
}

// GetSettlementStatement handles GET /api/v1/reports/settlement-statement?date=YYYY-MM-DD.
// Returns the authenticated participant's daily settlement statement.
func (h *Handlers) GetSettlementStatement(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication required")
		return
	}

	participantID := claims.ParticipantID
	if participantID == "" {
		participantID = claims.Sub
	}

	date := r.URL.Query().Get("date")
	if date == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Missing required parameter: date (YYYY-MM-DD)")
		return
	}
	if !isValidDate(date) {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Invalid date format, expected YYYY-MM-DD")
		return
	}

	stmt, err := h.store.GetDailyStatement(r.Context(), participantID, date)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch settlement statement")
		return
	}
	if stmt == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "No settlement statement found for the given date")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": stmt,
	})
}

// GetTradeSummary handles GET /api/v1/reports/trade-summary?from=YYYY-MM-DD&to=YYYY-MM-DD.
// Returns the authenticated participant's trades within the date range.
func (h *Handlers) GetTradeSummary(w http.ResponseWriter, r *http.Request) {
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

	if from == "" || to == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Missing required parameters: from and to (YYYY-MM-DD)")
		return
	}
	if !isValidDate(from) || !isValidDate(to) {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Invalid date format, expected YYYY-MM-DD")
		return
	}

	trades, err := h.store.ListTradesForParticipant(r.Context(), participantID, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch trade summary")
		return
	}

	if trades == nil {
		trades = []json.RawMessage{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  trades,
		"count": len(trades),
	})
}

// GetMarketSummary handles GET /api/v1/admin/reports/market-summary?date=YYYY-MM-DD.
// Returns market summaries for all instruments on the given date (admin only).
func (h *Handlers) GetMarketSummary(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Missing required parameter: date (YYYY-MM-DD)")
		return
	}
	if !isValidDate(date) {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Invalid date format, expected YYYY-MM-DD")
		return
	}

	summaries, err := h.store.ListMarketSummaries(r.Context(), date)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch market summaries")
		return
	}

	if summaries == nil {
		summaries = []MarketSummary{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  summaries,
		"count": len(summaries),
	})
}

// GetLargeTraders handles GET /api/v1/admin/reports/large-traders?date=YYYY-MM-DD.
// Returns large trader positions for the given date (admin only).
func (h *Handlers) GetLargeTraders(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Missing required parameter: date (YYYY-MM-DD)")
		return
	}
	if !isValidDate(date) {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Invalid date format, expected YYYY-MM-DD")
		return
	}

	positions, err := h.store.ListLargeTraderPositions(r.Context(), date)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch large trader positions")
		return
	}

	if positions == nil {
		positions = []LargeTraderPosition{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  positions,
		"count": len(positions),
	})
}

// isValidDate checks if a string is in YYYY-MM-DD format.
func isValidDate(s string) bool {
	if len(s) != 10 {
		return false
	}
	// YYYY-MM-DD
	for i, c := range s {
		switch i {
		case 4, 7:
			if c != '-' {
				return false
			}
		default:
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
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
