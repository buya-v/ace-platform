package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/garudax-platform/gateway/internal/auth"
	"github.com/garudax-platform/gateway/internal/middleware"
	"github.com/garudax-platform/gateway/internal/proxy"
	"github.com/garudax-platform/gateway/internal/router"
)

// mockClient records forwarded requests and returns canned responses.
type mockClient struct {
	lastReq *proxy.BackendRequest
	resp    *proxy.BackendResponse
	err     error
}

func (m *mockClient) Forward(req *proxy.BackendRequest) (*proxy.BackendResponse, error) {
	m.lastReq = req
	if m.err != nil {
		return nil, m.err
	}
	if m.resp != nil {
		return m.resp, nil
	}
	return &proxy.BackendResponse{
		StatusCode: 200,
		Body:       json.RawMessage(`{"status":"ok"}`),
	}, nil
}

func setupRouter(mc *mockClient) *router.Router {
	h := New(mc)
	rt := router.New()
	h.RegisterRoutes(rt)
	return rt
}

func TestRouteRegistration(t *testing.T) {
	rt := setupRouter(&mockClient{})
	routes := rt.GetRoutes()

	// 2 health + 6 orders + 3 market + 6 admin + 3 clearing + 4 margin + 2 settlement
	// + 7 auth + 7 onboarding + 5 screening + 6 compliance admin = 51 routes
	if len(routes) < 48 {
		t.Errorf("expected at least 48 routes, got %d", len(routes))
	}

	expectedRoutes := []string{
		"POST /api/v1/orders",
		"GET /api/v1/orders/{order_id}",
		"GET /api/v1/instruments/{instrument_id}/book",
		"GET /api/v1/clearing/positions",
		"GET /api/v1/margin",
		"GET /api/v1/settlement/cycles",
		"POST /api/v1/auth/login",
		"POST /api/v1/participants",
		"GET /healthz",
		"GET /readyz",
	}

	routeMap := make(map[string]bool)
	for _, r := range routes {
		routeMap[r.Method+" "+r.Pattern] = true
	}

	for _, expected := range expectedRoutes {
		if !routeMap[expected] {
			t.Errorf("route %q not registered", expected)
		}
	}
}

