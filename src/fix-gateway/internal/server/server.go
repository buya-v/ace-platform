package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/garudax-platform/fix-gateway/internal/broker"
	"github.com/garudax-platform/fix-gateway/internal/session"
)

// Server manages the HTTP admin and health servers for the FIX gateway.
type Server struct {
	logger     *slog.Logger
	adminAddr  string
	healthAddr string
	adminSrv   *http.Server
	healthSrv  *http.Server
	sessionMgr *session.SessionManager
	brokerStore broker.BrokerStore
}

// New creates a new Server.
func New(logger *slog.Logger, adminAddr, healthAddr string, sessionMgr *session.SessionManager, brokerStore broker.BrokerStore) *Server {
	return &Server{
		logger:      logger,
		adminAddr:   adminAddr,
		healthAddr:  healthAddr,
		sessionMgr:  sessionMgr,
		brokerStore: brokerStore,
	}
}

// Start starts both the admin and health HTTP servers.
func (s *Server) Start() error {
	// Admin server (broker management API).
	adminMux := http.NewServeMux()
	brokerHandlers := broker.NewHandlers(s.brokerStore, s.sessionMgr)
	brokerHandlers.RegisterRoutes(adminMux)

	s.adminSrv = &http.Server{
		Addr:         s.adminAddr,
		Handler:      adminMux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Health server.
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", s.handleHealth)
	healthMux.HandleFunc("/readyz", s.handleReady)
	healthMux.HandleFunc("/metrics", s.handleMetrics)

	s.healthSrv = &http.Server{
		Addr:         s.healthAddr,
		Handler:      healthMux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	// Start health server in background.
	go func() {
		s.logger.Info("health server listening", slog.String("addr", s.healthAddr))
		if err := s.healthSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("health server error", slog.String("error", err.Error()))
		}
	}()

	// Start admin server (blocking).
	s.logger.Info("admin server listening", slog.String("addr", s.adminAddr))
	if err := s.adminSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Stop gracefully shuts down both servers.
func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var firstErr error
	if s.adminSrv != nil {
		if err := s.adminSrv.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if s.healthSrv != nil {
		if err := s.healthSrv.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"service": "fix-gateway",
	})
}

func (s *Server) handleReady(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ready",
		"service": "fix-gateway",
	})
}

func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	sessionCount := s.sessionMgr.SessionCount()
	brokers, _ := s.brokerStore.List()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"fix_sessions_total": sessionCount,
		"fix_brokers_total":  len(brokers),
	})
}
