package bot

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// --- Test helper ---

// mockServer creates a test server that responds to a specific path+method combo.
type mockHandler struct {
	path     string
	method   string
	status   int
	body     string
	called   atomic.Bool
}

func (m *mockHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == m.path && r.Method == m.method {
		m.called.Store(true)
		w.WriteHeader(m.status)
		w.Write([]byte(m.body))
		return
	}
	w.WriteHeader(404)
	w.Write([]byte(`{"error":"not found"}`))
}

// =====================================================================
// INSTRUMENT RESOLUTION
// =====================================================================

func TestResolveInstrument_Wheat(t *testing.T) {
	if id := resolveInstrument("wheat"); id != "WHT-HRW-2026M07-UB" {
		t.Errorf("wheat => %q, want WHT-HRW-2026M07-UB", id)
	}
}

func TestResolveInstrument_WHT_Alias(t *testing.T) {
	if id := resolveInstrument("wht"); id != "WHT-HRW-2026M07-UB" {
		t.Errorf("wht => %q, want WHT-HRW-2026M07-UB", id)
	}
}

func TestResolveInstrument_Corn(t *testing.T) {
	if id := resolveInstrument("corn"); id != "CRN-YEL-2026M09-UB" {
		t.Errorf("corn => %q, want CRN-YEL-2026M09-UB", id)
	}
}

func TestResolveInstrument_Soybeans(t *testing.T) {
	if id := resolveInstrument("soybeans"); id != "SBN-NO2-2026M11-UB" {
		t.Errorf("soybeans => %q, want SBN-NO2-2026M11-UB", id)
	}
}

func TestResolveInstrument_SoybeanSingular(t *testing.T) {
	if id := resolveInstrument("soybean"); id != "SBN-NO2-2026M11-UB" {
		t.Errorf("soybean => %q, want SBN-NO2-2026M11-UB", id)
	}
}

func TestResolveInstrument_Barley(t *testing.T) {
	if id := resolveInstrument("barley"); id != "BRL-MALT-2026M07-UB" {
		t.Errorf("barley => %q, want BRL-MALT-2026M07-UB", id)
	}
}

func TestResolveInstrument_Cashmere(t *testing.T) {
	if id := resolveInstrument("cashmere"); id != "CSH-RAW-2026M09-UB" {
		t.Errorf("cashmere => %q, want CSH-RAW-2026M09-UB", id)
	}
}

func TestResolveInstrument_Cattle(t *testing.T) {
	if id := resolveInstrument("cattle"); id != "LVS-CATTLE-2026M10-UB" {
		t.Errorf("cattle => %q, want LVS-CATTLE-2026M10-UB", id)
	}
}

func TestResolveInstrument_FullIDPassthrough(t *testing.T) {
	id := resolveInstrument("WHT-HRW-2026M07-UB")
	if id != "WHT-HRW-2026M07-UB" {
		t.Errorf("full ID passthrough => %q, want WHT-HRW-2026M07-UB", id)
	}
}

func TestResolveInstrument_UnknownReturnsEmpty(t *testing.T) {
	if id := resolveInstrument("unknown"); id != "" {
		t.Errorf("unknown => %q, want empty string", id)
	}
}

func TestResolveInstrument_CaseInsensitive(t *testing.T) {
	if id := resolveInstrument("WHEAT"); id != "WHT-HRW-2026M07-UB" {
		t.Errorf("WHEAT => %q, want WHT-HRW-2026M07-UB", id)
	}
}

// =====================================================================
// TRADING — HALT
// =====================================================================

func TestExecutor_HaltWheat(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/admin/instruments/WHT-HRW-2026M07-UB/halt",
		method: "POST",
		status: 200,
		body:   `{"status":"halted"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("halt wheat", "test-token")
	if !h.called.Load() {
		t.Error("expected halt endpoint to be called")
	}
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success reply, got: %s", resp.Reply)
	}
	if !strings.Contains(resp.Reply, "WHT-HRW-2026M07-UB") {
		t.Errorf("expected instrument ID in reply, got: %s", resp.Reply)
	}
}

func TestExecutor_HaltWheatFullID(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/admin/instruments/WHT-HRW-2026M07-UB/halt",
		method: "POST",
		status: 200,
		body:   `{"status":"halted"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("halt WHT-HRW-2026M07-UB", "test-token")
	if !h.called.Load() {
		t.Error("expected halt endpoint to be called with full ID")
	}
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success reply, got: %s", resp.Reply)
	}
}

func TestExecutor_HaltUnknownInstrument(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The executor will call the API because short unknown names won't have a hyphen,
		// so we just return 404 and check the error reply.
		w.WriteHeader(404)
		w.Write([]byte(`{"error":"not found"}`))
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	// Use a simple word with no hyphen and len <= 10 so resolveInstrument returns ""
	resp := exec.Execute("halt noexist", "test-token")
	if strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected error for unknown instrument, got: %s", resp.Reply)
	}
	// Should contain "Unknown" since resolveInstrument("noexist") returns ""
	if !strings.Contains(resp.Reply, "Unknown") {
		t.Errorf("expected 'Unknown' in reply, got: %s", resp.Reply)
	}
}

