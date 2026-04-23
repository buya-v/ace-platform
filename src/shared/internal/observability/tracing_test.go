package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// ParseTraceparent — valid inputs
// ---------------------------------------------------------------------------

func TestParseTraceparent_Valid(t *testing.T) {
	header := "00-4bf92f3577b6a27520e8ecf678db76fc-00f067aa0ba902b7-01"
	tc, err := ParseTraceparent(header)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tc.TraceID != "4bf92f3577b6a27520e8ecf678db76fc" {
		t.Errorf("TraceID = %q, want %q", tc.TraceID, "4bf92f3577b6a27520e8ecf678db76fc")
	}
	// The incoming parent-id (remote span's ID) is stored as SpanID so that
	// NewSpan can set it as ParentSpanID of the new child span.
	if tc.SpanID != "00f067aa0ba902b7" {
		t.Errorf("SpanID (incoming parent-id) = %q, want %q", tc.SpanID, "00f067aa0ba902b7")
	}
	if !tc.Sampled {
		t.Error("Sampled should be true for flags 01")
	}
}

func TestParseTraceparent_NotSampled(t *testing.T) {
	header := "00-4bf92f3577b6a27520e8ecf678db76fc-00f067aa0ba902b7-00"
	tc, err := ParseTraceparent(header)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tc.Sampled {
		t.Error("Sampled should be false for flags 00")
	}
}

// ---------------------------------------------------------------------------
// ParseTraceparent — invalid inputs
// ---------------------------------------------------------------------------

func TestParseTraceparent_Empty(t *testing.T) {
	_, err := ParseTraceparent("")
	if err == nil {
		t.Error("expected error for empty header")
	}
}

func TestParseTraceparent_WrongFieldCount(t *testing.T) {
	cases := []string{
		"00-4bf92f3577b6a27520e8ecf678db76fc-00f067aa0ba902b7",         // 3 parts
		"00-4bf92f3577b6a27520e8ecf678db76fc-00f067aa0ba902b7-01-extra", // 5 parts
	}
	for _, h := range cases {
		_, err := ParseTraceparent(h)
		if err == nil {
			t.Errorf("expected error for header %q", h)
		}
	}
}

func TestParseTraceparent_ReservedVersion(t *testing.T) {
	header := "ff-4bf92f3577b6a27520e8ecf678db76fc-00f067aa0ba902b7-01"
	_, err := ParseTraceparent(header)
	if err == nil {
		t.Error("expected error for reserved version ff")
	}
}

func TestParseTraceparent_InvalidHex(t *testing.T) {
	// Non-hex character in trace-id
	header := "00-4bf92f3577b6a275ZZZZE8ecf678db76fc-00f067aa0ba902b7-01"
	_, err := ParseTraceparent(header)
	if err == nil {
		t.Error("expected error for non-hex trace-id")
	}
}

func TestParseTraceparent_AllZeroTraceID(t *testing.T) {
	header := "00-00000000000000000000000000000000-00f067aa0ba902b7-01"
	_, err := ParseTraceparent(header)
	if err == nil {
		t.Error("expected error for all-zero trace-id")
	}
}

func TestParseTraceparent_AllZeroParentID(t *testing.T) {
	header := "00-4bf92f3577b6a27520e8ecf678db76fc-0000000000000000-01"
	_, err := ParseTraceparent(header)
	if err == nil {
		t.Error("expected error for all-zero parent-id")
	}
}

func TestParseTraceparent_WrongTraceIDLength(t *testing.T) {
	header := "00-4bf92f-00f067aa0ba902b7-01"
	_, err := ParseTraceparent(header)
	if err == nil {
		t.Error("expected error for short trace-id")
	}
}

func TestParseTraceparent_WrongParentIDLength(t *testing.T) {
	header := "00-4bf92f3577b6a27520e8ecf678db76fc-00f067-01"
	_, err := ParseTraceparent(header)
	if err == nil {
		t.Error("expected error for short parent-id")
	}
}

