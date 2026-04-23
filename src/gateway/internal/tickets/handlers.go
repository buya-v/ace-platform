package tickets

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/garudax-platform/gateway/internal/middleware"
	"github.com/garudax-platform/gateway/internal/router"
)

// Handlers provides HTTP handlers for ticket management endpoints.
type Handlers struct {
	store Store
}

// NewHandlers creates ticket handlers backed by the given store.
func NewHandlers(store Store) *Handlers {
	return &Handlers{store: store}
}

// RegisterRoutes registers ticket routes on the router.
func (h *Handlers) RegisterRoutes(rt *router.Router) {
	// Authenticated routes
	rt.Handle("POST", "/api/v1/tickets", h.CreateTicket)
	rt.Handle("GET", "/api/v1/tickets", h.ListTickets)
	rt.Handle("GET", "/api/v1/tickets/{id}", h.GetTicket)
	rt.Handle("PATCH", "/api/v1/tickets/{id}", h.UpdateTicket)
	rt.Handle("POST", "/api/v1/tickets/{id}/comments", h.CreateComment)

	// Admin-only routes
	rt.Handle("GET", "/api/v1/admin/tickets/stats", h.GetTicketStats)
}

// CreateTicketRequest is the payload for creating a new ticket.
type CreateTicketRequest struct {
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Category    string          `json:"category"`
	Priority    string          `json:"priority,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
}

// CreateTicket handles POST /api/v1/tickets.
func (h *Handlers) CreateTicket(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication required")
		return
	}

	var req CreateTicketRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Invalid request body")
		return
	}

	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Missing title")
		return
	}
	if req.Description == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Missing description")
		return
	}
	if !isValidCategory(req.Category) {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "category must be one of: bug_report, customization, support, feature_request")
		return
	}

	priority := req.Priority
	if priority == "" {
		priority = "medium"
	}
	if !isValidPriority(priority) {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "priority must be one of: low, medium, high, critical")
		return
	}

	reporterID := claims.ParticipantID
	if reporterID == "" {
		reporterID = claims.Sub
	}

	ticket := Ticket{
		ID:          GenerateID("TKT-"),
		Title:       req.Title,
		Description: req.Description,
		Category:    req.Category,
		Priority:    priority,
		Status:      "open",
		ReporterID:  reporterID,
		Tags:        req.Tags,
		Metadata:    req.Metadata,
	}

	if err := h.store.CreateTicket(r.Context(), ticket); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create ticket")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"data": ticket,
	})
}

// ListTickets handles GET /api/v1/tickets.
// Admins see all tickets; regular users see only their own.
func (h *Handlers) ListTickets(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication required")
		return
	}

	filters := ListFilters{
		Status:   r.URL.Query().Get("status"),
		Category: r.URL.Query().Get("category"),
		Priority: r.URL.Query().Get("priority"),
	}

	// Non-admin users can only see their own tickets
	if !claims.HasAnyRole("admin", "exchange_admin") {
		reporterID := claims.ParticipantID
		if reporterID == "" {
			reporterID = claims.Sub
		}
		filters.ReporterID = reporterID
	}

	tickets, err := h.store.ListTickets(r.Context(), filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list tickets")
		return
	}
	if tickets == nil {
		tickets = []Ticket{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  tickets,
		"count": len(tickets),
	})
}

// GetTicket handles GET /api/v1/tickets/{id}.
func (h *Handlers) GetTicket(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication required")
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Missing ticket id")
		return
	}

	ticket, err := h.store.GetTicket(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get ticket")
		return
	}
	if ticket == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Ticket not found")
		return
	}

	// Non-admin users can only view their own tickets
	if !claims.HasAnyRole("admin", "exchange_admin") {
		reporterID := claims.ParticipantID
		if reporterID == "" {
			reporterID = claims.Sub
		}
		if ticket.ReporterID != reporterID {
			writeError(w, http.StatusForbidden, "PERMISSION_DENIED", "Cannot view other users' tickets")
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": ticket,
	})
}

// UpdateTicketRequest is the payload for updating a ticket.
type UpdateTicketRequest struct {
	Status     string `json:"status,omitempty"`
	AssigneeID string `json:"assignee_id,omitempty"`
	Priority   string `json:"priority,omitempty"`
}

// UpdateTicket handles PATCH /api/v1/tickets/{id}. Admin only.
func (h *Handlers) UpdateTicket(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication required")
		return
	}

	if !claims.HasAnyRole("admin", "exchange_admin") {
		writeError(w, http.StatusForbidden, "PERMISSION_DENIED", "Admin access required")
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Missing ticket id")
		return
	}

	var req UpdateTicketRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Invalid request body")
		return
	}

	updates := make(map[string]interface{})
	if req.Status != "" {
		if !isValidStatus(req.Status) {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "status must be one of: open, in_progress, resolved, closed")
			return
		}
		updates["status"] = req.Status
	}
	if req.AssigneeID != "" {
		updates["assignee_id"] = req.AssigneeID
	}
	if req.Priority != "" {
		if !isValidPriority(req.Priority) {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "priority must be one of: low, medium, high, critical")
			return
		}
		updates["priority"] = req.Priority
	}

	if len(updates) == 0 {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "No fields to update")
		return
	}

	if err := h.store.UpdateTicket(r.Context(), id, updates); err != nil {
		if err == ErrNotFound {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Ticket not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update ticket")
		return
	}

	// Fetch updated ticket to return
	ticket, err := h.store.GetTicket(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch updated ticket")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": ticket,
	})
}

// CreateCommentRequest is the payload for adding a comment.
type CreateCommentRequest struct {
	Body string `json:"body"`
}

// CreateComment handles POST /api/v1/tickets/{id}/comments.
// Allowed for the ticket reporter or admins.
func (h *Handlers) CreateComment(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication required")
		return
	}

	ticketID := r.URL.Query().Get("id")
	if ticketID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Missing ticket id")
		return
	}

	// Verify ticket exists and user has access
	ticket, err := h.store.GetTicket(r.Context(), ticketID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get ticket")
		return
	}
	if ticket == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Ticket not found")
		return
	}

	authorID := claims.ParticipantID
	if authorID == "" {
		authorID = claims.Sub
	}

	isAdmin := claims.HasAnyRole("admin", "exchange_admin")
	if !isAdmin && ticket.ReporterID != authorID {
		writeError(w, http.StatusForbidden, "PERMISSION_DENIED", "Only the ticket reporter or an admin can comment")
		return
	}

	var req CreateCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Invalid request body")
		return
	}

	if strings.TrimSpace(req.Body) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Missing comment body")
		return
	}

	comment := Comment{
		ID:       GenerateID("CMT-"),
		TicketID: ticketID,
		AuthorID: authorID,
		Body:     req.Body,
	}

	if err := h.store.CreateComment(r.Context(), comment); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create comment")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"data": comment,
	})
}

// GetTicketStats handles GET /api/v1/admin/tickets/stats. Admin only.
func (h *Handlers) GetTicketStats(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication required")
		return
	}

	if !claims.HasAnyRole("admin", "exchange_admin") {
		writeError(w, http.StatusForbidden, "PERMISSION_DENIED", "Admin access required")
		return
	}

	stats, err := h.store.GetTicketStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get ticket stats")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": stats,
	})
}

// --- validation helpers ---

func isValidCategory(c string) bool {
	switch c {
	case "bug_report", "customization", "support", "feature_request":
		return true
	}
	return false
}

func isValidPriority(p string) bool {
	switch p {
	case "low", "medium", "high", "critical":
		return true
	}
	return false
}

func isValidStatus(s string) bool {
	switch s {
	case "open", "in_progress", "resolved", "closed":
		return true
	}
	return false
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
