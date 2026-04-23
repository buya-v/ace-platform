package server

import (
	"os"
	"strconv"
)

type Config struct {
	GRPCPort    int
	HealthPort  int
	BindAddress string
}

func DefaultConfig() Config {
	return Config{
		GRPCPort:    50056,
		HealthPort:  8086,
		BindAddress: "0.0.0.0",
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
	return cfg
}
