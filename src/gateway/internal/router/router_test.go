package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMatchPath(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		match   bool
		params  map[string]string
	}{
		{"/api/v1/orders", "/api/v1/orders", true, map[string]string{}},
		{"/api/v1/orders/{order_id}", "/api/v1/orders/abc-123", true, map[string]string{"order_id": "abc-123"}},
		{"/api/v1/orders/{order_id}", "/api/v1/orders", false, nil},
		{"/api/v1/instruments/{id}/book", "/api/v1/instruments/WHT/book", true, map[string]string{"id": "WHT"}},
		{"/api/v1/instruments/{id}/book/l3", "/api/v1/instruments/WHT/book/l3", true, map[string]string{"id": "WHT"}},
		{"/api/v1/orders", "/api/v1/positions", false, nil},
		{"/healthz", "/healthz", true, map[string]string{}},
		{"/api/v1/admin/participants/{id}/disable", "/api/v1/admin/participants/p1/disable", true, map[string]string{"id": "p1"}},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.path, func(t *testing.T) {
			params, ok := matchPath(tt.pattern, tt.path)
			if ok != tt.match {
				t.Errorf("matchPath(%q, %q) match = %v, want %v", tt.pattern, tt.path, ok, tt.match)
			}
			if ok && tt.params != nil {
				for k, v := range tt.params {
					if params[k] != v {
						t.Errorf("param %q = %q, want %q", k, params[k], v)
					}
				}
			}
		})
	}
}

func TestRouterBasicRouting(t *testing.T) {
	rt := New()

	var called string
	rt.Handle("GET", "/api/v1/orders", func(w http.ResponseWriter, r *http.Request) {
		called = "list_orders"
		w.WriteHeader(200)
	})
	rt.Handle("POST", "/api/v1/orders", func(w http.ResponseWriter, r *http.Request) {
		called = "create_order"
		w.WriteHeader(201)
	})
	rt.Handle("GET", "/api/v1/orders/{order_id}", func(w http.ResponseWriter, r *http.Request) {
		called = "get_order:" + r.URL.Query().Get("order_id")
		w.WriteHeader(200)
	})
	rt.Handle("DELETE", "/api/v1/orders/{order_id}", func(w http.ResponseWriter, r *http.Request) {
		called = "cancel_order:" + r.URL.Query().Get("order_id")
		w.WriteHeader(200)
	})

	tests := []struct {
		method     string
		path       string
		wantCode   int
		wantCalled string
	}{
		{"GET", "/api/v1/orders", 200, "list_orders"},
		{"POST", "/api/v1/orders", 201, "create_order"},
		{"GET", "/api/v1/orders/abc-123", 200, "get_order:abc-123"},
		{"DELETE", "/api/v1/orders/abc-123", 200, "cancel_order:abc-123"},
		{"PUT", "/api/v1/orders", 405, ""}, // method not allowed
		{"GET", "/api/v1/missing", 404, ""},
	}

	for _, tt := range tests {
		t.Run(tt.method+"_"+tt.path, func(t *testing.T) {
			called = ""
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			rt.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if tt.wantCalled != "" && called != tt.wantCalled {
				t.Errorf("called = %q, want %q", called, tt.wantCalled)
			}
		})
	}
}

func TestRouterPathParams(t *testing.T) {
	rt := New()

	rt.Handle("GET", "/api/v1/instruments/{instrument_id}/book", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(r.URL.Query().Get("instrument_id")))
	})

	req := httptest.NewRequest("GET", "/api/v1/instruments/WHT-HRW-2026M07-UB/book?depth=20", nil)
	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "WHT-HRW-2026M07-UB" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "WHT-HRW-2026M07-UB")
	}
}
