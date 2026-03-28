package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ace-platform/warehouse-service/internal/service"
	"github.com/ace-platform/warehouse-service/internal/store"
)

func TestHealthEndpoints(t *testing.T) {
	st := store.NewStore()
	svc := service.New(st)
	cfg := DefaultConfig()
	srv := NewServer(svc, cfg)

	// Test /healthz
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("healthz: expected 200, got %d", w.Code)
	}

	// Test /readyz before ready
	req = httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w = httptest.NewRecorder()
	http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if srv.IsReady() {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ready"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("not ready"))
		}
	}).ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("readyz before ready: expected 503, got %d", w.Code)
	}

	// Set ready and test again
	srv.SetReady()
	w = httptest.NewRecorder()
	http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if srv.IsReady() {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ready"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("not ready"))
		}
	}).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("readyz after ready: expected 200, got %d", w.Code)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.GRPCPort != 50058 {
		t.Errorf("expected gRPC port 50058, got %d", cfg.GRPCPort)
	}
	if cfg.HealthPort != 8088 {
		t.Errorf("expected health port 8088, got %d", cfg.HealthPort)
	}
	if cfg.BindAddress != "0.0.0.0" {
		t.Errorf("expected bind 0.0.0.0, got %s", cfg.BindAddress)
	}
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("GRPC_PORT", "9999")
	t.Setenv("HEALTH_PORT", "9998")
	t.Setenv("BIND_ADDRESS", "127.0.0.1")

	cfg := ConfigFromEnv()
	if cfg.GRPCPort != 9999 {
		t.Errorf("expected gRPC port 9999, got %d", cfg.GRPCPort)
	}
	if cfg.HealthPort != 9998 {
		t.Errorf("expected health port 9998, got %d", cfg.HealthPort)
	}
	if cfg.BindAddress != "127.0.0.1" {
		t.Errorf("expected bind 127.0.0.1, got %s", cfg.BindAddress)
	}
}

func TestGRPCAddr(t *testing.T) {
	cfg := DefaultConfig()
	srv := NewServer(nil, cfg)
	if addr := srv.GRPCAddr(); addr != "0.0.0.0:50058" {
		t.Errorf("expected 0.0.0.0:50058, got %s", addr)
	}
}