// ---------------------------------------------------------------------------
// ID generation
// ---------------------------------------------------------------------------

func TestGenerateTraceIDBytes_Length(t *testing.T) {
	id := generateTraceIDBytes()
	if len(id) != 32 {
		t.Errorf("trace_id length = %d, want 32", len(id))
	}
}

func TestGenerateTraceIDBytes_IsHex(t *testing.T) {
	id := generateTraceIDBytes()
	if !isHex(id) {
		t.Errorf("trace_id %q contains non-hex characters", id)
	}
}

func TestGenerateSpanID_Length(t *testing.T) {
	id := generateSpanID()
	if len(id) != 16 {
		t.Errorf("span_id length = %d, want 16", len(id))
	}
}

func TestGenerateSpanID_IsHex(t *testing.T) {
	id := generateSpanID()
	if !isHex(id) {
		t.Errorf("span_id %q contains non-hex characters", id)
	}
}

func TestGenerateTraceIDBytes_Unique(t *testing.T) {
	ids := make(map[string]struct{}, 100)
	for i := 0; i < 100; i++ {
		id := generateTraceIDBytes()
		if _, dup := ids[id]; dup {
			t.Fatalf("duplicate trace_id generated: %q", id)
		}
		ids[id] = struct{}{}
	}
}

func TestGenerateSpanID_Unique(t *testing.T) {
	ids := make(map[string]struct{}, 100)
	for i := 0; i < 100; i++ {
		id := generateSpanID()
		if _, dup := ids[id]; dup {
			t.Fatalf("duplicate span_id generated: %q", id)
		}
		ids[id] = struct{}{}
	}
}

// ---------------------------------------------------------------------------
// NewTraceContext / NewSpan
// ---------------------------------------------------------------------------

func TestNewTraceContext(t *testing.T) {
	tc := NewTraceContext()
	if len(tc.TraceID) != 32 {
		t.Errorf("TraceID length = %d, want 32", len(tc.TraceID))
	}
	if len(tc.SpanID) != 16 {
		t.Errorf("SpanID length = %d, want 16", len(tc.SpanID))
	}
	if tc.ParentSpanID != "" {
		t.Errorf("ParentSpanID should be empty for root span, got %q", tc.ParentSpanID)
	}
	if !tc.Sampled {
		t.Error("new trace contexts should be sampled by default")
	}
}

func TestNewSpan(t *testing.T) {
	parent := NewTraceContext()
	child := NewSpan(parent)

	if child.TraceID != parent.TraceID {
		t.Errorf("child TraceID %q != parent TraceID %q", child.TraceID, parent.TraceID)
	}
	if child.SpanID == parent.SpanID {
		t.Error("child SpanID should differ from parent SpanID")
	}
	if child.ParentSpanID != parent.SpanID {
		t.Errorf("child ParentSpanID = %q, want %q", child.ParentSpanID, parent.SpanID)
	}
	if child.Sampled != parent.Sampled {
		t.Error("child should inherit parent's Sampled flag")
	}
}

// ---------------------------------------------------------------------------
// FormatTraceparent
// ---------------------------------------------------------------------------

func TestFormatTraceparent_Sampled(t *testing.T) {
	tc := TraceContext{
		TraceID: "4bf92f3577b6a27520e8ecf678db76fc",
		SpanID:  "00f067aa0ba902b7",
		Sampled: true,
	}
	got := FormatTraceparent(tc)
	want := "00-4bf92f3577b6a27520e8ecf678db76fc-00f067aa0ba902b7-01"
	if got != want {
		t.Errorf("FormatTraceparent = %q, want %q", got, want)
	}
}

func TestFormatTraceparent_NotSampled(t *testing.T) {
	tc := TraceContext{
		TraceID: "4bf92f3577b6a27520e8ecf678db76fc",
		SpanID:  "00f067aa0ba902b7",
		Sampled: false,
	}
	got := FormatTraceparent(tc)
	if !strings.HasSuffix(got, "-00") {
		t.Errorf("unsampled traceparent should end with -00, got %q", got)
	}
}

