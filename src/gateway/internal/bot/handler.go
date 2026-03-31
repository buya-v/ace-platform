package bot

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/garudax-platform/gateway/internal/middleware"
	"github.com/garudax-platform/gateway/internal/router"
)

// Handlers provides HTTP handlers for bot chat endpoints.
type Handlers struct {
	bridge   *Bridge
	executor *ActionExecutor
}

// NewHandlers creates bot handlers with the given orchestrator bridge.
func NewHandlers(bridge *Bridge) *Handlers {
	return &Handlers{
		bridge:   bridge,
		executor: NewActionExecutor("http://127.0.0.1:8080"),
	}
}

// RegisterRoutes registers bot routes on the router.
// All bot endpoints require admin authentication (enforced by middleware).
func (h *Handlers) RegisterRoutes(rt *router.Router) {
	rt.Handle("POST", "/api/v1/bot/chat", h.Chat)
	rt.Handle("GET", "/api/v1/bot/suggestions", h.Suggestions)
}

// ChatRequest is the payload for the bot chat endpoint.
type ChatRequest struct {
	Message string            `json:"message"`
	Context map[string]string `json:"context,omitempty"`
}

// ChatResponse is the response from the bot chat endpoint.
type ChatResponse struct {
	Reply       string       `json:"reply"`
	Actions     []Action     `json:"actions"`
	Suggestions []Suggestion `json:"suggestions"`
}

// Chat handles POST /api/v1/bot/chat.
// If the orchestrator is available, proxies the request to it.
// Otherwise, uses built-in keyword responses (MVP fallback).
func (h *Handlers) Chat(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication required")
		return
	}

	if !claims.HasAnyRole("admin", "exchange_admin") {
		writeError(w, http.StatusForbidden, "PERMISSION_DENIED", "Admin access required")
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Invalid request body")
		return
	}

	if req.Message == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Missing message")
		return
	}

	// Try orchestrator first if available
	if h.bridge.IsAvailable() {
		orchResp, err := h.bridge.ProxyToOrchestrator(req.Message, req.Context)
		if err == nil {
			// Convert orchestrator suggestions to our Suggestion type
			suggestions := make([]Suggestion, 0, len(orchResp.Suggestions))
			for _, s := range orchResp.Suggestions {
				suggestions = append(suggestions, Suggestion{Text: s})
			}

			resp := ChatResponse{
				Reply:       orchResp.Reply,
				Actions:     orchResp.Actions,
				Suggestions: suggestions,
			}

			writeJSON(w, http.StatusOK, map[string]interface{}{
				"data": resp,
			})
			return
		}
		// Fall through to fallback on error
	}

	// Fallback mode: execute actions directly using user's JWT token
	userToken := ""
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		userToken = strings.TrimPrefix(auth, "Bearer ")
	}

	execResp := h.executor.Execute(req.Message, userToken)

	// Get page-aware suggestions from context
	page := ""
	if req.Context != nil {
		page = req.Context["page"]
	}
	if len(execResp.Suggestions) == 0 {
		execResp.Suggestions = GetSuggestions(page)
	}
	if execResp.Actions == nil {
		execResp.Actions = []Action{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": execResp,
	})
}

// Suggestions handles GET /api/v1/bot/suggestions?page=.
// Returns context-aware quick action suggestions.
func (h *Handlers) Suggestions(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication required")
		return
	}

	if !claims.HasAnyRole("admin", "exchange_admin") {
		writeError(w, http.StatusForbidden, "PERMISSION_DENIED", "Admin access required")
		return
	}

	page := r.URL.Query().Get("page")
	suggestions := GetSuggestions(page)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": suggestions,
	})
}

// --- JSON helpers ---

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
