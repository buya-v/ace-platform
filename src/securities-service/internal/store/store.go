// Package store defines repository interfaces and in-memory implementations
// for the securities-service.
package store

import (
	"errors"
	"sync"

	"github.com/garudax-platform/securities-service/internal/types"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

// InstrumentFilters carries optional filter parameters for listing instruments.
type InstrumentFilters struct {
	AssetClass    types.AssetClass
	TradingStatus types.TradingStatus
	ExchangeCode  string
}

// OrderFilters carries optional filter parameters for listing orders.
type OrderFilters struct {
	InstrumentID  string
	ParticipantID string
	Status        types.OrderStatus
}

// InstrumentUpdate carries the fields that may be patched on an existing instrument.
// Zero values are ignored (partial update semantics).
type InstrumentUpdate struct {
	Name              string
	TradingStatus     types.TradingStatus
	LotSize           int
	TickSize          float64
	OutstandingShares int64
}

// InstrumentStore defines the repository contract for instrument reference data.
type InstrumentStore interface {
	List(filters InstrumentFilters) ([]types.Instrument, error)
	Get(id string) (*types.Instrument, error)
	Create(instrument *types.Instrument) error
	Update(id string, partial InstrumentUpdate) error
	UpdateStatus(id string, status types.TradingStatus) error
}

// OrderStore defines the repository contract for securities orders.
type OrderStore interface {
	Submit(order *types.SecurityOrder) error
	Get(id string) (*types.SecurityOrder, error)
	List(filters OrderFilters) ([]types.SecurityOrder, error)
	Cancel(id string) error
}

// --- InMemoryInstrumentStore ---

// InMemoryInstrumentStore is a thread-safe, in-memory implementation of InstrumentStore.
type InMemoryInstrumentStore struct {
	mu   sync.RWMutex
	data map[string]*types.Instrument
}

// NewInMemoryInstrumentStore returns an empty InMemoryInstrumentStore.
func NewInMemoryInstrumentStore() *InMemoryInstrumentStore {
	return &InMemoryInstrumentStore{
		data: make(map[string]*types.Instrument),
	}
}

// List returns all instruments that match the given filters.
// A zero-value filter field means "no filter on this field".
func (s *InMemoryInstrumentStore) List(filters InstrumentFilters) ([]types.Instrument, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]types.Instrument, 0, len(s.data))
	for _, inst := range s.data {
		if filters.AssetClass != "" && inst.AssetClass != filters.AssetClass {
			continue
		}
		if filters.TradingStatus != "" && inst.TradingStatus != filters.TradingStatus {
			continue
		}
		if filters.ExchangeCode != "" && inst.ExchangeCode != filters.ExchangeCode {
			continue
		}
		result = append(result, *inst)
	}
	return result, nil
}

// Get retrieves an instrument by its ID.
func (s *InMemoryInstrumentStore) Get(id string) (*types.Instrument, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	inst, ok := s.data[id]
	if !ok {
		return nil, ErrNotFound
	}
	copy := *inst
	return &copy, nil
}

// Create stores a new instrument. Returns an error if one with the same ID already exists.
func (s *InMemoryInstrumentStore) Create(instrument *types.Instrument) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.data[instrument.ID]; exists {
		return errors.New("instrument already exists: " + instrument.ID)
	}
	copy := *instrument
	s.data[instrument.ID] = &copy
	return nil
}

// Update applies a partial update to an existing instrument.
func (s *InMemoryInstrumentStore) Update(id string, partial InstrumentUpdate) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	inst, ok := s.data[id]
	if !ok {
		return ErrNotFound
	}
	if partial.Name != "" {
		inst.Name = partial.Name
	}
	if partial.TradingStatus != "" {
		inst.TradingStatus = partial.TradingStatus
	}
	if partial.LotSize != 0 {
		inst.LotSize = partial.LotSize
	}
	if partial.TickSize != 0 {
		inst.TickSize = partial.TickSize
	}
	if partial.OutstandingShares != 0 {
		inst.OutstandingShares = partial.OutstandingShares
	}
	return nil
}

// UpdateStatus changes the trading status of an instrument.
func (s *InMemoryInstrumentStore) UpdateStatus(id string, status types.TradingStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	inst, ok := s.data[id]
	if !ok {
		return ErrNotFound
	}
	inst.TradingStatus = status
	return nil
}

// --- InMemoryOrderStore ---

// InMemoryOrderStore is a thread-safe, in-memory implementation of OrderStore.
type InMemoryOrderStore struct {
	mu   sync.RWMutex
	data map[string]*types.SecurityOrder
}

// NewInMemoryOrderStore returns an empty InMemoryOrderStore.
func NewInMemoryOrderStore() *InMemoryOrderStore {
	return &InMemoryOrderStore{
		data: make(map[string]*types.SecurityOrder),
	}
}

// Submit stores a new order. Returns an error if an order with the same ID already exists.
func (s *InMemoryOrderStore) Submit(order *types.SecurityOrder) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.data[order.ID]; exists {
		return errors.New("order already exists: " + order.ID)
	}
	copy := *order
	s.data[order.ID] = &copy
	return nil
}

// Get retrieves an order by its ID.
func (s *InMemoryOrderStore) Get(id string) (*types.SecurityOrder, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	order, ok := s.data[id]
	if !ok {
		return nil, ErrNotFound
	}
	copy := *order
	return &copy, nil
}

// List returns all orders that match the given filters.
func (s *InMemoryOrderStore) List(filters OrderFilters) ([]types.SecurityOrder, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]types.SecurityOrder, 0, len(s.data))
	for _, order := range s.data {
		if filters.InstrumentID != "" && order.InstrumentID != filters.InstrumentID {
			continue
		}
		if filters.ParticipantID != "" && order.ParticipantID != filters.ParticipantID {
			continue
		}
		if filters.Status != "" && order.Status != filters.Status {
			continue
		}
		result = append(result, *order)
	}
	return result, nil
}

// Cancel transitions an order to CANCELLED status.
// Returns ErrNotFound if the order does not exist.
// Returns an error if the order is already in a terminal state.
func (s *InMemoryOrderStore) Cancel(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	order, ok := s.data[id]
	if !ok {
		return ErrNotFound
	}
	switch order.Status {
	case types.OrderStatusFilled, types.OrderStatusCancelled,
		types.OrderStatusRejected, types.OrderStatusExpired:
		return errors.New("order is already in a terminal state: " + string(order.Status))
	}
	order.Status = types.OrderStatusCancelled
	return nil
}
