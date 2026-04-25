package broker

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/garudax-platform/fix-gateway/internal/session"
)

// TestBrokerStore_Create_Get verifies that a broker can be created and retrieved by ID.
func TestBrokerStore_Create_Get(t *testing.T) {
	store := NewInMemoryStore()

	b := &FIXBroker{
		ID:       "BROKER-TEST-001",
		CompID:   "BRKR001",
		TenantID: "mse-equities",
		Name:     "Test Broker",
		Status:   "ACTIVE",
		Config:   map[string]string{"heartbeat_interval_sec": "30"},
	}

	if err := store.Create(b); err != nil {
		t.Fatalf("Create error: %v", err)
	}

	// Verify timestamps were set.
	if b.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set after Create")
	}
	if b.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set after Create")
	}

	got, err := store.GetByID("BROKER-TEST-001")
	if err != nil {
		t.Fatalf("GetByID error: %v", err)
	}
	if got.ID != "BROKER-TEST-001" {
		t.Errorf("ID: got %q, want BROKER-TEST-001", got.ID)
	}
	if got.CompID != "BRKR001" {
		t.Errorf("CompID: got %q, want BRKR001", got.CompID)
	}
	if got.TenantID != "mse-equities" {
		t.Errorf("TenantID: got %q, want mse-equities", got.TenantID)
	}
	if got.Status != "ACTIVE" {
		t.Errorf("Status: got %q, want ACTIVE", got.Status)
	}
	if got.Name != "Test Broker" {
		t.Errorf("Name: got %q, want Test Broker", got.Name)
	}
}

// TestBrokerStore_Create_Duplicate verifies that creating a broker with a duplicate ID returns an error.
func TestBrokerStore_Create_Duplicate(t *testing.T) {
	store := NewInMemoryStore()

	b := &FIXBroker{
		ID:       "DUP-001",
		CompID:   "DUP001",
		TenantID: "mse-equities",
		Name:     "Dup Broker",
		Status:   "ACTIVE",
	}

	if err := store.Create(b); err != nil {
		t.Fatalf("first Create error: %v", err)
	}

	// Create with same ID again.
	b2 := &FIXBroker{
		ID:       "DUP-001",
		CompID:   "DUP002",
		TenantID: "mse-equities",
		Name:     "Dup Broker 2",
		Status:   "ACTIVE",
	}
	if err := store.Create(b2); err == nil {
		t.Fatal("expected error for duplicate ID, got nil")
	}
}

// TestBrokerStore_GetByID_NotFound verifies that GetByID returns an error for unknown IDs.
func TestBrokerStore_GetByID_NotFound(t *testing.T) {
	store := NewInMemoryStore()
	_, err := store.GetByID("NONEXISTENT")
	if err == nil {
		t.Fatal("expected error for unknown ID, got nil")
	}
}

// TestBrokerStore_GetByCompID verifies the seeded MSE-BROKER-001 is retrievable by CompID.
func TestBrokerStore_GetByCompID(t *testing.T) {
	store := NewInMemoryStore()

	// MSE-BROKER-001 is seeded with CompID="MSE001".
	b, err := store.GetByCompID("MSE001")
	if err != nil {
		t.Fatalf("GetByCompID(MSE001) error: %v", err)
	}
	if b.ID != "MSE-BROKER-001" {
		t.Errorf("ID: got %q, want MSE-BROKER-001", b.ID)
	}
	if b.CompID != "MSE001" {
		t.Errorf("CompID: got %q, want MSE001", b.CompID)
	}
	if b.TenantID != "mse-equities" {
		t.Errorf("TenantID: got %q, want mse-equities", b.TenantID)
	}
	if b.Status != "ACTIVE" {
		t.Errorf("Status: got %q, want ACTIVE", b.Status)
	}
	if b.Config["heartbeat_interval_sec"] != "30" {
		t.Errorf("Config[heartbeat_interval_sec]: got %q, want 30", b.Config["heartbeat_interval_sec"])
	}
}

