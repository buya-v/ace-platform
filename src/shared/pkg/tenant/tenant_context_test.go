package tenant_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garudax-platform/tenant"
)

// These tests complement middleware_test.go. They focus on two areas the
// existing suite does not exercise:
//
//  1. Observability label injection (observability.go) — TenantLogger and
//     TenantMetricLabels, which satisfy GarudaX_Strategy_Directive §3.3:
//     "every metric, log, and trace carries tenant_id as a required label."
//  2. Middleware contract edge cases — that next is NOT invoked on failure,
//     that error responses are JSON, and that validation is exact (case
//     sensitive, no implicit trimming).

// --- Observability: TenantMetricLabels ---

func TestTenantMetricLabels_WithTenant(t *testing.T) {
	ctx := tenant.WithTenant(context.Background(), tenant.TenantID("mse-equities"))

	labels := tenant.TenantMetricLabels(ctx)

	got, ok := labels["tenant_id"]
	if !ok {
		t.Fatal("expected tenant_id label to be present")
	}
	if got != "mse-equities" {
		t.Fatalf("expected tenant_id=mse-equities, got %q", got)
	}
	if len(labels) != 1 {
		t.Fatalf("expected exactly 1 label, got %d: %v", len(labels), labels)
	}
}

func TestTenantMetricLabels_NoTenant(t *testing.T) {
	// Even without a tenant, the label key must be present (empty value) so
	// callers can attach it unconditionally without a Prometheus label-cardinality
	// mismatch between the two code paths.
	labels := tenant.TenantMetricLabels(context.Background())

	got, ok := labels["tenant_id"]
	if !ok {
		t.Fatal("expected tenant_id label key to be present even when no tenant is set")
	}
	if got != "" {
		t.Fatalf("expected empty tenant_id value, got %q", got)
	}
}

// --- Observability: TenantLogger ---

// newJSONLogger returns a slog.Logger that writes JSON records into buf so the
// test can inspect the attributes attached to each log line.
func newJSONLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

// lastLogRecord decodes the final JSON log line written to buf.
func lastLogRecord(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	out := buf.Bytes()
	if len(out) == 0 {
		t.Fatal("no log output captured")
	}
	var rec map[string]any
	if err := json.Unmarshal(out, &rec); err != nil {
		t.Fatalf("failed to decode log record %q: %v", out, err)
	}
	return rec
}

func TestTenantLogger_InjectsTenantLabel(t *testing.T) {
	var buf bytes.Buffer
	base := newJSONLogger(&buf)
	ctx := tenant.WithTenant(context.Background(), tenant.TenantID("ace-commodities"))

	logger := tenant.TenantLogger(ctx, base)
	logger.Info("order accepted")

	rec := lastLogRecord(t, &buf)
	got, ok := rec["tenant_id"]
	if !ok {
		t.Fatal("expected tenant_id attribute on log record")
	}
	if got != "ace-commodities" {
		t.Fatalf("expected tenant_id=ace-commodities in log, got %v", got)
	}
}

func TestTenantLogger_NoTenantReturnsBaseUnchanged(t *testing.T) {
	var buf bytes.Buffer
	base := newJSONLogger(&buf)

	logger := tenant.TenantLogger(context.Background(), base)

	// When no tenant is present, the same logger instance is returned so callers
	// on bypass/health paths are not penalised with a wrapping layer.
	if logger != base {
		t.Fatal("expected TenantLogger to return the base logger unchanged when no tenant is set")
	}

	logger.Info("health check")
	rec := lastLogRecord(t, &buf)
	if _, ok := rec["tenant_id"]; ok {
		t.Fatalf("did not expect a tenant_id attribute when no tenant is set: %v", rec)
	}
}

// --- End-to-end propagation: middleware → context → observability ---

// TestObservabilityLabelsAfterMiddleware verifies the full propagation chain:
// the middleware validates the header and injects the tenant, and the
// observability helpers downstream read the same value out of the request context.
func TestObservabilityLabelsAfterMiddleware(t *testing.T) {
	var (
		gotLabels map[string]string
		logBuf    bytes.Buffer
	)
	base := newJSONLogger(&logBuf)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotLabels = tenant.TenantMetricLabels(r.Context())
		tenant.TenantLogger(r.Context(), base).Info("handled")
		w.WriteHeader(http.StatusOK)
	})

	mw := tenant.TenantMiddleware([]string{"ace-commodities", "mse-equities"})
	handler := mw(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/orders", nil)
	req.Header.Set("X-GarudaX-Tenant", "mse-equities")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if gotLabels["tenant_id"] != "mse-equities" {
		t.Fatalf("metric label not propagated: got %q", gotLabels["tenant_id"])
	}
	rec := lastLogRecord(t, &logBuf)
	if rec["tenant_id"] != "mse-equities" {
		t.Fatalf("log label not propagated: got %v", rec["tenant_id"])
	}
}

