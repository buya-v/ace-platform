// Package server provides the HTTP server for the securities-service.
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// Config holds the server configuration.
type Config struct {
	// APIPort is the port for the main API HTTP server.
	APIPort int
	// HealthPort is the port for the health/readiness HTTP server.
	HealthPort int
	// BindAddress is the interface address to bind to (default "0.0.0.0").
	BindAddress string
}

// DefaultConfig returns a Config with the standard port allocation for securities-service.
func DefaultConfig() Config {
	return Config{
		APIPort:     8085,
		HealthPort:  9085,
		BindAddress: "0.0.0.0",
	}
}

// Server is the HTTP server for the securities-service.
type Server struct {
	cfg             Config
	instrumentStore store.InstrumentStore
	orderStore      store.OrderStore
	ready           atomic.Int32
}

// New creates a new Server with the given stores and configuration.
func New(
	instrumentStore store.InstrumentStore,
	orderStore store.OrderStore,
	cfg Config,
) *Server {
	return &Server{
		cfg:             cfg,
		instrumentStore: instrumentStore,
		orderStore:      orderStore,
	}
}

// SetReady marks the server as ready to serve traffic.
func (s *Server) SetReady() {
	s.ready.Store(1)
}

// isReady reports whether the server has been marked ready.
func (s *Server) isReady() bool {
	return s.ready.Load() == 1
}

// StartHealthServer starts the health/readiness HTTP server on HealthPort.
// It blocks until the server fails; call it in a goroutine.
func (s *Server) StartHealthServer() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/readyz", s.readyz)

	addr := fmt.Sprintf("%s:%d", s.cfg.BindAddress, s.cfg.HealthPort)
	return http.ListenAndServe(addr, mux)
}

// StartAPIServer starts the main API HTTP server on APIPort.
// It blocks until the server fails; call it in a goroutine.
func (s *Server) StartAPIServer() error {
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	addr := fmt.Sprintf("%s:%d", s.cfg.BindAddress, s.cfg.APIPort)
	return http.ListenAndServe(addr, mux)
}

// registerRoutes wires all API routes onto the given ServeMux.
// Placeholder handlers are used for routes not yet implemented.
func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Instruments
	mux.HandleFunc("/api/v1/securities/instruments", s.handleInstruments)
	mux.HandleFunc("/api/v1/securities/instruments/", s.handleInstrument)

	// Orders
	mux.HandleFunc("/api/v1/securities/orders", s.handleOrders)
	mux.HandleFunc("/api/v1/securities/orders/", s.handleOrder)
}

// --- Health endpoints ---

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"service": "securities-service",
	})
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !s.isReady() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "not_ready",
		})
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ready",
	})
}

// --- Instrument route handlers ---

// handleInstruments dispatches GET /api/v1/securities/instruments (list)
// and POST /api/v1/securities/instruments (create).
func (s *Server) handleInstruments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListInstruments(w, r)
	case http.MethodPost:
		s.handleCreateInstrument(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleInstrument dispatches GET/PATCH /api/v1/securities/instruments/{id}
// and PUT /api/v1/securities/instruments/{id}/status.
func (s *Server) handleInstrument(w http.ResponseWriter, r *http.Request) {
	// Detect the /status sub-resource.
	if strings.HasSuffix(strings.TrimSuffix(r.URL.Path, "/"), "/status") {
		if r.Method == http.MethodPut {
			s.handleUpdateInstrumentStatus(w, r)
		} else {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetInstrument(w, r)
	case http.MethodPatch:
		s.handleUpdateInstrument(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// --- Order route handlers ---

// handleOrders dispatches GET /api/v1/securities/orders (list)
// and POST /api/v1/securities/orders (submit).
func (s *Server) handleOrders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listOrders(w, r)
	case http.MethodPost:
		s.submitOrder(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleOrder dispatches GET/DELETE /api/v1/securities/orders/{id}.
func (s *Server) handleOrder(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.getOrder(w, r)
	case http.MethodDelete:
		s.cancelOrder(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// listOrders handles GET /api/v1/securities/orders
func (s *Server) listOrders(w http.ResponseWriter, r *http.Request) {
	filters := store.OrderFilters{
		InstrumentID:  r.URL.Query().Get("instrument_id"),
		ParticipantID: r.URL.Query().Get("participant_id"),
		Status:        types.OrderStatus(r.URL.Query().Get("status")),
	}

	orders, err := s.orderStore.List(filters)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  orders,
		"total": len(orders),
	})
}

// submitOrder handles POST /api/v1/securities/orders
func (s *Server) submitOrder(w http.ResponseWriter, r *http.Request) {
	var order types.SecurityOrder
	if err := json.NewDecoder(r.Body).Decode(&order); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}
	if order.ID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "id is required", nil)
		return
	}
	if order.InstrumentID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "instrument_id is required", nil)
		return
	}
	if order.Status == "" {
		order.Status = types.OrderStatusPending
	}

	if err := s.orderStore.Submit(&order); err != nil {
		s.writeError(w, http.StatusConflict, "ALREADY_EXISTS", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusCreated, order)
}

// getOrder handles GET /api/v1/securities/orders/{id}
func (s *Server) getOrder(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/v1/securities/orders/"):]
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "order id is required", nil)
		return
	}

	order, err := s.orderStore.Get(id)
	if err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND", "order not found", nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, order)
}

// cancelOrder handles DELETE /api/v1/securities/orders/{id}
func (s *Server) cancelOrder(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/v1/securities/orders/"):]
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "order id is required", nil)
		return
	}

	if err := s.orderStore.Cancel(id); err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND", "order not found", nil)
			return
		}
		s.writeError(w, http.StatusConflict, "INVALID_STATE", err.Error(), nil)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- JSON helpers ---

func (s *Server) writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func (s *Server) writeError(w http.ResponseWriter, status int, code, message string, details []string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(types.ErrorResponse{
		Error: types.ErrorDetail{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}
