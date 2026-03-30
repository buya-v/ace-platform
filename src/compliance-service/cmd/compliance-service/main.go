package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/garudax-platform/compliance-service/internal/onboarding"
	"github.com/garudax-platform/compliance-service/internal/screening"
	"github.com/garudax-platform/compliance-service/internal/server"
)

func main() {
	log.Println("GarudaX Compliance Service starting...")

	cfg := server.ConfigFromEnv()

	store := onboarding.NewInMemoryStore()
	onboardingSvc := onboarding.NewService(store)

	screeningStore := screening.NewInMemoryStore()
	provider := screening.NewDefaultProvider()
	screeningSvc := screening.NewService(screeningStore, provider, store)

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
