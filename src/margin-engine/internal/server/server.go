package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"

	"github.com/ace-platform/margin-engine/internal/engine"
	"github.com/ace-platform/margin-engine/internal/types"
)

// Server wraps the margin engine with health checks and HTTP endpoints.
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

// GetPortfolioMargin returns the cached portfolio margin for a participant.
func (s *Server) GetPortfolioMargin(participantID string) (types.PortfolioMargin, bool) {
	return s.engine.GetPortfolioMargin(participantID)
}

// CalculateMargin triggers margin calculation for a participant.
func (s *Server) CalculateMargin(participantID string, positions []types.Position) (*types.PortfolioMargin, error) {
	return s.engine.CalculateMargin(participantID, positions)
}

// StartHealthServer starts HTTP health, readiness, and query endpoints.
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

	mux.HandleFunc("/margin", func(w http.ResponseWriter, r *http.Request) {
		participantID := r.URL.Query().Get("participant_id")
		if participantID == "" {
			// Return empty margin summary when no participant specified
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"margins": []interface{}{},
				"total":   0,
			})
			return
		}
		pm, ok := s.engine.GetPortfolioMargin(participantID)
		if !ok {
			http.Error(w, "no margin data for participant", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pm)
	})

	mux.HandleFunc("/margin-calls", func(w http.ResponseWriter, r *http.Request) {
		calls := s.engine.GetAllActiveMarginCalls()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(calls)
	})

	mux.HandleFunc("/margin-call-stats", func(w http.ResponseWriter, r *http.Request) {
		stats := s.engine.GetMarginCallStats()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	})

	addr := fmt.Sprintf("%s:%d", s.cfg.BindAddress, s.cfg.HealthPort)
	return http.ListenAndServe(addr, mux)
}

// ListenGRPC creates a TCP listener for the gRPC port.
func (s *Server) ListenGRPC() (net.Listener, error) {
	addr := fmt.Sprintf("%s:%d", s.cfg.BindAddress, s.cfg.GRPCPort)
	return net.Listen("tcp", addr)
}