// TestBrokerStore_GetByCompID_NotFound verifies that GetByCompID returns an error for unknown CompID.
func TestBrokerStore_GetByCompID_NotFound(t *testing.T) {
	store := NewInMemoryStore()
	_, err := store.GetByCompID("UNKNOWNCOMP")
	if err == nil {
		t.Fatal("expected error for unknown CompID, got nil")
	}
}

// TestBrokerStore_List verifies that List returns all brokers including the seeded one.
func TestBrokerStore_List(t *testing.T) {
	store := NewInMemoryStore()

	// Initially contains only the seeded MSE-BROKER-001.
	brokers, err := store.List()
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(brokers) != 1 {
		t.Fatalf("List initial: got %d brokers, want 1 (seeded)", len(brokers))
	}
	if brokers[0].ID != "MSE-BROKER-001" {
		t.Errorf("List[0].ID: got %q, want MSE-BROKER-001", brokers[0].ID)
	}

	// Add two more brokers.
	for _, id := range []string{"BROKER-A", "BROKER-B"} {
		b := &FIXBroker{
			ID:       id,
			CompID:   id + "-COMP",
			TenantID: "mse-equities",
			Name:     id + " Name",
			Status:   "ACTIVE",
		}
		if err := store.Create(b); err != nil {
			t.Fatalf("Create %s error: %v", id, err)
		}
	}

	brokers, err = store.List()
	if err != nil {
		t.Fatalf("List error after creates: %v", err)
	}
	if len(brokers) != 3 {
		t.Errorf("List after 2 creates: got %d, want 3", len(brokers))
	}
}

// TestBrokerStore_UpdateStatus verifies that Update modifies existing broker fields.
func TestBrokerStore_UpdateStatus(t *testing.T) {
	store := NewInMemoryStore()

	b := &FIXBroker{
		ID:       "UPDATE-001",
		CompID:   "UPD001",
		TenantID: "mse-equities",
		Name:     "Before Update",
		Status:   "ACTIVE",
	}
	if err := store.Create(b); err != nil {
		t.Fatalf("Create error: %v", err)
	}

	// Record the original UpdatedAt.
	originalUpdated := b.UpdatedAt

	// Give it a short sleep so UpdatedAt can differ.
	time.Sleep(time.Millisecond)

	// Modify and update.
	b.Status = "SUSPENDED"
	b.Name = "After Update"
	if err := store.Update(b); err != nil {
		t.Fatalf("Update error: %v", err)
	}

	// Verify UpdatedAt advanced.
	if !b.UpdatedAt.After(originalUpdated) {
		t.Errorf("UpdatedAt should advance after Update; original=%v updated=%v", originalUpdated, b.UpdatedAt)
	}

	// Verify the change is reflected on retrieval.
	got, err := store.GetByID("UPDATE-001")
	if err != nil {
		t.Fatalf("GetByID after update error: %v", err)
	}
	if got.Status != "SUSPENDED" {
		t.Errorf("Status after update: got %q, want SUSPENDED", got.Status)
	}
	if got.Name != "After Update" {
		t.Errorf("Name after update: got %q, want After Update", got.Name)
	}
}

// TestBrokerStore_Update_NotFound verifies that updating an unknown broker returns an error.
func TestBrokerStore_Update_NotFound(t *testing.T) {
	store := NewInMemoryStore()
	b := &FIXBroker{
		ID:     "NONEXISTENT",
		CompID: "NONE",
		Status: "ACTIVE",
	}
	err := store.Update(b)
	if err == nil {
		t.Fatal("expected error when updating non-existent broker, got nil")
	}
}

// TestBrokerStore_SeededBroker_Config verifies all Config fields on the seeded broker.
func TestBrokerStore_SeededBroker_Config(t *testing.T) {
	store := NewInMemoryStore()
	b, err := store.GetByID("MSE-BROKER-001")
	if err != nil {
		t.Fatalf("GetByID seeded: %v", err)
	}

	expectedConfig := map[string]string{
		"heartbeat_interval_sec": "30",
		"max_message_size":       "8192",
		"market_data_enabled":    "true",
	}

	for k, want := range expectedConfig {
		got, ok := b.Config[k]
		if !ok {
			t.Errorf("Config key %q missing", k)
			continue
		}
		if got != want {
			t.Errorf("Config[%q]: got %q, want %q", k, got, want)
		}
	}
}

