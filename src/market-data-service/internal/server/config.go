package server

import (
	"os"
	"strconv"
)

// Config holds server configuration.
type Config struct {
	GRPCPort       int
	HealthPort     int
	BindAddress    string
}

// DefaultConfig returns the default server configuration.
func DefaultConfig() Config {
	return Config{
		GRPCPort:    50057,
		HealthPort:  8087,
		BindAddress: "0.0.0.0",
	}
}

// ConfigFromEnv reads configuration from environment variables.
func ConfigFromEnv() Config {
	cfg := DefaultConfig()

	if v := os.Getenv("GRPC_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.GRPCPort = port
		}
	}
	if v := os.Getenv("HEALTH_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.HealthPort = port
		}
	}
	if v := os.Getenv("BIND_ADDRESS"); v != "" {
		cfg.BindAddress = v
	}
	return cfg
}