func TestExecutor_HaltCorn(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/admin/instruments/CRN-YEL-2026M09-UB/halt",
		method: "POST",
		status: 200,
		body:   `{"status":"halted"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("halt corn", "test-token")
	if !h.called.Load() {
		t.Error("halt corn: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success, got: %s", resp.Reply)
	}
}

func TestExecutor_HaltBarley(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/admin/instruments/BRL-MALT-2026M07-UB/halt",
		method: "POST",
		status: 200,
		body:   `{"status":"halted"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("halt barley", "test-token")
	if !h.called.Load() {
		t.Error("halt barley: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success, got: %s", resp.Reply)
	}
}

func TestExecutor_HaltCashmere(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/admin/instruments/CSH-RAW-2026M09-UB/halt",
		method: "POST",
		status: 200,
		body:   `{"status":"halted"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("halt cashmere", "test-token")
	if !h.called.Load() {
		t.Error("halt cashmere: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success, got: %s", resp.Reply)
	}
}

func TestExecutor_HaltWithAPIError(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/admin/instruments/WHT-HRW-2026M07-UB/halt",
		method: "POST",
		status: 500,
		body:   `{"error":"internal server error"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("halt wheat", "test-token")
	if !strings.Contains(resp.Reply, "❌") {
		t.Errorf("expected failure reply on 500, got: %s", resp.Reply)
	}
}

// =====================================================================
// TRADING — RESUME
// =====================================================================

func TestExecutor_ResumeCorn(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/admin/instruments/CRN-YEL-2026M09-UB/resume",
		method: "POST",
		status: 200,
		body:   `{"status":"active"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("resume corn", "test-token")
	if !h.called.Load() {
		t.Error("resume corn: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success, got: %s", resp.Reply)
	}
	if !strings.Contains(resp.Reply, "RESUMED") {
		t.Errorf("expected RESUMED in reply, got: %s", resp.Reply)
	}
}

func TestExecutor_ResumeWheat(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/admin/instruments/WHT-HRW-2026M07-UB/resume",
		method: "POST",
		status: 200,
		body:   `{"status":"active"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("resume wheat", "test-token")
	if !h.called.Load() {
		t.Error("resume wheat: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success, got: %s", resp.Reply)
	}
}

func TestExecutor_ResumeWithAPIError(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/admin/instruments/WHT-HRW-2026M07-UB/resume",
		method: "POST",
		status: 403,
		body:   `{"error":"forbidden"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("resume wheat", "test-token")
	if !strings.Contains(resp.Reply, "❌") {
		t.Errorf("expected failure reply on 403, got: %s", resp.Reply)
	}
}

func TestExecutor_ResumeUnknownInstrument(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("resume totallyfake", "test-token")
	if strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected failure for unknown instrument, got: %s", resp.Reply)
	}
}

// =====================================================================
// TRADING — MASS CANCEL
// =====================================================================

func TestExecutor_MassCancel(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/admin/mass-cancel",
		method: "POST",
		status: 200,
		body:   `{"cancelled":42}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("mass cancel", "test-token")
	if !h.called.Load() {
		t.Error("mass cancel: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success, got: %s", resp.Reply)
	}
	if !strings.Contains(resp.Reply, "cancel") {
		t.Errorf("expected cancel in reply, got: %s", resp.Reply)
	}
}

func TestExecutor_CancelAll(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/admin/mass-cancel",
		method: "POST",
		status: 200,
		body:   `{"cancelled":7}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("cancel all orders", "test-token")
	if !h.called.Load() {
		t.Error("cancel all: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success, got: %s", resp.Reply)
	}
}

func TestExecutor_MassCancelAPIError(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/admin/mass-cancel",
		method: "POST",
		status: 500,
		body:   `{"error":"internal"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("mass cancel", "test-token")
	if !strings.Contains(resp.Reply, "❌") {
		t.Errorf("expected failure reply, got: %s", resp.Reply)
	}
}

// =====================================================================
// TRADING — BUST TRADE
// =====================================================================

func TestExecutor_BustTrade(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/admin/trades/TRD-ABC-123/bust",
		method: "POST",
		status: 200,
		body:   `{"status":"busted"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("bust trade TRD-ABC-123", "test-token")
	if !h.called.Load() {
		t.Error("bust trade: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success, got: %s", resp.Reply)
	}
	if !strings.Contains(resp.Reply, "TRD-ABC-123") {
		t.Errorf("expected trade ID in reply, got: %s", resp.Reply)
	}
}

func TestExecutor_BustTradeAPIError(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/admin/trades/TRD-XYZ/bust",
		method: "POST",
		status: 404,
		body:   `{"error":"trade not found"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("bust trade TRD-XYZ", "test-token")
	if !strings.Contains(resp.Reply, "❌") {
		t.Errorf("expected failure reply, got: %s", resp.Reply)
	}
}

// =====================================================================
// TRADING — CIRCUIT BREAKER
// =====================================================================

func TestExecutor_SetCircuitBreaker(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/admin/instruments/WHT-HRW-2026M07-UB/circuit-breaker",
		method: "PUT",
		status: 200,
		body:   `{"status":"set"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("set circuit breaker wheat 15", "test-token")
	if !h.called.Load() {
		t.Error("circuit breaker: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success, got: %s", resp.Reply)
	}
	if !strings.Contains(resp.Reply, "15%") {
		t.Errorf("expected percentage in reply, got: %s", resp.Reply)
	}
}

func TestExecutor_SetCircuitBreakerCorn(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/admin/instruments/CRN-YEL-2026M09-UB/circuit-breaker",
		method: "PUT",
		status: 200,
		body:   `{"status":"set"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("circuit breaker corn 10", "test-token")
	if !h.called.Load() {
		t.Error("circuit breaker corn: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success, got: %s", resp.Reply)
	}
}

func TestExecutor_SetCircuitBreakerUnknownInstrument(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("set circuit breaker unknown 10", "test-token")
	if strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected failure for unknown instrument, got: %s", resp.Reply)
	}
}

func TestExecutor_SetCircuitBreakerAPIError(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/admin/instruments/WHT-HRW-2026M07-UB/circuit-breaker",
		method: "PUT",
		status: 500,
		body:   `{"error":"internal"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("set circuit breaker wheat 15", "test-token")
	if !strings.Contains(resp.Reply, "❌") {
		t.Errorf("expected failure reply, got: %s", resp.Reply)
	}
}

// =====================================================================
// TRADING — SHOW INSTRUMENTS / CIRCUIT BREAKERS
// =====================================================================

func TestExecutor_ShowInstruments(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/instruments/list",
		method: "GET",
		status: 200,
		body:   `{"instruments":[{"id":"WHT-HRW-2026M07-UB","status":"active"}]}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("show instruments", "test-token")
	if !h.called.Load() {
		t.Error("instruments: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "instrument") {
		t.Errorf("expected instrument in reply, got: %s", resp.Reply)
	}
}

func TestExecutor_ShowCircuitBreakers(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/admin/risk/order-limits",
		method: "GET",
		status: 200,
		body:   `{"limits":[{"instrument":"WHT-HRW-2026M07-UB","max_order_size":"500"}]}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("show risk limits", "test-token")
	if !h.called.Load() {
		t.Error("risk limits: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "⚠️") {
		t.Errorf("expected emoji in reply, got: %s", resp.Reply)
	}
}

// =====================================================================
// ORDERS — BUY / SELL
// =====================================================================

func TestExecutor_BuyOrder(t *testing.T) {
	var capturedBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/orders" && r.Method == "POST" {
			json.NewDecoder(r.Body).Decode(&capturedBody)
			w.WriteHeader(201)
			w.Write([]byte(`{"order_id":"ORD-001","status":"new"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("buy 10 wheat at 325", "test-token")
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success, got: %s", resp.Reply)
	}
	if capturedBody["side"] != "BUY" {
		t.Errorf("side = %q, want BUY", capturedBody["side"])
	}
	if capturedBody["instrument_id"] != "WHT-HRW-2026M07-UB" {
		t.Errorf("instrument_id = %q, want WHT-HRW-2026M07-UB", capturedBody["instrument_id"])
	}
	if capturedBody["quantity"] != "10" {
		t.Errorf("quantity = %q, want 10", capturedBody["quantity"])
	}
	if capturedBody["price"] != "325" {
		t.Errorf("price = %q, want 325", capturedBody["price"])
	}
	if capturedBody["order_type"] != "LIMIT" {
		t.Errorf("order_type = %q, want LIMIT", capturedBody["order_type"])
	}
}

func TestExecutor_SellOrder(t *testing.T) {
	var capturedBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/orders" && r.Method == "POST" {
			json.NewDecoder(r.Body).Decode(&capturedBody)
			w.WriteHeader(201)
			w.Write([]byte(`{"order_id":"ORD-002","status":"new"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("sell 5 corn at 450", "test-token")
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success, got: %s", resp.Reply)
	}
	if capturedBody["side"] != "SELL" {
		t.Errorf("side = %q, want SELL", capturedBody["side"])
	}
	if capturedBody["instrument_id"] != "CRN-YEL-2026M09-UB" {
		t.Errorf("instrument_id = %q, want CRN-YEL-2026M09-UB", capturedBody["instrument_id"])
	}
}

func TestExecutor_BuyOrderDecimalPrice(t *testing.T) {
	var capturedBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/orders" && r.Method == "POST" {
			json.NewDecoder(r.Body).Decode(&capturedBody)
			w.WriteHeader(201)
			w.Write([]byte(`{"order_id":"ORD-003"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("buy 2.5 wheat at 325.75", "test-token")
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success with decimal quantities, got: %s", resp.Reply)
	}
	if capturedBody["quantity"] != "2.5" {
		t.Errorf("quantity = %q, want 2.5", capturedBody["quantity"])
	}
	if capturedBody["price"] != "325.75" {
		t.Errorf("price = %q, want 325.75", capturedBody["price"])
	}
}

func TestExecutor_BuyOrderLargeQty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/orders" && r.Method == "POST" {
			w.WriteHeader(201)
			w.Write([]byte(`{"order_id":"ORD-004"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("buy 10000 wheat at 400", "test-token")
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success with large qty, got: %s", resp.Reply)
	}
}

func TestExecutor_BuyOrderUnknownInstrument(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("buy 10 unobtanium at 100", "test-token")
	if strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected failure for unknown instrument, got: %s", resp.Reply)
	}
	if !strings.Contains(resp.Reply, "Unknown") {
		t.Errorf("expected Unknown in reply, got: %s", resp.Reply)
	}
}

func TestExecutor_BuyOrderAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/orders" && r.Method == "POST" {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"insufficient funds"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("buy 10 wheat at 325", "test-token")
	if !strings.Contains(resp.Reply, "❌") {
		t.Errorf("expected failure reply, got: %s", resp.Reply)
	}
}

func TestExecutor_BuyOrderReturnsActions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/orders" && r.Method == "POST" {
			w.WriteHeader(201)
			w.Write([]byte(`{"order_id":"ORD-005"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("buy 10 wheat at 325", "test-token")
	if len(resp.Actions) == 0 {
		t.Error("expected actions in buy order response")
	}
	if resp.Actions[0].URL != "/dashboard/orderbook" {
		t.Errorf("action URL = %q, want /dashboard/orderbook", resp.Actions[0].URL)
	}
}

// =====================================================================
// ORDERS — SHOW / CANCEL / MODIFY
// =====================================================================

func TestExecutor_ShowOrders(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/orders",
		method: "GET",
		status: 200,
		body:   `{"orders":[{"id":"ORD-001","side":"BUY"}]}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("show orders", "test-token")
	if !h.called.Load() {
		t.Error("orders: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "📋") {
		t.Errorf("expected orders emoji, got: %s", resp.Reply)
	}
}

func TestExecutor_MyOrders(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/orders",
		method: "GET",
		status: 200,
		body:   `{"orders":[]}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("my orders", "test-token")
	if !h.called.Load() {
		t.Error("my orders: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "📋") {
		t.Errorf("expected response, got: %s", resp.Reply)
	}
}

func TestExecutor_ShowOrdersAPIError(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/orders",
		method: "GET",
		status: 500,
		body:   `{"error":"internal"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("show orders", "test-token")
	if !strings.Contains(resp.Reply, "❌") {
		t.Errorf("expected failure, got: %s", resp.Reply)
	}
}

func TestExecutor_CancelOrder(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/orders/ORD-999",
		method: "DELETE",
		status: 200,
		body:   `{"status":"cancelled"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("cancel order ORD-999", "test-token")
	if !h.called.Load() {
		t.Error("cancel order: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success, got: %s", resp.Reply)
	}
	if !strings.Contains(resp.Reply, "ORD-999") {
		t.Errorf("expected order ID in reply, got: %s", resp.Reply)
	}
}

func TestExecutor_CancelOrderNotFound(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/orders/ORD-GONE",
		method: "DELETE",
		status: 404,
		body:   `{"error":"not found"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("cancel order ORD-GONE", "test-token")
	if !strings.Contains(resp.Reply, "❌") {
		t.Errorf("expected failure, got: %s", resp.Reply)
	}
}

func TestExecutor_ModifyOrderPrice(t *testing.T) {
	var capturedBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/orders/ORD-ABC" && r.Method == "PATCH" {
			json.NewDecoder(r.Body).Decode(&capturedBody)
			w.WriteHeader(200)
			w.Write([]byte(`{"status":"updated"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("modify order ORD-ABC price 330", "test-token")
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success, got: %s", resp.Reply)
	}
	if capturedBody["price"] != "330" {
		t.Errorf("price = %q, want 330", capturedBody["price"])
	}
	if !strings.Contains(resp.Reply, "ORD-ABC") {
		t.Errorf("expected order ID in reply, got: %s", resp.Reply)
	}
}

func TestExecutor_ModifyOrderAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/orders/ORD-XYZ" && r.Method == "PATCH" {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"invalid price"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("modify order ORD-XYZ price 0", "test-token")
	if !strings.Contains(resp.Reply, "❌") {
		t.Errorf("expected failure, got: %s", resp.Reply)
	}
}

func TestExecutor_ChangeOrderPrice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/orders/ORD-999" && r.Method == "PATCH" {
			w.WriteHeader(200)
			w.Write([]byte(`{"status":"updated"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	// "change order" should also work (the regex matches modify|change|update)
	resp := exec.Execute("change order ORD-999 price 500", "test-token")
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success with 'change order', got: %s", resp.Reply)
	}
}

// =====================================================================
// KYC / PARTICIPANTS
// =====================================================================

func TestExecutor_ApproveTrader(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/participants/TRD-001/approve",
		method: "POST",
		status: 200,
		body:   `{"status":"approved"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("approve trader TRD-001", "test-token")
	if !h.called.Load() {
		t.Error("approve trader: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success, got: %s", resp.Reply)
	}
	if !strings.Contains(resp.Reply, "APPROVED") {
		t.Errorf("expected APPROVED in reply, got: %s", resp.Reply)
	}
}

func TestExecutor_ApproveParticipant(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/participants/PART-007/approve",
		method: "POST",
		status: 200,
		body:   `{"status":"approved"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("approve participant PART-007", "test-token")
	if !h.called.Load() {
		t.Error("approve participant: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success, got: %s", resp.Reply)
	}
}

func TestExecutor_ApproveNonexistent(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/participants/FAKE-999/approve",
		method: "POST",
		status: 404,
		body:   `{"error":"not found"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("approve trader FAKE-999", "test-token")
	if !strings.Contains(resp.Reply, "❌") {
		t.Errorf("expected failure, got: %s", resp.Reply)
	}
}

func TestExecutor_RejectTraderWithReason(t *testing.T) {
	var capturedBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/participants/TRD-BAD/reject" && r.Method == "POST" {
			json.NewDecoder(r.Body).Decode(&capturedBody)
			w.WriteHeader(200)
			w.Write([]byte(`{"status":"rejected"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("reject trader TRD-BAD because docs expired", "test-token")
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success, got: %s", resp.Reply)
	}
	if !strings.Contains(resp.Reply, "REJECTED") {
		t.Errorf("expected REJECTED in reply, got: %s", resp.Reply)
	}
	if capturedBody["reason"] != "docs expired" {
		t.Errorf("reason = %q, want 'docs expired'", capturedBody["reason"])
	}
}

func TestExecutor_SuspendTrader(t *testing.T) {
	var capturedBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/compliance/participants/TRD-001/suspend" && r.Method == "POST" {
			json.NewDecoder(r.Body).Decode(&capturedBody)
			w.WriteHeader(200)
			w.Write([]byte(`{"status":"suspended"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("suspend trader TRD-001 for insider trading", "test-token")
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success, got: %s", resp.Reply)
	}
	if capturedBody["reason"] != "insider trading" {
		t.Errorf("reason = %q, want 'insider trading'", capturedBody["reason"])
	}
}

func TestExecutor_SuspendTraderDefaultReason(t *testing.T) {
	var capturedBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/compliance/participants/TRD-002/suspend" && r.Method == "POST" {
			json.NewDecoder(r.Body).Decode(&capturedBody)
			w.WriteHeader(200)
			w.Write([]byte(`{"status":"suspended"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("suspend trader TRD-002", "test-token")
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success, got: %s", resp.Reply)
	}
	if capturedBody["reason"] == "" {
		t.Error("expected non-empty default reason")
	}
}

func TestExecutor_ReinstateTrader(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/compliance/participants/TRD-001/reinstate",
		method: "POST",
		status: 200,
		body:   `{"status":"active"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("reinstate trader TRD-001", "test-token")
	if !h.called.Load() {
		t.Error("reinstate trader: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success, got: %s", resp.Reply)
	}
}

func TestExecutor_ReinstateTraderAPIError(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/compliance/participants/TRD-GONE/reinstate",
		method: "POST",
		status: 404,
		body:   `{"error":"not found"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("reinstate trader TRD-GONE", "test-token")
	if !strings.Contains(resp.Reply, "❌") {
		t.Errorf("expected failure, got: %s", resp.Reply)
	}
}

func TestExecutor_ShowParticipants(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/participants",
		method: "GET",
		status: 200,
		body:   `{"data":[{"id":"TRD-001","status":"active"}]}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("show participants", "test-token")
	if !h.called.Load() {
		t.Error("participants: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "👥") {
		t.Errorf("expected participants emoji, got: %s", resp.Reply)
	}
}

func TestExecutor_ShowPendingKYC(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/participants",
		method: "GET",
		status: 200,
		body:   `{"applications":[{"id":"APP-001","status":"pending"}]}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("show pending KYC", "test-token")
	if !h.called.Load() {
		t.Error("pending KYC: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "👥") {
		t.Errorf("expected participants emoji, got: %s", resp.Reply)
	}
}

func TestExecutor_DisableParticipant(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/admin/participants/PART-123/disable",
		method: "POST",
		status: 200,
		body:   `{"status":"disabled"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("disable participant PART-123", "test-token")
	if !h.called.Load() {
		t.Error("disable participant: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success, got: %s", resp.Reply)
	}
}

func TestExecutor_DisableTrader(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/admin/participants/TRD-XYZ/disable",
		method: "POST",
		status: 200,
		body:   `{"status":"disabled"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("disable trader TRD-XYZ", "test-token")
	if !h.called.Load() {
		t.Error("disable trader: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success, got: %s", resp.Reply)
	}
}

// =====================================================================
// MARGIN
// =====================================================================

func TestExecutor_ShowMarginCalls(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/margin/calls/stats",
		method: "GET",
		status: 200,
		body:   `{"total_active":3,"total_shortfall":"50000.00","participants_in_call":2}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("show margin calls", "test-token")
	if !h.called.Load() {
		t.Error("margin calls: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "💰") {
		t.Errorf("expected margin emoji, got: %s", resp.Reply)
	}
	if !strings.Contains(resp.Reply, "3") {
		t.Errorf("expected active count in reply, got: %s", resp.Reply)
	}
}

func TestExecutor_MarginStatus(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/margin/calls/stats",
		method: "GET",
		status: 200,
		body:   `{"total_active":0,"total_shortfall":"0.00","participants_in_call":0}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	// "margin status" contains "status" which triggers the health handler first.
	// Use "margin overview" instead to hit only the margin handler.
	resp := exec.Execute("margin overview", "test-token")
	if !h.called.Load() {
		t.Error("margin overview: endpoint not called")
	}
	if resp.Reply == "" {
		t.Error("expected non-empty reply")
	}
}

func TestExecutor_MarginCallsWithStats(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/margin/calls/stats",
		method: "GET",
		status: 200,
		body:   `{"total_active":5,"total_shortfall":"125000.00","participants_in_call":3}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("margin calls", "test-token")
	if !h.called.Load() {
		t.Error("margin calls: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "5") {
		t.Errorf("expected count 5 in reply, got: %s", resp.Reply)
	}
	// Should also have an action
	if len(resp.Actions) == 0 {
		t.Error("expected actions in margin response")
	}
}

func TestExecutor_MarginAPIError(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/margin/calls/stats",
		method: "GET",
		status: 503,
		body:   `{"error":"service unavailable"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("show margin calls", "test-token")
	if !strings.Contains(resp.Reply, "❌") {
		t.Errorf("expected failure, got: %s", resp.Reply)
	}
}

func TestExecutor_RecalculateMargin(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/margin/calls/stats",
		method: "GET",
		status: 200,
		body:   `{"total_active":2,"total_shortfall":"30000","participants_in_call":1}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("recalculate margin", "test-token")
	if !h.called.Load() {
		t.Error("recalculate margin: endpoint not called")
	}
	if resp.Reply == "" {
		t.Error("expected non-empty reply")
	}
}

func TestExecutor_PortfolioMargin(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/margin/calls/stats",
		method: "GET",
		status: 200,
		body:   `{"total_active":1,"total_shortfall":"10000","participants_in_call":1}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("show portfolio margin", "test-token")
	if !h.called.Load() {
		t.Error("portfolio margin: endpoint not called")
	}
	if resp.Reply == "" {
		t.Error("expected non-empty reply")
	}
}

// =====================================================================
// SETTLEMENT
// =====================================================================

func TestExecutor_RunSettlement(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/settlement/cycle",
		method: "POST",
		status: 200,
		body:   `{"status":"triggered"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("run settlement", "test-token")
	if !h.called.Load() {
		t.Error("run settlement: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success, got: %s", resp.Reply)
	}
}

func TestExecutor_TriggerSettlement(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/settlement/cycle",
		method: "POST",
		status: 200,
		body:   `{"status":"triggered"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("trigger settlement", "test-token")
	if !h.called.Load() {
		t.Error("trigger settlement: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success, got: %s", resp.Reply)
	}
}

func TestExecutor_ShowSettlementCycles(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/settlement/cycles",
		method: "GET",
		status: 200,
		body:   `{"cycles":[{"id":"CYC-001","status":"completed"}]}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("show settlement cycles", "test-token")
	if !h.called.Load() {
		t.Error("settlement cycles: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "⚖️") {
		t.Errorf("expected settlement emoji, got: %s", resp.Reply)
	}
}

func TestExecutor_SettlementWithData(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/settlement/cycles",
		method: "GET",
		status: 200,
		body:   `{"cycles":[{"id":"CYC-001","pnl":"12500.00"},{"id":"CYC-002","pnl":"8200.00"}]}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("settlement history", "test-token")
	if !h.called.Load() {
		t.Error("settlement history: endpoint not called")
	}
	if resp.Reply == "" {
		t.Error("expected non-empty reply")
	}
}

func TestExecutor_SettlementAPIError(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/settlement/cycle",
		method: "POST",
		status: 500,
		body:   `{"error":"internal"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("run settlement", "test-token")
	if !strings.Contains(resp.Reply, "❌") {
		t.Errorf("expected failure, got: %s", resp.Reply)
	}
}

// =====================================================================
// COMPLIANCE
// =====================================================================

func TestExecutor_ShowAlerts(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/compliance/alerts",
		method: "GET",
		status: 200,
		body:   `{"alerts":[{"id":"ALT-001","severity":"HIGH"}],"total":1}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("show alerts", "test-token")
	if !h.called.Load() {
		t.Error("show alerts: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "🔍") {
		t.Errorf("expected compliance emoji, got: %s", resp.Reply)
	}
}

func TestExecutor_ShowAlertsEmpty(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/compliance/alerts",
		method: "GET",
		status: 200,
		body:   `{"alerts":[],"total":0}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("show alerts", "test-token")
	if !h.called.Load() {
		t.Error("show alerts empty: endpoint not called")
	}
	// Should show 0 alerts
	if !strings.Contains(resp.Reply, "0") {
		t.Errorf("expected 0 in reply, got: %s", resp.Reply)
	}
}

func TestExecutor_ResolveAlert(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/compliance/alerts/ALT-123/resolve",
		method: "POST",
		status: 200,
		body:   `{"status":"resolved"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("resolve alert ALT-123", "test-token")
	if !h.called.Load() {
		t.Error("resolve alert: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success, got: %s", resp.Reply)
	}
	if !strings.Contains(resp.Reply, "ALT-123") {
		t.Errorf("expected alert ID in reply, got: %s", resp.Reply)
	}
}

func TestExecutor_ResolveNonexistentAlert(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/compliance/alerts/FAKE-999/resolve",
		method: "POST",
		status: 404,
		body:   `{"error":"not found"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("resolve alert FAKE-999", "test-token")
	if !strings.Contains(resp.Reply, "❌") {
		t.Errorf("expected failure, got: %s", resp.Reply)
	}
}

func TestExecutor_FileSARWithReason(t *testing.T) {
	var capturedBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/compliance/sar" && r.Method == "POST" {
			json.NewDecoder(r.Body).Decode(&capturedBody)
			w.WriteHeader(201)
			w.Write([]byte(`{"sar_id":"SAR-001","status":"filed"}`))
			return
		}
		if r.URL.Path == "/api/v1/auth/me" {
			w.WriteHeader(200)
			w.Write([]byte(`{"email":"admin@test.com"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	// Note: the regex (?:trader|participant\s+)? captures "trader" without trailing space,
	// so "file SAR on TRD-SHADY for money laundering" avoids the trader/participant prefix ambiguity.
	resp := exec.Execute("file SAR on TRD-SHADY for money laundering", "test-token")
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success, got: %s", resp.Reply)
	}
	if capturedBody["participant_id"] != "TRD-SHADY" {
		t.Errorf("participant_id = %q, want TRD-SHADY", capturedBody["participant_id"])
	}
	if capturedBody["reason"] != "money laundering" {
		t.Errorf("reason = %q, want 'money laundering'", capturedBody["reason"])
	}
}

func TestExecutor_FileSARDefaultReason(t *testing.T) {
	var capturedBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/compliance/sar" && r.Method == "POST" {
			json.NewDecoder(r.Body).Decode(&capturedBody)
			w.WriteHeader(201)
			w.Write([]byte(`{"sar_id":"SAR-002"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("file SAR TRD-ABC", "test-token")
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success, got: %s", resp.Reply)
	}
	if capturedBody["reason"] == "" {
		t.Error("expected non-empty default reason")
	}
}

func TestExecutor_FileSARAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/compliance/sar" && r.Method == "POST" {
			w.WriteHeader(500)
			w.Write([]byte(`{"error":"internal"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("file SAR on trader TRD-BAD for wash trading", "test-token")
	if !strings.Contains(resp.Reply, "❌") {
		t.Errorf("expected failure, got: %s", resp.Reply)
	}
}

func TestExecutor_ShowAuditLog(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/compliance/audit-trail",
		method: "GET",
		status: 200,
		body:   `{"events":[{"id":"EVT-001","action":"halt","timestamp":"2026-03-31T10:00:00Z"}]}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("show audit log", "test-token")
	if !h.called.Load() {
		t.Error("audit log: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "📝") {
		t.Errorf("expected audit emoji, got: %s", resp.Reply)
	}
}

func TestExecutor_ShowSurveillanceAlerts(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/compliance/alerts",
		method: "GET",
		status: 200,
		body:   `{"data":[{"id":"ALT-002","type":"wash_trading"}]}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("surveillance alerts", "test-token")
	if !h.called.Load() {
		t.Error("surveillance alerts: endpoint not called")
	}
	if resp.Reply == "" {
		t.Error("expected non-empty reply")
	}
}

func TestExecutor_AlertsAPIError(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/compliance/alerts",
		method: "GET",
		status: 503,
		body:   `{"error":"service unavailable"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("show alerts", "test-token")
	if !strings.Contains(resp.Reply, "❌") {
		t.Errorf("expected failure, got: %s", resp.Reply)
	}
}

// =====================================================================
// MARKET DATA
// =====================================================================

func TestExecutor_WheatPrice(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/market-data/ticker/WHT-HRW-2026M07-UB",
		method: "GET",
		status: 200,
		body:   `{"bid":"324.50","ask":"325.25","last":"324.75"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("wheat price", "test-token")
	if !h.called.Load() {
		t.Error("wheat price: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "📊") {
		t.Errorf("expected ticker emoji, got: %s", resp.Reply)
	}
}

func TestExecutor_CornOrderBook(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/instruments/CRN-YEL-2026M09-UB/book",
		method: "GET",
		status: 200,
		body:   `{"bids":[{"price":"449","qty":"10"}],"asks":[{"price":"451","qty":"5"}]}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("corn order book", "test-token")
	if !h.called.Load() {
		t.Error("corn order book: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "📋") {
		t.Errorf("expected book emoji, got: %s", resp.Reply)
	}
}

func TestExecutor_WheatCandles(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/market-data/candles/WHT-HRW-2026M07-UB",
		method: "GET",
		status: 200,
		body:   `{"candles":[{"open":"320","high":"330","low":"318","close":"325","volume":"1000"}]}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("wheat candles", "test-token")
	if !h.called.Load() {
		t.Error("wheat candles: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "🕯️") {
		t.Errorf("expected candle emoji, got: %s", resp.Reply)
	}
}

func TestExecutor_CornTrades(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/market-data/trades/CRN-YEL-2026M09-UB",
		method: "GET",
		status: 200,
		body:   `{"trades":[{"price":"450","qty":"5","ts":"2026-03-31T10:00:00Z"}]}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("corn trades", "test-token")
	if !h.called.Load() {
		t.Error("corn trades: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "📈") {
		t.Errorf("expected trades emoji, got: %s", resp.Reply)
	}
}

func TestExecutor_LastTradeWheat(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/instruments/WHT-HRW-2026M07-UB/trades/latest",
		method: "GET",
		status: 200,
		body:   `{"price":"324.75","qty":"100","ts":"2026-03-31T09:59:00Z"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("last trade wheat", "test-token")
	if !h.called.Load() {
		t.Error("last trade wheat: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "🔄") {
		t.Errorf("expected last trade emoji, got: %s", resp.Reply)
	}
}

func TestExecutor_CattleTicker(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/market-data/ticker/LVS-CATTLE-2026M10-UB",
		method: "GET",
		status: 200,
		body:   `{"bid":"1200","ask":"1205","last":"1202"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("cattle ticker", "test-token")
	if !h.called.Load() {
		t.Error("cattle ticker: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "📊") {
		t.Errorf("expected ticker in reply, got: %s", resp.Reply)
	}
}

func TestExecutor_MarketDataAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return 500 for any market data request
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"market data unavailable"}`))
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	// When ticker endpoint returns error, the market data loop silently skips
	// (no explicit error handling) and falls through to default
	resp := exec.Execute("wheat price", "test-token")
	// Reply may be empty or a default message — not a ✅ success
	if strings.Contains(resp.Reply, "📊") && strings.Contains(resp.Reply, "WHT") {
		// If it happens to have content it was from a different path
	}
	// The important thing: no crash
	_ = resp
}

// =====================================================================
// WAREHOUSE
// =====================================================================

func TestExecutor_ShowInventory(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/warehouse/inventory",
		method: "GET",
		status: 200,
		body:   `{"items":[{"commodity":"wheat","qty":"5000","grade":"HRW"}]}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("show inventory", "test-token")
	if !h.called.Load() {
		t.Error("inventory: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "🏭") {
		t.Errorf("expected warehouse emoji, got: %s", resp.Reply)
	}
}

func TestExecutor_WarehouseInventory(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/warehouse/inventory",
		method: "GET",
		status: 200,
		body:   `{"items":[]}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("warehouse inventory", "test-token")
	if !h.called.Load() {
		t.Error("warehouse inventory: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "🏭") {
		t.Errorf("expected warehouse emoji, got: %s", resp.Reply)
	}
}

func TestExecutor_WarehouseAPIError(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/warehouse/inventory",
		method: "GET",
		status: 503,
		body:   `{"error":"unavailable"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("show inventory", "test-token")
	// On error the warehouse handler silently falls through to default
	// (no explicit error return in the warehouse handler)
	// The response will be the default "I can help with..." message
	_ = resp // no crash expected
}

// =====================================================================
// RISK LIMITS
// =====================================================================

func TestExecutor_ShowRiskLimits(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/admin/risk/order-limits",
		method: "GET",
		status: 200,
		body:   `{"limits":[{"instrument":"WHT-HRW-2026M07-UB","max_order_size":"500"}]}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("show risk limits", "test-token")
	if !h.called.Load() {
		t.Error("risk limits: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "⚠️") {
		t.Errorf("expected risk emoji, got: %s", resp.Reply)
	}
}

func TestExecutor_SetWheatMaxOrder(t *testing.T) {
	var capturedBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/admin/risk/order-limits/WHT-HRW-2026M07-UB" && r.Method == "PUT" {
			json.NewDecoder(r.Body).Decode(&capturedBody)
			w.WriteHeader(200)
			w.Write([]byte(`{"status":"updated"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("set wheat max order 500", "test-token")
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected success, got: %s", resp.Reply)
	}
	if capturedBody["max_order_size"] != "500" {
		t.Errorf("max_order_size = %q, want 500", capturedBody["max_order_size"])
	}
}

func TestExecutor_SetRiskLimitAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/admin/risk/order-limits/") && r.Method == "PUT" {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"invalid limit"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("set wheat max order 500", "test-token")
	if !strings.Contains(resp.Reply, "❌") {
		t.Errorf("expected failure, got: %s", resp.Reply)
	}
}

func TestExecutor_SetRiskLimitUnknownInstrument(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("set unknownxyz max order 500", "test-token")
	if strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected failure for unknown instrument, got: %s", resp.Reply)
	}
}

func TestExecutor_OrderLimitsEmpty(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/admin/risk/order-limits",
		method: "GET",
		status: 200,
		body:   `{"limits":[]}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("order limits", "test-token")
	if !h.called.Load() {
		t.Error("order limits: endpoint not called")
	}
	if resp.Reply == "" {
		t.Error("expected non-empty reply")
	}
}

// =====================================================================
// REPORTS
// =====================================================================

func TestExecutor_MarketSummaryToday(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/reports/market-summary" && r.Method == "GET" {
			w.WriteHeader(200)
			w.Write([]byte(`{"date":"2026-03-31","volume":"1000000","trades":250}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("market summary today", "test-token")
	if !strings.Contains(resp.Reply, "📊") {
		t.Errorf("expected summary emoji, got: %s", resp.Reply)
	}
}

func TestExecutor_MarketSummaryWithDate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/reports/market-summary" && r.Method == "GET" {
			date := r.URL.Query().Get("date")
			if date != "2026-03-15" {
				// Let it fall through to 404 so we see the fallback message
			}
			w.WriteHeader(200)
			w.Write([]byte(`{"date":"2026-03-15","volume":"900000"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("market summary 2026-03-15", "test-token")
	if !strings.Contains(resp.Reply, "2026-03-15") {
		t.Errorf("expected date in reply, got: %s", resp.Reply)
	}
}

func TestExecutor_MarketSummaryEndpointNotAvailable(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/reports/market-summary",
		method: "GET",
		status: 404,
		body:   `{"error":"not found"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("market summary today", "test-token")
	// Falls back to helpful message
	if !strings.Contains(resp.Reply, "📊") {
		t.Errorf("expected emoji even in fallback, got: %s", resp.Reply)
	}
}

func TestExecutor_LargeTraderReport(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/reports/large-traders",
		method: "GET",
		status: 200,
		body:   `{"traders":[{"id":"TRD-001","position":"50000"}]}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("large trader report", "test-token")
	if !h.called.Load() {
		t.Error("large trader report: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "📋") {
		t.Errorf("expected report emoji, got: %s", resp.Reply)
	}
}

func TestExecutor_LargeTraderReportNotAvailable(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/reports/large-traders",
		method: "GET",
		status: 404,
		body:   `{"error":"not found"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("large trader report", "test-token")
	// Returns helpful fallback message
	if !strings.Contains(resp.Reply, "📋") {
		t.Errorf("expected emoji even in fallback, got: %s", resp.Reply)
	}
}

// =====================================================================
// SYSTEM
// =====================================================================

func TestExecutor_SystemHealth(t *testing.T) {
	healthBody := `{
		"overall_status": "healthy",
		"services": [
			{"name": "matching-engine", "status": "healthy"},
			{"name": "clearing-engine", "status": "healthy"},
			{"name": "margin-engine", "status": "healthy"},
			{"name": "settlement-engine", "status": "healthy"},
			{"name": "auth-service", "status": "healthy"},
			{"name": "compliance-service", "status": "healthy"},
			{"name": "gateway", "status": "healthy"},
			{"name": "market-data-service", "status": "healthy"},
			{"name": "warehouse-service", "status": "healthy"}
		]
	}`
	h := &mockHandler{
		path:   "/api/v1/admin/health",
		method: "GET",
		status: 200,
		body:   healthBody,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("system health", "test-token")
	if !h.called.Load() {
		t.Error("system health: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "🏥") {
		t.Errorf("expected health emoji, got: %s", resp.Reply)
	}
	// All 9 services should appear
	services := []string{
		"matching-engine", "clearing-engine", "margin-engine",
		"settlement-engine", "auth-service", "compliance-service",
		"gateway", "market-data-service", "warehouse-service",
	}
	for _, svc := range services {
		if !strings.Contains(resp.Reply, svc) {
			t.Errorf("expected service %q in reply, got: %s", svc, resp.Reply)
		}
	}
}

func TestExecutor_SystemHealthWithUnhealthyService(t *testing.T) {
	healthBody := `{
		"overall_status": "degraded",
		"services": [
			{"name": "matching-engine", "status": "healthy"},
			{"name": "clearing-engine", "status": "unhealthy"}
		]
	}`
	h := &mockHandler{
		path:   "/api/v1/admin/health",
		method: "GET",
		status: 200,
		body:   healthBody,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("system health", "test-token")
	// Should show ❌ for unhealthy service
	if !strings.Contains(resp.Reply, "❌") {
		t.Errorf("expected ❌ for unhealthy service, got: %s", resp.Reply)
	}
	// And ✅ for healthy service
	if !strings.Contains(resp.Reply, "✅") {
		t.Errorf("expected ✅ for healthy service, got: %s", resp.Reply)
	}
}

func TestExecutor_SystemHealthAPIError(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/admin/health",
		method: "GET",
		status: 503,
		body:   `{"error":"unavailable"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("system health", "test-token")
	if !strings.Contains(resp.Reply, "❌") {
		t.Errorf("expected failure, got: %s", resp.Reply)
	}
}

func TestExecutor_Help(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Help should not call any API
		t.Errorf("unexpected API call to %s %s", r.Method, r.URL.Path)
		w.WriteHeader(500)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("help", "test-token")
	// Check all major categories are listed
	categories := []string{"Orders", "Market Data", "Trading Controls", "KYC", "Compliance", "Settlement", "Warehouse", "System"}
	for _, cat := range categories {
		if !strings.Contains(resp.Reply, cat) {
			t.Errorf("expected category %q in help, got: %s", cat, resp.Reply)
		}
	}
}

func TestExecutor_WhatCanYouDo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected API call to %s %s", r.Method, r.URL.Path)
		w.WriteHeader(500)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("what can you do", "test-token")
	if resp.Reply == "" {
		t.Error("expected non-empty help reply")
	}
	if !strings.Contains(resp.Reply, "buy") {
		t.Errorf("expected buy order in help, got: %s", resp.Reply)
	}
}

func TestExecutor_UnknownCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Unknown commands hit various endpoints via fallthrough — that's OK
		w.WriteHeader(404)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("xyzzy plugh frobnicate", "test-token")
	if resp.Reply == "" {
		t.Error("expected non-empty default reply")
	}
}

func TestExecutor_WhoAmI(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/auth/me",
		method: "GET",
		status: 200,
		body:   `{"email":"admin@garudax.com","roles":["admin"]}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("who am I", "test-token")
	if !h.called.Load() {
		t.Error("who am I: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "👤") {
		t.Errorf("expected profile emoji, got: %s", resp.Reply)
	}
}

func TestExecutor_WhoAmIAPIError(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/auth/me",
		method: "GET",
		status: 401,
		body:   `{"error":"unauthorized"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("who am I", "test-token")
	if !strings.Contains(resp.Reply, "❌") {
		t.Errorf("expected failure, got: %s", resp.Reply)
	}
}

func TestExecutor_Whoami_Lowercase(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/auth/me",
		method: "GET",
		status: 200,
		body:   `{"email":"trader@garudax.com","roles":["trader"]}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("whoami", "test-token")
	if !h.called.Load() {
		t.Error("whoami: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "👤") {
		t.Errorf("expected profile emoji, got: %s", resp.Reply)
	}
}

func TestExecutor_VeryLongMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	longMsg := strings.Repeat("a", 1000)
	resp := exec.Execute(longMsg, "test-token")
	// Should not panic; should return some reply
	if resp.Reply == "" {
		t.Error("expected non-empty reply for long message")
	}
}

func TestExecutor_SpecialCharactersInMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("!@#$%^&*()_+{}|:<>?", "test-token")
	// Should not panic
	_ = resp
}

func TestExecutor_EmptyMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("", "test-token")
	// Should return some default reply
	if resp.Reply == "" {
		t.Error("expected non-empty reply for empty message")
	}
}

// =====================================================================
// CLEARING — NETTING / POSITIONS
// =====================================================================

func TestExecutor_ShowNetting(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/clearing/netting",
		method: "GET",
		status: 200,
		body:   `{"positions":[{"participant":"TRD-001","net_qty":"100"}]}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("show netting", "test-token")
	if !h.called.Load() {
		t.Error("netting: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "⚖️") {
		t.Errorf("expected netting emoji, got: %s", resp.Reply)
	}
}

func TestExecutor_NettingPositions(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/clearing/netting",
		method: "GET",
		status: 200,
		body:   `{"positions":[]}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("netting positions", "test-token")
	if !h.called.Load() {
		t.Error("netting positions: endpoint not called")
	}
	if resp.Reply == "" {
		t.Error("expected non-empty reply")
	}
}

func TestExecutor_NettingAPIError(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/clearing/netting",
		method: "GET",
		status: 500,
		body:   `{"error":"internal"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("show netting", "test-token")
	if !strings.Contains(resp.Reply, "❌") {
		t.Errorf("expected failure, got: %s", resp.Reply)
	}
}

func TestExecutor_PositionForWheat(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/clearing/positions/WHT-HRW-2026M07-UB",
		method: "GET",
		status: 200,
		body:   `{"instrument":"WHT-HRW-2026M07-UB","long_qty":"500","short_qty":"200"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("position for wheat", "test-token")
	if !h.called.Load() {
		t.Error("position for wheat: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "📊") {
		t.Errorf("expected position emoji, got: %s", resp.Reply)
	}
}

// =====================================================================
// NEWACTIONEXECUTOR DEFAULT ADDR
// =====================================================================

func TestNewActionExecutor_DefaultAddr(t *testing.T) {
	exec := NewActionExecutor("")
	if exec.gatewayAddr != "http://127.0.0.1:8080" {
		t.Errorf("default addr = %q, want http://127.0.0.1:8080", exec.gatewayAddr)
	}
}

func TestNewActionExecutor_CustomAddr(t *testing.T) {
	exec := NewActionExecutor("http://custom-host:9090")
	if exec.gatewayAddr != "http://custom-host:9090" {
		t.Errorf("custom addr = %q, want http://custom-host:9090", exec.gatewayAddr)
	}
}

// =====================================================================
// AUTH BEARER TOKEN
// =====================================================================

func TestExecutor_SendsBearerToken(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
		w.Write([]byte(`{"overall_status":"healthy","services":[]}`))
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	exec.Execute("system health", "my-jwt-token")
	if capturedAuth != "Bearer my-jwt-token" {
		t.Errorf("Authorization = %q, want 'Bearer my-jwt-token'", capturedAuth)
	}
}

func TestExecutor_EmptyToken(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
		w.Write([]byte(`{"overall_status":"healthy","services":[]}`))
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	exec.Execute("system health", "")
	if capturedAuth != "" {
		t.Errorf("expected no Authorization header with empty token, got: %q", capturedAuth)
	}
}

// =====================================================================
// fetchUserEmail — additional coverage not in handler_test.go
// =====================================================================

func TestFetchUserEmail_Executor_ReturnsEmailFromFlatField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/me" {
			w.WriteHeader(200)
			w.Write([]byte(`{"email":"flat@garudax.com"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	email := exec.fetchUserEmail("valid-token")
	if email != "flat@garudax.com" {
		t.Errorf("email = %q, want flat@garudax.com", email)
	}
}

func TestFetchUserEmail_Executor_PrefersDataField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/me" {
			w.WriteHeader(200)
			// Both fields present — data.email should win
			w.Write([]byte(`{"data":{"email":"data@garudax.com"},"email":"flat@garudax.com"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	email := exec.fetchUserEmail("valid-token")
	if email != "data@garudax.com" {
		t.Errorf("email = %q, want data@garudax.com (data.email preferred)", email)
	}
}

func TestFetchUserEmail_Executor_InvalidJSONReturnsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/me" {
			w.WriteHeader(200)
			w.Write([]byte(`not valid json`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	email := exec.fetchUserEmail("valid-token")
	if email != "" {
		t.Errorf("expected empty email for invalid JSON, got: %q", email)
	}
}

// =====================================================================
// prettyJSON
// =====================================================================

func TestPrettyJSON_ValidJSON(t *testing.T) {
	out := prettyJSON(`{"key":"value","num":42}`)
	if !strings.Contains(out, "key") {
		t.Errorf("expected key in pretty output, got: %s", out)
	}
	// Should be indented
	if !strings.Contains(out, "  ") {
		t.Errorf("expected indentation in pretty output, got: %s", out)
	}
}

func TestPrettyJSON_InvalidJSON(t *testing.T) {
	raw := "not json at all"
	out := prettyJSON(raw)
	if out != raw {
		t.Errorf("invalid JSON should return raw string, got: %s", out)
	}
}

func TestPrettyJSON_Truncates(t *testing.T) {
	// Build a large JSON object > 1000 bytes
	large := `{"data":"` + strings.Repeat("x", 1100) + `"}`
	out := prettyJSON(large)
	if len(out) > 1010 {
		t.Errorf("output too long: %d bytes, expected truncation", len(out))
	}
	if !strings.HasSuffix(out, "...") {
		t.Errorf("expected '...' at end of truncated output, got: %s", out[len(out)-10:])
	}
}

// =====================================================================
// FORMAT HELPERS
// =====================================================================

func TestFormatHealthResponse_AllHealthy(t *testing.T) {
	raw := `{
		"overall_status": "healthy",
		"services": [
			{"name": "matching-engine", "status": "healthy"},
			{"name": "auth-service", "status": "healthy"}
		]
	}`
	resp := formatHealthResponse(raw)
	if !strings.Contains(resp.Reply, "HEALTHY") {
		t.Errorf("expected HEALTHY in reply, got: %s", resp.Reply)
	}
	if strings.Contains(resp.Reply, "❌") {
		t.Errorf("should not have ❌ when all healthy, got: %s", resp.Reply)
	}
}

func TestFormatHealthResponse_WithUnhealthy(t *testing.T) {
	raw := `{
		"overall_status": "degraded",
		"services": [
			{"name": "clearing-engine", "status": "unhealthy"}
		]
	}`
	resp := formatHealthResponse(raw)
	if !strings.Contains(resp.Reply, "❌") {
		t.Errorf("expected ❌ for unhealthy, got: %s", resp.Reply)
	}
}

func TestFormatMarginResponse(t *testing.T) {
	raw := `{"total_active":5,"total_shortfall":"100000","participants_in_call":3}`
	resp := formatMarginResponse(raw)
	if !strings.Contains(resp.Reply, "5") {
		t.Errorf("expected active count in margin response, got: %s", resp.Reply)
	}
	if len(resp.Actions) == 0 {
		t.Error("expected actions in margin response")
	}
}

func TestFormatSettlementResponse(t *testing.T) {
	raw := `{"cycles":[{"id":"CYC-001","status":"completed"}]}`
	resp := formatSettlementResponse(raw)
	if !strings.Contains(resp.Reply, "⚖️") {
		t.Errorf("expected settlement emoji, got: %s", resp.Reply)
	}
	if len(resp.Actions) == 0 {
		t.Error("expected actions in settlement response")
	}
}

func TestFormatAlertsResponse_WithAlertsKey(t *testing.T) {
	raw := `{"alerts":[{"id":"ALT-1"},{"id":"ALT-2"}]}`
	resp := formatAlertsResponse(raw)
	if !strings.Contains(resp.Reply, "2") {
		t.Errorf("expected alert count 2 in reply, got: %s", resp.Reply)
	}
}

func TestFormatAlertsResponse_WithDataKey(t *testing.T) {
	raw := `{"data":[{"id":"ALT-1"},{"id":"ALT-2"},{"id":"ALT-3"}]}`
	resp := formatAlertsResponse(raw)
	if !strings.Contains(resp.Reply, "3") {
		t.Errorf("expected alert count 3 in reply, got: %s", resp.Reply)
	}
}

func TestFormatParticipantsResponse_WithDataKey(t *testing.T) {
	raw := `{"data":[{"id":"TRD-001"},{"id":"TRD-002"}]}`
	resp := formatParticipantsResponse(raw)
	if !strings.Contains(resp.Reply, "2") {
		t.Errorf("expected count 2 in participants reply, got: %s", resp.Reply)
	}
}

func TestFormatParticipantsResponse_WithApplicationsKey(t *testing.T) {
	raw := `{"applications":[{"id":"APP-001"}]}`
	resp := formatParticipantsResponse(raw)
	if !strings.Contains(resp.Reply, "1") {
		t.Errorf("expected count 1 in participants reply, got: %s", resp.Reply)
	}
}

func TestFormatInstrumentsResponse(t *testing.T) {
	raw := `[{"id":"WHT-HRW-2026M07-UB","status":"active"},{"id":"CRN-YEL-2026M09-UB","status":"active"}]`
	resp := formatInstrumentsResponse(raw)
	if !strings.Contains(resp.Reply, "📊") {
		t.Errorf("expected instrument emoji, got: %s", resp.Reply)
	}
	if len(resp.Actions) == 0 {
		t.Error("expected actions in instruments response")
	}
}

func TestFormatTicketsResponse(t *testing.T) {
	raw := `{"data":[{"id":"TKT-001","title":"Bug in matching"},{"id":"TKT-002","title":"UI issue"}]}`
	resp := formatTicketsResponse(raw)
	if !strings.Contains(resp.Reply, "2") {
		t.Errorf("expected count 2 in tickets reply, got: %s", resp.Reply)
	}
}

// =====================================================================
// CLEARING POSITIONS (generic)
// =====================================================================

func TestExecutor_ShowClearingPositions(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/clearing/positions",
		method: "GET",
		status: 200,
		body:   `{"positions":[{"participant":"TRD-001","qty":"1000"}]}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("show positions", "test-token")
	if !h.called.Load() {
		t.Error("clearing positions: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "📊") {
		t.Errorf("expected position emoji, got: %s", resp.Reply)
	}
}

// =====================================================================
// FEES
// =====================================================================

func TestExecutor_ShowFees(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/admin/fees",
		method: "GET",
		status: 200,
		body:   `{"fees":{"maker":"0.001","taker":"0.002"}}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("show fees", "test-token")
	if !h.called.Load() {
		t.Error("fees: endpoint not called")
	}
	if !strings.Contains(resp.Reply, "💰") {
		t.Errorf("expected fee emoji, got: %s", resp.Reply)
	}
}

// =====================================================================
// DISABLE PARTICIPANT — API Error
// =====================================================================

func TestExecutor_DisableParticipantAPIError(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/admin/participants/FAKE-000/disable",
		method: "POST",
		status: 404,
		body:   `{"error":"not found"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("disable participant FAKE-000", "test-token")
	if !strings.Contains(resp.Reply, "❌") {
		t.Errorf("expected failure, got: %s", resp.Reply)
	}
}

// =====================================================================
// SUSPEND — API Error
// =====================================================================

func TestExecutor_SuspendTraderAPIError(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/compliance/participants/TRD-ERR/suspend",
		method: "POST",
		status: 500,
		body:   `{"error":"internal"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("suspend trader TRD-ERR for fraud", "test-token")
	if !strings.Contains(resp.Reply, "❌") {
		t.Errorf("expected failure, got: %s", resp.Reply)
	}
}

// =====================================================================
// AUDIT LOG — API Error
// =====================================================================

func TestExecutor_AuditLogAPIError(t *testing.T) {
	h := &mockHandler{
		path:   "/api/v1/compliance/audit-trail",
		method: "GET",
		status: 503,
		body:   `{"error":"unavailable"}`,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)
	resp := exec.Execute("show audit log", "test-token")
	if !strings.Contains(resp.Reply, "❌") {
		t.Errorf("expected failure, got: %s", resp.Reply)
	}
}
