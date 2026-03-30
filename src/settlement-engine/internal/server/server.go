package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"

	"github.com/garudax-platform/settlement-engine/internal/engine"
)

// Server wraps the settlement engine with health checks and a listener
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

// StartHealthServer starts HTTP health, readiness, and status endpoints.
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

	mux.HandleFunc("/cycles", func(w http.ResponseWriter, r *http.Request) {
		cycleID := r.URL.Query().Get("cycle_id")
		w.Header().Set("Content-Type", "application/json")
		if cycleID != "" {
			cycle, ok := s.engine.GetCycle(cycleID)
			if !ok {
				http.Error(w, "cycle not found", http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(cycle)
			return
		}
		cycles := s.engine.GetAllCycles()
		json.NewEncoder(w).Encode(cycles)
	})

	addr := fmt.Sprintf("%s:%d", s.cfg.BindAddress, s.cfg.HealthPort)
	return http.ListenAndServe(addr, mux)
}

// ListenGRPC creates a TCP listener for the gRPC port.
func (s *Server) ListenGRPC() (net.Listener, error) {
	addr := fmt.Sprintf("%s:%d", s.cfg.BindAddress, s.cfg.GRPCPort)
	return net.Listen("tcp", addr)
}
