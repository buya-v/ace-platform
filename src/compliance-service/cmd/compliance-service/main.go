package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/garudax-platform/compliance-service/integration"
	"github.com/garudax-platform/compliance-service/internal/onboarding"
	"github.com/garudax-platform/compliance-service/internal/screening"
	"github.com/garudax-platform/compliance-service/internal/server"
	"github.com/garudax-platform/compliance-service/internal/store"
	"github.com/garudax-platform/compliance-service/reporting"
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

	// FRC regulatory reporting pipeline (mse-equities flagship). The tenant is
	// configurable; FRC is the regulator for the MSE venue. The RecordingPublisher
	// is the in-memory delivery sink — a real Kafka+S3 publisher is swapped in at
	// deployment behind the same reporting.Publisher interface.
	frcTenant := envOrDefault("FRC_TENANT_ID", "mse-equities")
	frcPublisher := &reporting.RecordingPublisher{}
	if frcReporter, err := reporting.NewReporter(frcTenant, frcPublisher); err != nil {
		log.Printf("FRC reporting disabled: %v", err)
	} else {
		srv.SetFRCReporter(frcReporter, frcPublisher)
		log.Printf("FRC reporting enabled for tenant %s", frcTenant)
	}

	// MCSD custody/settlement integration (mse-equities only). In-memory stub
	// adapter for now; a real ISO 20022 adapter swaps in behind integration.CSDAdapter.
	srv.SetCSDAdapter(integration.NewStubAdapter())
	log.Println("MCSD integration enabled (in-memory stub adapter)")

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
