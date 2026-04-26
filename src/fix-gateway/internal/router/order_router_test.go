package router

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// capturedRequest stores the method, path, headers, and body of the last request
// received by the mock server.
type capturedRequest struct {
	method string
	path   string
	header http.Header
	body   []byte
}

// newMockServer returns a test HTTP server that captures the last request and
// responds with the provided status code and JSON body.
func newMockServer(t *testing.T, status int, respBody string) (*httptest.Server, *capturedRequest) {
	t.Helper()
	captured := &capturedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.method = r.Method
		captured.path = r.URL.Path
		captured.header = r.Header.Clone()
		body, _ := io.ReadAll(r.Body)
		captured.body = body

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write([]byte(respBody))
	}))
	t.Cleanup(srv.Close)
	return srv, captured
}

// ---------- SubmitOrder tests ----------

// TestOrderRouter_SubmitOrder verifies that SubmitOrder sends a POST to
// /api/v1/securities/orders with the correct JSON body.
func TestOrderRouter_SubmitOrder(t *testing.T) {
	respJSON := `{"order_id":"ORD-001","status":"NEW"}`
	srv, captured := newMockServer(t, http.StatusCreated, respJSON)

	r := NewOrderRouter(srv.URL)

	order := map[string]interface{}{
		"instrument_id":   "SEC-001",
		"side":            "BUY",
		"order_type":      "LIMIT",
		"quantity":        100,
		"price":           42.50,
		"client_order_id": "CLO-XYZ",
	}

	result, err := r.SubmitOrder(order, "mse-equities")
	if err != nil {
		t.Fatalf("SubmitOrder: %v", err)
	}

	// Verify the HTTP method.
	if captured.method != http.MethodPost {
		t.Errorf("method: got %q, want POST", captured.method)
	}

	// Verify the request path.
	const wantPath = "/api/v1/securities/orders"
	if captured.path != wantPath {
		t.Errorf("path: got %q, want %q", captured.path, wantPath)
	}

	// Verify Content-Type.
	if ct := captured.header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q, want application/json", ct)
	}

	// Verify request body is valid JSON containing the key fields.
	var sentOrder map[string]interface{}
	if err := json.Unmarshal(captured.body, &sentOrder); err != nil {
		t.Fatalf("request body is not valid JSON: %v (body=%q)", err, captured.body)
	}
	if sentOrder["instrument_id"] != "SEC-001" {
		t.Errorf("body instrument_id: got %v, want SEC-001", sentOrder["instrument_id"])
	}
	if sentOrder["side"] != "BUY" {
		t.Errorf("body side: got %v, want BUY", sentOrder["side"])
	}

	// Verify the parsed response.
	if result["order_id"] != "ORD-001" {
		t.Errorf("response order_id: got %v, want ORD-001", result["order_id"])
	}
}

// TestOrderRouter_SubmitOrder_ErrorStatus verifies that a non-2xx response is returned as an error.
func TestOrderRouter_SubmitOrder_ErrorStatus(t *testing.T) {
	srv, _ := newMockServer(t, http.StatusBadRequest, `{"error":"invalid order"}`)

	r := NewOrderRouter(srv.URL)
	order := map[string]interface{}{"instrument_id": "BAD"}

	_, err := r.SubmitOrder(order, "mse-equities")
	if err == nil {
		t.Fatal("expected error for 400 response, got nil")
	}
}

// ---------- CancelOrder tests ----------

// TestOrderRouter_CancelOrder verifies that CancelOrder sends a DELETE to
// /api/v1/securities/orders/{orderID}.
func TestOrderRouter_CancelOrder(t *testing.T) {
	srv, captured := newMockServer(t, http.StatusNoContent, "")

	r := NewOrderRouter(srv.URL)

	if err := r.CancelOrder("ORD-999", "mse-equities"); err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}

	// Verify the HTTP method.
	if captured.method != http.MethodDelete {
		t.Errorf("method: got %q, want DELETE", captured.method)
	}

	// Verify the path includes the order ID.
	const wantPath = "/api/v1/securities/orders/ORD-999"
	if captured.path != wantPath {
		t.Errorf("path: got %q, want %q", captured.path, wantPath)
	}
}

// TestOrderRouter_CancelOrder_ErrorStatus verifies that a non-2xx cancel response is an error.
func TestOrderRouter_CancelOrder_ErrorStatus(t *testing.T) {
	srv, _ := newMockServer(t, http.StatusNotFound, `{"error":"order not found"}`)

	r := NewOrderRouter(srv.URL)

	err := r.CancelOrder("NO-SUCH-ORDER", "mse-equities")
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}

// ---------- Tenant header tests ----------

// TestOrderRouter_TenantHeader verifies that X-GarudaX-Tenant is set on both
// SubmitOrder and CancelOrder requests.
func TestOrderRouter_TenantHeader(t *testing.T) {
	t.Run("SubmitOrder", func(t *testing.T) {
		srv, captured := newMockServer(t, http.StatusCreated, `{"order_id":"X"}`)
		r := NewOrderRouter(srv.URL)

		_, err := r.SubmitOrder(map[string]interface{}{"instrument_id": "SEC-001"}, "ace-commodities")
		if err != nil {
			t.Fatalf("SubmitOrder: %v", err)
		}

		got := captured.header.Get("X-GarudaX-Tenant")
		if got != "ace-commodities" {
			t.Errorf("X-GarudaX-Tenant: got %q, want ace-commodities", got)
		}
	})

	t.Run("CancelOrder", func(t *testing.T) {
		srv, captured := newMockServer(t, http.StatusNoContent, "")
		r := NewOrderRouter(srv.URL)

		if err := r.CancelOrder("ORD-777", "ace-commodities"); err != nil {
			t.Fatalf("CancelOrder: %v", err)
		}

		got := captured.header.Get("X-GarudaX-Tenant")
		if got != "ace-commodities" {
			t.Errorf("X-GarudaX-Tenant: got %q, want ace-commodities", got)
		}
	})

	t.Run("TenantIsolation", func(t *testing.T) {
		// Verify two calls with different tenant IDs do not bleed.
		srv1, cap1 := newMockServer(t, http.StatusCreated, `{"order_id":"A"}`)
		srv2, cap2 := newMockServer(t, http.StatusCreated, `{"order_id":"B"}`)

		r1 := NewOrderRouter(srv1.URL)
		r2 := NewOrderRouter(srv2.URL)

		_, _ = r1.SubmitOrder(map[string]interface{}{"instrument_id": "X"}, "mse-equities")
		_, _ = r2.SubmitOrder(map[string]interface{}{"instrument_id": "Y"}, "ace-commodities")

		if cap1.header.Get("X-GarudaX-Tenant") != "mse-equities" {
			t.Errorf("r1 tenant: got %q, want mse-equities", cap1.header.Get("X-GarudaX-Tenant"))
		}
		if cap2.header.Get("X-GarudaX-Tenant") != "ace-commodities" {
			t.Errorf("r2 tenant: got %q, want ace-commodities", cap2.header.Get("X-GarudaX-Tenant"))
		}
	})
}
