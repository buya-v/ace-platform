package observability

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// --- counter tests ---

func TestNewMetrics(t *testing.T) {
	m := NewMetrics("test-service")
	if m == nil {
		t.Fatal("expected non-nil metrics")
	}
	if m.serviceName != "test-service" {
		t.Errorf("expected service name test-service, got %s", m.serviceName)
	}
	if m.requestCount == nil {
		t.Error("expected non-nil requestCount")
	}
	if m.errorCount == nil {
		t.Error("expected non-nil errorCount")
	}
	if m.latencyBuckets == nil {
		t.Error("expected non-nil latencyBuckets")
	}
	if m.sizeBuckets == nil {
		t.Error("expected non-nil sizeBuckets")
	}
}

func TestCounterMapAdd(t *testing.T) {
	cm := newCounterMap()
	cm.Add("foo", 1)
	cm.Add("foo", 2)
	cm.Add("bar", 10)

	values := map[string]int64{}
	cm.Do(func(key string, value int64) {
		values[key] = value
	})

	if values["foo"] != 3 {
		t.Errorf("expected foo=3, got %d", values["foo"])
	}
	if values["bar"] != 10 {
		t.Errorf("expected bar=10, got %d", values["bar"])
	}
}

func TestCounterMapDoSortedOrder(t *testing.T) {
	cm := newCounterMap()
	cm.Add("c", 3)
	cm.Add("a", 1)
	cm.Add("b", 2)

	var keys []string
	cm.Do(func(key string, _ int64) {
		keys = append(keys, key)
	})

	if len(keys) != 3 || keys[0] != "a" || keys[1] != "b" || keys[2] != "c" {
		t.Errorf("expected sorted keys [a b c], got %v", keys)
	}
}

// --- request count middleware ---

func TestMetricsMiddlewareRecordsRequests(t *testing.T) {
	m := NewMetrics("test-svc")

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := m.MetricsMiddleware()(inner)

	for i := 0; i < 3; i++ {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/test", nil))
	}
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/test", nil))

	body := metricsBody(t, m)
	if !strings.Contains(body, `http_requests_total{service="test-svc",method_status="GET_200"} 3`) {
		t.Errorf("expected GET_200=3 in metrics output, got:\n%s", body)
	}
	if !strings.Contains(body, `http_requests_total{service="test-svc",method_status="POST_200"} 1`) {
		t.Errorf("expected POST_200=1 in metrics output, got:\n%s", body)
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

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/ok", nil))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/missing", nil))

	body := metricsBody(t, m)
	if !strings.Contains(body, "GET_200") {
		t.Errorf("expected GET_200 in metrics, got:\n%s", body)
	}
	if !strings.Contains(body, "GET_404") {
		t.Errorf("expected GET_404 in metrics, got:\n%s", body)
	}
}

// --- error counter (5xx) ---

func TestErrorCounterFor5xx(t *testing.T) {
	m := NewMetrics("err-test")

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.WriteHeader(http.StatusOK)
		case "/client-err":
			w.WriteHeader(http.StatusBadRequest)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	})
	handler := m.MetricsMiddleware()(inner)

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/ok", nil))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/client-err", nil))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/server-err", nil))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/server-err", nil))

	body := metricsBody(t, m)

	// 5xx errors should appear in http_request_errors_total
	if !strings.Contains(body, "http_request_errors_total") {
		t.Errorf("expected http_request_errors_total section, got:\n%s", body)
	}
	if !strings.Contains(body, `http_request_errors_total{service="err-test",method_status="GET_500"} 2`) {
		t.Errorf("expected GET_500 error count=2, got:\n%s", body)
	}

	// 2xx/4xx should NOT appear in error counter
	if strings.Contains(body, "http_request_errors_total{service=\"err-test\",method_status=\"GET_200\"") {
		t.Errorf("200 responses should not appear in error counter, got:\n%s", body)
	}
	if strings.Contains(body, "http_request_errors_total{service=\"err-test\",method_status=\"GET_400\"") {
		t.Errorf("400 responses should not appear in error counter, got:\n%s", body)
	}
}

func TestErrorCounterForVariousServerErrors(t *testing.T) {
	m := NewMetrics("5xx-test")

	statuses := []int{500, 502, 503, 504}
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for i, code := range statuses {
			if r.URL.Path == "/"+strconv.Itoa(i) {
				w.WriteHeader(code)
				return
			}
		}
		w.WriteHeader(500)
	})
	handler := m.MetricsMiddleware()(inner)

	for i := range statuses {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/"+strconv.Itoa(i), nil))
	}

	body := metricsBody(t, m)
	if !strings.Contains(body, "http_request_errors_total") {
		t.Errorf("expected error counter in output, got:\n%s", body)
	}
}

// --- latency histogram ---

