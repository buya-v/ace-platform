package observability

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/garudax-platform/shared/internal/tenant"
)

// statusRecorder wraps http.ResponseWriter to capture the status code
// and response size for logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
	size   int
}

func (sr *statusRecorder) WriteHeader(status int) {
	sr.status = status
	sr.ResponseWriter.WriteHeader(status)
}

func (sr *statusRecorder) Write(b []byte) (int, error) {
	n, err := sr.ResponseWriter.Write(b)
	sr.size += n
	return n, err
}

// Hijack implements http.Hijacker so WebSocket upgrades work through
// the tracing middleware.
func (sr *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := sr.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not support hijacking")
}

// TracingMiddleware creates HTTP middleware that:
// - Extracts or generates a trace_id from the X-Request-ID header
// - Stores trace_id and request_id in the request context
// - Logs request start and completion with duration, status, and path
// - Sets X-Request-ID and X-Trace-ID response headers
func TracingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Extract or generate trace ID from X-Request-ID header
			traceID := r.Header.Get("X-Request-ID")
			if traceID == "" {
				traceID = generateTraceID()
			}

			// Store in context
			ctx := WithTraceID(r.Context(), traceID)
			ctx = WithRequestID(ctx, traceID)

			// Set response headers
			w.Header().Set("X-Request-ID", traceID)
			w.Header().Set("X-Trace-ID", traceID)

			// Extract tenant context if present and propagate in header
			tenantID := ""
			if tid, ok := tenant.TenantFromContext(r.Context()); ok {
				tenantID = tid.String()
				w.Header().Set("X-GarudaX-Tenant", tenantID)
			}

			// Wrap writer to capture status
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			// Log request start — include tenant_id when available
			reqLogger := LoggerFromContext(ctx, logger)
			startAttrs := []any{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("remote_addr", r.RemoteAddr),
				slog.String("user_agent", r.UserAgent()),
			}
			if tenantID != "" {
				startAttrs = append(startAttrs, slog.String("tenant_id", tenantID))
			}
			reqLogger.Info("request_started", startAttrs...)

			// Serve the request
			next.ServeHTTP(rec, r.WithContext(ctx))

			// Log request completion
			duration := time.Since(start)
			level := slog.LevelInfo
			if rec.status >= 500 {
				level = slog.LevelError
			} else if rec.status >= 400 {
				level = slog.LevelWarn
			}

			completedAttrs := []any{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Int("response_bytes", rec.size),
				slog.String("duration", duration.String()),
				slog.Int64("duration_ms", duration.Milliseconds()),
				slog.String("remote_addr", r.RemoteAddr),
			}
			if tenantID != "" {
				completedAttrs = append(completedAttrs, slog.String("tenant_id", tenantID))
			}
			reqLogger.Log(r.Context(), level, "request_completed", completedAttrs...)
		})
	}
}

// generateTraceID creates a random 16-byte hex trace ID.
func generateTraceID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
