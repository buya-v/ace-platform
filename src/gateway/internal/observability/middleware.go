package observability

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
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
//   - Parses the incoming W3C "traceparent" header (version-trace_id-parent_id-flags).
//   - Falls back to X-Request-ID when no valid traceparent is present.
//   - Generates a new trace_id (16 bytes hex) and span_id (8 bytes hex) when neither exists.
//   - Creates a child span for each hop (new span_id; parent set to incoming parent_id).
//   - Stores trace_id in the request context via WithTraceID for LoggerFromContext.
//   - Injects the outgoing "traceparent" and legacy X-Request-ID / X-Trace-ID headers.
//   - Logs request start and completion with duration, status, path, and trace_id.
//
// Position in chain: after RequestID middleware, before auth middleware.
func TracingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			var traceID, spanID, parentSpanID string
			sampled := true

			// 1. Try to parse W3C traceparent header.
			if incoming := r.Header.Get("traceparent"); incoming != "" {
				if tid, pid, _, s, err := parseTraceparent(incoming); err == nil {
					traceID = tid
					parentSpanID = pid // incoming parent-id becomes our ParentSpanID
					spanID = generateSpanID()
					sampled = s
				}
			}

			// 2. Fall back to legacy X-Request-ID header.
			if traceID == "" {
				if xid := r.Header.Get("X-Request-ID"); xid != "" {
					traceID = xid
					spanID = generateSpanID()
				}
			}

			// 3. Generate a fresh root trace when neither header is present.
			if traceID == "" {
				traceID = generateTraceID()
				spanID = generateSpanID()
			}

			// Inject W3C traceparent response header.
			flags := "00"
			if sampled {
				flags = "01"
			}
			outgoing := fmt.Sprintf("00-%s-%s-%s", traceID, spanID, flags)
			w.Header().Set("traceparent", outgoing)

			// Inject legacy headers for backward compatibility.
			w.Header().Set("X-Request-ID", traceID)
			w.Header().Set("X-Trace-ID", traceID)

			// Store trace_id in context.
			ctx := WithTraceID(r.Context(), traceID)
			ctx = WithRequestID(ctx, traceID)

			// Wrap writer to capture status.
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			// Log request start with trace context.
			reqLogger := LoggerFromContext(ctx, logger)
			reqLogger.Info("request_started",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("remote_addr", r.RemoteAddr),
				slog.String("user_agent", r.UserAgent()),
				slog.String("trace_id", traceID),
				slog.String("span_id", spanID),
				slog.String("parent_span_id", parentSpanID),
			)

			// Serve the request.
			next.ServeHTTP(rec, r.WithContext(ctx))

			// Log request completion.
			duration := time.Since(start)
			level := slog.LevelInfo
			if rec.status >= 500 {
				level = slog.LevelError
			} else if rec.status >= 400 {
				level = slog.LevelWarn
			}

			reqLogger.Log(r.Context(), level, "request_completed",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Int("response_bytes", rec.size),
				slog.String("duration", duration.String()),
				slog.Int64("duration_ms", duration.Milliseconds()),
				slog.String("remote_addr", r.RemoteAddr),
				slog.String("trace_id", traceID),
				slog.String("span_id", spanID),
			)
		})
	}
}

// parseTraceparent parses a W3C traceparent header value.
// Returns traceID, parentID, version, sampled, and any parse error.
// Validates lowercase-hex encoding with no external dependencies.
func parseTraceparent(header string) (traceID, parentID, version string, sampled bool, err error) {
	parts := strings.Split(header, "-")
	if len(parts) != 4 {
		return "", "", "", false, fmt.Errorf("traceparent: expected 4 fields, got %d", len(parts))
	}
	ver, tid, pid, flags := parts[0], parts[1], parts[2], parts[3]

	if len(ver) != 2 || !isTraceparentHex(ver) || ver == "ff" {
		return "", "", "", false, fmt.Errorf("traceparent: invalid version %q", ver)
	}
	if len(tid) != 32 || !isTraceparentHex(tid) || isAllZeroString(tid) {
		return "", "", "", false, fmt.Errorf("traceparent: invalid trace-id")
	}
	if len(pid) != 16 || !isTraceparentHex(pid) || isAllZeroString(pid) {
		return "", "", "", false, fmt.Errorf("traceparent: invalid parent-id")
	}
	if len(flags) != 2 || !isTraceparentHex(flags) {
		return "", "", "", false, fmt.Errorf("traceparent: invalid trace-flags")
	}
	return tid, pid, ver, flags == "01", nil
}

// generateTraceID creates a random 16-byte (32 hex char) trace ID.
func generateTraceID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// generateSpanID creates a random 8-byte (16 hex char) span ID.
func generateSpanID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// isTraceparentHex returns true if s contains only lowercase hex characters.
func isTraceparentHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// isAllZeroString returns true when every character is '0'.
func isAllZeroString(s string) bool {
	for _, c := range s {
		if c != '0' {
			return false
		}
	}
	return true
}
