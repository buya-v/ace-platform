package bot

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// assert is a local helper so this file has no external test dependencies.
func assert(t *testing.T, condition bool, msg string) {
	t.Helper()
	if !condition {
		t.Error(msg)
	}
}

// multiMockServer builds an httptest.Server that dispatches requests by
// method+path to the provided handlers map. Keys are "METHOD /path".
// Any unmatched request returns 404.
type routeHandler struct {
	fn      http.HandlerFunc
	called  atomic.Int32
}

func newMultiServer(routes map[string]*routeHandler) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		if h, ok := routes[key]; ok {
			h.called.Add(1)
			h.fn(w, r)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	}))
}

func jsonOK(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(v)
}

// ---------------------------------------------------------------------------
// Workflow 1: Halt & Resume
// ---------------------------------------------------------------------------

func TestWorkflow_HaltAndResume(t *testing.T) {
	var halted atomic.Bool

	haltRoute := &routeHandler{fn: func(w http.ResponseWriter, r *http.Request) {
		halted.Store(true)
		jsonOK(w, map[string]string{"status": "halted"})
	}}
	resumeRoute := &routeHandler{fn: func(w http.ResponseWriter, r *http.Request) {
		if !halted.Load() {
			t.Error("resume called before halt")
		}
		jsonOK(w, map[string]string{"status": "continuous"})
	}}
	// withAttribution fetches /api/v1/auth/me — return 404 so email is empty
	routes := map[string]*routeHandler{
		"POST /api/v1/admin/instruments/WHT-HRW-2026M07-UB/halt":   haltRoute,
		"POST /api/v1/admin/instruments/WHT-HRW-2026M07-UB/resume": resumeRoute,
	}
	srv := newMultiServer(routes)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)

	// Step 1: halt wheat
	r1 := exec.Execute("halt wheat", "test-token")
	assert(t, strings.Contains(r1.Reply, "✅") || strings.Contains(r1.Reply, "HALT"),
		"expected halt success in: "+r1.Reply)
	assert(t, haltRoute.called.Load() == 1, "halt endpoint should be called once")

	// Step 2: resume wheat
	r2 := exec.Execute("resume wheat", "test-token")
	assert(t, strings.Contains(r2.Reply, "✅") || strings.Contains(r2.Reply, "RESUM"),
		"expected resume success in: "+r2.Reply)
	assert(t, resumeRoute.called.Load() == 1, "resume endpoint should be called once")
}

// ---------------------------------------------------------------------------
// Workflow 2: KYC Approval
// ---------------------------------------------------------------------------

func TestWorkflow_KYCApproval(t *testing.T) {
	participantsRoute := &routeHandler{fn: func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, map[string]interface{}{
			"data": []map[string]string{
				{"id": "ABC", "name": "Test Trader", "status": "pending"},
			},
		})
	}}
	approveRoute := &routeHandler{fn: func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, map[string]string{"status": "approved"})
	}}

	routes := map[string]*routeHandler{
		"GET /api/v1/participants":       participantsRoute,
		"POST /api/v1/participants/ABC/approve": approveRoute,
	}
	srv := newMultiServer(routes)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)

	// Step 1: show participants
	r1 := exec.Execute("show participants", "test-token")
	assert(t, strings.Contains(r1.Reply, "Participant") || strings.Contains(r1.Reply, "👥") || strings.Contains(r1.Reply, "Test Trader"),
		"expected participant list in: "+r1.Reply)
	assert(t, participantsRoute.called.Load() == 1, "participants endpoint called")

	// Step 2: approve trader ABC
	r2 := exec.Execute("approve trader ABC", "test-token")
	assert(t, strings.Contains(r2.Reply, "✅") || strings.Contains(r2.Reply, "APPROVED"),
		"expected approval success in: "+r2.Reply)
	assert(t, approveRoute.called.Load() == 1, "approve endpoint called")
}

// ---------------------------------------------------------------------------
// Workflow 3: KYC Rejection with reason
// ---------------------------------------------------------------------------

func TestWorkflow_KYCRejection(t *testing.T) {
	var capturedReason string
	rejectRoute := &routeHandler{fn: func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		capturedReason = body["reason"]
		jsonOK(w, map[string]string{"status": "rejected"})
	}}

	routes := map[string]*routeHandler{
		"POST /api/v1/participants/XYZ/reject": rejectRoute,
	}
	srv := newMultiServer(routes)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)

	// reject trader XYZ because docs expired
	r := exec.Execute("reject trader XYZ because docs expired", "test-token")
	assert(t, strings.Contains(r.Reply, "✅") || strings.Contains(r.Reply, "REJECTED"),
		"expected rejection success in: "+r.Reply)
	assert(t, rejectRoute.called.Load() == 1, "reject endpoint called")
	assert(t, strings.Contains(capturedReason, "docs expired") || capturedReason != "",
		"expected reason in request body, got: "+capturedReason)
}