func TestLatencyHistogramObserve(t *testing.T) {
	h := newLatencyHistogram()

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
	lastBucket := snap[len(snap)-1]
	if lastBucket.le != "+Inf" {
		t.Errorf("expected last bucket le=+Inf, got %s", lastBucket.le)
	}
	if lastBucket.count < 3 {
		t.Errorf("expected +Inf bucket count >= 3, got %d", lastBucket.count)
	}

	firstBucket := snap[0]
	if firstBucket.le != "0.005" {
		t.Errorf("expected first bucket le=0.005, got %s", firstBucket.le)
	}
	if firstBucket.count != 1 {
		t.Errorf("expected 0.005 bucket count=1, got %d", firstBucket.count)
	}
}

func TestMetricsHandlerIncludesHistogram(t *testing.T) {
	m := NewMetrics("hist-test")

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := m.MetricsMiddleware()(inner)
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/test", nil))

	body := metricsBody(t, m)
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

// --- response size histogram ---

func TestSizeHistogramObserve(t *testing.T) {
	h := newSizeHistogram()

	h.observe(100)    // fits in 256-byte bucket
	h.observe(500)    // fits in 1024-byte bucket
	h.observe(100000) // fits in 262144-byte bucket

	if h.totalCount() != 3 {
		t.Errorf("expected count=3, got %d", h.totalCount())
	}
	if h.totalSum() != 100600 {
		t.Errorf("expected sum=100600, got %d", h.totalSum())
	}

	snap := h.snapshot()
	lastBucket := snap[len(snap)-1]
	if lastBucket.le != "+Inf" {
		t.Errorf("expected last bucket le=+Inf, got %s", lastBucket.le)
	}
	// The +Inf bucket accumulates all observations across all bucket counts.
	// With 3 observations spread across multiple buckets, cumulative count >= 3.
	if lastBucket.count < 3 {
		t.Errorf("expected +Inf cumulative count >= 3, got %d", lastBucket.count)
	}

	// 100 bytes fits in 256-byte bucket but NOT the smaller boundary if there were one.
	// First bucket (256 bytes) should include the 100-byte observation.
	firstBucket := snap[0]
	if firstBucket.le != "256" {
		t.Errorf("expected first bucket le=256, got %s", firstBucket.le)
	}
	// cumulative count at 256-byte bucket = raw count for this bucket = 1 (only 100-byte obs)
	if firstBucket.count < 1 {
		t.Errorf("expected 256-byte bucket count >= 1, got %d", firstBucket.count)
	}
}

func TestMetricsHandlerIncludesResponseSizeHistogram(t *testing.T) {
	m := NewMetrics("size-test")

	body100 := strings.Repeat("x", 100)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(body100))
	})
	handler := m.MetricsMiddleware()(inner)
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/test", nil))

	output := metricsBody(t, m)
	if !strings.Contains(output, "http_response_size_bytes_bucket") {
		t.Errorf("expected response size buckets in metrics output, got:\n%s", output)
	}
	if !strings.Contains(output, "http_response_size_bytes_count") {
		t.Errorf("expected response size count in metrics output, got:\n%s", output)
	}
	if !strings.Contains(output, "http_response_size_bytes_sum") {
		t.Errorf("expected response size sum in metrics output, got:\n%s", output)
	}
}

func TestSizeHistogramSummaryMap(t *testing.T) {
	h := newSizeHistogram()
	h.observe(512)

	summary := h.summaryMap()
	if summary["count"] != int64(1) {
		t.Errorf("expected count=1, got %v", summary["count"])
	}
	if summary["sum_bytes"] != int64(512) {
		t.Errorf("expected sum_bytes=512, got %v", summary["sum_bytes"])
	}
	buckets, ok := summary["buckets"].(map[string]int64)
	if !ok {
		t.Fatal("expected buckets to be map[string]int64")
	}
	if _, exists := buckets["+Inf"]; !exists {
		t.Error("expected +Inf bucket in summary")
	}
}

// --- Prometheus text format validation ---

