package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ace-platform/warehouse-service/internal/server"
	"github.com/ace-platform/warehouse-service/internal/service"
	"github.com/ace-platform/warehouse-service/internal/store"
)

func main() {
	log.Println("ACE Warehouse Service starting...")

	cfg := server.ConfigFromEnv()
	st := store.NewStore()
	svc := service.New(st)
	srv := server.NewServer(svc, cfg)

	// Start health server in background
	go func() {
		if err := srv.StartHealthServer(); err != nil {
			log.Fatalf("Health server error: %v", err)
		}
	}()

	// Create gRPC listener
	lis, err := srv.ListenGRPC()
	if err != nil {
		log.Fatalf("Failed to create gRPC listener: %v", err)
	}

	srv.SetReady()
	log.Printf("ACE Warehouse Service ready (gRPC=%s, health=%s)",
		lis.Addr().String(), srv.GRPCAddr())

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("Received signal %s, shutting down...", sig)
	lis.Close()
}
