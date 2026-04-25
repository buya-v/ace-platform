package broker

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/garudax-platform/fix-gateway/internal/session"
)

// Handlers provides HTTP handlers for broker management.
type Handlers struct {
	store      BrokerStore
	sessionMgr *session.SessionManager
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(store BrokerStore, sessionMgr *session.SessionManager) *Handlers {
	return &Handlers{
		store:      store,
		sessionMgr: sessionMgr,
	}
}

// CreateBrokerRequest is the request body for creating a broker.
type CreateBrokerRequest struct {
	ID       string            `json:"id"`
	CompID   string            `json:"comp_id"`
	TenantID string            `json:"tenant_id"`
	Name     string            `json:"name"`
	Config   map[string]string `json:"config"`
}

// RegisterRoutes registers broker HTTP routes on the given mux.
func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/fix/brokers", h.handleBrokers)
	mux.HandleFunc("/api/v1/fix/brokers/", h.handleBrokerByID)
	mux.HandleFunc("/api/v1/fix/sessions", h.handleSessions)
}

func (h *Handlers) handleBrokers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listBrokers(w, r)
	case http.MethodPost:
		h.createBroker(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *Handlers) handleBrokerByID(w http.ResponseWriter, r *http.Request) {
	// Extract broker ID from path: /api/v1/fix/brokers/{id}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/fix/brokers/")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing broker ID"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.getBroker(w, r, path)
	case http.MethodPut:
		h.updateBroker(w, r, path)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *Handlers) listBrokers(w http.ResponseWriter, _ *http.Request) {
	brokers, err := h.store.List()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, brokers)
}

func (h *Handlers) createBroker(w http.ResponseWriter, r *http.Request) {
	var req CreateBrokerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}

	if req.CompID == "" || req.TenantID == "" || req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "comp_id, tenant_id, and name are required"})
		return
	}

	if req.ID == "" {
		req.ID = fmt.Sprintf("BRK-%d", time.Now().UnixNano())
	}

	broker := &FIXBroker{
		ID:       req.ID,
		CompID:   req.CompID,
		TenantID: req.TenantID,
		Name:     req.Name,
		Status:   "PENDING",
		Config:   req.Config,
	}

	if err := h.store.Create(broker); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, broker)
}

func (h *Handlers) getBroker(w http.ResponseWriter, _ *http.Request, id string) {
	broker, err := h.store.GetByID(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, broker)
}

func (h *Handlers) updateBroker(w http.ResponseWriter, r *http.Request, id string) {
	existing, err := h.store.GetByID(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}

	if name, ok := updates["name"].(string); ok && name != "" {
		existing.Name = name
	}
	if status, ok := updates["status"].(string); ok && status != "" {
		existing.Status = status
	}

	if err := h.store.Update(existing); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, existing)
}

// SessionInfo is a JSON-friendly representation of a FIX session.
type SessionInfo struct {
	SessionKey        string `json:"session_key"`
	SenderCompID      string `json:"sender_comp_id"`
	TargetCompID      string `json:"target_comp_id"`
	TenantID          string `json:"tenant_id"`
	State             string `json:"state"`
	InSeqNum          uint64 `json:"in_seq_num"`
	OutSeqNum         uint64 `json:"out_seq_num"`
	HeartbeatInterval int    `json:"heartbeat_interval"`
	LastRecvTime      string `json:"last_recv_time"`
}

func (h *Handlers) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	sessions := h.sessionMgr.ListSessions()
	result := make([]SessionInfo, 0, len(sessions))
	for _, s := range sessions {
		result = append(result, SessionInfo{
			SessionKey:        s.SessionKey(),
			SenderCompID:      s.SenderCompID,
			TargetCompID:      s.TargetCompID,
			TenantID:          s.TenantID,
			State:             s.State.String(),
			InSeqNum:          s.InSeqNum,
			OutSeqNum:         s.OutSeqNum,
			HeartbeatInterval: s.HeartbeatInterval,
			LastRecvTime:      s.LastRecvTime.Format(time.RFC3339),
		})
	}

	writeJSON(w, http.StatusOK, result)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
