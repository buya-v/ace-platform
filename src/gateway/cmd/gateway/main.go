package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/ace-platform/gateway/internal/auth"
	"github.com/ace-platform/gateway/internal/config"
	"github.com/ace-platform/gateway/internal/handler"
	"github.com/ace-platform/gateway/internal/middleware"
	"github.com/ace-platform/gateway/internal/proxy"
	"github.com/ace-platform/gateway/internal/router"
	"github.com/ace-platform/gateway/internal/websocket"
)

func main() {
	log.Println("ACE API Gateway starting...")

	cfg := config.FromEnv()

	// Initialize JWT validator
	jwtValidator := auth.NewJWTValidator(cfg.JWTSecret, cfg.JWTIssuer, cfg.JWTAudience)

	// Initialize backend client (stub for now — real gRPC client would go here)
	var backendClient proxy.BackendClient = &proxy.StubBackendClient{}

	// Initialize handler and router
	h := handler.New(backendClient)
	rt := router.New()
	h.RegisterRoutes(rt)

	// Register WebSocket routes
	wsHandler := websocket.NewHandler()
	rt.Handle("GET", "/api/v1/ws/trades/{instrument_id}", wsHandler.TradesHandler)
	rt.Handle("GET", "/api/v1/ws/book/{instrument_id}", wsHandler.BookHandler)
	rt.Handle("GET", "/api/v1/ws/executions", wsHandler.ExecutionsHandler)

	// Configure public paths (no auth required)
	authCfg := &middleware.AuthConfig{
		PublicPaths: map[string]bool{
			"/healthz":                         true,
			"/readyz":                          true,
			"POST /api/v1/auth/login":          true,
			"POST /api/v1/auth/register":       true,
			"POST /api/v1/auth/password/reset":  true,
			"POST /api/v1/auth/refresh":         true,
		},
		PublicPrefixes: []string{
			"/api/v1/instruments/",
			"/api/v1/ws/trades/",
			"/api/v1/ws/book/",
		},
	}

	// Rate limit configuration
	rateLimitGroup := middleware.NewRateLimitGroup()
	groupFn := func(r *http.Request) string {
		path := r.URL.Path
		switch {
		case r.Method == "POST" && path == "/api/v1/orders":
			return "order_submit"
		case r.Method == "DELETE" && strings.HasPrefix(path, "/api/v1/orders"):
			return "order_cancel"
		case strings.HasPrefix(path, "/api/v1/orders"):
			return "order_query"
		case strings.HasPrefix(path, "/api/v1/instruments/"):
			return "market_public"
		case strings.HasPrefix(path, "/api/v1/admin/"):
			return "admin"
		case strings.HasPrefix(path, "/api/v1/auth/"):
			return "auth"
		case strings.HasPrefix(path, "/api/v1/compliance/") ||
			strings.HasPrefix(path, "/api/v1/screening/") ||
			strings.HasPrefix(path, "/api/v1/participants") ||
			strings.HasPrefix(path, "/api/v1/risk-scores/"):
			return "compliance"
		default:
			return "default"
		}
	}
	keyFn := func(r *http.Request) string {
		if claims := middleware.ClaimsFromContext(r.Context()); claims != nil {
			return claims.Sub
		}
		// Fall back to IP for unauthenticated requests
		return r.RemoteAddr
	}

	// Build middleware chain
	var httpHandler http.Handler = rt
	if cfg.RateLimitEnabled {
		httpHandler = middleware.RateLimit(rateLimitGroup, groupFn, keyFn)(httpHandler)
	}
	httpHandler = middleware.Auth(jwtValidator, authCfg)(httpHandler)
	httpHandler = middleware.BodyLimit(cfg.MaxBodySize)(httpHandler)
	httpHandler = middleware.Logging()(httpHandler)
	httpHandler = middleware.RequestID()(httpHandler)

	// Create HTTP server
	httpAddr := fmt.Sprintf("%s:%d", cfg.BindAddress, cfg.HTTPPort)
	srv := &http.Server{
		Addr:         httpAddr,
		Handler:      httpHandler,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	// Start health server on separate port
	healthAddr := fmt.Sprintf("%s:%d", cfg.BindAddress, cfg.HealthPort)
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"status":"ok","service":"ace-gateway"}`))
	})
	healthMux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if handler.IsReady() {
			w.WriteHeader(200)
			w.Write([]byte(`{"status":"ready"}`))
		} else {
			w.WriteHeader(503)
			w.Write([]byte(`{"status":"not_ready"}`))
		}
	})
	healthSrv := &http.Server{
		Addr:    healthAddr,
		Handler: healthMux,
	}

	// Start servers
	go func() {
		log.Printf("Health server listening on %s", healthAddr)
		if err := healthSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Health server error: %v", err)
		}
	}()

	go func() {
		handler.SetReady()
		log.Printf("ACE API Gateway ready on %s", httpAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("Received signal %s, shutting down gracefully...", sig)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}
	if err := healthSrv.Shutdown(ctx); err != nil {
		log.Printf("Health server shutdown error: %v", err)
	}

	log.Println("ACE API Gateway stopped")
}