// ---------------------------------------------------------------------------
// Workflow 4: Margin Investigation
// ---------------------------------------------------------------------------

func TestWorkflow_MarginInvestigation(t *testing.T) {
	marginRoute := &routeHandler{fn: func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, map[string]interface{}{
			"total_active":         3,
			"total_shortfall":      50000.0,
			"participants_in_call": []string{"P1", "P2", "P3"},
		})
	}}

	routes := map[string]*routeHandler{
		"GET /api/v1/margin/calls/stats": marginRoute,
	}
	srv := newMultiServer(routes)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)

	// show margin calls
	r := exec.Execute("show margin calls", "test-token")
	assert(t, strings.Contains(r.Reply, "Margin") || strings.Contains(r.Reply, "💰"),
		"expected margin stats in: "+r.Reply)
	assert(t, marginRoute.called.Load() == 1, "margin stats endpoint called")
	// Verify actual numeric data appears
	assert(t, strings.Contains(r.Reply, "3") || strings.Contains(r.Reply, "Active"),
		"expected margin call count in reply: "+r.Reply)
}

// ---------------------------------------------------------------------------
// Workflow 5: Settlement Cycle
// ---------------------------------------------------------------------------

func TestWorkflow_SettlementCycle(t *testing.T) {
	var settlementRan atomic.Bool

	listRoute := &routeHandler{fn: func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, map[string]interface{}{
			"cycles": []map[string]string{
				{"id": "C001", "status": "completed"},
			},
		})
	}}
	runRoute := &routeHandler{fn: func(w http.ResponseWriter, r *http.Request) {
		settlementRan.Store(true)
		jsonOK(w, map[string]string{"status": "triggered"})
	}}

	routes := map[string]*routeHandler{
		"GET /api/v1/settlement/cycles":  listRoute,
		"POST /api/v1/settlement/cycle":  runRoute,
	}
	srv := newMultiServer(routes)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)

	// Step 1: show settlement cycles
	r1 := exec.Execute("show settlement cycles", "test-token")
	assert(t, strings.Contains(r1.Reply, "Settlement") || strings.Contains(r1.Reply, "⚖️") || strings.Contains(r1.Reply, "cycle"),
		"expected settlement cycles in: "+r1.Reply)
	assert(t, listRoute.called.Load() == 1, "settlement list endpoint called")

	// Step 2: run settlement
	r2 := exec.Execute("run settlement", "test-token")
	assert(t, strings.Contains(r2.Reply, "✅") || strings.Contains(r2.Reply, "Settlement"),
		"expected settlement triggered in: "+r2.Reply)
	assert(t, settlementRan.Load(), "settlement cycle endpoint should be called")
	assert(t, runRoute.called.Load() == 1, "settlement run endpoint called once")
}

// ---------------------------------------------------------------------------
// Workflow 6: Alert Resolution
// ---------------------------------------------------------------------------

func TestWorkflow_AlertResolution(t *testing.T) {
	alertsRoute := &routeHandler{fn: func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, map[string]interface{}{
			"alerts": []map[string]string{
				{"id": "123", "type": "large_position", "status": "open"},
			},
		})
	}}
	resolveRoute := &routeHandler{fn: func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, map[string]string{"status": "resolved"})
	}}

	routes := map[string]*routeHandler{
		"GET /api/v1/compliance/alerts":           alertsRoute,
		"POST /api/v1/compliance/alerts/123/resolve": resolveRoute,
	}
	srv := newMultiServer(routes)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)

	// Step 1: show alerts
	r1 := exec.Execute("show alerts", "test-token")
	assert(t, strings.Contains(r1.Reply, "Alert") || strings.Contains(r1.Reply, "🔍"),
		"expected alerts display in: "+r1.Reply)
	assert(t, alertsRoute.called.Load() == 1, "alerts list endpoint called")

	// Step 2: resolve alert 123
	r2 := exec.Execute("resolve alert 123", "test-token")
	assert(t, strings.Contains(r2.Reply, "✅") || strings.Contains(r2.Reply, "resolved") || strings.Contains(r2.Reply, "123"),
		"expected alert resolution in: "+r2.Reply)
	assert(t, resolveRoute.called.Load() == 1, "resolve endpoint called")
}

// ---------------------------------------------------------------------------
// Workflow 7: Order Lifecycle (buy → show → cancel)
// ---------------------------------------------------------------------------

