package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/garudax-platform/gateway/internal/auth"
	"github.com/garudax-platform/gateway/internal/middleware"
	"github.com/garudax-platform/gateway/internal/router"
)

// --- Test helpers ---

func withClaims(r *http.Request, claims *auth.Claims) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.ClaimsContextKey, claims)
	return r.WithContext(ctx)
}

func adminClaims(sub string) *auth.Claims {
	return &auth.Claims{
		Sub:   sub,
		Roles: []string{"admin"},
	}
}

func userClaims(sub string) *auth.Claims {
	return &auth.Claims{
		Sub:   sub,
		Roles: []string{"trader"},
	}
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	return body
}

func setupRouter(orchestratorURL string) *router.Router {
	bridge := NewBridge(orchestratorURL)
	h := NewHandlers(bridge)
	rt := router.New()
	h.RegisterRoutes(rt)
	return rt
}

// --- Chat endpoint tests ---

func TestChat_RequiresAuth(t *testing.T) {
	rt := setupRouter("")
	body := `{"message":"hello"}`
	req := httptest.NewRequest("POST", "/api/v1/bot/chat", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	resp := decodeBody(t, rec)
	errObj := resp["error"].(map[string]interface{})
	if errObj["code"] != "UNAUTHENTICATED" {
		t.Errorf("error code = %v, want UNAUTHENTICATED", errObj["code"])
	}
}

func TestChat_RequiresAdminRole(t *testing.T) {
	rt := setupRouter("")
	body := `{"message":"hello"}`
	req := httptest.NewRequest("POST", "/api/v1/bot/chat", bytes.NewBufferString(body))
	req = withClaims(req, userClaims("user-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	resp := decodeBody(t, rec)
	errObj := resp["error"].(map[string]interface{})
	if errObj["code"] != "PERMISSION_DENIED" {
		t.Errorf("error code = %v, want PERMISSION_DENIED", errObj["code"])
	}
}

func TestChat_InvalidBody(t *testing.T) {
	rt := setupRouter("")
	req := httptest.NewRequest("POST", "/api/v1/bot/chat", bytes.NewBufferString("not json"))
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestChat_EmptyMessage(t *testing.T) {
	rt := setupRouter("")
	body := `{"message":""}`
	req := httptest.NewRequest("POST", "/api/v1/bot/chat", bytes.NewBufferString(body))
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	resp := decodeBody(t, rec)
	errObj := resp["error"].(map[string]interface{})
	if errObj["message"] != "Missing message" {
		t.Errorf("message = %v, want Missing message", errObj["message"])
	}
}

func TestChat_FallbackMode_HealthKeyword(t *testing.T) {
	rt := setupRouter("")
	body := `{"message":"What is the system health?"}`
	req := httptest.NewRequest("POST", "/api/v1/bot/chat", bytes.NewBufferString(body))
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	resp := decodeBody(t, rec)
	data := resp["data"].(map[string]interface{})
	reply := data["reply"].(string)
	if reply == "" {
		t.Error("reply should not be empty")
	}
	if data["actions"] == nil {
		t.Error("actions should not be nil")
	}
	if data["suggestions"] == nil {
		t.Error("suggestions should not be nil")
	}
}

func TestChat_FallbackMode_AlertsKeyword(t *testing.T) {
	rt := setupRouter("")
	body := `{"message":"Show me alerts"}`
	req := httptest.NewRequest("POST", "/api/v1/bot/chat", bytes.NewBufferString(body))
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	resp := decodeBody(t, rec)
	data := resp["data"].(map[string]interface{})
	reply := data["reply"].(string)
	if reply == "" {
		t.Error("reply should not be empty for alerts keyword")
	}
}

func TestChat_FallbackMode_MarginKeyword(t *testing.T) {
	rt := setupRouter("")
	body := `{"message":"margin calls"}`
	req := httptest.NewRequest("POST", "/api/v1/bot/chat", bytes.NewBufferString(body))
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	resp := decodeBody(t, rec)
	data := resp["data"].(map[string]interface{})
	reply := data["reply"].(string)
	if reply == "" {
		t.Error("reply should not be empty for margin keyword")
	}
}

func TestChat_FallbackMode_TicketKeyword(t *testing.T) {
	rt := setupRouter("")
	body := `{"message":"report a bug"}`
	req := httptest.NewRequest("POST", "/api/v1/bot/chat", bytes.NewBufferString(body))
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	resp := decodeBody(t, rec)
	data := resp["data"].(map[string]interface{})
	reply := data["reply"].(string)
	if reply == "" {
		t.Error("reply should not be empty for ticket keyword")
	}
}

func TestChat_FallbackMode_DefaultKeyword(t *testing.T) {
	rt := setupRouter("")
	body := `{"message":"something random"}`
	req := httptest.NewRequest("POST", "/api/v1/bot/chat", bytes.NewBufferString(body))
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	resp := decodeBody(t, rec)
	data := resp["data"].(map[string]interface{})
	reply := data["reply"].(string)
	if !strings.Contains(reply, "I didn't understand that") {
		t.Errorf("unexpected default reply: %s", reply)
	}
}

func TestChat_FallbackMode_WithPageContext(t *testing.T) {
	rt := setupRouter("")
	body := `{"message":"help","context":{"page":"surveillance"}}`
	req := httptest.NewRequest("POST", "/api/v1/bot/chat", bytes.NewBufferString(body))
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	resp := decodeBody(t, rec)
	data := resp["data"].(map[string]interface{})

	suggestions := data["suggestions"].([]interface{})
	if len(suggestions) != 3 {
		t.Errorf("surveillance suggestions count = %d, want 3", len(suggestions))
	}
}

func TestChat_FallbackMode_SettlementKeyword(t *testing.T) {
	rt := setupRouter("")
	body := `{"message":"settlement cycle"}`
	req := httptest.NewRequest("POST", "/api/v1/bot/chat", bytes.NewBufferString(body))
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	resp := decodeBody(t, rec)
	data := resp["data"].(map[string]interface{})
	reply := data["reply"].(string)
	if reply == "" {
		t.Error("reply should not be empty for settlement keyword")
	}
}

func TestChat_FallbackMode_TradingKeyword(t *testing.T) {
	rt := setupRouter("")
	body := `{"message":"how do I place an order?"}`
	req := httptest.NewRequest("POST", "/api/v1/bot/chat", bytes.NewBufferString(body))
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	resp := decodeBody(t, rec)
	data := resp["data"].(map[string]interface{})
	reply := data["reply"].(string)
	if reply == "" {
		t.Error("reply should not be empty for trading keyword")
	}
}

func TestChat_FallbackMode_KYCKeyword(t *testing.T) {
	rt := setupRouter("")
	body := `{"message":"KYC applications"}`
	req := httptest.NewRequest("POST", "/api/v1/bot/chat", bytes.NewBufferString(body))
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	resp := decodeBody(t, rec)
	data := resp["data"].(map[string]interface{})
	reply := data["reply"].(string)
	if reply == "" {
		t.Error("reply should not be empty for KYC keyword")
	}
}

func TestChat_OrchestratorProxy_Success(t *testing.T) {
	// Mock orchestrator server
	mockOrch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat" {
			t.Errorf("orchestrator path = %s, want /chat", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("orchestrator method = %s, want POST", r.Method)
		}

		var req OrchestratorRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Message != "test message" {
			t.Errorf("orchestrator message = %s, want test message", req.Message)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(OrchestratorResponse{
			Reply:       "orchestrator reply",
			Actions:     []Action{{Type: "link", Label: "View", URL: "/dashboard"}},
			Suggestions: []string{"Follow up 1", "Follow up 2"},
		})
	}))
	defer mockOrch.Close()

	rt := setupRouter(mockOrch.URL)
	body := `{"message":"test message","context":{"page":"dashboard"}}`
	req := httptest.NewRequest("POST", "/api/v1/bot/chat", bytes.NewBufferString(body))
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	resp := decodeBody(t, rec)
	data := resp["data"].(map[string]interface{})
	if data["reply"] != "orchestrator reply" {
		t.Errorf("reply = %v, want orchestrator reply", data["reply"])
	}
	actions := data["actions"].([]interface{})
	if len(actions) != 1 {
		t.Errorf("actions count = %d, want 1", len(actions))
	}
	suggestions := data["suggestions"].([]interface{})
	if len(suggestions) != 2 {
		t.Errorf("suggestions count = %d, want 2", len(suggestions))
	}
}

func TestChat_OrchestratorProxy_FallsBackOnError(t *testing.T) {
	// Mock orchestrator that returns error
	mockOrch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal"}`))
	}))
	defer mockOrch.Close()

	rt := setupRouter(mockOrch.URL)
	body := `{"message":"health check"}`
	req := httptest.NewRequest("POST", "/api/v1/bot/chat", bytes.NewBufferString(body))
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; should fall back on orchestrator error", rec.Code, http.StatusOK)
	}
	resp := decodeBody(t, rec)
	data := resp["data"].(map[string]interface{})
	reply := data["reply"].(string)
	if reply == "" {
		t.Error("reply should not be empty in fallback mode")
	}
}

