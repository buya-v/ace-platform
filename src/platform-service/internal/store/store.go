// Package store provides tenant data access abstractions and in-memory implementations.
package store

import (
	"fmt"
	"sync"
	"time"

	"github.com/garudax-platform/platform-service/internal/types"
)

// TenantStore defines the data access interface for tenant registry operations.
type TenantStore interface {
	// List returns all tenants.
	List() ([]types.Tenant, error)
	// Get returns the tenant with the given ID, or an error if not found.
	Get(id string) (*types.Tenant, error)
	// Create persists a new tenant record.
	Create(t *types.Tenant) error
	// Update applies the given field updates to the tenant with the given ID.
	Update(id string, updates map[string]interface{}) error
	// UpdateStatus changes the status of the tenant with the given ID.
	UpdateStatus(id, status string) error
}

// InMemoryTenantStore is a thread-safe in-memory implementation of TenantStore.
// Seeded with the two known GarudaX tenants from V29__platform_schemas.sql.
type InMemoryTenantStore struct {
	mu      sync.RWMutex
	tenants map[string]*types.Tenant
}

// NewInMemoryTenantStore creates a store seeded with the two known platform tenants.
func NewInMemoryTenantStore() *InMemoryTenantStore {
	now := time.Now().UTC().Format(time.RFC3339)
	s := &InMemoryTenantStore{
		tenants: make(map[string]*types.Tenant),
	}
	s.tenants["ace-commodities"] = &types.Tenant{
		ID:                 "ace-commodities",
		Name:               "ACE Commodity Exchange",
		Status:             types.TenantStatusActive,
		Flagship:           false,
		GovernanceTier:     "STANDARD",
		OnboardingMetadata: map[string]interface{}{},
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	s.tenants["mse-equities"] = &types.Tenant{
		ID:                 "mse-equities",
		Name:               "Mongolian Stock Exchange",
		Status:             types.TenantStatusOnboarding,
		Flagship:           true,
		GovernanceTier:     "FLAGSHIP",
		OnboardingMetadata: map[string]interface{}{},
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	return s
}

// List returns all tenants in the store.
func (s *InMemoryTenantStore) List() ([]types.Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]types.Tenant, 0, len(s.tenants))
	for _, t := range s.tenants {
		cp := *t
		result = append(result, cp)
	}
	return result, nil
}

// Get returns the tenant with the given ID. Returns an error if not found.
func (s *InMemoryTenantStore) Get(id string) (*types.Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tenants[id]
	if !ok {
		return nil, fmt.Errorf("tenant not found: %s", id)
	}
	cp := *t
	return &cp, nil
}

// Create persists a new tenant record. Returns an error if the ID already exists.
func (s *InMemoryTenantStore) Create(t *types.Tenant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.tenants[t.ID]; exists {
		return fmt.Errorf("tenant already exists: %s", t.ID)
	}
	cp := *t
	s.tenants[t.ID] = &cp
	return nil
}

// Update applies the given field updates to the identified tenant.
// Supported update keys: "name", "governance_tier".
func (s *InMemoryTenantStore) Update(id string, updates map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tenants[id]
	if !ok {
		return fmt.Errorf("tenant not found: %s", id)
	}
	if v, ok := updates["name"]; ok {
		if name, ok := v.(string); ok {
			t.Name = name
		}
	}
	if v, ok := updates["governance_tier"]; ok {
		if tier, ok := v.(string); ok {
			t.GovernanceTier = tier
		}
	}
	t.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return nil
}

// UpdateStatus changes the status field of the identified tenant.
func (s *InMemoryTenantStore) UpdateStatus(id, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tenants[id]
	if !ok {
		return fmt.Errorf("tenant not found: %s", id)
	}
	t.Status = status
	t.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return nil
}
