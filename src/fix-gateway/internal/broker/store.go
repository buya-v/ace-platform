package broker

import (
	"fmt"
	"sync"
	"time"
)

// FIXBroker represents a registered FIX broker-dealer.
type FIXBroker struct {
	ID        string            `json:"id"`
	CompID    string            `json:"comp_id"`
	TenantID  string            `json:"tenant_id"`
	Name      string            `json:"name"`
	Status    string            `json:"status"`
	Config    map[string]string `json:"config"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// BrokerStore defines the interface for broker persistence.
type BrokerStore interface {
	Create(broker *FIXBroker) error
	GetByID(id string) (*FIXBroker, error)
	GetByCompID(compID string) (*FIXBroker, error)
	Update(broker *FIXBroker) error
	List() ([]*FIXBroker, error)
}

// InMemoryStore is an in-memory implementation of BrokerStore.
type InMemoryStore struct {
	mu      sync.RWMutex
	brokers map[string]*FIXBroker // keyed by ID
}

// NewInMemoryStore creates a new InMemoryStore pre-seeded with MSE-BROKER-001.
func NewInMemoryStore() *InMemoryStore {
	s := &InMemoryStore{
		brokers: make(map[string]*FIXBroker),
	}

	// Seed default broker.
	now := time.Now()
	s.brokers["MSE-BROKER-001"] = &FIXBroker{
		ID:       "MSE-BROKER-001",
		CompID:   "MSE001",
		TenantID: "mse-equities",
		Name:     "MSE Default Broker",
		Status:   "ACTIVE",
		Config: map[string]string{
			"heartbeat_interval_sec": "30",
			"max_message_size":       "8192",
			"market_data_enabled":    "true",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	return s
}

// Create adds a new broker to the store.
func (s *InMemoryStore) Create(broker *FIXBroker) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.brokers[broker.ID]; exists {
		return fmt.Errorf("broker already exists: %s", broker.ID)
	}

	broker.CreatedAt = time.Now()
	broker.UpdatedAt = time.Now()
	s.brokers[broker.ID] = broker
	return nil
}

// GetByID retrieves a broker by its ID.
func (s *InMemoryStore) GetByID(id string) (*FIXBroker, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	b, ok := s.brokers[id]
	if !ok {
		return nil, fmt.Errorf("broker not found: %s", id)
	}
	return b, nil
}

// GetByCompID retrieves a broker by its FIX CompID.
func (s *InMemoryStore) GetByCompID(compID string) (*FIXBroker, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, b := range s.brokers {
		if b.CompID == compID {
			return b, nil
		}
	}
	return nil, fmt.Errorf("broker not found for CompID: %s", compID)
}

// Update updates an existing broker.
func (s *InMemoryStore) Update(broker *FIXBroker) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.brokers[broker.ID]; !exists {
		return fmt.Errorf("broker not found: %s", broker.ID)
	}

	broker.UpdatedAt = time.Now()
	s.brokers[broker.ID] = broker
	return nil
}

// List returns all brokers.
func (s *InMemoryStore) List() ([]*FIXBroker, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*FIXBroker, 0, len(s.brokers))
	for _, b := range s.brokers {
		result = append(result, b)
	}
	return result, nil
}
