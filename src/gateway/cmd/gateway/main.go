package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/garudax-platform/gateway/internal/auth"
	"github.com/garudax-platform/gateway/internal/bot"
	"github.com/garudax-platform/gateway/internal/config"
	"github.com/garudax-platform/gateway/internal/fees"
	"github.com/garudax-platform/gateway/internal/handler"
	"github.com/garudax-platform/gateway/internal/middleware"
	"github.com/garudax-platform/gateway/internal/observability"
	"github.com/garudax-platform/gateway/internal/proxy"
	"github.com/garudax-platform/gateway/internal/refdata"
	"github.com/garudax-platform/gateway/internal/reporting"
	"github.com/garudax-platform/gateway/internal/router"
	"github.com/garudax-platform/gateway/internal/tickets"
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
		"warehouse-service":    fmt.Sprintf("http://%s", strings.Replace(cfg.WarehouseServiceAddr, ":50058", ":8088", 1)),
		"securities-service":   fmt.Sprintf("http://%s", strings.Replace(cfg.SecuritiesServiceAddr, ":50059", ":8089", 1)),
		"platform-service":     fmt.Sprintf("http://%s", cfg.PlatformServiceAddr),
	})

	// Initialize handler and router
	h := handler.New(backendClient)
	rt := router.New()
	h.RegisterRoutes(rt)

	// Register reporting routes (settlement statements, market summaries, large trader reports)
	// Uses a no-op store by default; a real PgStore is injected when DB is configured.
	reportingHandlers := reporting.NewHandlers(reporting.NewNoOpStore())
	reportingHandlers.RegisterRoutes(rt)

	// Register ticket routes (support tickets, bug reports, feature requests)
	// Uses InMemoryStore by default; PgStore when DATABASE_URL is configured.
	ticketHandlers := tickets.NewHandlers(tickets.NewInMemoryStore())
	ticketHandlers.RegisterRoutes(rt)

	// Register reference data routes (commodities, instruments — public read + admin write).
	// Uses in-memory fallback store when DATABASE_URL is not set.
	refdataStore := refdata.NewPgStore(nil) // nil db → in-memory session fallback
	refdataHandlers := refdata.NewHandlers(refdataStore)
	refdataHandlers.RegisterRoutes(rt)
	refdataHandlers.RegisterAdminRoutes(rt)

	// Register fee routes (schedules, rules, tiers — authenticated read + admin write).
	// Uses in-memory fallback store when DATABASE_URL is not set.
	feesStore := fees.NewPgStore(nil) // nil db → in-memory session fallback
	feesHandlers := fees.NewHandlers(feesStore)
	feesHandlers.RegisterRoutes(rt)
	feesHandlers.RegisterAdminRoutes(rt)

	// Seed default master data so the in-memory store is non-empty on first start.
	// Both methods are no-ops when data already exists (idempotent).
	seedCtx := context.Background()
	if err := refdataStore.SeedDefaults(seedCtx); err != nil {
		logger.Warn("refdata seed failed", slog.String("error", err.Error()))
	} else {
		logger.Info("refdata master data ready")
	}
	if err := feesStore.SeedDefaults(seedCtx); err != nil {
		logger.Warn("fees seed failed", slog.String("error", err.Error()))
	} else {
		logger.Info("fees master data ready")
	}

	// Register bot chat routes (AI assistant proxy to orchestrator)
	// BOT_ORCHESTRATOR_URL env var configures the orchestrator; empty = fallback mode
	botOrchestratorURL := os.Getenv("BOT_ORCHESTRATOR_URL")
	botBridge := bot.NewBridge(botOrchestratorURL)
	botHandlers := bot.NewHandlers(botBridge)
	botHandlers.RegisterRoutes(rt)
	if botOrchestratorURL != "" {
		logger.Info("Bot orchestrator configured", slog.String("url", botOrchestratorURL))
	} else {
		logger.Info("Bot running in fallback mode (no orchestrator)")
	}

	// Register WebSocket routes (both path-param and query-param styles)
	wsHandler := websocket.NewHandler()
	rt.Handle("GET", "/api/v1/ws/trades/{instrument_id}", wsHandler.TradesHandler)
	rt.Handle("GET", "/api/v1/ws/trades", wsHandler.TradesHandler)
	rt.Handle("GET", "/api/v1/ws/book/{instrument_id}", wsHandler.BookHandler)
	rt.Handle("GET", "/api/v1/ws/book", wsHandler.BookHandler)
	rt.Handle("GET", "/api/v1/ws/executions", wsHandler.ExecutionsHandler)

	// Register platform-service routes: /platform/v1/* → platform-service:8095
	// These routes SKIP tenant middleware (platform API is above tenant scope).
	// The bypass is handled in TenantMiddleware via tenantBypassPrefixes.
	platformBaseURL := fmt.Sprintf("http://%s", cfg.PlatformServiceAddr)
	platformTarget, err := url.Parse(platformBaseURL)
	if err != nil {
		logger.Error("invalid platform service address", slog.String("addr", cfg.PlatformServiceAddr), slog.String("error", err.Error()))
		os.Exit(1)
	}
	platformProxy := httputil.NewSingleHostReverseProxy(platformTarget)
	platformHandler := func(w http.ResponseWriter, r *http.Request) {
		platformProxy.ServeHTTP(w, r)
	}
	rt.Handle("GET", "/platform/v1/tenants", platformHandler)
	rt.Handle("POST", "/platform/v1/tenants", platformHandler)
	rt.Handle("GET", "/platform/v1/tenants/{id}", platformHandler)
	rt.Handle("PATCH", "/platform/v1/tenants/{id}", platformHandler)
	rt.Handle("PUT", "/platform/v1/tenants/{id}/status", platformHandler)
	logger.Info("platform-service routes registered", slog.String("upstream", platformBaseURL))

	// Register securities-service routes: /api/v1/securities/* → securities-service:8089
	// Securities-service is HTTP-only (no gRPC), so we use reverse proxy instead of h.forward().
	securitiesAddr := strings.Replace(cfg.SecuritiesServiceAddr, ":50059", ":8089", 1)
	securitiesBaseURL := fmt.Sprintf("http://%s", securitiesAddr)
	securitiesTarget, err := url.Parse(securitiesBaseURL)
	if err != nil {
		logger.Error("invalid securities service address", slog.String("addr", securitiesAddr), slog.String("error", err.Error()))
		os.Exit(1)
	}
	securitiesProxy := httputil.NewSingleHostReverseProxy(securitiesTarget)
	secHandler := func(w http.ResponseWriter, r *http.Request) {
		securitiesProxy.ServeHTTP(w, r)
	}
	// Override the gRPC-forward handlers with HTTP reverse proxy
	rt.Handle("GET", "/api/v1/securities/instruments", secHandler)
	rt.Handle("POST", "/api/v1/securities/instruments", secHandler)
	rt.Handle("GET", "/api/v1/securities/instruments/{id}", secHandler)
	rt.Handle("PATCH", "/api/v1/securities/instruments/{id}", secHandler)
	rt.Handle("PUT", "/api/v1/securities/instruments/{id}", secHandler)
	rt.Handle("PUT", "/api/v1/securities/instruments/{id}/status", secHandler)
	rt.Handle("GET", "/api/v1/securities/orders", secHandler)
	rt.Handle("POST", "/api/v1/securities/orders", secHandler)
	rt.Handle("GET", "/api/v1/securities/orders/{id}", secHandler)
	rt.Handle("DELETE", "/api/v1/securities/orders/{id}", secHandler)
	rt.Handle("GET", "/api/v1/securities/positions", secHandler)
	rt.Handle("GET", "/api/v1/securities/settlements", secHandler)
	rt.Handle("POST", "/api/v1/securities/settlements/cycle", secHandler)
	rt.Handle("GET", "/api/v1/securities/corporate-actions", secHandler)
	rt.Handle("POST", "/api/v1/securities/corporate-actions", secHandler)
	rt.Handle("GET", "/api/v1/securities/corporate-actions/{id}", secHandler)
	rt.Handle("POST", "/api/v1/securities/corporate-actions/{id}/process", secHandler)
	rt.Handle("GET", "/api/v1/securities/sessions", secHandler)
	rt.Handle("POST", "/api/v1/securities/sessions/{id}/transition", secHandler)
	rt.Handle("GET", "/api/v1/securities/reports/frc", secHandler)
	// Markets and Segments (MillenniumIT P1)
	rt.Handle("GET", "/api/v1/securities/markets", secHandler)
	rt.Handle("POST", "/api/v1/securities/markets", secHandler)
	rt.Handle("GET", "/api/v1/securities/markets/{id}", secHandler)
	rt.Handle("PUT", "/api/v1/securities/markets/{id}/status", secHandler)
	rt.Handle("GET", "/api/v1/securities/segments", secHandler)
	rt.Handle("POST", "/api/v1/securities/segments", secHandler)
	// Circuit Breakers (MillenniumIT P1)
	rt.Handle("GET", "/api/v1/securities/circuit-breakers", secHandler)
	rt.Handle("GET", "/api/v1/securities/circuit-breakers/{id}", secHandler)
	rt.Handle("PUT", "/api/v1/securities/circuit-breakers/{id}", secHandler)
	rt.Handle("DELETE", "/api/v1/securities/circuit-breakers/{id}", secHandler)
	// Mass Cancel
	rt.Handle("POST", "/api/v1/securities/orders/mass-cancel", secHandler)
	// Firms
	rt.Handle("GET", "/api/v1/securities/firms", secHandler)
	rt.Handle("POST", "/api/v1/securities/firms", secHandler)
	rt.Handle("GET", "/api/v1/securities/firms/{id}", secHandler)
	rt.Handle("PUT", "/api/v1/securities/firms/{id}/status", secHandler)
	// Participants
	rt.Handle("GET", "/api/v1/securities/participants", secHandler)
	rt.Handle("POST", "/api/v1/securities/participants", secHandler)
	rt.Handle("GET", "/api/v1/securities/participants/{id}", secHandler)
	rt.Handle("PUT", "/api/v1/securities/participants/{id}/permissions", secHandler)
	rt.Handle("POST", "/api/v1/securities/participants/{id}/force-logout", secHandler)
	rt.Handle("POST", "/api/v1/securities/participants/{id}/suspend", secHandler)
	rt.Handle("POST", "/api/v1/securities/participants/{id}/reinstate", secHandler)
	// Announcements
	rt.Handle("GET", "/api/v1/securities/announcements", secHandler)
	rt.Handle("POST", "/api/v1/securities/announcements", secHandler)
	// Audit trail
	rt.Handle("GET", "/api/v1/securities/audit-trail", secHandler)
	// Day lifecycle
	rt.Handle("GET", "/api/v1/securities/day/status", secHandler)
	rt.Handle("POST", "/api/v1/securities/day/start", secHandler)
	rt.Handle("POST", "/api/v1/securities/day/trading", secHandler)
	rt.Handle("POST", "/api/v1/securities/day/end-trading", secHandler)
	rt.Handle("POST", "/api/v1/securities/day/end", secHandler)
	// Trades and trade corrections (Part A)
	rt.Handle("GET", "/api/v1/securities/trades", secHandler)
	rt.Handle("GET", "/api/v1/securities/trades/{id}", secHandler)
	rt.Handle("POST", "/api/v1/securities/trades/{id}/bust", secHandler)
	rt.Handle("POST", "/api/v1/securities/trades/{id}/correct", secHandler)
	rt.Handle("POST", "/api/v1/securities/trades/{id}/reinstate", secHandler)
	rt.Handle("GET", "/api/v1/securities/trades/{id}/corrections", secHandler)
	// Tick tables (Part B)
	rt.Handle("GET", "/api/v1/securities/tick-tables/{id}", secHandler)
	rt.Handle("PUT", "/api/v1/securities/tick-tables/{id}", secHandler)
	rt.Handle("DELETE", "/api/v1/securities/tick-tables/{id}", secHandler)
	// Positions (P2c Part E)
	rt.Handle("GET", "/api/v1/securities/positions", secHandler)
	// Pending changes (P2c Part C)
	rt.Handle("POST", "/api/v1/securities/pending-changes", secHandler)
	rt.Handle("GET", "/api/v1/securities/pending-changes", secHandler)
	rt.Handle("POST", "/api/v1/securities/pending-changes/{id}/approve", secHandler)
	rt.Handle("POST", "/api/v1/securities/pending-changes/{id}/reject", secHandler)
	// Reference prices (P2c Part D)
	rt.Handle("GET", "/api/v1/securities/instruments/{id}/reference-price", secHandler)
	rt.Handle("POST", "/api/v1/securities/instruments/{id}/reference-price", secHandler)
	logger.Info("securities-service routes registered (HTTP proxy)", slog.String("upstream", securitiesBaseURL))

	// Configure public paths (no auth required)
	authCfg := &middleware.AuthConfig{
		PublicPaths: map[string]bool{
			"/healthz":                        true,
			"/readyz":                         true,
			"/metrics":                        true,
			"POST /api/v1/auth/login":         true,
			"POST /api/v1/auth/register":      true,
			"POST /api/v1/auth/password/reset": true,
			"POST /api/v1/auth/refresh":        true,
		},
		PublicPrefixes: []string{
			"/api/v1/instruments/",
			"/api/v1/market-data/",
			"/api/v1/ws/",
			"/api/v1/admin/demo/",
			"/platform/",
		},
		// Wire the router as a RouteChecker so unknown paths get 404 before auth runs.
		RouteChecker: rt,
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
		case strings.HasPrefix(path, "/api/v1/securities/"):
			return "securities"
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

	// Build tenant middleware with the registered tenant whitelist.
	// TenantMiddleware must run BEFORE auth so that tenant context is available
	// to all downstream handlers. Health/metrics bypass paths are exempt.
	tenantMW := middleware.TenantMiddleware([]string{"ace-commodities", "mse-equities"})

	// Build middleware chain: RequestID → Tracing → Metrics → BodyLimit → Tenant → Auth → RateLimit → Router
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
	httpHandler = tenantMW(httpHandler)
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
