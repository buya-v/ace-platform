// Package observability provides structured logging, HTTP tracing middleware,
// and basic metrics for GarudaX services. It uses only Go stdlib packages
// (log/slog, expvar, net/http) to maintain the zero-dependency pattern.
package observability

import (
	"context"
	"log/slog"
	"os"
)

// contextKey is an unexported type for context keys in this package.
type contextKey string

const (
	// TraceIDKey is the context key for the distributed trace ID.
	TraceIDKey contextKey = "trace_id"
	// RequestIDKey is the context key for the request ID.
	RequestIDKey contextKey = "request_id"
)

// NewLogger creates a structured JSON logger for a service. The logger
// automatically includes the service_name field in every log entry.
// Output goes to stdout for container log aggregation.
func NewLogger(serviceName string) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	return slog.New(handler).With(
		slog.String("service_name", serviceName),
	)
}

// NewLoggerWithLevel creates a structured JSON logger with a configurable level.
func NewLoggerWithLevel(serviceName string, level slog.Level) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})
	return slog.New(handler).With(
		slog.String("service_name", serviceName),
	)
}

// WithTraceID adds a trace_id to the context.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, TraceIDKey, traceID)
}

// TraceIDFromContext extracts the trace_id from the context.
func TraceIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(TraceIDKey).(string); ok {
		return id
	}
	return ""
}

// WithRequestID adds a request_id to the context.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// RequestIDFromContext extracts the request_id from the context.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(RequestIDKey).(string); ok {
		return id
	}
	return ""
}

// LoggerFromContext returns a logger enriched with trace_id and request_id
// from the context. This is the primary way to get a context-aware logger.
func LoggerFromContext(ctx context.Context, base *slog.Logger) *slog.Logger {
	l := base
	if traceID := TraceIDFromContext(ctx); traceID != "" {
		l = l.With(slog.String("trace_id", traceID))
	}
	if reqID := RequestIDFromContext(ctx); reqID != "" {
		l = l.With(slog.String("request_id", reqID))
	}
	return l
}
