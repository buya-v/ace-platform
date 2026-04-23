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
	Update(order *types.SecurityOrder) error
}

// TradeStore defines the repository contract for security trades.
type TradeStore interface {
	Create(trade *types.SecurityTrade) error
	Get(id string) (*types.SecurityTrade, error)
	ListByInstrument(instrumentID string) ([]types.SecurityTrade, error)
}

// PositionStore defines the repository contract for participant positions.
type PositionStore interface {
	GetOrCreate(participantID, instrumentID string) (*types.Position, error)
	Update(position *types.Position) error
	List(participantID string) ([]types.Position, error)
}

// SettlementStore defines the repository contract for settlement obligations.
type SettlementStore interface {
	Create(obligation *types.SettlementObligation) error
	Get(id string) (*types.SettlementObligation, error)
	ListByDate(date string) ([]types.SettlementObligation, error)
	ListByStatus(status types.SettlementStatus) ([]types.SettlementObligation, error)
	UpdateStatus(id string, status types.SettlementStatus) error
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

// Update replaces an existing order record in the store.
func (s *InMemoryOrderStore) Update(order *types.SecurityOrder) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.data[order.ID]; !exists {
		return ErrNotFound
	}
	copy := *order
	s.data[order.ID] = &copy
	return nil
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

// --- InMemoryTradeStore ---

// InMemoryTradeStore is a thread-safe, in-memory implementation of TradeStore.
type InMemoryTradeStore struct {
	mu   sync.RWMutex
	data map[string]*types.SecurityTrade
}

// NewInMemoryTradeStore returns an empty InMemoryTradeStore.
func NewInMemoryTradeStore() *InMemoryTradeStore {
	return &InMemoryTradeStore{
		data: make(map[string]*types.SecurityTrade),
	}
}

// Create stores a new trade.
func (s *InMemoryTradeStore) Create(trade *types.SecurityTrade) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.data[trade.ID]; exists {
		return errors.New("trade already exists: " + trade.ID)
	}
	copy := *trade
	s.data[trade.ID] = &copy
	return nil
}

// Get retrieves a trade by its ID.
func (s *InMemoryTradeStore) Get(id string) (*types.SecurityTrade, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	trade, ok := s.data[id]
	if !ok {
		return nil, ErrNotFound
	}
	copy := *trade
	return &copy, nil
}

// ListByInstrument returns all trades for a given instrument.
func (s *InMemoryTradeStore) ListByInstrument(instrumentID string) ([]types.SecurityTrade, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]types.SecurityTrade, 0)
	for _, trade := range s.data {
		if trade.InstrumentID == instrumentID {
			result = append(result, *trade)
		}
	}
	return result, nil
}

// --- InMemoryPositionStore ---

// InMemoryPositionStore is a thread-safe, in-memory implementation of PositionStore.
// Key format: "participantID:instrumentID".
type InMemoryPositionStore struct {
	mu   sync.RWMutex
	data map[string]*types.Position
}

// NewInMemoryPositionStore returns an empty InMemoryPositionStore.
func NewInMemoryPositionStore() *InMemoryPositionStore {
	return &InMemoryPositionStore{
		data: make(map[string]*types.Position),
	}
}

// positionKey builds the composite map key.
func positionKey(participantID, instrumentID string) string {
	return participantID + ":" + instrumentID
}

// GetOrCreate retrieves an existing position or creates a new zero-quantity position.
func (s *InMemoryPositionStore) GetOrCreate(participantID, instrumentID string) (*types.Position, error) {
	key := positionKey(participantID, instrumentID)

	s.mu.Lock()
	defer s.mu.Unlock()

	pos, ok := s.data[key]
	if ok {
		copy := *pos
		return &copy, nil
	}

	// Create a new zero position.
	newPos := &types.Position{
		ID:            key,
		ParticipantID: participantID,
		InstrumentID:  instrumentID,
	}
	s.data[key] = newPos
	copy := *newPos
	return &copy, nil
}

// Update replaces an existing position record.
func (s *InMemoryPositionStore) Update(position *types.Position) error {
	key := positionKey(position.ParticipantID, position.InstrumentID)

	s.mu.Lock()
	defer s.mu.Unlock()

	copy := *position
	s.data[key] = &copy
	return nil
}

// List returns all positions for a given participant.
func (s *InMemoryPositionStore) List(participantID string) ([]types.Position, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]types.Position, 0)
	for _, pos := range s.data {
		if pos.ParticipantID == participantID {
			result = append(result, *pos)
		}
	}
	return result, nil
}

// --- InMemorySettlementStore ---

// InMemorySettlementStore is a thread-safe, in-memory implementation of SettlementStore.
type InMemorySettlementStore struct {
	mu   sync.RWMutex
	data map[string]*types.SettlementObligation
}

// NewInMemorySettlementStore returns an empty InMemorySettlementStore.
func NewInMemorySettlementStore() *InMemorySettlementStore {
	return &InMemorySettlementStore{
		data: make(map[string]*types.SettlementObligation),
	}
}

// Create stores a new settlement obligation.
func (s *InMemorySettlementStore) Create(obligation *types.SettlementObligation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.data[obligation.ID]; exists {
		return errors.New("settlement obligation already exists: " + obligation.ID)
	}
	copy := *obligation
	s.data[obligation.ID] = &copy
	return nil
}

// Get retrieves a settlement obligation by its ID.
func (s *InMemorySettlementStore) Get(id string) (*types.SettlementObligation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ob, ok := s.data[id]
	if !ok {
		return nil, ErrNotFound
	}
	copy := *ob
	return &copy, nil
}

// ListByDate returns all settlement obligations for a given settlement date.
func (s *InMemorySettlementStore) ListByDate(date string) ([]types.SettlementObligation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]types.SettlementObligation, 0)
	for _, ob := range s.data {
		if ob.SettlementDate == date {
			result = append(result, *ob)
		}
	}
	return result, nil
}

// ListByStatus returns all settlement obligations with the given status.
func (s *InMemorySettlementStore) ListByStatus(status types.SettlementStatus) ([]types.SettlementObligation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]types.SettlementObligation, 0)
	for _, ob := range s.data {
		if ob.Status == status {
			result = append(result, *ob)
		}
	}
	return result, nil
}

// UpdateStatus changes the settlement status of an obligation.
func (s *InMemorySettlementStore) UpdateStatus(id string, status types.SettlementStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ob, ok := s.data[id]
	if !ok {
		return ErrNotFound
	}
	ob.Status = status
	return nil
}
