package observability

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTracingMiddlewareGeneratesTraceID(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler).With(slog.String("service_name", "test"))

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify trace ID is in context
		traceID := TraceIDFromContext(r.Context())
		if traceID == "" {
			t.Error("expected trace_id in context")
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mw := TracingMiddleware(logger)(inner)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	// Check response headers
	xReqID := rec.Header().Get("X-Request-ID")
	if xReqID == "" {
		t.Error("expected X-Request-ID header")
	}
	xTraceID := rec.Header().Get("X-Trace-ID")
	if xTraceID == "" {
		t.Error("expected X-Trace-ID header")
	}
	if xReqID != xTraceID {
		t.Errorf("expected X-Request-ID=%s to equal X-Trace-ID=%s", xReqID, xTraceID)
	}
}

func TestTracingMiddlewareUsesExistingRequestID(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler).With(slog.String("service_name", "test"))

	expectedID := "existing-trace-id-123"

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := TraceIDFromContext(r.Context())
		if traceID != expectedID {
			t.Errorf("expected trace_id=%s, got %s", expectedID, traceID)
		}
		w.WriteHeader(http.StatusOK)
	})

	mw := TracingMiddleware(logger)(inner)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", expectedID)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Header().Get("X-Request-ID") != expectedID {
		t.Errorf("expected X-Request-ID=%s, got %s", expectedID, rec.Header().Get("X-Request-ID"))
	}
}

func TestTracingMiddlewareLogsRequestStartAndComplete(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler).With(slog.String("service_name", "test"))

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"status":"created"}`))
	})

	mw := TracingMiddleware(logger)(inner)

	req := httptest.NewRequest("POST", "/api/orders", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	// Parse log lines (two JSON objects separated by newlines)
	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 log lines, got %d: %s", len(lines), buf.String())
	}

	// First line: request_started
	var startEntry map[string]interface{}
	if err := json.Unmarshal(lines[0], &startEntry); err != nil {
		t.Fatalf("failed to parse start log: %v", err)
	}
	if startEntry["msg"] != "request_started" {
		t.Errorf("expected msg=request_started, got %v", startEntry["msg"])
	}
	if startEntry["method"] != "POST" {
		t.Errorf("expected method=POST, got %v", startEntry["method"])
	}
	if startEntry["path"] != "/api/orders" {
		t.Errorf("expected path=/api/orders, got %v", startEntry["path"])
	}

	// Second line: request_completed
	var endEntry map[string]interface{}
	if err := json.Unmarshal(lines[1], &endEntry); err != nil {
		t.Fatalf("failed to parse end log: %v", err)
	}
	if endEntry["msg"] != "request_completed" {
		t.Errorf("expected msg=request_completed, got %v", endEntry["msg"])
	}
	if endEntry["status"] != float64(201) {
		t.Errorf("expected status=201, got %v", endEntry["status"])
	}
	if endEntry["response_bytes"] != float64(20) {
		t.Errorf("expected response_bytes=20, got %v", endEntry["response_bytes"])
	}
	if _, ok := endEntry["duration"]; !ok {
		t.Error("expected duration field in completed log")
	}
	if _, ok := endEntry["duration_ms"]; !ok {
		t.Error("expected duration_ms field in completed log")
	}
}

func TestTracingMiddleware500LogsError(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler).With(slog.String("service_name", "test"))

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	mw := TracingMiddleware(logger)(inner)
	req := httptest.NewRequest("GET", "/fail", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	if len(lines) < 2 {
		t.Fatalf("expected 2 log lines, got %d", len(lines))
	}

	var endEntry map[string]interface{}
	json.Unmarshal(lines[1], &endEntry)
	if endEntry["level"] != "ERROR" {
		t.Errorf("expected level=ERROR for 500, got %v", endEntry["level"])
	}
}

func TestTracingMiddleware4xxLogsWarn(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler).With(slog.String("service_name", "test"))

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	mw := TracingMiddleware(logger)(inner)
	req := httptest.NewRequest("GET", "/missing", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	if len(lines) < 2 {
		t.Fatalf("expected 2 log lines, got %d", len(lines))
	}

	var endEntry map[string]interface{}
	json.Unmarshal(lines[1], &endEntry)
	if endEntry["level"] != "WARN" {
		t.Errorf("expected level=WARN for 404, got %v", endEntry["level"])
	}
}

func TestGenerateTraceID(t *testing.T) {
	id1 := generateTraceID()
	id2 := generateTraceID()

	if len(id1) != 32 {
		t.Errorf("expected 32-char hex string, got %d chars: %s", len(id1), id1)
	}
	if id1 == id2 {
		t.Error("expected unique trace IDs")
	}
}