func TestFormatTraceparent_RoundTrip(t *testing.T) {
	original := "00-4bf92f3577b6a27520e8ecf678db76fc-00f067aa0ba902b7-01"
	tc, err := ParseTraceparent(original)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	// Create a child span and format it.
	child := NewSpan(tc)
	formatted := FormatTraceparent(child)

	// Verify the formatted header is parseable and preserves trace_id.
	reparsed, err := ParseTraceparent(formatted)
	if err != nil {
		t.Fatalf("reparsed error: %v", err)
	}
	if reparsed.TraceID != tc.TraceID {
		t.Errorf("trace_id changed after round-trip: got %q, want %q", reparsed.TraceID, tc.TraceID)
	}
}

// ---------------------------------------------------------------------------
// Context helpers
// ---------------------------------------------------------------------------

func TestTraceFromContext_Missing(t *testing.T) {
	ctx := context.Background()
	tc := TraceFromContext(ctx)
	if tc.TraceID != "" {
		t.Errorf("expected empty TraceContext, got TraceID %q", tc.TraceID)
	}
}

func TestTraceFromContext_Present(t *testing.T) {
	want := NewTraceContext()
	ctx := withTraceContext(context.Background(), want)
	got := TraceFromContext(ctx)
	if got.TraceID != want.TraceID {
		t.Errorf("TraceID = %q, want %q", got.TraceID, want.TraceID)
	}
	if got.SpanID != want.SpanID {
		t.Errorf("SpanID = %q, want %q", got.SpanID, want.SpanID)
	}
}

// ---------------------------------------------------------------------------
// InjectTraceparent
// ---------------------------------------------------------------------------

func TestInjectTraceparent(t *testing.T) {
	tc := TraceContext{
		TraceID: "4bf92f3577b6a27520e8ecf678db76fc",
		SpanID:  "00f067aa0ba902b7",
		Sampled: true,
	}
	req, _ := http.NewRequest("GET", "http://example.com/", nil)
	InjectTraceparent(req, tc)

	got := req.Header.Get("traceparent")
	if got == "" {
		t.Fatal("traceparent header was not set")
	}
	want := "00-4bf92f3577b6a27520e8ecf678db76fc-00f067aa0ba902b7-01"
	if got != want {
		t.Errorf("traceparent = %q, want %q", got, want)
	}
}

func TestInjectTraceparent_EmptyContext_NoOp(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://example.com/", nil)
	InjectTraceparent(req, TraceContext{}) // empty — should be a no-op
	if h := req.Header.Get("traceparent"); h != "" {
		t.Errorf("expected no traceparent header, got %q", h)
	}
}

// ---------------------------------------------------------------------------
// TraceMiddleware — HTTP middleware integration
// ---------------------------------------------------------------------------

func TestTraceMiddleware_NoIncomingTraceparent(t *testing.T) {
	var capturedCtx context.Context
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCtx = r.Context()
		w.WriteHeader(http.StatusOK)
	})

	mw := TraceMiddleware(nil)(handler)
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	// Response must include traceparent.
	tp := rr.Header().Get("traceparent")
	if tp == "" {
		t.Fatal("response should include traceparent header")
	}
	// Validate it's a legal traceparent.
	_, err := ParseTraceparent(tp)
	if err != nil {
		t.Errorf("response traceparent is invalid: %v", err)
	}

	// Context must contain TraceContext.
	tc := TraceFromContext(capturedCtx)
	if tc.TraceID == "" {
		t.Error("request context should contain TraceContext.TraceID")
	}
	if tc.SpanID == "" {
		t.Error("request context should contain TraceContext.SpanID")
	}
	// No parent since there was no incoming header.
	if tc.ParentSpanID != "" {
		t.Errorf("ParentSpanID should be empty for root request, got %q", tc.ParentSpanID)
	}
}

