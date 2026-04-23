// Package store_test exercises InMemoryTenantStore.
package store_test

import (
	"testing"

	"github.com/garudax-platform/platform-service/internal/store"
	"github.com/garudax-platform/platform-service/internal/types"
)

// newStore is a helper that always returns a freshly-seeded store.
func newStore() *store.InMemoryTenantStore {
	return store.NewInMemoryTenantStore()
}

// TestTenantStore_List_Seeded verifies the store is seeded with the two known tenants.
func TestTenantStore_List_Seeded(t *testing.T) {
	s := newStore()
	tenants, err := s.List()
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(tenants) != 2 {
		t.Fatalf("List() returned %d tenants, want 2", len(tenants))
	}
	ids := map[string]bool{}
	for _, tn := range tenants {
		ids[tn.ID] = true
	}
	for _, want := range []string{"ace-commodities", "mse-equities"} {
		if !ids[want] {
			t.Errorf("List() missing tenant %q", want)
		}
	}
}

// TestTenantStore_Get_Existing verifies a seeded tenant can be retrieved.
func TestTenantStore_Get_Existing(t *testing.T) {
	s := newStore()
	tn, err := s.Get("ace-commodities")
	if err != nil {
		t.Fatalf("Get(ace-commodities) unexpected error: %v", err)
	}
	if tn == nil {
		t.Fatal("Get(ace-commodities) returned nil tenant")
	}
	if tn.ID != "ace-commodities" {
		t.Errorf("Get() ID = %q, want %q", tn.ID, "ace-commodities")
	}
	if tn.Status != types.TenantStatusActive {
		t.Errorf("Get() Status = %q, want %q", tn.Status, types.TenantStatusActive)
	}
}

// TestTenantStore_Get_NotFound verifies that Get returns an error for an unknown ID.
func TestTenantStore_Get_NotFound(t *testing.T) {
	s := newStore()
	tn, err := s.Get("nonexistent")
	if err == nil {
		t.Fatal("Get(nonexistent) expected error, got nil")
	}
	if tn != nil {
		t.Errorf("Get(nonexistent) returned non-nil tenant on error")
	}
}

// TestTenantStore_Create_Valid verifies a new tenant can be created and retrieved.
func TestTenantStore_Create_Valid(t *testing.T) {
	s := newStore()
	newTenant := &types.Tenant{
		ID:             "test-exchange",
		Name:           "Test Exchange",
		Status:         types.TenantStatusOnboarding,
		GovernanceTier: "STANDARD",
	}
	if err := s.Create(newTenant); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	tenants, _ := s.List()
	if len(tenants) != 3 {
		t.Errorf("List() after Create returned %d tenants, want 3", len(tenants))
	}

	got, err := s.Get("test-exchange")
	if err != nil {
		t.Fatalf("Get(test-exchange) after Create unexpected error: %v", err)
	}
	if got.Name != "Test Exchange" {
		t.Errorf("Get() Name = %q, want %q", got.Name, "Test Exchange")
	}
}

// TestTenantStore_Create_Duplicate verifies that creating with an existing ID returns an error.
func TestTenantStore_Create_Duplicate(t *testing.T) {
	s := newStore()
	dup := &types.Tenant{
		ID:   "ace-commodities",
		Name: "Duplicate",
	}
	err := s.Create(dup)
	if err == nil {
		t.Fatal("Create(duplicate ID) expected error, got nil")
	}
}

// TestTenantStore_Update verifies that the name field can be updated.
func TestTenantStore_Update(t *testing.T) {
	s := newStore()
	err := s.Update("ace-commodities", map[string]interface{}{
		"name": "Updated Name",
	})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}

	tn, _ := s.Get("ace-commodities")
	if tn.Name != "Updated Name" {
		t.Errorf("after Update, Name = %q, want %q", tn.Name, "Updated Name")
	}
}

// TestTenantStore_Update_NotFound verifies that updating an unknown ID returns an error.
func TestTenantStore_Update_NotFound(t *testing.T) {
	s := newStore()
	err := s.Update("nonexistent", map[string]interface{}{"name": "x"})
	if err == nil {
		t.Fatal("Update(nonexistent) expected error, got nil")
	}
}

// TestTenantStore_UpdateStatus_Valid verifies that status can be promoted from ONBOARDING to ACTIVE.
func TestTenantStore_UpdateStatus_Valid(t *testing.T) {
	s := newStore()
	// mse-equities is seeded as ONBOARDING.
	err := s.UpdateStatus("mse-equities", types.TenantStatusActive)
	if err != nil {
		t.Fatalf("UpdateStatus() unexpected error: %v", err)
	}

	tn, _ := s.Get("mse-equities")
	if tn.Status != types.TenantStatusActive {
		t.Errorf("after UpdateStatus, Status = %q, want %q", tn.Status, types.TenantStatusActive)
	}
}

// TestTenantStore_UpdateStatus_InvalidStatus verifies that the store accepts any non-empty string
// (validation is the handler's responsibility) — so an "invalid" status is stored as-is.
// This tests the store contract: it persists whatever status string is given.
func TestTenantStore_UpdateStatus_InvalidStatus(t *testing.T) {
	s := newStore()
	// The store itself does not validate the status value; that is the handler's job.
	// Call with an unrecognised status and confirm it is persisted (store accepts it).
	err := s.UpdateStatus("ace-commodities", "NOT_A_REAL_STATUS")
	if err != nil {
		t.Fatalf("UpdateStatus with unrecognised status unexpected error: %v", err)
	}
	tn, _ := s.Get("ace-commodities")
	if tn.Status != "NOT_A_REAL_STATUS" {
		t.Errorf("Status = %q, want %q", tn.Status, "NOT_A_REAL_STATUS")
	}
}

// TestTenantStore_UpdateStatus_NotFound verifies that updating the status of an unknown ID returns an error.
func TestTenantStore_UpdateStatus_NotFound(t *testing.T) {
	s := newStore()
	err := s.UpdateStatus("nonexistent", types.TenantStatusActive)
	if err == nil {
		t.Fatal("UpdateStatus(nonexistent) expected error, got nil")
	}
}