func TestChat_OrchestratorProxy_FallsBackOnUnreachable(t *testing.T) {
	// Use a URL that will fail to connect
	rt := setupRouter("http://127.0.0.1:1") // port 1 is unlikely to be listening
	body := `{"message":"status check"}`
	req := httptest.NewRequest("POST", "/api/v1/bot/chat", bytes.NewBufferString(body))
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; should fall back when orchestrator unreachable", rec.Code, http.StatusOK)
	}
	resp := decodeBody(t, rec)
	data := resp["data"].(map[string]interface{})
	reply := data["reply"].(string)
	if reply == "" {
		t.Error("reply should not be empty in fallback mode")
	}
}

func TestChat_ExchangeAdminRole(t *testing.T) {
	rt := setupRouter("")
	body := `{"message":"health"}`
	req := httptest.NewRequest("POST", "/api/v1/bot/chat", bytes.NewBufferString(body))
	req = withClaims(req, &auth.Claims{Sub: "ea-1", Roles: []string{"exchange_admin"}})
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; exchange_admin should have access", rec.Code, http.StatusOK)
	}
}

// --- Suggestions endpoint tests ---

func TestSuggestions_RequiresAuth(t *testing.T) {
	rt := setupRouter("")
	req := httptest.NewRequest("GET", "/api/v1/bot/suggestions", nil)
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestSuggestions_RequiresAdminRole(t *testing.T) {
	rt := setupRouter("")
	req := httptest.NewRequest("GET", "/api/v1/bot/suggestions", nil)
	req = withClaims(req, userClaims("user-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestSuggestions_DefaultPage(t *testing.T) {
	rt := setupRouter("")
	req := httptest.NewRequest("GET", "/api/v1/bot/suggestions", nil)
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	resp := decodeBody(t, rec)
	data := resp["data"].([]interface{})
	if len(data) != 4 {
		t.Errorf("default suggestions count = %d, want 4", len(data))
	}
}

func TestSuggestions_DashboardPage(t *testing.T) {
	rt := setupRouter("")
	req := httptest.NewRequest("GET", "/api/v1/bot/suggestions?page=dashboard", nil)
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	resp := decodeBody(t, rec)
	data := resp["data"].([]interface{})
	if len(data) != 4 {
		t.Errorf("dashboard suggestions count = %d, want 4", len(data))
	}
}

func TestSuggestions_SurveillancePage(t *testing.T) {
	rt := setupRouter("")
	req := httptest.NewRequest("GET", "/api/v1/bot/suggestions?page=surveillance", nil)
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	resp := decodeBody(t, rec)
	data := resp["data"].([]interface{})
	if len(data) != 3 {
		t.Errorf("surveillance suggestions count = %d, want 3", len(data))
	}
}

func TestSuggestions_MarginPage(t *testing.T) {
	rt := setupRouter("")
	req := httptest.NewRequest("GET", "/api/v1/bot/suggestions?page=margin", nil)
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	resp := decodeBody(t, rec)
	data := resp["data"].([]interface{})
	if len(data) != 3 {
		t.Errorf("margin suggestions count = %d, want 3", len(data))
	}
}

func TestSuggestions_SettlementPage(t *testing.T) {
	rt := setupRouter("")
	req := httptest.NewRequest("GET", "/api/v1/bot/suggestions?page=settlement", nil)
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	resp := decodeBody(t, rec)
	data := resp["data"].([]interface{})
	if len(data) != 3 {
		t.Errorf("settlement suggestions count = %d, want 3", len(data))
	}
}

func TestSuggestions_TicketsPage(t *testing.T) {
	rt := setupRouter("")
	req := httptest.NewRequest("GET", "/api/v1/bot/suggestions?page=tickets", nil)
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	resp := decodeBody(t, rec)
	data := resp["data"].([]interface{})
	if len(data) != 3 {
		t.Errorf("tickets suggestions count = %d, want 3", len(data))
	}
}

func TestSuggestions_ParticipantsPage(t *testing.T) {
	rt := setupRouter("")
	req := httptest.NewRequest("GET", "/api/v1/bot/suggestions?page=participants", nil)
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	resp := decodeBody(t, rec)
	data := resp["data"].([]interface{})
	if len(data) != 2 {
		t.Errorf("participants suggestions count = %d, want 2", len(data))
	}
}

func TestSuggestions_UnknownPage(t *testing.T) {
	rt := setupRouter("")
	req := httptest.NewRequest("GET", "/api/v1/bot/suggestions?page=unknown", nil)
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	resp := decodeBody(t, rec)
	data := resp["data"].([]interface{})
	if len(data) != 4 {
		t.Errorf("unknown page should return default suggestions, got count = %d, want 4", len(data))
	}
}

// --- Bridge unit tests ---

func TestBridge_IsAvailable(t *testing.T) {
	b := NewBridge("")
	if b.IsAvailable() {
		t.Error("bridge should not be available when URL is empty")
	}

	b2 := NewBridge("http://localhost:8090")
	if !b2.IsAvailable() {
		t.Error("bridge should be available when URL is set")
	}
}

func TestBridge_ProxyToOrchestrator_NotConfigured(t *testing.T) {
	b := NewBridge("")
	_, err := b.ProxyToOrchestrator("hello", nil)
	if err == nil {
		t.Error("expected error when orchestrator not configured")
	}
}

// --- Fallback unit tests ---

func TestFallbackResponse_Keywords(t *testing.T) {
	tests := []struct {
		message  string
		contains string
	}{
		{"health check", "health"},
		{"system status", "health"},
		{"show alerts", "alert"},
		{"margin calls", "Margin"},
		{"create a ticket", "ticket"},
		{"report a bug", "ticket"},
		{"settlement status", "Settlement"},
		{"trading volume", "Trading"},
		{"KYC process", "KYC"},
		{"help me", "help with"},
		{"random input", "I can help"},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			reply := FallbackResponse(tt.message)
			if reply == "" {
				t.Errorf("FallbackResponse(%q) returned empty string", tt.message)
			}
		})
	}
}

func TestContainsAny(t *testing.T) {
	if !containsAny("hello world", "world") {
		t.Error("should find 'world' in 'hello world'")
	}
	if !containsAny("hello world", "foo", "world") {
		t.Error("should find 'world' in 'hello world' with multiple args")
	}
	if containsAny("hello world", "foo", "bar") {
		t.Error("should not find 'foo' or 'bar' in 'hello world'")
	}
}

// --- GetSuggestions unit tests ---

func TestGetSuggestions_AllPages(t *testing.T) {
	pages := []string{"dashboard", "surveillance", "margin", "settlement", "tickets", "participants"}
	for _, page := range pages {
		t.Run(page, func(t *testing.T) {
			s := GetSuggestions(page)
			if len(s) == 0 {
				t.Errorf("GetSuggestions(%q) returned empty", page)
			}
			for _, suggestion := range s {
				if suggestion.Text == "" {
					t.Errorf("GetSuggestions(%q) has suggestion with empty text", page)
				}
			}
		})
	}
}

func TestGetSuggestions_Default(t *testing.T) {
	s := GetSuggestions("")
	if len(s) != 4 {
		t.Errorf("default suggestions count = %d, want 4", len(s))
	}
}

// --- Attribution tests ---

func TestFetchUserEmail_EmptyToken(t *testing.T) {
	e := NewActionExecutor("")
	email := e.fetchUserEmail("")
	if email != "" {
		t.Errorf("fetchUserEmail with empty token = %q, want empty", email)
	}
}

func TestFetchUserEmail_ServerReturnsEmail(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/me" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"email":"alice@garudax.mn"}}`))
	}))
	defer mock.Close()

	e := NewActionExecutor(mock.URL)
	email := e.fetchUserEmail("test-token")
	if email != "alice@garudax.mn" {
		t.Errorf("fetchUserEmail = %q, want alice@garudax.mn", email)
	}
}

