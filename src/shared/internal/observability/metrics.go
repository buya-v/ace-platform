package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/garudax-platform/shared/internal/tenant"
)

// Metrics provides Prometheus-compatible basic metrics.
// It tracks request counts, error counts, latency histograms, and response
// size histograms per service. Uses sync/atomic and sync.Mutex instead of
// expvar to allow multiple instances in tests without global registry conflicts.
type Metrics struct {
	serviceName    string
	requestCount   *counterMap
	errorCount     *counterMap
	latencyBuckets *latencyHistogram
	sizeBuckets    *sizeHistogram
}

// NewMetrics creates a new Metrics instance for a service.
func NewMetrics(serviceName string) *Metrics {
	return &Metrics{
		serviceName:    serviceName,
		requestCount:   newCounterMap(),
		errorCount:     newCounterMap(),
		latencyBuckets: newLatencyHistogram(),
		sizeBuckets:    newSizeHistogram(),
	}
}

// MetricsMiddleware creates HTTP middleware that records:
//   - http_requests_total: request count by method+status (+ tenant_id when present)
//   - http_request_errors_total: count of 5xx responses
//   - http_request_duration_seconds: latency histogram
//   - http_response_size_bytes: response size histogram
//
// When a tenant context is present in the request context, the metric key
// is prefixed with the tenant_id so metrics are tenant-scoped by default.
func (m *Metrics) MetricsMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			rec := &metricsRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)

			duration := time.Since(start)

			// Build metric key prefix: include tenant_id when available so metrics
			// are tenant-scoped by default (platform invariant: every metric carries tenant_id).
			prefix := ""
			if tid, ok := tenant.TenantFromContext(r.Context()); ok {
				prefix = tid.String() + "_"
			}

			// Record request count by tenant+method+status
			key := prefix + r.Method + "_" + strconv.Itoa(rec.status)
			m.requestCount.Add(key, 1)

			// Record 5xx errors
			if rec.status >= 500 {
				errKey := prefix + r.Method + "_" + strconv.Itoa(rec.status)
				m.errorCount.Add(errKey, 1)
			}

			// Record latency
			m.latencyBuckets.observe(duration)

			// Record response size
			m.sizeBuckets.observe(rec.size)
		})
	}
}

// MetricsHandler returns an HTTP handler that serves metrics in proper
// Prometheus text exposition format (# HELP, # TYPE, metric lines).
func (m *Metrics) MetricsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		svc := m.serviceName

		// --- http_requests_total ---
		fmt.Fprintf(w, "# HELP http_requests_total Total number of HTTP requests.\n")
		fmt.Fprintf(w, "# TYPE http_requests_total counter\n")
		m.requestCount.Do(func(key string, value int64) {
			fmt.Fprintf(w, "http_requests_total{service=%q,method_status=%q} %d\n",
				svc, key, value)
		})

		// --- http_request_errors_total ---
		fmt.Fprintf(w, "# HELP http_request_errors_total Total number of HTTP 5xx error responses.\n")
		fmt.Fprintf(w, "# TYPE http_request_errors_total counter\n")
		m.errorCount.Do(func(key string, value int64) {
			fmt.Fprintf(w, "http_request_errors_total{service=%q,method_status=%q} %d\n",
				svc, key, value)
		})

		// --- http_request_duration_seconds ---
		fmt.Fprintf(w, "# HELP http_request_duration_seconds HTTP request latency in seconds.\n")
		fmt.Fprintf(w, "# TYPE http_request_duration_seconds histogram\n")
		latBuckets := m.latencyBuckets.snapshot()
		for _, b := range latBuckets {
			fmt.Fprintf(w, "http_request_duration_seconds_bucket{service=%q,le=%q} %d\n",
				svc, b.le, b.count)
		}
		fmt.Fprintf(w, "http_request_duration_seconds_count{service=%q} %d\n",
			svc, m.latencyBuckets.totalCount())
		fmt.Fprintf(w, "http_request_duration_seconds_sum{service=%q} %s\n",
			svc, strconv.FormatFloat(m.latencyBuckets.totalSum(), 'f', 6, 64))

		// --- http_response_size_bytes ---
		fmt.Fprintf(w, "# HELP http_response_size_bytes HTTP response size in bytes.\n")
		fmt.Fprintf(w, "# TYPE http_response_size_bytes histogram\n")
		szBuckets := m.sizeBuckets.snapshot()
		for _, b := range szBuckets {
			fmt.Fprintf(w, "http_response_size_bytes_bucket{service=%q,le=%q} %d\n",
				svc, b.le, b.count)
		}
		fmt.Fprintf(w, "http_response_size_bytes_count{service=%q} %d\n",
			svc, m.sizeBuckets.totalCount())
		fmt.Fprintf(w, "http_response_size_bytes_sum{service=%q} %d\n",
			svc, m.sizeBuckets.totalSum())
	})
}

