package observability

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewMetrics(t *testing.T) {
	m := NewMetrics("test-service")
	if m == nil {
		t.Fatal("expected non-nil metrics")
	}
	if m.serviceName != "test-service" {
		t.Errorf("expected service name test-service, got %s", m.serviceName)
	}
}

func TestMetricsMiddlewareRecordsRequests(t *testing.T) {
	m := NewMetrics("test-svc")

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := m.MetricsMiddleware()(inner)

	// Make 3 requests
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	// Make 1 POST request
	req := httptest.NewRequest("POST", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Check metrics output
	metricsReq := httptest.NewRequest("GET", "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	m.MetricsHandler().ServeHTTP(metricsRec, metricsReq)

	body := metricsRec.Body.String()
	if !strings.Contains(body, `http_requests_total{service="test-svc",method_status="GET_200"} 3`) {
		t.Errorf("expected GET_200 count of 3 in metrics output, got:\n%s", body)
	}
	if !strings.Contains(body, `http_requests_total{service="test-svc",method_status="POST_200"} 1`) {
		t.Errorf("expected POST_200 count of 1 in metrics output, got:\n%s", body)
	}
}

func TestMetricsHandlerIncludesHistogram(t *testing.T) {
	m := NewMetrics("hist-test")

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := m.MetricsMiddleware()(inner)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	metricsReq := httptest.NewRequest("GET", "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	m.MetricsHandler().ServeHTTP(metricsRec, metricsReq)

	body := metricsRec.Body.String()
	if !strings.Contains(body, "http_request_duration_seconds_bucket") {
		t.Errorf("expected histogram buckets in metrics output, got:\n%s", body)
	}
	if !strings.Contains(body, "http_request_duration_seconds_count") {
		t.Errorf("expected histogram count in metrics output, got:\n%s", body)
	}
	if !strings.Contains(body, "http_request_duration_seconds_sum") {
		t.Errorf("expected histogram sum in metrics output, got:\n%s", body)
	}
}

func TestMetricsJSONHandler(t *testing.T) {
	m := NewMetrics("json-test")

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := m.MetricsMiddleware()(inner)
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	metricsReq := httptest.NewRequest("GET", "/metrics.json", nil)
	metricsRec := httptest.NewRecorder()
	m.MetricsJSON().ServeHTTP(metricsRec, metricsReq)

	if ct := metricsRec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}

	body := metricsRec.Body.String()
	if !strings.Contains(body, "json-test") {
		t.Errorf("expected service name in JSON output, got: %s", body)
	}
	if !strings.Contains(body, "request_counts") {
		t.Errorf("expected request_counts in JSON output, got: %s", body)
	}
}

func TestLatencyHistogramObserve(t *testing.T) {
	h := newLatencyHistogram()

	// Observe a few durations
	h.observe(1 * time.Millisecond)   // falls in 0.005s bucket
	h.observe(50 * time.Millisecond)  // falls in 0.05s bucket
	h.observe(500 * time.Millisecond) // falls in 0.5s bucket

	if h.totalCount() != 3 {
		t.Errorf("expected count=3, got %d", h.totalCount())
	}

	sum := h.totalSum()
	if sum < 0.5 || sum > 0.6 {
		t.Errorf("expected sum ~0.551, got %f", sum)
	}

	snap := h.snapshot()
	// The +Inf bucket should have cumulative count equal to total observations
	lastBucket := snap[len(snap)-1]
	if lastBucket.le != "+Inf" {
		t.Errorf("expected last bucket le=+Inf, got %s", lastBucket.le)
	}
	// Each observation increments all buckets with upperMs >= the observed value.
	// snapshot() then computes a cumulative sum across buckets.
	// With 3 observations, the +Inf cumulative count should be >= 3.
	if lastBucket.count < 3 {
		t.Errorf("expected +Inf bucket count >= 3, got %d", lastBucket.count)
	}

	// The 0.005 (5ms) bucket should only contain the 1ms observation
	firstBucket := snap[0]
	if firstBucket.le != "0.005" {
		t.Errorf("expected first bucket le=0.005, got %s", firstBucket.le)
	}
	if firstBucket.count != 1 {
		t.Errorf("expected 0.005 bucket count=1, got %d", firstBucket.count)
	}
}

func TestLatencyHistogramSummaryMap(t *testing.T) {
	h := newLatencyHistogram()
	h.observe(10 * time.Millisecond)

	summary := h.summaryMap()
	if summary["count"] != int64(1) {
		t.Errorf("expected count=1, got %v", summary["count"])
	}
	buckets, ok := summary["buckets"].(map[string]int64)
	if !ok {
		t.Fatal("expected buckets to be map[string]int64")
	}
	if _, exists := buckets["+Inf"]; !exists {
		t.Error("expected +Inf bucket in summary")
	}
}

func TestMetricsMiddlewareRecordsDifferentStatusCodes(t *testing.T) {
	m := NewMetrics("status-test")

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	})

	handler := m.MetricsMiddleware()(inner)

	// 200 request
	req := httptest.NewRequest("GET", "/ok", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// 404 request
	req = httptest.NewRequest("GET", "/missing", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	metricsReq := httptest.NewRequest("GET", "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	m.MetricsHandler().ServeHTTP(metricsRec, metricsReq)

	body := metricsRec.Body.String()
	if !strings.Contains(body, "GET_200") {
		t.Errorf("expected GET_200 in metrics, got:\n%s", body)
	}
	if !strings.Contains(body, "GET_404") {
		t.Errorf("expected GET_404 in metrics, got:\n%s", body)
	}
}
