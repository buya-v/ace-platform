// Package server provides the HTTP server for the platform-service.
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/garudax-platform/platform-service/internal/config"
	"github.com/garudax-platform/platform-service/internal/provisioning"
	"github.com/garudax-platform/platform-service/internal/store"
	"github.com/garudax-platform/platform-service/internal/types"
)

// Config holds server configuration.
type Config struct {
	// APIPort is the port for the main REST API HTTP server.
	APIPort int
	// HealthPort is the port for the health/readiness HTTP server.
	HealthPort int
	// BindAddress is the interface to bind to (default "0.0.0.0").
	BindAddress string
}

// DefaultConfig returns a Config with the standard port allocation for platform-service.
func DefaultConfig() Config {
	return Config{
		APIPort:     8095,
		HealthPort:  9095,
		BindAddress: "0.0.0.0",
	}
}

// Server is the HTTP server for the platform-service.
type Server struct {
	cfg          Config
	tenantStore  store.TenantStore
	provisioner  *provisioning.Provisioner
	configLoader *config.ConfigLoader
	ready        atomic.Int32
}

// New creates a new Server with the given tenant store and configuration.
func New(tenantStore store.TenantStore, cfg Config) *Server {
	return &Server{
		cfg:          cfg,
		tenantStore:  tenantStore,
		provisioner:  provisioning.New(nil), // nil db = dry-run mode for MVP
		configLoader: config.NewConfigLoader(""),
	}
}

// NewWithProvisioner creates a new Server with an explicit Provisioner.
// Use this when you want to inject a non-default provisioner (e.g. with a real DB).
func NewWithProvisioner(tenantStore store.TenantStore, cfg Config, p *provisioning.Provisioner) *Server {
	return &Server{
		cfg:          cfg,
		tenantStore:  tenantStore,
		provisioner:  p,
		configLoader: config.NewConfigLoader(""),
	}
}

// NewWithConfig creates a new Server with an explicit ConfigLoader.
// Use this to override the default venues directory (e.g. in tests or custom deployments).
func NewWithConfig(tenantStore store.TenantStore, cfg Config, p *provisioning.Provisioner, cl *config.ConfigLoader) *Server {
	return &Server{
		cfg:          cfg,
		tenantStore:  tenantStore,
		provisioner:  p,
		configLoader: cl,
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

// StartAPIServer starts the main REST API HTTP server on APIPort.
// It blocks until the server fails; call it in a goroutine.
func (s *Server) StartAPIServer() error {
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	addr := fmt.Sprintf("%s:%d", s.cfg.BindAddress, s.cfg.APIPort)
	return http.ListenAndServe(addr, mux)
}

// registerRoutes wires all API routes onto the given ServeMux.
func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Tenant collection: GET (list), POST (create)
	mux.HandleFunc("/platform/v1/tenants", s.handleTenants)
	// Tenant config: GET /platform/v1/tenants/{id}/config
	// Registered before the wildcard tenant route so it takes precedence.
	mux.HandleFunc("/platform/v1/tenants/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(strings.TrimSuffix(r.URL.Path, "/"), "/config") {
			s.handleTenantConfig(w, r)
			return
		}
		s.handleTenant(w, r)
	})
}

// --- Health endpoints ---

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"service": "platform-service",
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
