package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"

	"github.com/ace-platform/clearing-engine/internal/engine"
	"github.com/ace-platform/clearing-engine/internal/types"
)

// Server wraps the clearing engine with health checks and a listener
// ready for gRPC integration.
type Server struct {
	engine *engine.Engine
	cfg    Config
	ready  atomic.Bool
}

func NewServer(eng *engine.Engine, cfg Config) *Server {
	return &Server{
		engine: eng,
		cfg:    cfg,
	}
}

func (s *Server) SetReady() {
	s.ready.Store(true)
}

// ClearTrade delegates to the engine.
func (s *Server) ClearTrade(trade types.Trade) (*engine.ClearingResult, error) {
	return s.engine.ClearTrade(trade)
}

// GetPosition delegates to the engine.
func (s *Server) GetPosition(participantID, instrumentID string) (types.Position, bool) {
	return s.engine.GetPosition(participantID, instrumentID)
}

// GetPositions delegates to the engine.
func (s *Server) GetPositions(participantID string) []types.Position {
	return s.engine.GetPositions(participantID)
}

// NetObligations delegates to the engine.
func (s *Server) NetObligations() []types.NettingResult {
	return s.engine.NetObligations()
}

// StartHealthServer starts HTTP health and readiness endpoints.
func (s *Server) StartHealthServer() error {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if s.ready.Load() {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, "ready")
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintln(w, "not ready")
		}
	})

	mux.HandleFunc("/positions", func(w http.ResponseWriter, r *http.Request) {
		participantID := r.URL.Query().Get("participant_id")
		if participantID == "" {
			http.Error(w, "participant_id required", http.StatusBadRequest)
			return
		}
		positions := s.engine.GetPositions(participantID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(positions)
	})

	mux.HandleFunc("/netting", func(w http.ResponseWriter, r *http.Request) {
		results := s.engine.NetObligations()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	})

	addr := fmt.Sprintf("%s:%d", s.cfg.BindAddress, s.cfg.HealthPort)
	return http.ListenAndServe(addr, mux)
}

// ListenGRPC creates a TCP listener for the gRPC port.
func (s *Server) ListenGRPC() (net.Listener, error) {
	addr := fmt.Sprintf("%s:%d", s.cfg.BindAddress, s.cfg.GRPCPort)
	return net.Listen("tcp", addr)
}
