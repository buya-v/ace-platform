package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/garudax-platform/auth-service/internal/auth"
	"github.com/garudax-platform/auth-service/internal/config"
	"github.com/garudax-platform/auth-service/internal/handler"
	"github.com/garudax-platform/auth-service/internal/server"
	"github.com/garudax-platform/auth-service/internal/store"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	log.Println("GarudaX Auth Service starting...")

	cfg := config.ConfigFromEnv()

	if cfg.JWTSigningKey == "" {
		log.Fatal("AUTH_JWT_SIGNING_KEY environment variable is required")
	}

	jwtSvc := auth.NewJWTService(cfg.JWTSigningKey, cfg.AccessTokenTTLSecs, cfg.RefreshTokenTTLSecs)

	// Use PostgreSQL store when DB_HOST is explicitly set, otherwise fall back to in-memory.
	var repo auth.Store
	if os.Getenv("DB_HOST") != "" {
		db, err := store.ConnectPostgres(
			cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPass, cfg.DBName, cfg.DBSSLMode,
		)
		if err != nil {
			log.Fatalf("Failed to connect to PostgreSQL: %v", err)
		}
		defer db.Close()
		pgStore := store.NewPostgresStore(db)
		repo = pgStore
		log.Printf("Using PostgreSQL store (%s:%d/%s)", cfg.DBHost, cfg.DBPort, cfg.DBName)
	} else {
		repo = store.NewInMemoryStore()
		log.Println("Using in-memory store (set DB_HOST to enable PostgreSQL)")
	}

	// Layer Redis session store on top when REDIS_URL is set.
	// Sessions are stored in Redis with TTL; all other data stays in the
	// underlying store (Postgres or in-memory).
	if cfg.RedisURL != "" {
		sessionTTL := time.Duration(cfg.RefreshTokenTTLSecs) * time.Second
		redisStore, err := store.NewRedisSessionStore(repo, cfg.RedisURL, sessionTTL)
		if err != nil {
			log.Printf("WARNING: Redis session store unavailable (%v) — using fallback", err)
		} else {
			repo = redisStore
			defer redisStore.Close()
			log.Printf("Using Redis session store (%s)", store.ParseRedisURL(cfg.RedisURL))
		}
	}

	lockoutDuration := time.Duration(cfg.LockoutDurationMins) * time.Minute
	authSvc := auth.NewService(repo, jwtSvc, cfg.BcryptCost, cfg.MaxFailedAttempts, lockoutDuration)

	h := handler.NewAuthHandler(authSvc)
	srv := server.NewServer(h, server.Config{
		GRPCPort:    cfg.GRPCPort,
		HealthPort:  cfg.HealthPort,
		BindAddress: cfg.BindAddress,
	})

	go func() {
		if err := srv.StartHealthServer(); err != nil {
			log.Fatalf("Health server error: %v", err)
		}
	}()

	lis, err := srv.ListenGRPC()
	if err != nil {
		log.Fatalf("Failed to create gRPC listener: %v", err)
	}

	srv.SetReady()
	log.Printf("GarudaX Auth Service ready (gRPC=%s, health=%s:%d)",
		lis.Addr().String(), cfg.BindAddress, cfg.HealthPort)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("Received signal %s, shutting down...", sig)
	lis.Close()
}