func TestFetchUserEmail_FlatEmailField(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"email":"bob@garudax.mn"}`))
	}))
	defer mock.Close()

	e := NewActionExecutor(mock.URL)
	email := e.fetchUserEmail("some-token")
	if email != "bob@garudax.mn" {
		t.Errorf("fetchUserEmail flat = %q, want bob@garudax.mn", email)
	}
}

func TestFetchUserEmail_ServerError(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mock.Close()

	e := NewActionExecutor(mock.URL)
	email := e.fetchUserEmail("some-token")
	if email != "" {
		t.Errorf("fetchUserEmail on server error = %q, want empty", email)
	}
}

func TestWithAttribution_AppendsEmail(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"email":"carol@garudax.mn"}}`))
	}))
	defer mock.Close()

	e := NewActionExecutor(mock.URL)
	result := e.withAttribution("✅ Action completed.", "my-jwt")
	want := "✅ Action completed.\n\nExecuted by: carol@garudax.mn"
	if result != want {
		t.Errorf("withAttribution = %q, want %q", result, want)
	}
}

func TestWithAttribution_NoEmailFallsThrough(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mock.Close()

	e := NewActionExecutor(mock.URL)
	result := e.withAttribution("✅ Action completed.", "my-jwt")
	if result != "✅ Action completed." {
		t.Errorf("withAttribution on failure = %q, want unchanged reply", result)
	}
}
