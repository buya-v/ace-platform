package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/garudax-platform/compliance-service/internal/onboarding"
	"github.com/garudax-platform/compliance-service/internal/screening"
	"github.com/garudax-platform/compliance-service/internal/server"
	"github.com/garudax-platform/compliance-service/internal/store"
)

func main() {
	log.Println("GarudaX Compliance Service starting...")

	cfg := server.ConfigFromEnv()

	var onboardStore onboarding.Store
	var screenStore screening.Store

	if dbHost := os.Getenv("DB_HOST"); dbHost != "" {
		log.Println("PostgreSQL mode: connecting to database...")
		dsn := buildDSN(dbHost)
		db, err := store.OpenPostgres(dsn)
		if err != nil {
			log.Fatalf("Failed to connect to PostgreSQL: %v", err)
		}
		defer db.Close()

		if err := db.Ping(); err != nil {
			log.Fatalf("Failed to ping PostgreSQL: %v", err)
		}
		log.Println("PostgreSQL connection established")

		onboardStore = store.NewPostgresOnboardingStore(db)
		screenStore = store.NewPostgresScreeningStore(db)
	} else {
		log.Println("In-memory mode: no DB_HOST set")
		memOnboard := onboarding.NewInMemoryStore()
		onboardStore = memOnboard
		screenStore = screening.NewInMemoryStore()
	}

	onboardingSvc := onboarding.NewService(onboardStore)

	provider := screening.NewDefaultProvider()
	screeningSvc := screening.NewService(screenStore, provider, onboardStore)

	srv := server.NewServer(onboardingSvc, screeningSvc, cfg)

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
	log.Printf("GarudaX Compliance Service ready (gRPC=%s, health=%s:%d)",
		lis.Addr().String(), cfg.BindAddress, cfg.HealthPort)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("Received signal %s, shutting down...", sig)
	lis.Close()
}

func buildDSN(host string) string {
	port := envOrDefault("DB_PORT", "5432")
	user := envOrDefault("DB_USER", "garudax")
	pass := envOrDefault("DB_PASSWORD", "garudax_dev_password")
	dbName := envOrDefault("DB_NAME", "garudax")
	sslMode := envOrDefault("DB_SSL_MODE", "disable")
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", user, pass, host, port, dbName, sslMode)
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
