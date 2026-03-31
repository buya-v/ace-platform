// Package db provides a PostgreSQL connection pool wrapper with configuration
// from environment variables, health checks, transaction helpers, and a simple
// migration runner. It uses pgxpool for connection pooling and is designed to
// be the shared database foundation for all GarudaX platform services.
package db

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Config holds database connection configuration.
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
	MaxConns int32
	MinConns int32
}

// DefaultConfig returns a Config populated from environment variables,
// falling back to sensible defaults for local development.
func DefaultConfig() Config {
	return Config{
		Host:     envOrDefault("DB_HOST", "localhost"),
		Port:     envOrDefaultInt("DB_PORT", 5432),
		User:     envOrDefault("DB_USER", "garudax"),
		Password: envOrDefault("DB_PASSWORD", "garudax_dev_password"),
		DBName:   envOrDefault("DB_NAME", "garudax"),
		SSLMode:  envOrDefault("DB_SSL_MODE", "disable"),
		MaxConns: int32(envOrDefaultInt("DB_MAX_CONNS", 10)),
		MinConns: int32(envOrDefaultInt("DB_MIN_CONNS", 2)),
	}
}

// DSN returns the PostgreSQL connection string for this configuration.
func (c Config) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.DBName, c.SSLMode,
	)
}

// Pool wraps a *pgxpool.Pool and provides convenience methods for
// health checks, transactions, and graceful shutdown.
type Pool struct {
	pool   *pgxpool.Pool
	config Config
}

// NewPool creates a new database connection pool from the given Config.
// It validates the configuration, builds a pgxpool, and verifies connectivity
// by pinging the database.
func NewPool(ctx context.Context, cfg Config) (*Pool, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("db: invalid config: %w", err)
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("db: parse config: %w", err)
	}

	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	poolCfg.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("db: create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping failed: %w", err)
	}

	return &Pool{pool: pool, config: cfg}, nil
}

// Raw returns the underlying *pgxpool.Pool for cases where direct
// access is needed (e.g., custom queries, batch operations).
func (p *Pool) Raw() *pgxpool.Pool {
	return p.pool
}

// Close gracefully shuts down the connection pool.
func (p *Pool) Close() {
	p.pool.Close()
}

// Config returns the configuration used to create this pool.
func (p *Pool) Config() Config {
	return p.config
}

func validateConfig(cfg Config) error {
	if cfg.Host == "" {
		return fmt.Errorf("host is required")
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", cfg.Port)
	}
	if cfg.User == "" {
		return fmt.Errorf("user is required")
	}
	if cfg.DBName == "" {
		return fmt.Errorf("database name is required")
	}
	if cfg.MaxConns < 1 {
		return fmt.Errorf("max_conns must be at least 1, got %d", cfg.MaxConns)
	}
	if cfg.MinConns < 0 {
		return fmt.Errorf("min_conns must be non-negative, got %d", cfg.MinConns)
	}
	if cfg.MinConns > cfg.MaxConns {
		return fmt.Errorf("min_conns (%d) must not exceed max_conns (%d)", cfg.MinConns, cfg.MaxConns)
	}
	return nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envOrDefaultInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
