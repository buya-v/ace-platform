package server

import (
	"os"
	"strconv"
)

// Config holds server configuration.
type Config struct {
	// GRPCPort is the port for the gRPC server.
	GRPCPort int
	// HealthPort is the port for health/readiness probes.
	HealthPort int
	// DisableIstioSidecar enables direct pod-to-pod communication
	// by binding to the pod IP instead of localhost.
	DirectPodComms bool
	// BindAddress overrides the default bind address.
	// Empty means "0.0.0.0" (all interfaces for direct pod-to-pod).
	BindAddress string
}

// DefaultConfig returns the default server configuration.
func DefaultConfig() Config {
	return Config{
		GRPCPort:       50051,
		HealthPort:     8081,
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
