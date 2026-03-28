package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds gateway configuration.
type Config struct {
	// Server
	HTTPPort    int
	HealthPort  int
	BindAddress string

	// Timeouts
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
	BackendTimeout  time.Duration

	// JWT
	JWTSecret    string
	JWTIssuer    string
	JWTAudience  string

	// Backend services
	MatchingEngineAddr  string
	ClearingEngineAddr  string
	MarginEngineAddr    string
	SettlementEngineAddr string
	AuthServiceAddr       string
	ComplianceServiceAddr string
	MarketDataServiceAddr string
	WarehouseServiceAddr  string

	// Rate limiting
	RateLimitEnabled bool

	// Request limits
	MaxBodySize int64
}

// FromEnv creates a Config from environment variables with sensible defaults.
func FromEnv() *Config {
	return &Config{
		HTTPPort:    envInt("HTTP_PORT", 8080),
		HealthPort:  envInt("HEALTH_PORT", 8090),
		BindAddress: envStr("BIND_ADDRESS", "0.0.0.0"),

		ReadTimeout:     envDuration("READ_TIMEOUT", 30*time.Second),
		WriteTimeout:    envDuration("WRITE_TIMEOUT", 30*time.Second),
		IdleTimeout:     envDuration("IDLE_TIMEOUT", 120*time.Second),
		ShutdownTimeout: envDuration("SHUTDOWN_TIMEOUT", 15*time.Second),
		BackendTimeout:  envDuration("BACKEND_TIMEOUT", 30*time.Second),

		JWTSecret:   envStr("JWT_SECRET", "ace-dev-secret-change-in-production"),
		JWTIssuer:   envStr("JWT_ISSUER", "ace-auth-service"),
		JWTAudience: envStr("JWT_AUDIENCE", "ace-api-gateway"),

		MatchingEngineAddr:    envStr("MATCHING_ENGINE_ADDR", "localhost:50051"),
		ClearingEngineAddr:    envStr("CLEARING_ENGINE_ADDR", "localhost:50052"),
		MarginEngineAddr:      envStr("MARGIN_ENGINE_ADDR", "localhost:50053"),
		SettlementEngineAddr:  envStr("SETTLEMENT_ENGINE_ADDR", "localhost:50054"),
		AuthServiceAddr:       envStr("AUTH_SERVICE_ADDR", "localhost:50055"),
		ComplianceServiceAddr: envStr("COMPLIANCE_SERVICE_ADDR", "localhost:50056"),
		MarketDataServiceAddr: envStr("MARKET_DATA_SERVICE_ADDR", "localhost:50057"),
		WarehouseServiceAddr:  envStr("WAREHOUSE_SERVICE_ADDR", "localhost:50058"),

		RateLimitEnabled: envBool("RATE_LIMIT_ENABLED", true),

		MaxBodySize: int64(envInt("MAX_BODY_SIZE", 1048576)), // 1MB
	}
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
