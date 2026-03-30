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
)

func main() {
	log.Println("GarudaX Auth Service starting...")

	cfg := config.ConfigFromEnv()

	if cfg.JWTSigningKey == "" {
		log.Fatal("AUTH_JWT_SIGNING_KEY environment variable is required")
	}

	jwtSvc := auth.NewJWTService(cfg.JWTSigningKey, cfg.AccessTokenTTLSecs, cfg.RefreshTokenTTLSecs)
	repo := store.NewInMemoryStore()
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
