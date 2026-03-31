package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/garudax-platform/warehouse-service/internal/server"
	"github.com/garudax-platform/warehouse-service/internal/service"
	"github.com/garudax-platform/warehouse-service/internal/store"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	log.Println("GarudaX Warehouse Service starting...")

	cfg := server.ConfigFromEnv()

	var ds store.DataStore

	if dbHost := os.Getenv("DB_HOST"); dbHost != "" {
		// PostgreSQL mode
		dsn := buildDSN(dbHost)
		log.Printf("Connecting to PostgreSQL at %s ...", dbHost)

		db, err := sql.Open("pgx", dsn)
		if err != nil {
			log.Fatalf("Failed to open database: %v", err)
		}
		defer db.Close()

		if err := db.Ping(); err != nil {
			log.Fatalf("Failed to ping database: %v", err)
		}
		log.Println("PostgreSQL connection established")

		ds = store.NewPostgresStore(db)
	} else {
		// In-memory fallback
		log.Println("No DB_HOST set — using in-memory store")
		ds = store.NewStore()
	}

	svc := service.New(ds)
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
	log.Printf("GarudaX Warehouse Service ready (gRPC=%s, health=%s)",
		lis.Addr().String(), srv.GRPCAddr())

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("Received signal %s, shutting down...", sig)
	lis.Close()
}

func buildDSN(host string) string {
	port := envOrDefault("DB_PORT", "5432")
	user := envOrDefault("DB_USER", "garudax_warehouse_svc")
	pass := envOrDefault("DB_PASSWORD", "")
	name := envOrDefault("DB_NAME", "garudax")
	sslmode := envOrDefault("DB_SSLMODE", "disable")

	dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=%s",
		host, port, user, name, sslmode)
	if pass != "" {
		dsn += fmt.Sprintf(" password=%s", pass)
	}
	if schema := os.Getenv("DB_SCHEMA"); schema != "" {
		dsn += fmt.Sprintf(" search_path=%s", schema)
	}
	return dsn
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