// TestBrokerStore_GetByCompID_AfterCreate verifies GetByCompID on a manually created broker.
func TestBrokerStore_GetByCompID_AfterCreate(t *testing.T) {
	store := NewInMemoryStore()

	b := &FIXBroker{
		ID:       "NEW-BROKER-001",
		CompID:   "NEWCOMP",
		TenantID: "ace-commodities",
		Name:     "New Broker",
		Status:   "ACTIVE",
	}
	if err := store.Create(b); err != nil {
		t.Fatalf("Create error: %v", err)
	}

	got, err := store.GetByCompID("NEWCOMP")
	if err != nil {
		t.Fatalf("GetByCompID error: %v", err)
	}
	if got.ID != "NEW-BROKER-001" {
		t.Errorf("ID: got %q, want NEW-BROKER-001", got.ID)
	}
	if got.TenantID != "ace-commodities" {
		t.Errorf("TenantID: got %q, want ace-commodities", got.TenantID)
	}
}

// TestBrokerStore_ConcurrentAccess verifies thread safety under concurrent reads and writes.
func TestBrokerStore_ConcurrentAccess(t *testing.T) {
	store := NewInMemoryStore()

	done := make(chan struct{})
	errCh := make(chan error, 10)

	// Concurrent creates.
	for i := 0; i < 5; i++ {
		i := i
		go func() {
			b := &FIXBroker{
				ID:       "CONCURRENT-" + string(rune('A'+i)),
				CompID:   "CONC" + string(rune('A'+i)),
				TenantID: "mse-equities",
				Status:   "ACTIVE",
			}
			if err := store.Create(b); err != nil {
				errCh <- err
			}
			done <- struct{}{}
		}()
	}

	// Concurrent reads.
	for i := 0; i < 5; i++ {
		go func() {
			_, _ = store.List()
			_, _ = store.GetByCompID("MSE001")
			done <- struct{}{}
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	select {
	case err := <-errCh:
		t.Errorf("concurrent operation error: %v", err)
	default:
	}

	brokers, err := store.List()
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	// 1 seeded + 5 concurrent creates = 6.
	if len(brokers) != 6 {
		t.Errorf("broker count after concurrent creates: got %d, want 6", len(brokers))
	}
}

// --- Handler tests ---

// newTestHandlers creates Handlers wired with a fresh InMemoryStore and SessionManager.
func newTestHandlers() *Handlers {
	store := NewInMemoryStore()
	mgr := session.NewSessionManager()
	return NewHandlers(store, mgr)
}

// TestHandlers_ListBrokers_GET verifies GET /api/v1/fix/brokers returns the seeded broker.
func TestHandlers_ListBrokers_GET(t *testing.T) {
	h := newTestHandlers()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/fix/brokers", nil)
	w := httptest.NewRecorder()

	h.handleBrokers(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}

	var brokers []FIXBroker
	if err := json.Unmarshal(w.Body.Bytes(), &brokers); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(brokers) != 1 {
		t.Errorf("broker count: got %d, want 1", len(brokers))
	}
}

// TestHandlers_CreateBroker_POST verifies POST /api/v1/fix/brokers creates a broker.
func TestHandlers_CreateBroker_POST(t *testing.T) {
	h := newTestHandlers()

	body, _ := json.Marshal(CreateBrokerRequest{
		ID:       "BROKER-NEW",
		CompID:   "BRKRNEW",
		TenantID: "mse-equities",
		Name:     "New Broker",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/fix/brokers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.handleBrokers(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status: got %d, want 201", w.Code)
	}

	var created FIXBroker
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.ID != "BROKER-NEW" {
		t.Errorf("created ID: got %q, want BROKER-NEW", created.ID)
	}
	if created.Status != "PENDING" {
		t.Errorf("created Status: got %q, want PENDING", created.Status)
	}
}

// TestHandlers_CreateBroker_AutoID verifies that a broker is created with an auto-generated ID
// when none is provided.
func TestHandlers_CreateBroker_AutoID(t *testing.T) {
	h := newTestHandlers()

	body, _ := json.Marshal(CreateBrokerRequest{
		CompID:   "AUTO001",
		TenantID: "ace-commodities",
		Name:     "Auto ID Broker",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/fix/brokers", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h.handleBrokers(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status: got %d, want 201", w.Code)
	}

	var created FIXBroker
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.ID == "" {
		t.Error("auto-generated ID should not be empty")
	}
}

// TestHandlers_CreateBroker_MissingFields verifies that missing required fields return 400.
func TestHandlers_CreateBroker_MissingFields(t *testing.T) {
	h := newTestHandlers()

	body, _ := json.Marshal(CreateBrokerRequest{
		ID: "BROKER-INCOMPLETE",
		// CompID, TenantID, Name all missing
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/fix/brokers", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h.handleBrokers(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", w.Code)
	}
}

// TestHandlers_CreateBroker_InvalidJSON verifies that malformed JSON returns 400.
func TestHandlers_CreateBroker_InvalidJSON(t *testing.T) {
	h := newTestHandlers()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/fix/brokers", bytes.NewReader([]byte("not-json")))
	w := httptest.NewRecorder()

	h.handleBrokers(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", w.Code)
	}
}

// TestHandlers_CreateBroker_Duplicate verifies that a duplicate broker returns 409.
func TestHandlers_CreateBroker_Duplicate(t *testing.T) {
	h := newTestHandlers()

	// MSE-BROKER-001 is already seeded.
	body, _ := json.Marshal(CreateBrokerRequest{
		ID:       "MSE-BROKER-001",
		CompID:   "MSE001",
		TenantID: "mse-equities",
		Name:     "Duplicate",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/fix/brokers", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h.handleBrokers(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("status: got %d, want 409", w.Code)
	}
}

// TestHandlers_ListBrokers_MethodNotAllowed verifies non-GET/POST returns 405.
func TestHandlers_ListBrokers_MethodNotAllowed(t *testing.T) {
	h := newTestHandlers()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/fix/brokers", nil)
	w := httptest.NewRecorder()

	h.handleBrokers(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status: got %d, want 405", w.Code)
	}
}

// TestHandlers_GetBrokerByID verifies GET /api/v1/fix/brokers/{id} returns a specific broker.
func TestHandlers_GetBrokerByID(t *testing.T) {
	h := newTestHandlers()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/fix/brokers/MSE-BROKER-001", nil)
	req.URL.Path = "/api/v1/fix/brokers/MSE-BROKER-001"
	w := httptest.NewRecorder()

	h.handleBrokerByID(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}

	var b FIXBroker
	if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if b.ID != "MSE-BROKER-001" {
		t.Errorf("ID: got %q, want MSE-BROKER-001", b.ID)
	}
}

// TestHandlers_GetBrokerByID_NotFound verifies 404 for unknown broker ID.
func TestHandlers_GetBrokerByID_NotFound(t *testing.T) {
	h := newTestHandlers()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/fix/brokers/UNKNOWN", nil)
	req.URL.Path = "/api/v1/fix/brokers/UNKNOWN"
	w := httptest.NewRecorder()

	h.handleBrokerByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", w.Code)
	}
}

// TestHandlers_GetBrokerByID_MissingID verifies 400 when no ID is in the path.
func TestHandlers_GetBrokerByID_MissingID(t *testing.T) {
	h := newTestHandlers()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/fix/brokers/", nil)
	req.URL.Path = "/api/v1/fix/brokers/"
	w := httptest.NewRecorder()

	h.handleBrokerByID(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", w.Code)
	}
}

// TestHandlers_UpdateBroker verifies PUT /api/v1/fix/brokers/{id} updates a broker.
func TestHandlers_UpdateBroker(t *testing.T) {
	h := newTestHandlers()

	body, _ := json.Marshal(map[string]interface{}{
		"status": "SUSPENDED",
		"name":   "Updated Name",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/fix/brokers/MSE-BROKER-001", bytes.NewReader(body))
	req.URL.Path = "/api/v1/fix/brokers/MSE-BROKER-001"
	w := httptest.NewRecorder()

	h.handleBrokerByID(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}

	var updated FIXBroker
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if updated.Status != "SUSPENDED" {
		t.Errorf("Status: got %q, want SUSPENDED", updated.Status)
	}
	if updated.Name != "Updated Name" {
		t.Errorf("Name: got %q, want Updated Name", updated.Name)
	}
}

// TestHandlers_UpdateBroker_NotFound verifies 404 when updating a non-existent broker.
func TestHandlers_UpdateBroker_NotFound(t *testing.T) {
	h := newTestHandlers()

	body, _ := json.Marshal(map[string]interface{}{"status": "SUSPENDED"})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/fix/brokers/UNKNOWN", bytes.NewReader(body))
	req.URL.Path = "/api/v1/fix/brokers/UNKNOWN"
	w := httptest.NewRecorder()

	h.handleBrokerByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", w.Code)
	}
}

// TestHandlers_UpdateBroker_InvalidJSON verifies 400 for malformed JSON body.
func TestHandlers_UpdateBroker_InvalidJSON(t *testing.T) {
	h := newTestHandlers()

	req := httptest.NewRequest(http.MethodPut, "/api/v1/fix/brokers/MSE-BROKER-001", bytes.NewReader([]byte("bad-json")))
	req.URL.Path = "/api/v1/fix/brokers/MSE-BROKER-001"
	w := httptest.NewRecorder()

	h.handleBrokerByID(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", w.Code)
	}
}

// TestHandlers_BrokerByID_MethodNotAllowed verifies 405 for unsupported methods on broker/{id}.
func TestHandlers_BrokerByID_MethodNotAllowed(t *testing.T) {
	h := newTestHandlers()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/fix/brokers/MSE-BROKER-001", nil)
	req.URL.Path = "/api/v1/fix/brokers/MSE-BROKER-001"
	w := httptest.NewRecorder()

	h.handleBrokerByID(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status: got %d, want 405", w.Code)
	}
}

// TestHandlers_ListSessions_Empty verifies GET /api/v1/fix/sessions returns empty list.
func TestHandlers_ListSessions_Empty(t *testing.T) {
	h := newTestHandlers()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/fix/sessions", nil)
	w := httptest.NewRecorder()

	h.handleSessions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}

	var sessions []SessionInfo
	if err := json.Unmarshal(w.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("sessions: got %d, want 0", len(sessions))
	}
}

// TestHandlers_ListSessions_WithSessions verifies sessions list includes created sessions.
func TestHandlers_ListSessions_WithSessions(t *testing.T) {
	store := NewInMemoryStore()
	mgr := session.NewSessionManager()
	mgr.CreateSession("BROKER001", "EXCHANGE", "mse-equities", 30)
	mgr.CreateSession("BROKER002", "EXCHANGE", "mse-equities", 60)
	h := NewHandlers(store, mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/fix/sessions", nil)
	w := httptest.NewRecorder()

	h.handleSessions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}

	var sessions []SessionInfo
	if err := json.Unmarshal(w.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("sessions: got %d, want 2", len(sessions))
	}
}

// TestHandlers_ListSessions_MethodNotAllowed verifies 405 for non-GET on sessions.
func TestHandlers_ListSessions_MethodNotAllowed(t *testing.T) {
	h := newTestHandlers()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/fix/sessions", nil)
	w := httptest.NewRecorder()

	h.handleSessions(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status: got %d, want 405", w.Code)
	}
}

// TestHandlers_RegisterRoutes verifies that routes are registered on the mux.
func TestHandlers_RegisterRoutes(t *testing.T) {
	h := newTestHandlers()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Test that the registered routes respond (not 404).
	paths := []string{
		"/api/v1/fix/brokers",
		"/api/v1/fix/sessions",
	}
	for _, path := range paths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code == http.StatusNotFound {
			t.Errorf("path %q: got 404, route not registered", path)
		}
	}
}