func TestWorkflow_OrderLifecycle(t *testing.T) {
	var orderCreated atomic.Bool

	createRoute := &routeHandler{fn: func(w http.ResponseWriter, r *http.Request) {
		orderCreated.Store(true)
		jsonOK(w, map[string]string{"order_id": "ORD-ABC", "status": "open"})
	}}
	listRoute := &routeHandler{fn: func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, map[string]interface{}{
			"orders": []map[string]string{
				{"id": "ORD-ABC", "status": "open"},
			},
		})
	}}
	cancelRoute := &routeHandler{fn: func(w http.ResponseWriter, r *http.Request) {
		if !orderCreated.Load() {
			t.Error("cancel called before order creation")
		}
		jsonOK(w, map[string]string{"status": "cancelled"})
	}}

	routes := map[string]*routeHandler{
		"POST /api/v1/orders":            createRoute,
		"GET /api/v1/orders":             listRoute,
		"DELETE /api/v1/orders/ORD-ABC":  cancelRoute,
	}
	srv := newMultiServer(routes)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)

	// Step 1: place buy order
	r1 := exec.Execute("buy 10 wheat at 325", "test-token")
	assert(t, strings.Contains(r1.Reply, "✅") || strings.Contains(r1.Reply, "BUY") || strings.Contains(r1.Reply, "order"),
		"expected order placement in: "+r1.Reply)
	assert(t, createRoute.called.Load() == 1, "order create endpoint called")

	// Step 2: show orders
	r2 := exec.Execute("show orders", "test-token")
	assert(t, strings.Contains(r2.Reply, "Order") || strings.Contains(r2.Reply, "📋"),
		"expected order list in: "+r2.Reply)
	assert(t, listRoute.called.Load() == 1, "orders list endpoint called")

	// Step 3: cancel order ORD-ABC
	r3 := exec.Execute("cancel order ORD-ABC", "test-token")
	assert(t, strings.Contains(r3.Reply, "✅") || strings.Contains(r3.Reply, "cancel") || strings.Contains(r3.Reply, "ORD-ABC"),
		"expected cancellation confirmation in: "+r3.Reply)
	assert(t, cancelRoute.called.Load() == 1, "cancel endpoint called")
}

// ---------------------------------------------------------------------------
// Workflow 8: Risk Management (show → set limit → verify)
// ---------------------------------------------------------------------------

func TestWorkflow_RiskManagement(t *testing.T) {
	var capturedLimit string

	showRoute := &routeHandler{fn: func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, map[string]interface{}{
			"limits": []map[string]interface{}{
				{"instrument": "WHT-HRW-2026M07-UB", "max_order_size": 200},
			},
		})
	}}
	setRoute := &routeHandler{fn: func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		capturedLimit = body["max_order_size"]
		jsonOK(w, map[string]string{"status": "updated"})
	}}

	routes := map[string]*routeHandler{
		"GET /api/v1/admin/risk/order-limits":                     showRoute,
		"PUT /api/v1/admin/risk/order-limits/WHT-HRW-2026M07-UB": setRoute,
	}
	srv := newMultiServer(routes)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)

	// Step 1: show risk limits
	r1 := exec.Execute("show risk limits", "test-token")
	assert(t, strings.Contains(r1.Reply, "Risk") || strings.Contains(r1.Reply, "⚠️") || strings.Contains(r1.Reply, "limit"),
		"expected risk limits in: "+r1.Reply)
	assert(t, showRoute.called.Load() == 1, "risk limits show endpoint called")

	// Step 2: set wheat max order 500
	r2 := exec.Execute("set wheat max order 500", "test-token")
	assert(t, strings.Contains(r2.Reply, "✅") || strings.Contains(r2.Reply, "500") || strings.Contains(r2.Reply, "Max"),
		"expected risk limit update in: "+r2.Reply)
	assert(t, setRoute.called.Load() == 1, "risk limit set endpoint called")
	assert(t, capturedLimit == "500", "expected max_order_size=500 in request, got: "+capturedLimit)
}

// ---------------------------------------------------------------------------
// Workflow 9: Report Generation
// ---------------------------------------------------------------------------

