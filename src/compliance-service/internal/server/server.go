package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"

	"github.com/ace-platform/compliance-service/internal/onboarding"
	"github.com/ace-platform/compliance-service/internal/screening"
)

// Server wraps the compliance services with health checks and HTTP endpoints.
type Server struct {
	onboarding *onboarding.Service
	screening  *screening.Service
	cfg        Config
	ready      atomic.Bool
}

func NewServer(onboardingSvc *onboarding.Service, screeningSvc *screening.Service, cfg Config) *Server {
	return &Server{
		onboarding: onboardingSvc,
		screening:  screeningSvc,
		cfg:        cfg,
	}
}

func (s *Server) SetReady() {
	s.ready.Store(true)
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

	mux.HandleFunc("/application", func(w http.ResponseWriter, r *http.Request) {
		appID := r.URL.Query().Get("id")
		if appID == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		app, err := s.onboarding.GetApplication(appID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(app)
	})

	mux.HandleFunc("/participant-status", func(w http.ResponseWriter, r *http.Request) {
		participantID := r.URL.Query().Get("participant_id")
		if participantID == "" {
			http.Error(w, "participant_id required", http.StatusBadRequest)
			return
		}
		status, err := s.onboarding.CheckParticipantStatus(participantID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	})

	addr := fmt.Sprintf("%s:%d", s.cfg.BindAddress, s.cfg.HealthPort)
	return http.ListenAndServe(addr, mux)
}

// ListenGRPC creates a TCP listener for the gRPC port.
func (s *Server) ListenGRPC() (net.Listener, error) {
	addr := fmt.Sprintf("%s:%d", s.cfg.BindAddress, s.cfg.GRPCPort)
	return net.Listen("tcp", addr)
}
