package server

import (
	"os"
	"strconv"
)

type Config struct {
	GRPCPort       int
	HealthPort     int
	DirectPodComms bool
	BindAddress    string
}

func DefaultConfig() Config {
	return Config{
		GRPCPort:       50054,
		HealthPort:     8084,
		DirectPodComms: true,
		BindAddress:    "0.0.0.0",
	}
}

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
