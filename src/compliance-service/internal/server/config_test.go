package server

import (
	"os"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.GRPCPort != 50056 {
		t.Errorf("expected gRPC port 50056, got %d", cfg.GRPCPort)
	}
	if cfg.HealthPort != 8086 {
		t.Errorf("expected health port 8086, got %d", cfg.HealthPort)
	}
	if cfg.BindAddress != "0.0.0.0" {
		t.Errorf("expected bind address 0.0.0.0, got %s", cfg.BindAddress)
	}
}

func TestConfigFromEnv(t *testing.T) {
	os.Setenv("GRPC_PORT", "50099")
	os.Setenv("HEALTH_PORT", "8099")
	os.Setenv("BIND_ADDRESS", "127.0.0.1")
	defer func() {
		os.Unsetenv("GRPC_PORT")
		os.Unsetenv("HEALTH_PORT")
		os.Unsetenv("BIND_ADDRESS")
	}()

	cfg := ConfigFromEnv()
	if cfg.GRPCPort != 50099 {
		t.Errorf("expected gRPC port 50099, got %d", cfg.GRPCPort)
	}
	if cfg.HealthPort != 8099 {
		t.Errorf("expected health port 8099, got %d", cfg.HealthPort)
	}
	if cfg.BindAddress != "127.0.0.1" {
		t.Errorf("expected bind address 127.0.0.1, got %s", cfg.BindAddress)
	}
}