// --- Middleware contract edge cases ---

// nextProbe records whether the wrapped handler was reached.
type nextProbe struct{ called bool }

func (n *nextProbe) ServeHTTP(http.ResponseWriter, *http.Request) { n.called = true }

func TestTenantMiddleware_DoesNotCallNextOnMissingHeader(t *testing.T) {
	probe := &nextProbe{}
	handler := tenant.TenantMiddleware([]string{"ace-commodities"})(probe)

	req := httptest.NewRequest(http.MethodGet, "/api/orders", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if probe.called {
		t.Fatal("next handler must not run when the tenant header is missing")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestTenantMiddleware_DoesNotCallNextOnUnknownTenant(t *testing.T) {
	probe := &nextProbe{}
	handler := tenant.TenantMiddleware([]string{"ace-commodities"})(probe)

	req := httptest.NewRequest(http.MethodGet, "/api/orders", nil)
	req.Header.Set("X-GarudaX-Tenant", "rogue-exchange")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if probe.called {
		t.Fatal("next handler must not run for an unregistered tenant")
	}
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestTenantMiddleware_ErrorResponseIsJSON(t *testing.T) {
	handler := tenant.TenantMiddleware([]string{"ace-commodities"})(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/orders", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("error body is not valid JSON: %v", err)
	}
	if body.Error.Message == "" {
		t.Fatal("expected a non-empty error message")
	}
}

func TestTenantMiddleware_ValidationIsCaseSensitive(t *testing.T) {
	// Tenant IDs are lowercase slugs (platform-architecture §2.2). An
	// upper-cased header must not match a registered lowercase tenant.
	handler := tenant.TenantMiddleware([]string{"ace-commodities"})(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/orders", nil)
	req.Header.Set("X-GarudaX-Tenant", "ACE-COMMODITIES")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for case-mismatched tenant, got %d", rr.Code)
	}
}

func TestTenantMiddleware_WhitespaceHeaderRejected(t *testing.T) {
	// A header that is present but only whitespace is not the empty string, so
	// it fails the whitelist check (403) rather than the presence check (401).
	// This documents that the middleware performs no implicit trimming.
	handler := tenant.TenantMiddleware([]string{"ace-commodities"})(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/orders", nil)
	req.Header.Set("X-GarudaX-Tenant", "   ")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for whitespace-only tenant, got %d", rr.Code)
	}
}

func TestTenantMiddleware_EmptyWhitelistRejectsEverything(t *testing.T) {
	handler := tenant.TenantMiddleware(nil)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/orders", nil)
	req.Header.Set("X-GarudaX-Tenant", "ace-commodities")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 with empty whitelist, got %d", rr.Code)
	}
}

func TestTenantMiddleware_MultipleTenantsResolveIndependently(t *testing.T) {
	cases := []string{"ace-commodities", "mse-equities"}
	for _, want := range cases {
		want := want
		t.Run(want, func(t *testing.T) {
			var got tenant.TenantID
			handler := tenant.TenantMiddleware(cases)(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					got = tenant.MustTenant(r.Context())
					w.WriteHeader(http.StatusOK)
				}),
			)

			req := httptest.NewRequest(http.MethodPost, "/api/orders", nil)
			req.Header.Set("X-GarudaX-Tenant", want)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", rr.Code)
			}
			if string(got) != want {
				t.Fatalf("expected resolved tenant %q, got %q", want, got)
			}
		})
	}
}

// --- Context helpers ---

func TestMustTenant_ReturnsValueWhenPresent(t *testing.T) {
	ctx := tenant.WithTenant(context.Background(), tenant.TenantID("ace-commodities"))
	if got := tenant.MustTenant(ctx); got != "ace-commodities" {
		t.Fatalf("expected ace-commodities, got %q", got)
	}
}

func TestTenantID_String(t *testing.T) {
	if got := tenant.TenantID("mse-equities").String(); got != "mse-equities" {
		t.Fatalf("expected mse-equities, got %q", got)
	}
}
