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

	// Initialize JWT service: RS256 if key paths are set, HS256 otherwise
	var jwtSvc *auth.JWTService
	if cfg.JWTRSAPrivateKeyPath != "" {
		privKey, pubKey, err := auth.LoadRSAPrivateKeyOnly(cfg.JWTRSAPrivateKeyPath)
		if err != nil {
			log.Fatalf("Failed to load RSA private key from %s: %v", cfg.JWTRSAPrivateKeyPath, err)
		}
		// If public key path is also set, load it explicitly (cross-validation)
		if cfg.JWTRSAPublicKeyPath != "" {
			_, pubKey, err = auth.LoadRSAKeyPair(cfg.JWTRSAPrivateKeyPath, cfg.JWTRSAPublicKeyPath)
			if err != nil {
				log.Fatalf("Failed to load RSA key pair: %v", err)
			}
		}
		jwtSvc = auth.NewJWTServiceRS256(privKey, pubKey, "key-1", cfg.AccessTokenTTLSecs, cfg.RefreshTokenTTLSecs)
		log.Println("JWT signing: RS256 (asymmetric)")
	} else {
		// HS256 mode
		if cfg.ProductionMode && cfg.JWTSigningKey == "" {
			log.Fatal("PRODUCTION_MODE is set: AUTH_JWT_SIGNING_KEY or JWT_RSA_PRIVATE_KEY_PATH is required")
		}
		if cfg.JWTSigningKey == "" {
			log.Println("WARNING: using default dev JWT secret — set AUTH_JWT_SIGNING_KEY for production")
			cfg.JWTSigningKey = "garudax-dev-secret-do-not-use-in-production"
		}
		jwtSvc = auth.NewJWTService(cfg.JWTSigningKey, cfg.AccessTokenTTLSecs, cfg.RefreshTokenTTLSecs)
		log.Println("JWT signing: HS256 (symmetric)")
	}

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

	// Wire demo reset — works with both in-memory and PostgreSQL stores
	switch s := repo.(type) {
	case *store.InMemoryStore:
		srv.SetResetter(s)
	case *store.PostgresStore:
		srv.SetResetter(s)
	}

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