// MetricsJSON returns an HTTP handler that serves metrics as JSON
// (useful for health dashboards).
func (m *Metrics) MetricsJSON() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		counts := make(map[string]int64)
		m.requestCount.Do(func(key string, value int64) {
			counts[key] = value
		})

		errors := make(map[string]int64)
		m.errorCount.Do(func(key string, value int64) {
			errors[key] = value
		})

		result := map[string]interface{}{
			"service":         m.serviceName,
			"request_counts":  counts,
			"error_counts":    errors,
			"latency":         m.latencyBuckets.summaryMap(),
			"response_size":   m.sizeBuckets.summaryMap(),
		}

		json.NewEncoder(w).Encode(result)
	})
}

// MetricsServer wraps a Metrics instance and manages a dedicated HTTP server
// for Prometheus scraping. It exposes /metrics on a configurable port so the
// main application server is not polluted with metrics traffic.
type MetricsServer struct {
	metrics *Metrics
	addr    string
	srv     *http.Server
	logger  *slog.Logger
}

// NewMetricsServer creates a MetricsServer that will listen on addr (e.g. ":9090").
// Pass a nil logger to disable startup/shutdown logging.
func NewMetricsServer(m *Metrics, addr string, logger *slog.Logger) *MetricsServer {
	mux := http.NewServeMux()
	mux.Handle("/metrics", m.MetricsHandler())
	mux.Handle("/metrics.json", m.MetricsJSON())

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return &MetricsServer{
		metrics: m,
		addr:    addr,
		srv:     srv,
		logger:  logger,
	}
}

// Start begins serving metrics in a background goroutine. It returns
// immediately. The server runs until Shutdown is called or the process exits.
func (ms *MetricsServer) Start() {
	go func() {
		if ms.logger != nil {
			ms.logger.Info("metrics server listening", slog.String("addr", ms.addr))
		}
		if err := ms.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			if ms.logger != nil {
				ms.logger.Error("metrics server error", slog.String("error", err.Error()))
			}
		}
	}()
}

// Shutdown gracefully stops the metrics server using the provided context.
func (ms *MetricsServer) Shutdown(ctx context.Context) error {
	return ms.srv.Shutdown(ctx)
}

// Handler returns the underlying http.Handler, useful for registering
// /metrics on an existing mux rather than starting a dedicated server.
func (ms *MetricsServer) Handler() http.Handler {
	return ms.srv.Handler
}

// --- internal types ---

// counterMap is a thread-safe map of string -> int64 counters.
type counterMap struct {
	mu       sync.RWMutex
	counters map[string]*atomic.Int64
}

func newCounterMap() *counterMap {
	return &counterMap{
		counters: make(map[string]*atomic.Int64),
	}
}

func (cm *counterMap) Add(key string, delta int64) {
	cm.mu.RLock()
	counter, ok := cm.counters[key]
	cm.mu.RUnlock()

	if ok {
		counter.Add(delta)
		return
	}

	cm.mu.Lock()
	// Double-check after acquiring write lock
	counter, ok = cm.counters[key]
	if !ok {
		counter = &atomic.Int64{}
		cm.counters[key] = counter
	}
	cm.mu.Unlock()
	counter.Add(delta)
}

// Do iterates over all counters in sorted key order.
func (cm *counterMap) Do(fn func(key string, value int64)) {
	cm.mu.RLock()
	keys := make([]string, 0, len(cm.counters))
	for k := range cm.counters {
		keys = append(keys, k)
	}
	cm.mu.RUnlock()

	sort.Strings(keys)
	for _, k := range keys {
		cm.mu.RLock()
		counter := cm.counters[k]
		cm.mu.RUnlock()
		fn(k, counter.Load())
	}
}

// metricsRecorder captures the HTTP status code and response size for metrics.
type metricsRecorder struct {
	http.ResponseWriter
	status int
	size   int
}

func (mr *metricsRecorder) WriteHeader(status int) {
	mr.status = status
	mr.ResponseWriter.WriteHeader(status)
}

func (mr *metricsRecorder) Write(b []byte) (int, error) {
	n, err := mr.ResponseWriter.Write(b)
	mr.size += n
	return n, err
}

