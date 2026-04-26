// Package store — RoleStore interface and in-memory implementation.
package store

import (
	"fmt"
	"sync"
	"time"

	"github.com/garudax-platform/securities-service/internal/types"
)

// RoleStore defines the repository contract for RBAC roles.
type RoleStore interface {
	// Get retrieves a role by ID.
	Get(id string) (*types.Role, error)
	// List returns all roles.
	List() ([]types.Role, error)
	// Create stores a new role.
	Create(role *types.Role) error
	// Update replaces the mutable fields of an existing role.
	Update(id string, name, description string, permissions []string) error
	// Delete removes a role by ID.
	Delete(id string) error
}

// InMemoryRoleStore is a thread-safe, in-memory implementation of RoleStore.
type InMemoryRoleStore struct {
	mu   sync.RWMutex
	data map[string]*types.Role
}

// NewInMemoryRoleStore returns an empty InMemoryRoleStore.
func NewInMemoryRoleStore() *InMemoryRoleStore {
	return &InMemoryRoleStore{
		data: make(map[string]*types.Role),
	}
}

// Get retrieves a role by ID.
func (s *InMemoryRoleStore) Get(id string) (*types.Role, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.data[id]
	if !ok {
		return nil, ErrNotFound
	}
	copy := *r
	perms := make([]string, len(r.Permissions))
	copySlice(perms, r.Permissions)
	copy.Permissions = perms
	return &copy, nil
}

// List returns all stored roles.
func (s *InMemoryRoleStore) List() ([]types.Role, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.Role, 0, len(s.data))
	for _, r := range s.data {
		cp := *r
		perms := make([]string, len(r.Permissions))
		copySlice(perms, r.Permissions)
		cp.Permissions = perms
		out = append(out, cp)
	}
	return out, nil
}

// Create stores a new role, returning an error if the ID already exists.
func (s *InMemoryRoleStore) Create(role *types.Role) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.data[role.ID]; exists {
		return fmt.Errorf("role %s already exists", role.ID)
	}
	cp := *role
	perms := make([]string, len(role.Permissions))
	copySlice(perms, role.Permissions)
	cp.Permissions = perms
	s.data[role.ID] = &cp
	return nil
}

// Update replaces the mutable fields of a role identified by id.
// Passing an empty name or nil permissions preserves existing values.
func (s *InMemoryRoleStore) Update(id string, name, description string, permissions []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.data[id]
	if !ok {
		return ErrNotFound
	}
	if name != "" {
		r.Name = name
	}
	r.Description = description
	if permissions != nil {
		perms := make([]string, len(permissions))
		copySlice(perms, permissions)
		r.Permissions = perms
	}
	r.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return nil
}

// Delete removes a role by ID.
func (s *InMemoryRoleStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[id]; !ok {
		return ErrNotFound
	}
	delete(s.data, id)
	return nil
}

// copySlice copies src into dst using the built-in copy.
// Named to avoid shadowing the built-in inside struct methods.
func copySlice(dst, src []string) {
	copy(dst, src)
}
