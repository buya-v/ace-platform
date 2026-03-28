package server

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"sync/atomic"

	"github.com/ace-platform/warehouse-service/internal/service"
)

// Server is the warehouse service gRPC server.
type Server struct {
	svc    *service.WarehouseService
	config Config
	ready  int32 // atomic: 1 = ready
}

// NewServer creates a new warehouse server.
func NewServer(svc *service.WarehouseService, cfg Config) *Server {
	return &Server{
		svc:    svc,
		config: cfg,
	}
}

// Service returns the warehouse service for handler registration.
func (s *Server) Service() *service.WarehouseService {
	return s.svc
}

// SetReady marks the server as ready to serve traffic.
func (s *Server) SetReady() {
	atomic.StoreInt32(&s.ready, 1)
}

// IsReady returns true if the server is ready.
func (s *Server) IsReady() bool {
	return atomic.LoadInt32(&s.ready) == 1
}

// StartHealthServer starts the HTTP health check server for Kubernetes probes.
func (s *Server) StartHealthServer() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if s.IsReady() {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ready"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("not ready"))
		}
	})

	addr := fmt.Sprintf("%s:%d", s.config.BindAddress, s.config.HealthPort)
	log.Printf("Health server listening on %s", addr)
	return http.ListenAndServe(addr, mux)
}

// GRPCAddr returns the address the gRPC server should bind to.
func (s *Server) GRPCAddr() string {
	return fmt.Sprintf("%s:%d", s.config.BindAddress, s.config.GRPCPort)
}

// ListenGRPC creates a TCP listener for the gRPC server.
func (s *Server) ListenGRPC() (net.Listener, error) {
	addr := s.GRPCAddr()
	log.Printf("gRPC server listening on %s", addr)
	return net.Listen("tcp", addr)
}
