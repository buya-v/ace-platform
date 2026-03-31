package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestNewLogger(t *testing.T) {
	logger := NewLogger("test-service")
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestNewLoggerWithLevel(t *testing.T) {
	logger := NewLoggerWithLevel("test-service", slog.LevelDebug)
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestNewLoggerOutputsJSON(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler).With(slog.String("service_name", "test-svc"))

	logger.Info("hello world", slog.String("key", "value"))

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("expected valid JSON, got: %s", buf.String())
	}

	if entry["service_name"] != "test-svc" {
		t.Errorf("expected service_name=test-svc, got %v", entry["service_name"])
	}
	if entry["msg"] != "hello world" {
		t.Errorf("expected msg=hello world, got %v", entry["msg"])
	}
	if entry["key"] != "value" {
		t.Errorf("expected key=value, got %v", entry["key"])
	}
	if _, ok := entry["time"]; !ok {
		t.Error("expected time field in log entry")
	}
	if entry["level"] != "INFO" {
		t.Errorf("expected level=INFO, got %v", entry["level"])
	}
}

func TestTraceIDContext(t *testing.T) {
	ctx := context.Background()

	// No trace ID initially
	if got := TraceIDFromContext(ctx); got != "" {
		t.Errorf("expected empty trace_id, got %s", got)
	}

	// Set and retrieve
	ctx = WithTraceID(ctx, "abc-123")
	if got := TraceIDFromContext(ctx); got != "abc-123" {
		t.Errorf("expected abc-123, got %s", got)
	}
}

func TestRequestIDContext(t *testing.T) {
	ctx := context.Background()

	if got := RequestIDFromContext(ctx); got != "" {
		t.Errorf("expected empty request_id, got %s", got)
	}

	ctx = WithRequestID(ctx, "req-456")
	if got := RequestIDFromContext(ctx); got != "req-456" {
		t.Errorf("expected req-456, got %s", got)
	}
}

func TestLoggerFromContext(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	base := slog.New(handler).With(slog.String("service_name", "ctx-test"))

	ctx := context.Background()
	ctx = WithTraceID(ctx, "trace-abc")
	ctx = WithRequestID(ctx, "req-xyz")

	logger := LoggerFromContext(ctx, base)
	logger.Info("test message")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("expected valid JSON, got: %s", buf.String())
	}

	if entry["trace_id"] != "trace-abc" {
		t.Errorf("expected trace_id=trace-abc, got %v", entry["trace_id"])
	}
	if entry["request_id"] != "req-xyz" {
		t.Errorf("expected request_id=req-xyz, got %v", entry["request_id"])
	}
	if entry["service_name"] != "ctx-test" {
		t.Errorf("expected service_name=ctx-test, got %v", entry["service_name"])
	}
}

func TestLoggerFromContextNoIDs(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	base := slog.New(handler).With(slog.String("service_name", "no-ids"))

	ctx := context.Background()
	logger := LoggerFromContext(ctx, base)
	logger.Info("bare message")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("expected valid JSON, got: %s", buf.String())
	}

	if _, ok := entry["trace_id"]; ok {
		t.Error("expected no trace_id field when not set in context")
	}
	if _, ok := entry["request_id"]; ok {
		t.Error("expected no request_id field when not set in context")
	}
}
