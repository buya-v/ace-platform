package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/garudax-platform/gateway/internal/auth"
	"github.com/garudax-platform/gateway/internal/config"
	"github.com/garudax-platform/gateway/internal/handler"
	"github.com/garudax-platform/gateway/internal/middleware"
	"github.com/garudax-platform/gateway/internal/observability"
	"github.com/garudax-platform/gateway/internal/proxy"
	"github.com/garudax-platform/gateway/internal/router"
	"github.com/garudax-platform/gateway/internal/websocket"
)

func main() {
	logger := observability.NewLogger("gateway")
	metrics := observability.NewMetrics("gateway")

	logger.Info("GarudaX API Gateway starting...")

	cfg := config.FromEnv()

	// Initialize JWT validator
	jwtValidator := auth.NewJWTValidator(cfg.JWTSecret, cfg.JWTIssuer, cfg.JWTAudience)

	// Initialize backend client — forwards to service HTTP APIs
	var backendClient proxy.BackendClient = proxy.NewHTTPBackendClient(map[string]string{
		"matching-engine":    fmt.Sprintf("http://%s", strings.Replace(cfg.MatchingEngineAddr, ":50051", ":8081", 1)),
		"clearing-engine":   fmt.Sprintf("http://%s", strings.Replace(cfg.ClearingEngineAddr, ":50052", ":8082", 1)),
		"margin-engine":     fmt.Sprintf("http://%s", strings.Replace(cfg.MarginEngineAddr, ":50053", ":8083", 1)),
		"settlement-engine": fmt.Sprintf("http://%s", strings.Replace(cfg.SettlementEngineAddr, ":50054", ":8084", 1)),
		"auth-service":      fmt.Sprintf("http://%s", strings.Replace(cfg.AuthServiceAddr, ":50055", ":8085", 1)),
		"compliance-service":   fmt.Sprintf("http://%s", strings.Replace(cfg.ComplianceServiceAddr, ":50056", ":8086", 1)),
		"market-data-service":  fmt.Sprintf("http://%s", strings.Replace(cfg.MarketDataServiceAddr, ":50057", ":8087", 1)),
		"warehouse-service":   fmt.Sprintf("http://%s", strings.Replace(cfg.WarehouseServiceAddr, ":50058", ":8088", 1)),
	})

	// Initialize handler and router
	h := handler.New(backendClient)
	rt := router.New()
	h.RegisterRoutes(rt)

	// Register WebSocket routes (both path-param and query-param styles)
	wsHandler := websocket.NewHandler()
	rt.Handle("GET", "/api/v1/ws/trades/{instrument_id}", wsHandler.TradesHandler)
	rt.Handle("GET", "/api/v1/ws/trades", wsHandler.TradesHandler)
	rt.Handle("GET", "/api/v1/ws/book/{instrument_id}", wsHandler.BookHandler)
	rt.Handle("GET", "/api/v1/ws/book", wsHandler.BookHandler)
	rt.Handle("GET", "/api/v1/ws/executions", wsHandler.ExecutionsHandler)

	// Configure public paths (no auth required)
	authCfg := &middleware.AuthConfig{
		PublicPaths: map[string]bool{
			"/healthz":                         true,
			"/readyz":                          true,
			"/metrics":                         true,
			"POST /api/v1/auth/login":          true,
			"POST /api/v1/auth/register":       true,
			"POST /api/v1/auth/password/reset":  true,
			"POST /api/v1/auth/refresh":         true,
		},
		PublicPrefixes: []string{
			"/api/v1/instruments/",
			"/api/v1/market-data/",
			"/api/v1/ws/",
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

	// Build middleware chain: RequestID → Tracing → Metrics → BodyLimit → Auth → RateLimit → Router
	var httpHandler http.Handler = rt
	if cfg.RateLimitEnabled {
		if cfg.RedisURL != "" {
			redisRL, err := middleware.NewRedisRateLimiter(cfg.RedisURL, rateLimitGroup, 0)
			if err != nil {
				logger.Warn("Redis rate limiter unavailable, using in-memory fallback",
					slog.String("error", err.Error()))
				httpHandler = middleware.RateLimit(rateLimitGroup, groupFn, keyFn)(httpHandler)
			} else {
				logger.Info("Using Redis rate limiter", slog.String("redis", cfg.RedisURL))
				httpHandler = middleware.RedisRateLimit(redisRL, groupFn, keyFn)(httpHandler)
			}
		} else {
			httpHandler = middleware.RateLimit(rateLimitGroup, groupFn, keyFn)(httpHandler)
		}
	}
	httpHandler = middleware.Auth(jwtValidator, authCfg)(httpHandler)
	httpHandler = middleware.BodyLimit(cfg.MaxBodySize)(httpHandler)
	httpHandler = metrics.MetricsMiddleware()(httpHandler)
	httpHandler = observability.TracingMiddleware(logger)(httpHandler)
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
	// Metrics endpoint on health port (Prometheus scraping)
	healthMux.Handle("/metrics", metrics.MetricsHandler())
	healthMux.Handle("/metrics.json", metrics.MetricsJSON())

	healthSrv := &http.Server{
		Addr:    healthAddr,
		Handler: healthMux,
	}

	// Start servers
	go func() {
		logger.Info("health server listening", slog.String("addr", healthAddr))
		if err := healthSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("health server error", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	go func() {
		handler.SetReady()
		logger.Info("GarudaX API Gateway ready", slog.String("addr", httpAddr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	logger.Info("received signal, shutting down gracefully...", slog.String("signal", sig.String()))

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("HTTP server shutdown error", slog.String("error", err.Error()))
	}
	if err := healthSrv.Shutdown(ctx); err != nil {
		logger.Error("health server shutdown error", slog.String("error", err.Error()))
	}

	logger.Info("GarudaX API Gateway stopped")
}
