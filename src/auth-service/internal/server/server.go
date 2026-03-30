package server

import (
	"fmt"
	"net"
	"net/http"
	"sync/atomic"

	"github.com/garudax-platform/auth-service/internal/handler"
)

type Server struct {
	handler *handler.AuthHandler
	cfg     Config
	ready   atomic.Bool
}

type Config struct {
	GRPCPort    int
	HealthPort  int
	BindAddress string
}

func NewServer(h *handler.AuthHandler, cfg Config) *Server {
	return &Server{
		handler: h,
		cfg:     cfg,
	}
}

func (s *Server) SetReady() {
	s.ready.Store(true)
}

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

	// Auth API endpoints
	mux.HandleFunc("/api/v1/register", s.handler.Register)
	mux.HandleFunc("/api/v1/login", s.handler.Login)
	mux.HandleFunc("/api/v1/refresh", s.handler.Refresh)
	mux.HandleFunc("/api/v1/authorize", s.handler.Authorize)
	mux.HandleFunc("/api/v1/exchange", s.handler.Exchange)
	mux.HandleFunc("/api/v1/token/validate", s.handler.ValidateToken)
	mux.HandleFunc("/api/v1/session/revoke", s.handler.RevokeSession)
	mux.HandleFunc("/api/v1/apikey/create", s.handler.CreateAPIKey)
	mux.HandleFunc("/api/v1/apikey/validate", s.handler.ValidateAPIKey)
	mux.HandleFunc("/api/v1/apikey/revoke", s.handler.RevokeAPIKey)
	mux.HandleFunc("/api/v1/users", s.handler.ListUsers)

	addr := fmt.Sprintf("%s:%d", s.cfg.BindAddress, s.cfg.HealthPort)
	return http.ListenAndServe(addr, mux)
}

func (s *Server) ListenGRPC() (net.Listener, error) {
	addr := fmt.Sprintf("%s:%d", s.cfg.BindAddress, s.cfg.GRPCPort)
	return net.Listen("tcp", addr)
}