func TestTraceMiddleware_WithIncomingTraceparent(t *testing.T) {
	incomingHeader := "00-4bf92f3577b6a27520e8ecf678db76fc-00f067aa0ba902b7-01"

	var capturedCtx context.Context
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCtx = r.Context()
		w.WriteHeader(http.StatusOK)
	})

	mw := TraceMiddleware(nil)(handler)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("traceparent", incomingHeader)
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	tc := TraceFromContext(capturedCtx)

	// trace_id must be propagated from the incoming header.
	if tc.TraceID != "4bf92f3577b6a27520e8ecf678db76fc" {
		t.Errorf("TraceID = %q, want %q", tc.TraceID, "4bf92f3577b6a27520e8ecf678db76fc")
	}

	// The incoming parent-id becomes ParentSpanID.
	if tc.ParentSpanID != "00f067aa0ba902b7" {
		t.Errorf("ParentSpanID = %q, want %q", tc.ParentSpanID, "00f067aa0ba902b7")
	}

	// A new SpanID should have been generated for this hop.
	if tc.SpanID == "00f067aa0ba902b7" {
		t.Error("SpanID should be a new value, not the incoming parent-id")
	}
	if len(tc.SpanID) != 16 {
		t.Errorf("SpanID length = %d, want 16", len(tc.SpanID))
	}

	// Response traceparent must carry the new span.
	outgoing := rr.Header().Get("traceparent")
	parsed, err := ParseTraceparent(outgoing)
	if err != nil {
		t.Fatalf("outgoing traceparent is invalid: %v", err)
	}
	if parsed.TraceID != tc.TraceID {
		t.Errorf("outgoing trace_id = %q, want %q", parsed.TraceID, tc.TraceID)
	}
	// FormatTraceparent puts tc.SpanID in the parent-id field of the header.
	// After parsing, that becomes parsed.SpanID (the caller's current span).
	if parsed.SpanID != tc.SpanID {
		t.Errorf("outgoing parent-id field = %q, want new span_id %q", parsed.SpanID, tc.SpanID)
	}
}

func TestTraceMiddleware_InvalidIncomingTraceparent(t *testing.T) {
	// Middleware should fall back to generating a new trace when the header is invalid.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := TraceMiddleware(nil)(handler)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("traceparent", "invalid-header")
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	tp := rr.Header().Get("traceparent")
	if tp == "" {
		t.Fatal("response should include traceparent even when incoming is invalid")
	}
	if _, err := ParseTraceparent(tp); err != nil {
		t.Errorf("response traceparent invalid: %v", err)
	}
}

func TestTraceMiddleware_TraceIDInContext(t *testing.T) {
	// TraceMiddleware must also store trace_id via WithTraceID for LoggerFromContext.
	var capturedCtx context.Context
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCtx = r.Context()
		w.WriteHeader(http.StatusOK)
	})

	mw := TraceMiddleware(nil)(handler)
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	traceID := TraceIDFromContext(capturedCtx)
	if traceID == "" {
		t.Error("trace_id should be stored via WithTraceID for LoggerFromContext compatibility")
	}
	tc := TraceFromContext(capturedCtx)
	if traceID != tc.TraceID {
		t.Errorf("TraceIDFromContext %q != TraceContext.TraceID %q", traceID, tc.TraceID)
	}
}

func TestTraceMiddleware_SampledPropagation(t *testing.T) {
	// When incoming traceparent is not sampled, the flag should be preserved.
	incomingHeader := "00-4bf92f3577b6a27520e8ecf678db76fc-00f067aa0ba902b7-00"

	var capturedCtx context.Context
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCtx = r.Context()
		w.WriteHeader(http.StatusOK)
	})

	mw := TraceMiddleware(nil)(handler)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("traceparent", incomingHeader)
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	tc := TraceFromContext(capturedCtx)
	if tc.Sampled {
		t.Error("Sampled should be false when incoming trace-flags is 00")
	}

	// Verify the response header also reflects not-sampled.
	outgoing := rr.Header().Get("traceparent")
	if !strings.HasSuffix(outgoing, "-00") {
		t.Errorf("response traceparent should end with -00 for unsampled, got %q", outgoing)
	}
}
