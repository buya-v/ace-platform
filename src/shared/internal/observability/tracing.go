// Package observability provides structured logging, metrics, and W3C traceparent
// propagation for GarudaX services. This file implements W3C Trace Context
// (https://www.w3.org/TR/trace-context/) with zero external dependencies.
package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
)

// traceContextKey is an unexported type for TraceContext context keys.
type traceContextKey struct{}

// TraceContext holds W3C traceparent fields for a single request span.
type TraceContext struct {
	// TraceID is a 16-byte (32 hex char) globally unique trace identifier.
	TraceID string
	// SpanID (called parent_id in traceparent) is the 8-byte (16 hex char)
	// identifier of the current span.
	SpanID string
	// ParentSpanID is the span_id received from the incoming traceparent header.
	// Empty when the request originated without a traceparent.
	ParentSpanID string
	// Sampled reflects the trace-flags sampling bit (01 = sampled, 00 = not sampled).
	Sampled bool
}

// traceparentVersion is the only version currently defined by the W3C spec.
const traceparentVersion = "00"

// ParseTraceparent parses a W3C traceparent header value and returns the
// TraceContext it describes.
//
// Format: version-trace_id-parent_id-trace_flags
// Example: 00-4bf92f3577b6a27520e8ecf678db76fc-00f067aa0ba902b7-01
//
// Returns an error if the header is malformed or uses an unsupported version.
func ParseTraceparent(header string) (TraceContext, error) {
	if header == "" {
		return TraceContext{}, fmt.Errorf("traceparent: empty header")
	}

	parts := strings.Split(header, "-")
	if len(parts) != 4 {
		return TraceContext{}, fmt.Errorf("traceparent: expected 4 dash-separated fields, got %d", len(parts))
	}

	version, traceID, parentID, traceFlags := parts[0], parts[1], parts[2], parts[3]

	// Version must be exactly 2 lowercase hex chars.
	if len(version) != 2 {
		return TraceContext{}, fmt.Errorf("traceparent: version must be 2 hex chars, got %q", version)
	}
	if !isHex(version) {
		return TraceContext{}, fmt.Errorf("traceparent: version contains non-hex chars: %q", version)
	}
	// Version 0xff is reserved and invalid.
	if version == "ff" {
		return TraceContext{}, fmt.Errorf("traceparent: version 0xff is reserved and invalid")
	}

	// trace-id must be 32 lowercase hex chars and must not be all zeros.
	if len(traceID) != 32 {
		return TraceContext{}, fmt.Errorf("traceparent: trace-id must be 32 hex chars, got %d", len(traceID))
	}
	if !isHex(traceID) {
		return TraceContext{}, fmt.Errorf("traceparent: trace-id contains non-hex chars")
	}
	if isAllZeros(traceID) {
		return TraceContext{}, fmt.Errorf("traceparent: trace-id must not be all zeros")
	}

	// parent-id must be 16 lowercase hex chars and must not be all zeros.
	if len(parentID) != 16 {
		return TraceContext{}, fmt.Errorf("traceparent: parent-id must be 16 hex chars, got %d", len(parentID))
	}
	if !isHex(parentID) {
		return TraceContext{}, fmt.Errorf("traceparent: parent-id contains non-hex chars")
	}
	if isAllZeros(parentID) {
		return TraceContext{}, fmt.Errorf("traceparent: parent-id must not be all zeros")
	}

	// trace-flags must be exactly 2 hex chars.
	if len(traceFlags) != 2 {
		return TraceContext{}, fmt.Errorf("traceparent: trace-flags must be 2 hex chars, got %d", len(traceFlags))
	}
	if !isHex(traceFlags) {
		return TraceContext{}, fmt.Errorf("traceparent: trace-flags contains non-hex chars")
	}

	// Sampling bit is the least-significant bit of the flags byte.
	sampled := traceFlags == "01"

	// The incoming parent-id is the ID of the remote span (the caller's current
	// span). We store it as SpanID so that NewSpan can set it as ParentSpanID
	// of the newly created child span. This matches how W3C trace context works:
	// the traceparent.parent-id identifies the caller's span, which becomes the
	// parent of our new span.
	return TraceContext{
		TraceID: traceID,
		SpanID:  parentID,
		Sampled: sampled,
	}, nil
}