func TestSubmitOrderForwarding(t *testing.T) {
	mc := &mockClient{
		resp: &proxy.BackendResponse{
			StatusCode: 201,
			Body:       json.RawMessage(`{"order_id":"ord-1"}`),
		},
	}
	rt := setupRouter(mc)

	body := `{"instrument_id":"WHT","side":"buy","price":"100.00","quantity":10}`
	req := httptest.NewRequest("POST", "/api/v1/orders", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != 201 {
		t.Errorf("status = %d, want 201", rec.Code)
	}
	if mc.lastReq == nil {
		t.Fatal("expected request to be forwarded")
	}
	if mc.lastReq.Service != "matching-engine" {
		t.Errorf("service = %q, want %q", mc.lastReq.Service, "matching-engine")
	}
	if mc.lastReq.Method != "OrderService/SubmitOrder" {
		t.Errorf("method = %q, want %q", mc.lastReq.Method, "OrderService/SubmitOrder")
	}
}

func TestForwardInjectsTenantMetadata(t *testing.T) {
	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"generic forward", "GET", "/api/v1/clearing/positions", ""},
		{"submit order", "POST", "/api/v1/orders", `{"instrument_id":"WHT","side":"buy","price":"100.00","quantity":10}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mc := &mockClient{}
			rt := setupRouter(mc)
			// Wrap with the real tenant middleware so the resolved tenant lands in
			// the request context exactly as it does in production.
			handler := middleware.TenantMiddleware([]string{"ace-commodities"})(rt)

			var bodyReader *strings.Reader
			if tc.body != "" {
				bodyReader = strings.NewReader(tc.body)
			} else {
				bodyReader = strings.NewReader("")
			}
			req := httptest.NewRequest(tc.method, tc.path, bodyReader)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-GarudaX-Tenant", "ace-commodities")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if mc.lastReq == nil {
				t.Fatal("expected request to be forwarded")
			}
			got := mc.lastReq.Metadata[middleware.TenantHeaderName]
			if got != "ace-commodities" {
				t.Errorf("forwarded tenant metadata = %q, want ace-commodities", got)
			}
		})
	}
}

func TestGetOrderBookPublic(t *testing.T) {
	mc := &mockClient{
		resp: &proxy.BackendResponse{
			StatusCode: 200,
			Body:       json.RawMessage(`{"bids":[],"asks":[]}`),
		},
	}
	rt := setupRouter(mc)

	req := httptest.NewRequest("GET", "/api/v1/instruments/WHT-HRW/book?depth=10", nil)
	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if mc.lastReq.Service != "matching-engine" {
		t.Errorf("service = %q, want %q", mc.lastReq.Service, "matching-engine")
	}
}

func TestServiceForwarding(t *testing.T) {
	tests := []struct {
		method      string
		path        string
		body        string
		wantService string
	}{
		{"GET", "/api/v1/clearing/positions", "", "clearing-engine"},
		{"GET", "/api/v1/margin", "", "margin-engine"},
		{"GET", "/api/v1/settlement/cycles", "", "settlement-engine"},
		{"POST", "/api/v1/auth/login", `{"username":"u","password":"p"}`, "auth-service"},
		{"POST", "/api/v1/participants", `{"name":"Test"}`, "compliance-service"},
		{"POST", "/api/v1/screening/check", `{}`, "compliance-service"},
		{"GET", "/api/v1/compliance/alerts", "", "compliance-service"},
	}

	for _, tt := range tests {
		t.Run(tt.method+"_"+tt.path, func(t *testing.T) {
			mc := &mockClient{}
			rt := setupRouter(mc)

			var req *http.Request
			if tt.body != "" {
				req = httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			} else {
				req = httptest.NewRequest(tt.method, tt.path, nil)
			}
			rec := httptest.NewRecorder()
			rt.ServeHTTP(rec, req)

			if mc.lastReq == nil {
				t.Fatal("expected request to be forwarded")
			}
			if mc.lastReq.Service != tt.wantService {
				t.Errorf("service = %q, want %q", mc.lastReq.Service, tt.wantService)
			}
		})
	}
}

func TestHealthEndpoint(t *testing.T) {
	rt := setupRouter(&mockClient{})

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	var resp map[string]string
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["status"] != "ok" {
		t.Errorf("health status = %q, want %q", resp["status"], "ok")
	}
}

func TestReadyzNotReady(t *testing.T) {
	rt := setupRouter(&mockClient{})

	req := httptest.NewRequest("GET", "/readyz", nil)
	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, req)

	if rec.Code != 503 {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}

func TestForwardWithClaims(t *testing.T) {
	mc := &mockClient{}
	rt := setupRouter(mc)

	req := httptest.NewRequest("GET", "/api/v1/orders", nil)
	claims := &auth.Claims{
		Sub:           "user-123",
		ParticipantID: "part-456",
		Roles:         []string{"trader"},
	}
	ctx := context.WithValue(req.Context(), middleware.ClaimsContextKey, claims)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, req)

	if mc.lastReq.Metadata["x-user-id"] != "user-123" {
		t.Errorf("x-user-id = %q, want %q", mc.lastReq.Metadata["x-user-id"], "user-123")
	}
	if mc.lastReq.Metadata["x-participant-id"] != "part-456" {
		t.Errorf("x-participant-id = %q, want %q", mc.lastReq.Metadata["x-participant-id"], "part-456")
	}
}

func TestNotFoundEndpoint(t *testing.T) {
	rt := setupRouter(&mockClient{})

	req := httptest.NewRequest("GET", "/api/v1/nonexistent", nil)
	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, req)

	if rec.Code != 404 {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestBackendError(t *testing.T) {
	mc := &mockClient{err: http.ErrServerClosed}
	rt := setupRouter(mc)

	req := httptest.NewRequest("GET", "/api/v1/orders", nil)
	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, req)

	if rec.Code != 502 {
		t.Errorf("status = %d, want 502", rec.Code)
	}
}

func TestAPIVersionHeader(t *testing.T) {
	mc := &mockClient{}
	rt := setupRouter(mc)

	req := httptest.NewRequest("GET", "/api/v1/orders", nil)
	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, req)

	if rec.Header().Get("X-API-Version") != "v1" {
		t.Errorf("X-API-Version = %q, want %q", rec.Header().Get("X-API-Version"), "v1")
	}
}