// latencyHistogram tracks request latency in Prometheus-style buckets.
type latencyHistogram struct {
	mu      sync.Mutex
	buckets []histBucket
	sum     float64
	count   int64
}

type histBucket struct {
	le      string
	upperMs float64
	count   int64
}

type bucketSnapshot struct {
	le    string
	count int64
}

func newLatencyHistogram() *latencyHistogram {
	// Standard Prometheus histogram bucket boundaries in milliseconds
	boundaries := []struct {
		le      string
		upperMs float64
	}{
		{"0.005", 5},
		{"0.01", 10},
		{"0.025", 25},
		{"0.05", 50},
		{"0.1", 100},
		{"0.25", 250},
		{"0.5", 500},
		{"1", 1000},
		{"2.5", 2500},
		{"5", 5000},
		{"10", 10000},
		{"+Inf", 1<<63 - 1},
	}

	buckets := make([]histBucket, len(boundaries))
	for i, b := range boundaries {
		buckets[i] = histBucket{le: b.le, upperMs: b.upperMs}
	}
	return &latencyHistogram{buckets: buckets}
}

func (h *latencyHistogram) observe(d time.Duration) {
	ms := float64(d.Milliseconds())
	seconds := d.Seconds()

	h.mu.Lock()
	defer h.mu.Unlock()

	h.sum += seconds
	h.count++

	for i := range h.buckets {
		if ms <= h.buckets[i].upperMs {
			h.buckets[i].count++
		}
	}
}

func (h *latencyHistogram) snapshot() []bucketSnapshot {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Return cumulative counts
	result := make([]bucketSnapshot, len(h.buckets))
	var cumulative int64
	for i, b := range h.buckets {
		cumulative += b.count
		result[i] = bucketSnapshot{le: b.le, count: cumulative}
	}
	return result
}

func (h *latencyHistogram) totalCount() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.count
}

func (h *latencyHistogram) totalSum() float64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sum
}

func (h *latencyHistogram) summaryMap() map[string]interface{} {
	h.mu.Lock()
	defer h.mu.Unlock()

	bucketMap := make(map[string]int64)
	var cumulative int64
	for _, b := range h.buckets {
		cumulative += b.count
		bucketMap[b.le] = cumulative
	}

	return map[string]interface{}{
		"count":   h.count,
		"sum_sec": h.sum,
		"buckets": bucketMap,
	}
}

// sizeHistogram tracks response sizes in Prometheus-style buckets (bytes).
type sizeHistogram struct {
	mu      sync.Mutex
	buckets []sizeBucket
	sum     int64
	count   int64
}

type sizeBucket struct {
	le        string
	upperBytes int64
	count      int64
}

type sizeBucketSnapshot struct {
	le    string
	count int64
}

func newSizeHistogram() *sizeHistogram {
	// Response size bucket boundaries in bytes
	boundaries := []struct {
		le        string
		upperBytes int64
	}{
		{"256", 256},
		{"1024", 1024},
		{"4096", 4096},
		{"16384", 16384},
		{"65536", 65536},
		{"262144", 262144},
		{"1048576", 1048576},
		{"+Inf", 1<<62 - 1},
	}

	buckets := make([]sizeBucket, len(boundaries))
	for i, b := range boundaries {
		buckets[i] = sizeBucket{le: b.le, upperBytes: b.upperBytes}
	}
	return &sizeHistogram{buckets: buckets}
}

func (h *sizeHistogram) observe(size int) {
	sz := int64(size)

	h.mu.Lock()
	defer h.mu.Unlock()

	h.sum += sz
	h.count++

	for i := range h.buckets {
		if sz <= h.buckets[i].upperBytes {
			h.buckets[i].count++
		}
	}
}

func (h *sizeHistogram) snapshot() []sizeBucketSnapshot {
	h.mu.Lock()
	defer h.mu.Unlock()

	result := make([]sizeBucketSnapshot, len(h.buckets))
	var cumulative int64
	for i, b := range h.buckets {
		cumulative += b.count
		result[i] = sizeBucketSnapshot{le: b.le, count: cumulative}
	}
	return result
}

func (h *sizeHistogram) totalCount() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.count
}

func (h *sizeHistogram) totalSum() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sum
}

func (h *sizeHistogram) summaryMap() map[string]interface{} {
	h.mu.Lock()
	defer h.mu.Unlock()

	bucketMap := make(map[string]int64)
	var cumulative int64
	for _, b := range h.buckets {
		cumulative += b.count
		bucketMap[b.le] = cumulative
	}

	return map[string]interface{}{
		"count":      h.count,
		"sum_bytes":  h.sum,
		"buckets":    bucketMap,
	}
}
