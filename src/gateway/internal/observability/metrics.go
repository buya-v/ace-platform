package observability

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Metrics provides Prometheus-compatible basic metrics.
// It tracks request counts and latency histograms per method+status.
// Uses sync/atomic and sync.Mutex instead of expvar to allow multiple
// instances in tests without global registry conflicts.
type Metrics struct {
	serviceName    string
	requestCount   *counterMap
	latencyBuckets *latencyHistogram
}

// NewMetrics creates a new Metrics instance for a service.
func NewMetrics(serviceName string) *Metrics {
	return &Metrics{
		serviceName:    serviceName,
		requestCount:   newCounterMap(),
		latencyBuckets: newLatencyHistogram(),
	}
}

// MetricsMiddleware creates HTTP middleware that records request count
// and latency for each request.
func (m *Metrics) MetricsMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			rec := &metricsRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)

			duration := time.Since(start)

			// Record request count by method+status
			key := r.Method + "_" + strconv.Itoa(rec.status)
			m.requestCount.Add(key, 1)

			// Record latency
			m.latencyBuckets.observe(duration)
		})
	}
}

// MetricsHandler returns an HTTP handler that serves metrics in a
// Prometheus-compatible text format at /metrics.
func (m *Metrics) MetricsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")

		// Write request counts
		m.requestCount.Do(func(key string, value int64) {
			_, _ = w.Write([]byte(
				"http_requests_total{service=\"" + m.serviceName +
					"\",method_status=\"" + key +
					"\"} " + strconv.FormatInt(value, 10) + "\n",
			))
		})

		// Write latency histogram
		buckets := m.latencyBuckets.snapshot()
		for _, b := range buckets {
			_, _ = w.Write([]byte(
				"http_request_duration_seconds_bucket{service=\"" + m.serviceName +
					"\",le=\"" + b.le +
					"\"} " + strconv.FormatInt(b.count, 10) + "\n",
			))
		}
		_, _ = w.Write([]byte(
			"http_request_duration_seconds_count{service=\"" + m.serviceName +
				"\"} " + strconv.FormatInt(m.latencyBuckets.totalCount(), 10) + "\n",
		))
		_, _ = w.Write([]byte(
			"http_request_duration_seconds_sum{service=\"" + m.serviceName +
				"\"} " + strconv.FormatFloat(m.latencyBuckets.totalSum(), 'f', 6, 64) + "\n",
		))
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

		result := map[string]interface{}{
			"service":        m.serviceName,
			"request_counts": counts,
			"latency":        m.latencyBuckets.summaryMap(),
		}

		json.NewEncoder(w).Encode(result)
	})
}

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

// metricsRecorder captures the HTTP status code for metrics.
type metricsRecorder struct {
	http.ResponseWriter
	status int
}

func (mr *metricsRecorder) WriteHeader(status int) {
	mr.status = status
	mr.ResponseWriter.WriteHeader(status)
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