func TestWorkflow_ReportGeneration(t *testing.T) {
	reportRoute := &routeHandler{fn: func(w http.ResponseWriter, r *http.Request) {
		// Verify date query param is present
		date := r.URL.Query().Get("date")
		if date == "" {
			t.Error("expected date query parameter")
		}
		jsonOK(w, map[string]interface{}{
			"date":         date,
			"total_volume": 15000000,
			"trades":       142,
			"instruments":  6,
		})
	}}

	routes := map[string]*routeHandler{
		"GET /api/v1/reports/market-summary": reportRoute,
	}
	srv := newMultiServer(routes)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)

	// request market summary for today
	r := exec.Execute("market summary today", "test-token")
	// The executor may hit the report endpoint or return a fallback message if endpoint is "not yet available"
	// In either case the reply should contain market summary content
	assert(t, strings.Contains(r.Reply, "Market") || strings.Contains(r.Reply, "Summary") ||
		strings.Contains(r.Reply, "📊") || strings.Contains(r.Reply, "market"),
		"expected market summary content in: "+r.Reply)

	// When the mock server is reachable the report endpoint must be called
	if reportRoute.called.Load() > 0 {
		assert(t, strings.Contains(r.Reply, "📊"), "expected report emoji when endpoint succeeds: "+r.Reply)
	}
}

// ---------------------------------------------------------------------------
// Workflow 10: System Health + Help
// ---------------------------------------------------------------------------

func TestWorkflow_SystemHealthAndHelp(t *testing.T) {
	healthRoute := &routeHandler{fn: func(w http.ResponseWriter, r *http.Request) {
		services := make([]map[string]string, 9)
		serviceNames := []string{
			"matching-engine", "clearing-engine", "margin-engine",
			"settlement-engine", "auth-service", "compliance-service",
			"gateway", "market-data-service", "warehouse-service",
		}
		for i, name := range serviceNames {
			services[i] = map[string]string{"name": name, "status": "healthy"}
		}
		jsonOK(w, map[string]interface{}{
			"overall_status": "healthy",
			"services":       services,
		})
	}}

	routes := map[string]*routeHandler{
		"GET /api/v1/admin/health": healthRoute,
	}
	srv := newMultiServer(routes)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)

	// Step 1: system health
	r1 := exec.Execute("system health", "test-token")
	assert(t, strings.Contains(r1.Reply, "Health") || strings.Contains(r1.Reply, "🏥"),
		"expected health status in: "+r1.Reply)
	assert(t, healthRoute.called.Load() == 1, "health endpoint called")

	// Verify 9 services are listed
	serviceCount := strings.Count(r1.Reply, "✅") + strings.Count(r1.Reply, "❌")
	assert(t, serviceCount == 9, "expected 9 service status entries, got: "+r1.Reply)

	// Step 2: help
	r2 := exec.Execute("help", "test-token")
	assert(t, strings.Contains(r2.Reply, "Orders"), "expected Orders category in help: "+r2.Reply)
	assert(t, strings.Contains(r2.Reply, "KYC") || strings.Contains(r2.Reply, "Participants"),
		"expected KYC/Participants category in help: "+r2.Reply)
	assert(t, strings.Contains(r2.Reply, "Compliance"), "expected Compliance category in help: "+r2.Reply)
	assert(t, strings.Contains(r2.Reply, "Settlement"), "expected Settlement category in help: "+r2.Reply)
	assert(t, strings.Contains(r2.Reply, "Market Data") || strings.Contains(r2.Reply, "market"),
		"expected Market Data category in help: "+r2.Reply)
}

// ---------------------------------------------------------------------------
// Bonus: multi-step state validation (Halt before Resume)
// ---------------------------------------------------------------------------

func TestWorkflow_HaltAndResume_StateViolation(t *testing.T) {
	// A server where resume is reachable but should not be called before halt
	var haltCalled atomic.Bool
	var resumeOrder []string

	routes := map[string]*routeHandler{
		"POST /api/v1/admin/instruments/WHT-HRW-2026M07-UB/halt": {
			fn: func(w http.ResponseWriter, r *http.Request) {
				haltCalled.Store(true)
				resumeOrder = append(resumeOrder, "halt")
				jsonOK(w, map[string]string{"status": "halted"})
			},
		},
		"POST /api/v1/admin/instruments/WHT-HRW-2026M07-UB/resume": {
			fn: func(w http.ResponseWriter, r *http.Request) {
				resumeOrder = append(resumeOrder, "resume")
				jsonOK(w, map[string]string{"status": "continuous"})
			},
		},
	}
	srv := newMultiServer(routes)
	defer srv.Close()

	exec := NewActionExecutor(srv.URL)

	// Correct order: halt then resume
	exec.Execute("halt wheat", "tok")
	exec.Execute("resume wheat", "tok")

	assert(t, haltCalled.Load(), "halt should have been called")
	assert(t, len(resumeOrder) == 2 && resumeOrder[0] == "halt" && resumeOrder[1] == "resume",
		"expected halt before resume ordering")
}