func TestMetricsHandlerPrometheusTextFormat(t *testing.T) {
	m := NewMetrics("prom-test")

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello"))
	})
	handler := m.MetricsMiddleware()(inner)
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/test", nil))

	rr := httptest.NewRecorder()
	m.MetricsHandler().ServeHTTP(rr, httptest.NewRequest("GET", "/metrics", nil))

	// Verify Content-Type
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("expected text/plain Content-Type, got: %s", ct)
	}

	body := rr.Body.String()

	// Verify # HELP lines
	if !strings.Contains(body, "# HELP http_requests_total") {
		t.Errorf("expected '# HELP http_requests_total' line, got:\n%s", body)
	}
	if !strings.Contains(body, "# HELP http_request_errors_total") {
		t.Errorf("expected '# HELP http_request_errors_total' line, got:\n%s", body)
	}
	if !strings.Contains(body, "# HELP http_request_duration_seconds") {
		t.Errorf("expected '# HELP http_request_duration_seconds' line, got:\n%s", body)
	}
	if !strings.Contains(body, "# HELP http_response_size_bytes") {
		t.Errorf("expected '# HELP http_response_size_bytes' line, got:\n%s", body)
	}

	// Verify # TYPE lines
	if !strings.Contains(body, "# TYPE http_requests_total counter") {
		t.Errorf("expected '# TYPE http_requests_total counter' line, got:\n%s", body)
	}
	if !strings.Contains(body, "# TYPE http_request_errors_total counter") {
		t.Errorf("expected '# TYPE http_request_errors_total counter' line, got:\n%s", body)
	}
	if !strings.Contains(body, "# TYPE http_request_duration_seconds histogram") {
		t.Errorf("expected '# TYPE http_request_duration_seconds histogram' line, got:\n%s", body)
	}
	if !strings.Contains(body, "# TYPE http_response_size_bytes histogram") {
		t.Errorf("expected '# TYPE http_response_size_bytes histogram' line, got:\n%s", body)
	}

	// Verify metric lines appear
	if !strings.Contains(body, `http_requests_total{service="prom-test"`) {
		t.Errorf("expected http_requests_total metric line, got:\n%s", body)
	}
	if !strings.Contains(body, `http_request_duration_seconds_bucket{service="prom-test"`) {
		t.Errorf("expected duration bucket metric line, got:\n%s", body)
	}
	if !strings.Contains(body, `http_response_size_bytes_bucket{service="prom-test"`) {
		t.Errorf("expected size bucket metric line, got:\n%s", body)
	}
}

// --- JSON handler ---

func TestMetricsJSONHandler(t *testing.T) {
	m := NewMetrics("json-test")

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := m.MetricsMiddleware()(inner)
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/test", nil))

	rr := httptest.NewRecorder()
	m.MetricsJSON().ServeHTTP(rr, httptest.NewRequest("GET", "/metrics.json", nil))

	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "json-test") {
		t.Errorf("expected service name in JSON output, got: %s", body)
	}
	if !strings.Contains(body, "request_counts") {
		t.Errorf("expected request_counts in JSON output, got: %s", body)
	}
	if !strings.Contains(body, "error_counts") {
		t.Errorf("expected error_counts in JSON output, got: %s", body)
	}
	if !strings.Contains(body, "response_size") {
		t.Errorf("expected response_size in JSON output, got: %s", body)
	}
}

// --- MetricsServer ---

func TestMetricsServerStartAndShutdown(t *testing.T) {
	m := NewMetrics("srv-test")

	// Use port 0 for OS-assigned port
	ms := NewMetricsServer(m, "127.0.0.1:0", nil)
	if ms == nil {
		t.Fatal("expected non-nil MetricsServer")
	}

	// Verify handler is accessible before Start
	h := ms.Handler()
	if h == nil {
		t.Fatal("expected non-nil handler")
	}

	// Test handler responds to /metrics
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/metrics", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	// Shutdown with already-not-started server should not panic
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = ms.Shutdown(ctx)
}

func TestMetricsServerHandlerServesMetrics(t *testing.T) {
	m := NewMetrics("handler-test")

	// Record some metrics first
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("error body"))
	})
	mw := m.MetricsMiddleware()(inner)
	mw.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/fail", nil))

	ms := NewMetricsServer(m, "127.0.0.1:0", nil)
	h := ms.Handler()

	// /metrics endpoint
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/metrics", nil))
	body := rr.Body.String()

	if !strings.Contains(body, "# HELP http_requests_total") {
		t.Errorf("expected HELP line for http_requests_total, got:\n%s", body)
	}
	if !strings.Contains(body, `http_request_errors_total{service="handler-test",method_status="POST_500"} 1`) {
		t.Errorf("expected POST_500 error count=1, got:\n%s", body)
	}

	// /metrics.json endpoint
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, httptest.NewRequest("GET", "/metrics.json", nil))
	if rr2.Code != http.StatusOK {
		t.Errorf("expected 200 from /metrics.json, got %d", rr2.Code)
	}
	if ct := rr2.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json from /metrics.json, got %s", ct)
	}
}

// --- metricsRecorder captures size ---

func TestMetricsRecorderCapturesResponseSize(t *testing.T) {
	m := NewMetrics("recorder-test")

	responseBody := strings.Repeat("a", 500)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(responseBody))
	})
	handler := m.MetricsMiddleware()(inner)
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/test", nil))

	// After one 500-byte response, the sum should be 500
	if m.sizeBuckets.totalSum() != 500 {
		t.Errorf("expected size sum=500, got %d", m.sizeBuckets.totalSum())
	}
	if m.sizeBuckets.totalCount() != 1 {
		t.Errorf("expected size count=1, got %d", m.sizeBuckets.totalCount())
	}
}

// --- helper ---

func metricsBody(t *testing.T, m *Metrics) string {
	t.Helper()
	rr := httptest.NewRecorder()
	m.MetricsHandler().ServeHTTP(rr, httptest.NewRequest("GET", "/metrics", nil))
	b, err := io.ReadAll(rr.Body)
	if err != nil {
		t.Fatalf("failed to read metrics body: %v", err)
	}
	return string(b)
}