// FormatTraceparent formats a TraceContext into a W3C traceparent header value.
// The SpanID is used as the parent-id field (this span becomes the parent for
// downstream services).
func FormatTraceparent(tc TraceContext) string {
	flags := "00"
	if tc.Sampled {
		flags = "01"
	}
	return fmt.Sprintf("%s-%s-%s-%s", traceparentVersion, tc.TraceID, tc.SpanID, flags)
}

// NewTraceContext generates a fresh TraceContext with a new trace_id and span_id.
// This is used when no incoming traceparent is present.
func NewTraceContext() TraceContext {
	return TraceContext{
		TraceID: generateTraceIDBytes(),
		SpanID:  generateSpanID(),
		Sampled: true,
	}
}

// NewSpan creates a child span for the given TraceContext. The incoming SpanID
// becomes the ParentSpanID and a new SpanID is generated.
func NewSpan(tc TraceContext) TraceContext {
	return TraceContext{
		TraceID:      tc.TraceID,
		SpanID:       generateSpanID(),
		ParentSpanID: tc.SpanID,
		Sampled:      tc.Sampled,
	}
}

// TraceFromContext extracts the TraceContext stored by TraceMiddleware.
// Returns an empty TraceContext if none is present.
func TraceFromContext(ctx context.Context) TraceContext {
	if tc, ok := ctx.Value(traceContextKey{}).(TraceContext); ok {
		return tc
	}
	return TraceContext{}
}

// withTraceContext stores a TraceContext in the context.
func withTraceContext(ctx context.Context, tc TraceContext) context.Context {
	return context.WithValue(ctx, traceContextKey{}, tc)
}

// InjectTraceparent sets the traceparent header on an outgoing request,
// enabling downstream services to continue the trace.
func InjectTraceparent(req *http.Request, tc TraceContext) {
	if tc.TraceID == "" || tc.SpanID == "" {
		return
	}
	req.Header.Set("traceparent", FormatTraceparent(tc))
}

// TraceMiddleware is an HTTP middleware that implements W3C traceparent
// propagation. It:
//  1. Parses the incoming "traceparent" header (if present).
//  2. Generates a new trace_id/span_id when no valid header is found.
//  3. Creates a new child span (new SpanID) for this hop.
//  4. Stores the TraceContext in the request context.
//  5. Also stores the trace_id via WithTraceID for LoggerFromContext compatibility.
//  6. Injects the outgoing traceparent into the response headers.
//
// Place this middleware after RequestID and before auth in the chain.
func TraceMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var span TraceContext

			incoming, err := ParseTraceparent(r.Header.Get("traceparent"))
			if err == nil {
				// Valid incoming traceparent — create a child span.
				span = NewSpan(incoming)
			} else {
				// No valid traceparent — start a new trace.
				span = NewTraceContext()
			}

			// Store the full TraceContext.
			ctx := withTraceContext(r.Context(), span)
			// Also store trace_id for LoggerFromContext compatibility.
			ctx = WithTraceID(ctx, span.TraceID)

			// Inject outgoing traceparent into response so clients can correlate.
			w.Header().Set("traceparent", FormatTraceparent(span))

			if logger != nil {
				logger.Debug("trace",
					slog.String("trace_id", span.TraceID),
					slog.String("span_id", span.SpanID),
					slog.String("parent_span_id", span.ParentSpanID),
					slog.Bool("sampled", span.Sampled),
				)
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// generateTraceIDBytes creates a random 16-byte (32 hex char) trace ID.
func generateTraceIDBytes() string {
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

// isHex returns true if s contains only lowercase hexadecimal characters.
func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// isAllZeros returns true if every character in s is '0'.
func isAllZeros(s string) bool {
	for _, c := range s {
		if c != '0' {
			return false
		}
	}
	return true
}
