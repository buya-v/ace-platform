package server

import (
	"os"
	"strconv"
)

// Config holds server configuration.
type Config struct {
	GRPCPort       int
	HealthPort     int
	DirectPodComms bool
	BindAddress    string
}

// DefaultConfig returns the default server configuration.
func DefaultConfig() Config {
	return Config{
		GRPCPort:       50058,
		HealthPort:     8088,
		DirectPodComms: true,
		BindAddress:    "0.0.0.0",
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
	if v := os.Getenv("DIRECT_POD_COMMS"); v == "false" {
		cfg.DirectPodComms = false
	}
	return cfg
}
