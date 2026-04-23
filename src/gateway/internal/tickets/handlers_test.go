package tickets

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/garudax-platform/gateway/internal/auth"
	"github.com/garudax-platform/gateway/internal/middleware"
	"github.com/garudax-platform/gateway/internal/router"
)

// --- Test helpers ---

func withClaims(r *http.Request, claims *auth.Claims) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.ClaimsContextKey, claims)
	return r.WithContext(ctx)
}

func userClaims(sub string) *auth.Claims {
	return &auth.Claims{
		Sub:   sub,
		Roles: []string{"trader"},
	}
}

func adminClaims(sub string) *auth.Claims {
	return &auth.Claims{
		Sub:   sub,
		Roles: []string{"admin"},
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

func setupRouter() (*router.Router, *Handlers) {
	store := NewInMemoryStore()
	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)
	return rt, h
}

// --- CreateTicket tests ---

func TestCreateTicket_Success(t *testing.T) {
	rt, _ := setupRouter()

	body := `{"title":"Login broken","description":"Cannot login after update","category":"bug_report"}`
	req := httptest.NewRequest("POST", "/api/v1/tickets", bytes.NewBufferString(body))
	req = withClaims(req, userClaims("user-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	resp := decodeBody(t, rec)
	data := resp["data"].(map[string]interface{})
	if data["title"] != "Login broken" {
		t.Errorf("title = %v, want Login broken", data["title"])
	}
	if data["status"] != "open" {
		t.Errorf("status = %v, want open", data["status"])
	}
	if data["priority"] != "medium" {
		t.Errorf("priority = %v, want medium (default)", data["priority"])
	}
	if data["reporter_id"] != "user-1" {
		t.Errorf("reporter_id = %v, want user-1", data["reporter_id"])
	}
}

func TestCreateTicket_WithPriority(t *testing.T) {
	rt, _ := setupRouter()

	body := `{"title":"Urgent issue","description":"System down","category":"support","priority":"critical"}`
	req := httptest.NewRequest("POST", "/api/v1/tickets", bytes.NewBufferString(body))
	req = withClaims(req, userClaims("user-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	resp := decodeBody(t, rec)
	data := resp["data"].(map[string]interface{})
	if data["priority"] != "critical" {
		t.Errorf("priority = %v, want critical", data["priority"])
	}
}

func TestCreateTicket_MissingTitle(t *testing.T) {
	rt, _ := setupRouter()

	body := `{"description":"Some desc","category":"support"}`
	req := httptest.NewRequest("POST", "/api/v1/tickets", bytes.NewBufferString(body))
	req = withClaims(req, userClaims("user-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateTicket_MissingDescription(t *testing.T) {
	rt, _ := setupRouter()

	body := `{"title":"Bug","category":"bug_report"}`
	req := httptest.NewRequest("POST", "/api/v1/tickets", bytes.NewBufferString(body))
	req = withClaims(req, userClaims("user-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateTicket_InvalidCategory(t *testing.T) {
	rt, _ := setupRouter()

	body := `{"title":"Bug","description":"desc","category":"invalid_cat"}`
	req := httptest.NewRequest("POST", "/api/v1/tickets", bytes.NewBufferString(body))
	req = withClaims(req, userClaims("user-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateTicket_InvalidPriority(t *testing.T) {
	rt, _ := setupRouter()

	body := `{"title":"Bug","description":"desc","category":"bug_report","priority":"ultra"}`
	req := httptest.NewRequest("POST", "/api/v1/tickets", bytes.NewBufferString(body))
	req = withClaims(req, userClaims("user-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateTicket_Unauthenticated(t *testing.T) {
	rt, _ := setupRouter()

	body := `{"title":"Bug","description":"desc","category":"support"}`
	req := httptest.NewRequest("POST", "/api/v1/tickets", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestCreateTicket_InvalidJSON(t *testing.T) {
	rt, _ := setupRouter()

	req := httptest.NewRequest("POST", "/api/v1/tickets", bytes.NewBufferString("{bad json"))
	req = withClaims(req, userClaims("user-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateTicket_WithTags(t *testing.T) {
	rt, _ := setupRouter()

	body := `{"title":"Feature","description":"Add X","category":"feature_request","tags":["ui","dashboard"]}`
	req := httptest.NewRequest("POST", "/api/v1/tickets", bytes.NewBufferString(body))
	req = withClaims(req, userClaims("user-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	resp := decodeBody(t, rec)
	data := resp["data"].(map[string]interface{})
	tags := data["tags"].([]interface{})
	if len(tags) != 2 {
		t.Errorf("tags count = %d, want 2", len(tags))
	}
}

// --- ListTickets tests ---

func TestListTickets_UserSeesOwnOnly(t *testing.T) {
	store := NewInMemoryStore()
	store.CreateTicket(context.Background(), Ticket{ID: "t1", Title: "T1", Description: "d", Category: "support", Priority: "low", Status: "open", ReporterID: "user-1"})
	store.CreateTicket(context.Background(), Ticket{ID: "t2", Title: "T2", Description: "d", Category: "support", Priority: "low", Status: "open", ReporterID: "user-2"})

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	req := httptest.NewRequest("GET", "/api/v1/tickets", nil)
	req = withClaims(req, userClaims("user-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	resp := decodeBody(t, rec)
	count := resp["count"].(float64)
	if count != 1 {
		t.Errorf("count = %v, want 1", count)
	}
}

func TestListTickets_AdminSeesAll(t *testing.T) {
	store := NewInMemoryStore()
	store.CreateTicket(context.Background(), Ticket{ID: "t1", Title: "T1", Description: "d", Category: "support", Priority: "low", Status: "open", ReporterID: "user-1"})
	store.CreateTicket(context.Background(), Ticket{ID: "t2", Title: "T2", Description: "d", Category: "support", Priority: "low", Status: "open", ReporterID: "user-2"})

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	req := httptest.NewRequest("GET", "/api/v1/tickets", nil)
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	resp := decodeBody(t, rec)
	count := resp["count"].(float64)
	if count != 2 {
		t.Errorf("count = %v, want 2", count)
	}
}

func TestListTickets_FilterByStatus(t *testing.T) {
	store := NewInMemoryStore()
	store.CreateTicket(context.Background(), Ticket{ID: "t1", Title: "T1", Description: "d", Category: "support", Priority: "low", Status: "open", ReporterID: "user-1"})
	store.CreateTicket(context.Background(), Ticket{ID: "t2", Title: "T2", Description: "d", Category: "support", Priority: "low", Status: "resolved", ReporterID: "user-1"})

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	req := httptest.NewRequest("GET", "/api/v1/tickets?status=open", nil)
	req = withClaims(req, userClaims("user-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	resp := decodeBody(t, rec)
	count := resp["count"].(float64)
	if count != 1 {
		t.Errorf("count = %v, want 1", count)
	}
}

func TestListTickets_Unauthenticated(t *testing.T) {
	rt, _ := setupRouter()

	req := httptest.NewRequest("GET", "/api/v1/tickets", nil)
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestListTickets_EmptyResult(t *testing.T) {
	rt, _ := setupRouter()

	req := httptest.NewRequest("GET", "/api/v1/tickets", nil)
	req = withClaims(req, userClaims("user-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	resp := decodeBody(t, rec)
	data := resp["data"].([]interface{})
	if len(data) != 0 {
		t.Errorf("expected empty array, got %d items", len(data))
	}
}

// --- GetTicket tests ---

func TestGetTicket_Success(t *testing.T) {
	store := NewInMemoryStore()
	store.CreateTicket(context.Background(), Ticket{ID: "t1", Title: "Bug", Description: "d", Category: "bug_report", Priority: "high", Status: "open", ReporterID: "user-1"})

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	req := httptest.NewRequest("GET", "/api/v1/tickets/t1", nil)
	req = withClaims(req, userClaims("user-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	resp := decodeBody(t, rec)
	data := resp["data"].(map[string]interface{})
	if data["id"] != "t1" {
		t.Errorf("id = %v, want t1", data["id"])
	}
}

func TestGetTicket_NotFound(t *testing.T) {
	rt, _ := setupRouter()

	req := httptest.NewRequest("GET", "/api/v1/tickets/nonexistent", nil)
	req = withClaims(req, userClaims("user-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGetTicket_OtherUserForbidden(t *testing.T) {
	store := NewInMemoryStore()
	store.CreateTicket(context.Background(), Ticket{ID: "t1", Title: "Bug", Description: "d", Category: "bug_report", Priority: "high", Status: "open", ReporterID: "user-1"})

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	req := httptest.NewRequest("GET", "/api/v1/tickets/t1", nil)
	req = withClaims(req, userClaims("user-2"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestGetTicket_AdminCanViewAny(t *testing.T) {
	store := NewInMemoryStore()
	store.CreateTicket(context.Background(), Ticket{ID: "t1", Title: "Bug", Description: "d", Category: "bug_report", Priority: "high", Status: "open", ReporterID: "user-1"})

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	req := httptest.NewRequest("GET", "/api/v1/tickets/t1", nil)
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// --- UpdateTicket tests ---

func TestUpdateTicket_AdminCanUpdate(t *testing.T) {
	store := NewInMemoryStore()
	store.CreateTicket(context.Background(), Ticket{ID: "t1", Title: "Bug", Description: "d", Category: "bug_report", Priority: "low", Status: "open", ReporterID: "user-1"})

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	body := `{"status":"in_progress","assignee_id":"admin-1","priority":"high"}`
	req := httptest.NewRequest("PATCH", "/api/v1/tickets/t1", bytes.NewBufferString(body))
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	resp := decodeBody(t, rec)
	data := resp["data"].(map[string]interface{})
	if data["status"] != "in_progress" {
		t.Errorf("status = %v, want in_progress", data["status"])
	}
	if data["priority"] != "high" {
		t.Errorf("priority = %v, want high", data["priority"])
	}
	if data["assignee_id"] != "admin-1" {
		t.Errorf("assignee_id = %v, want admin-1", data["assignee_id"])
	}
}

func TestUpdateTicket_NonAdminForbidden(t *testing.T) {
	store := NewInMemoryStore()
	store.CreateTicket(context.Background(), Ticket{ID: "t1", Title: "Bug", Description: "d", Category: "bug_report", Priority: "low", Status: "open", ReporterID: "user-1"})

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	body := `{"status":"resolved"}`
	req := httptest.NewRequest("PATCH", "/api/v1/tickets/t1", bytes.NewBufferString(body))
	req = withClaims(req, userClaims("user-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestUpdateTicket_NotFound(t *testing.T) {
	rt, _ := setupRouter()

	body := `{"status":"closed"}`
	req := httptest.NewRequest("PATCH", "/api/v1/tickets/nonexistent", bytes.NewBufferString(body))
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestUpdateTicket_InvalidStatus(t *testing.T) {
	store := NewInMemoryStore()
	store.CreateTicket(context.Background(), Ticket{ID: "t1", Title: "Bug", Description: "d", Category: "bug_report", Priority: "low", Status: "open", ReporterID: "user-1"})

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	body := `{"status":"invalid_status"}`
	req := httptest.NewRequest("PATCH", "/api/v1/tickets/t1", bytes.NewBufferString(body))
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdateTicket_EmptyUpdate(t *testing.T) {
	store := NewInMemoryStore()
	store.CreateTicket(context.Background(), Ticket{ID: "t1", Title: "Bug", Description: "d", Category: "bug_report", Priority: "low", Status: "open", ReporterID: "user-1"})

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	body := `{}`
	req := httptest.NewRequest("PATCH", "/api/v1/tickets/t1", bytes.NewBufferString(body))
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdateTicket_Resolve_SetsResolvedAt(t *testing.T) {
	store := NewInMemoryStore()
	now := time.Now().Add(-time.Hour)
	store.CreateTicket(context.Background(), Ticket{ID: "t1", Title: "Bug", Description: "d", Category: "bug_report", Priority: "low", Status: "open", ReporterID: "user-1", CreatedAt: now})

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	body := `{"status":"resolved"}`
	req := httptest.NewRequest("PATCH", "/api/v1/tickets/t1", bytes.NewBufferString(body))
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	resp := decodeBody(t, rec)
	data := resp["data"].(map[string]interface{})
	if data["resolved_at"] == nil {
		t.Error("resolved_at should be set when status is resolved")
	}
}

// --- CreateComment tests ---

func TestCreateComment_ByReporter(t *testing.T) {
	store := NewInMemoryStore()
	store.CreateTicket(context.Background(), Ticket{ID: "t1", Title: "Bug", Description: "d", Category: "bug_report", Priority: "low", Status: "open", ReporterID: "user-1"})

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	body := `{"body":"Here is more detail about the bug"}`
	req := httptest.NewRequest("POST", "/api/v1/tickets/t1/comments", bytes.NewBufferString(body))
	req = withClaims(req, userClaims("user-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	resp := decodeBody(t, rec)
	data := resp["data"].(map[string]interface{})
	if data["body"] != "Here is more detail about the bug" {
		t.Errorf("body = %v", data["body"])
	}
}

func TestCreateComment_ByAdmin(t *testing.T) {
	store := NewInMemoryStore()
	store.CreateTicket(context.Background(), Ticket{ID: "t1", Title: "Bug", Description: "d", Category: "bug_report", Priority: "low", Status: "open", ReporterID: "user-1"})

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	body := `{"body":"We are looking into this"}`
	req := httptest.NewRequest("POST", "/api/v1/tickets/t1/comments", bytes.NewBufferString(body))
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
}

func TestCreateComment_OtherUserForbidden(t *testing.T) {
	store := NewInMemoryStore()
	store.CreateTicket(context.Background(), Ticket{ID: "t1", Title: "Bug", Description: "d", Category: "bug_report", Priority: "low", Status: "open", ReporterID: "user-1"})

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	body := `{"body":"Me too"}`
	req := httptest.NewRequest("POST", "/api/v1/tickets/t1/comments", bytes.NewBufferString(body))
	req = withClaims(req, userClaims("user-2"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestCreateComment_TicketNotFound(t *testing.T) {
	rt, _ := setupRouter()

	body := `{"body":"Hello"}`
	req := httptest.NewRequest("POST", "/api/v1/tickets/nonexistent/comments", bytes.NewBufferString(body))
	req = withClaims(req, userClaims("user-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestCreateComment_EmptyBody(t *testing.T) {
	store := NewInMemoryStore()
	store.CreateTicket(context.Background(), Ticket{ID: "t1", Title: "Bug", Description: "d", Category: "bug_report", Priority: "low", Status: "open", ReporterID: "user-1"})

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	body := `{"body":"   "}`
	req := httptest.NewRequest("POST", "/api/v1/tickets/t1/comments", bytes.NewBufferString(body))
	req = withClaims(req, userClaims("user-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// --- GetTicketStats tests ---

func TestGetTicketStats_Admin(t *testing.T) {
	store := NewInMemoryStore()
	now := time.Now()
	resolved := now.Add(2 * time.Hour)
	store.CreateTicket(context.Background(), Ticket{ID: "t1", Title: "T1", Description: "d", Category: "support", Priority: "low", Status: "open", ReporterID: "user-1", CreatedAt: now})
	store.CreateTicket(context.Background(), Ticket{ID: "t2", Title: "T2", Description: "d", Category: "support", Priority: "low", Status: "resolved", ReporterID: "user-2", CreatedAt: now, ResolvedAt: &resolved})
	store.CreateTicket(context.Background(), Ticket{ID: "t3", Title: "T3", Description: "d", Category: "support", Priority: "low", Status: "closed", ReporterID: "user-3", CreatedAt: now})

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	req := httptest.NewRequest("GET", "/api/v1/admin/tickets/stats", nil)
	req = withClaims(req, adminClaims("admin-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	resp := decodeBody(t, rec)
	data := resp["data"].(map[string]interface{})
	if data["total"].(float64) != 3 {
		t.Errorf("total = %v, want 3", data["total"])
	}
	if data["open"].(float64) != 1 {
		t.Errorf("open = %v, want 1", data["open"])
	}
	if data["resolved"].(float64) != 1 {
		t.Errorf("resolved = %v, want 1", data["resolved"])
	}
	if data["closed"].(float64) != 1 {
		t.Errorf("closed = %v, want 1", data["closed"])
	}
}

func TestGetTicketStats_NonAdminForbidden(t *testing.T) {
	rt, _ := setupRouter()

	req := httptest.NewRequest("GET", "/api/v1/admin/tickets/stats", nil)
	req = withClaims(req, userClaims("user-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

// --- GetTicket with comments ---

func TestGetTicket_WithComments(t *testing.T) {
	store := NewInMemoryStore()
	store.CreateTicket(context.Background(), Ticket{ID: "t1", Title: "Bug", Description: "d", Category: "bug_report", Priority: "low", Status: "open", ReporterID: "user-1"})
	store.CreateComment(context.Background(), Comment{ID: "c1", TicketID: "t1", AuthorID: "user-1", Body: "First comment"})
	store.CreateComment(context.Background(), Comment{ID: "c2", TicketID: "t1", AuthorID: "admin-1", Body: "Reply"})

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	req := httptest.NewRequest("GET", "/api/v1/tickets/t1", nil)
	req = withClaims(req, userClaims("user-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	resp := decodeBody(t, rec)
	data := resp["data"].(map[string]interface{})
	comments := data["comments"].([]interface{})
	if len(comments) != 2 {
		t.Errorf("comments count = %d, want 2", len(comments))
	}
}

// --- Store unit tests ---

func TestInMemoryStore_DuplicateID(t *testing.T) {
	store := NewInMemoryStore()
	err := store.CreateTicket(context.Background(), Ticket{ID: "t1", Title: "T1", Description: "d", Category: "support", Priority: "low", Status: "open", ReporterID: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	err = store.CreateTicket(context.Background(), Ticket{ID: "t1", Title: "T2", Description: "d", Category: "support", Priority: "low", Status: "open", ReporterID: "u2"})
	if err != ErrDuplicateID {
		t.Errorf("err = %v, want ErrDuplicateID", err)
	}
}

func TestInMemoryStore_UpdateNotFound(t *testing.T) {
	store := NewInMemoryStore()
	err := store.UpdateTicket(context.Background(), "nonexistent", map[string]interface{}{"status": "closed"})
	if err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestInMemoryStore_CommentOnNonexistentTicket(t *testing.T) {
	store := NewInMemoryStore()
	err := store.CreateComment(context.Background(), Comment{ID: "c1", TicketID: "nonexistent", AuthorID: "u1", Body: "test"})
	if err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestGenerateID_HasPrefix(t *testing.T) {
	id := GenerateID("TKT-")
	if len(id) < 5 {
		t.Errorf("id too short: %q", id)
	}
	if id[:4] != "TKT-" {
		t.Errorf("id should start with TKT-, got %q", id)
	}
}

func TestListTickets_FilterByCategory(t *testing.T) {
	store := NewInMemoryStore()
	store.CreateTicket(context.Background(), Ticket{ID: "t1", Title: "T1", Description: "d", Category: "bug_report", Priority: "low", Status: "open", ReporterID: "user-1"})
	store.CreateTicket(context.Background(), Ticket{ID: "t2", Title: "T2", Description: "d", Category: "support", Priority: "low", Status: "open", ReporterID: "user-1"})

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	req := httptest.NewRequest("GET", "/api/v1/tickets?category=bug_report", nil)
	req = withClaims(req, userClaims("user-1"))
	rec := httptest.NewRecorder()

	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	resp := decodeBody(t, rec)
	count := resp["count"].(float64)
	if count != 1 {
		t.Errorf("count = %v, want 1", count)
	}
}

// --- Validation helpers tests ---

func TestIsValidCategory(t *testing.T) {
	valid := []string{"bug_report", "customization", "support", "feature_request"}
	for _, c := range valid {
		if !isValidCategory(c) {
			t.Errorf("isValidCategory(%q) = false, want true", c)
		}
	}
	if isValidCategory("invalid") {
		t.Error("isValidCategory(\"invalid\") = true, want false")
	}
}

func TestIsValidPriority(t *testing.T) {
	valid := []string{"low", "medium", "high", "critical"}
	for _, p := range valid {
		if !isValidPriority(p) {
			t.Errorf("isValidPriority(%q) = false, want true", p)
		}
	}
	if isValidPriority("ultra") {
		t.Error("isValidPriority(\"ultra\") = true, want false")
	}
}

func TestIsValidStatus(t *testing.T) {
	valid := []string{"open", "in_progress", "resolved", "closed"}
	for _, s := range valid {
		if !isValidStatus(s) {
			t.Errorf("isValidStatus(%q) = false, want true", s)
		}
	}
	if isValidStatus("invalid") {
		t.Error("isValidStatus(\"invalid\") = true, want false")
	}
}

func TestCreateTicket_AllCategories(t *testing.T) {
	categories := []string{"bug_report", "customization", "support", "feature_request"}
	for _, cat := range categories {
		rt, _ := setupRouter()
		body := `{"title":"Test","description":"desc","category":"` + cat + `"}`
		req := httptest.NewRequest("POST", "/api/v1/tickets", bytes.NewBufferString(body))
		req = withClaims(req, userClaims("user-1"))
		rec := httptest.NewRecorder()

		rt.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Errorf("category %q: status = %d, want %d", cat, rec.Code, http.StatusCreated)
		}
	}
}
