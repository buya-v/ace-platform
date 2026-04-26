// Package store defines repository interfaces and in-memory implementations
// for the securities-service.
package store

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/garudax-platform/securities-service/internal/types"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

// InstrumentFilters carries optional filter parameters for listing instruments.
type InstrumentFilters struct {
	AssetClass    types.AssetClass
	TradingStatus types.TradingStatus
	ExchangeCode  string
	SegmentID     string
	Search        string // ILIKE on ticker and name
	Limit         int
	Offset        int
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
	DeletionStatus    string // P3b: "FLAGGED" when instrument is marked for deletion
	DeletionDate      string // P3b: ISO date 30 days after flagging
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
	List() ([]types.SecurityTrade, error)
	ListByInstrument(instrumentID string) ([]types.SecurityTrade, error)
	UpdateStatus(id string, status types.TradeStatus) error
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
	Update(obligation *types.SettlementObligation) error
}

// FirmStore defines the repository contract for member firms.
type FirmStore interface {
	Get(id string) (*types.Firm, error)
	List() ([]types.Firm, error)
	Create(f *types.Firm) error
	UpdateStatus(id string, status types.FirmStatus) error
}

// ParticipantFilters carries optional filter parameters for listing participants.
type ParticipantFilters struct {
	FirmID string
}

// ParticipantStore defines the repository contract for exchange participants.
type ParticipantStore interface {
	Get(id string) (*types.ExchangeParticipant, error)
	List(filters ParticipantFilters) ([]types.ExchangeParticipant, error)
	Create(p *types.ExchangeParticipant) error
	UpdateStatus(id string, status types.ParticipantStatus) error
	UpdatePermissions(id string, permissions []string) error
}

// InMemoryParticipantStore is a thread-safe, in-memory implementation of ParticipantStore.
type InMemoryParticipantStore struct {
	mu   sync.RWMutex
	data map[string]*types.ExchangeParticipant
}

// NewInMemoryParticipantStore returns an empty InMemoryParticipantStore.
func NewInMemoryParticipantStore() *InMemoryParticipantStore {
	return &InMemoryParticipantStore{
		data: make(map[string]*types.ExchangeParticipant),
	}
}

// Get retrieves a participant by ID.
func (s *InMemoryParticipantStore) Get(id string) (*types.ExchangeParticipant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.data[id]
	if !ok {
		return nil, ErrNotFound
	}
	copy := *p
	return &copy, nil
}

// List returns participants, optionally filtered by FirmID.
func (s *InMemoryParticipantStore) List(filters ParticipantFilters) ([]types.ExchangeParticipant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.ExchangeParticipant, 0, len(s.data))
	for _, p := range s.data {
		if filters.FirmID != "" && p.FirmID != filters.FirmID {
			continue
		}
		out = append(out, *p)
	}
	return out, nil
}

// Create stores a new participant.
func (s *InMemoryParticipantStore) Create(p *types.ExchangeParticipant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.data[p.ID]; exists {
		return fmt.Errorf("participant %s already exists", p.ID)
	}
	copy := *p
	s.data[p.ID] = &copy
	return nil
}

// UpdateStatus changes the status of a participant.
func (s *InMemoryParticipantStore) UpdateStatus(id string, status types.ParticipantStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.data[id]
	if !ok {
		return ErrNotFound
	}
	p.Status = status
	p.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return nil
}

// UpdatePermissions replaces the permissions slice of a participant.
func (s *InMemoryParticipantStore) UpdatePermissions(id string, permissions []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.data[id]
	if !ok {
		return ErrNotFound
	}
	perms := make([]string, len(permissions))
	copy(perms, permissions)
	p.Permissions = perms
	p.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return nil
}

// --- InMemoryFirmStore ---

// InMemoryFirmStore is a thread-safe, in-memory implementation of FirmStore.
type InMemoryFirmStore struct {
	mu   sync.RWMutex
	data map[string]*types.Firm
}

// NewInMemoryFirmStore returns an empty InMemoryFirmStore.
func NewInMemoryFirmStore() *InMemoryFirmStore {
	return &InMemoryFirmStore{
		data: make(map[string]*types.Firm),
	}
}

// Get retrieves a firm by ID.
func (s *InMemoryFirmStore) Get(id string) (*types.Firm, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, ok := s.data[id]
	if !ok {
		return nil, ErrNotFound
	}
	c := *f
	return &c, nil
}

// List returns all firms.
func (s *InMemoryFirmStore) List() ([]types.Firm, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.Firm, 0, len(s.data))
	for _, f := range s.data {
		out = append(out, *f)
	}
	return out, nil
}

// Create stores a new firm.
func (s *InMemoryFirmStore) Create(f *types.Firm) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.data[f.ID]; exists {
		return fmt.Errorf("firm %s already exists", f.ID)
	}
	c := *f
	s.data[f.ID] = &c
	return nil
}

// UpdateStatus changes the status of a firm.
func (s *InMemoryFirmStore) UpdateStatus(id string, status types.FirmStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.data[id]
	if !ok {
		return ErrNotFound
	}
	f.Status = status
	f.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return nil
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
	if partial.DeletionStatus != "" {
		inst.DeletionStatus = partial.DeletionStatus
	}
	if partial.DeletionDate != "" {
		inst.DeletionDate = partial.DeletionDate
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

// List returns all trades.
func (s *InMemoryTradeStore) List() ([]types.SecurityTrade, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]types.SecurityTrade, 0, len(s.data))
	for _, trade := range s.data {
		result = append(result, *trade)
	}
	return result, nil
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

// UpdateStatus changes the status of a trade.
func (s *InMemoryTradeStore) UpdateStatus(id string, status types.TradeStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	trade, ok := s.data[id]
	if !ok {
		return ErrNotFound
	}
	trade.Status = status
	return nil
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
// If participantID is empty, all positions are returned.
func (s *InMemoryPositionStore) List(participantID string) ([]types.Position, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]types.Position, 0)
	for _, pos := range s.data {
		if participantID != "" && pos.ParticipantID != participantID {
			continue
		}
		result = append(result, *pos)
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

// Update replaces the stored obligation with the provided copy.
func (s *InMemorySettlementStore) Update(obligation *types.SettlementObligation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.data[obligation.ID]; !ok {
		return ErrNotFound
	}
	cp := *obligation
	s.data[obligation.ID] = &cp
	return nil
}

// --- CorporateActionStore ---

// CorporateActionFilters carries optional filter parameters for listing corporate actions.
type CorporateActionFilters struct {
	InstrumentID string
	ActionType   types.CorporateActionType
}

// CorporateActionStore defines the repository contract for corporate actions.
type CorporateActionStore interface {
	Create(ca *types.CorporateAction) error
	Get(id string) (*types.CorporateAction, error)
	List(filters CorporateActionFilters) ([]types.CorporateAction, error)
	UpdateStatus(id string, status types.CorporateActionStatus) error
}

// --- EntitlementStore ---

// EntitlementStore defines the repository contract for corporate action entitlements.
type EntitlementStore interface {
	Create(e *types.Entitlement) error
	ListByAction(corporateActionID string) ([]types.Entitlement, error)
	ListByParticipant(participantID string) ([]types.Entitlement, error)
}

// --- InMemoryCorporateActionStore ---

// InMemoryCorporateActionStore is a thread-safe, in-memory implementation of CorporateActionStore.
type InMemoryCorporateActionStore struct {
	mu   sync.RWMutex
	data map[string]*types.CorporateAction
}

// NewInMemoryCorporateActionStore returns an empty InMemoryCorporateActionStore.
func NewInMemoryCorporateActionStore() *InMemoryCorporateActionStore {
	return &InMemoryCorporateActionStore{
		data: make(map[string]*types.CorporateAction),
	}
}

// Create stores a new corporate action.
func (s *InMemoryCorporateActionStore) Create(ca *types.CorporateAction) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.data[ca.ID]; exists {
		return errors.New("corporate action already exists: " + ca.ID)
	}
	copy := *ca
	s.data[ca.ID] = &copy
	return nil
}

// Get retrieves a corporate action by its ID.
func (s *InMemoryCorporateActionStore) Get(id string) (*types.CorporateAction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ca, ok := s.data[id]
	if !ok {
		return nil, ErrNotFound
	}
	copy := *ca
	return &copy, nil
}

// List returns all corporate actions that match the given filters.
func (s *InMemoryCorporateActionStore) List(filters CorporateActionFilters) ([]types.CorporateAction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]types.CorporateAction, 0, len(s.data))
	for _, ca := range s.data {
		if filters.InstrumentID != "" && ca.InstrumentID != filters.InstrumentID {
			continue
		}
		if filters.ActionType != "" && ca.ActionType != filters.ActionType {
			continue
		}
		result = append(result, *ca)
	}
	return result, nil
}

// UpdateStatus changes the status of a corporate action.
func (s *InMemoryCorporateActionStore) UpdateStatus(id string, status types.CorporateActionStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ca, ok := s.data[id]
	if !ok {
		return ErrNotFound
	}
	ca.Status = status
	return nil
}

// --- InMemoryEntitlementStore ---

// InMemoryEntitlementStore is a thread-safe, in-memory implementation of EntitlementStore.
type InMemoryEntitlementStore struct {
	mu   sync.RWMutex
	data map[string]*types.Entitlement
}

// NewInMemoryEntitlementStore returns an empty InMemoryEntitlementStore.
func NewInMemoryEntitlementStore() *InMemoryEntitlementStore {
	return &InMemoryEntitlementStore{
		data: make(map[string]*types.Entitlement),
	}
}

// Create stores a new entitlement.
func (s *InMemoryEntitlementStore) Create(e *types.Entitlement) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.data[e.ID]; exists {
		return errors.New("entitlement already exists: " + e.ID)
	}
	copy := *e
	s.data[e.ID] = &copy
	return nil
}

// ListByAction returns all entitlements for a given corporate action.
func (s *InMemoryEntitlementStore) ListByAction(corporateActionID string) ([]types.Entitlement, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]types.Entitlement, 0)
	for _, e := range s.data {
		if e.CorporateActionID == corporateActionID {
			result = append(result, *e)
		}
	}
	return result, nil
}

// ListByParticipant returns all entitlements for a given participant.
func (s *InMemoryEntitlementStore) ListByParticipant(participantID string) ([]types.Entitlement, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]types.Entitlement, 0)
	for _, e := range s.data {
		if e.ParticipantID == participantID {
			result = append(result, *e)
		}
	}
	return result, nil
}

// ── Market Store (MillenniumIT P1) ───────────────────────────────────────────

type MarketStore interface {
	Create(m *types.Market) error
	Get(id string) (*types.Market, error)
	List() ([]types.Market, error)
	UpdateStatus(id, status string) error
}

type InMemoryMarketStore struct {
	mu   sync.RWMutex
	data map[string]*types.Market
}

func NewInMemoryMarketStore() *InMemoryMarketStore {
	s := &InMemoryMarketStore{data: make(map[string]*types.Market)}
	now := time.Now().UTC().Format(time.RFC3339)
	s.data["MSE"] = &types.Market{ID: "MSE", Name: "Mongolian Stock Exchange", Status: types.MarketActive, Timezone: "Asia/Ulaanbaatar", CreatedAt: now, UpdatedAt: now}
	return s
}

func (s *InMemoryMarketStore) Create(m *types.Market) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.data[m.ID]; exists { return fmt.Errorf("market %s already exists", m.ID) }
	s.data[m.ID] = m
	return nil
}

func (s *InMemoryMarketStore) Get(id string) (*types.Market, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.data[id]
	if !ok { return nil, fmt.Errorf("market %s not found", id) }
	c := *m; return &c, nil
}

func (s *InMemoryMarketStore) List() ([]types.Market, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.Market, 0, len(s.data))
	for _, m := range s.data { out = append(out, *m) }
	return out, nil
}

func (s *InMemoryMarketStore) UpdateStatus(id, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.data[id]
	if !ok { return fmt.Errorf("market %s not found", id) }
	m.Status = status; m.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return nil
}

// ── Segment Store (MillenniumIT P1) ──────────────────────────────────────────

type SegmentStore interface {
	Create(seg *types.Segment) error
	Get(id string) (*types.Segment, error)
	ListByMarket(marketID string) ([]types.Segment, error)
	UpdateStatus(id, status string) error
}

type InMemorySegmentStore struct {
	mu   sync.RWMutex
	data map[string]*types.Segment
}

func NewInMemorySegmentStore() *InMemorySegmentStore {
	s := &InMemorySegmentStore{data: make(map[string]*types.Segment)}
	now := time.Now().UTC().Format(time.RFC3339)
	s.data["EQUITY"] = &types.Segment{ID: "EQUITY", MarketID: "MSE", Name: "Equities", Status: types.SegActive, CreatedAt: now, UpdatedAt: now}
	return s
}

func (s *InMemorySegmentStore) Create(seg *types.Segment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.data[seg.ID]; exists { return fmt.Errorf("segment %s already exists", seg.ID) }
	s.data[seg.ID] = seg
	return nil
}

func (s *InMemorySegmentStore) Get(id string) (*types.Segment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seg, ok := s.data[id]
	if !ok { return nil, fmt.Errorf("segment %s not found", id) }
	c := *seg; return &c, nil
}

func (s *InMemorySegmentStore) ListByMarket(marketID string) ([]types.Segment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []types.Segment
	for _, seg := range s.data {
		if marketID == "" || seg.MarketID == marketID { out = append(out, *seg) }
	}
	return out, nil
}

func (s *InMemorySegmentStore) UpdateStatus(id, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	seg, ok := s.data[id]
	if !ok { return fmt.Errorf("segment %s not found", id) }
	seg.Status = status; seg.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return nil
}

// ── Circuit Breaker Store (MillenniumIT P1) ──────────────────────────────────

type CircuitBreakerStore interface {
	Get(instrumentID string) (*types.CircuitBreaker, error)
	Set(cb *types.CircuitBreaker) error
	List() ([]types.CircuitBreaker, error)
	UpdateStatus(instrumentID, status string) error
	UpdateLastPrice(instrumentID string, price float64) error
	Delete(instrumentID string) error
}

type InMemoryCircuitBreakerStore struct {
	mu   sync.RWMutex
	data map[string]*types.CircuitBreaker
}

func NewInMemoryCircuitBreakerStore() *InMemoryCircuitBreakerStore {
	return &InMemoryCircuitBreakerStore{data: make(map[string]*types.CircuitBreaker)}
}

func (s *InMemoryCircuitBreakerStore) Get(instrumentID string) (*types.CircuitBreaker, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cb, ok := s.data[instrumentID]
	if !ok { return nil, nil }
	c := *cb; return &c, nil
}

func (s *InMemoryCircuitBreakerStore) Set(cb *types.CircuitBreaker) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[cb.InstrumentID] = cb
	return nil
}

func (s *InMemoryCircuitBreakerStore) List() ([]types.CircuitBreaker, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.CircuitBreaker, 0, len(s.data))
	for _, cb := range s.data { out = append(out, *cb) }
	return out, nil
}

func (s *InMemoryCircuitBreakerStore) UpdateStatus(instrumentID, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cb, ok := s.data[instrumentID]
	if !ok { return fmt.Errorf("circuit breaker for %s not found", instrumentID) }
	cb.Status = status
	if status == types.CBTriggered { cb.TriggeredAt = time.Now().UTC().Format(time.RFC3339) }
	return nil
}

func (s *InMemoryCircuitBreakerStore) UpdateLastPrice(instrumentID string, price float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cb, ok := s.data[instrumentID]
	if !ok { return nil }
	cb.LastTradedPrice = price
	return nil
}

func (s *InMemoryCircuitBreakerStore) Delete(instrumentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, instrumentID)
	return nil
}

// ── TradeCorrectionStore ──────────────────────────────────────────────────────

// TradeCorrectionStore defines the repository contract for trade corrections.
type TradeCorrectionStore interface {
	Create(correction *types.TradeCorrection) error
	ListByTrade(tradeID string) ([]types.TradeCorrection, error)
}

// InMemoryTradeCorrectionStore is a thread-safe, in-memory implementation of TradeCorrectionStore.
type InMemoryTradeCorrectionStore struct {
	mu   sync.RWMutex
	data []types.TradeCorrection
}

// NewInMemoryTradeCorrectionStore returns an empty InMemoryTradeCorrectionStore.
func NewInMemoryTradeCorrectionStore() *InMemoryTradeCorrectionStore {
	return &InMemoryTradeCorrectionStore{}
}

// Create appends a new trade correction record.
func (s *InMemoryTradeCorrectionStore) Create(correction *types.TradeCorrection) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := *correction
	s.data = append(s.data, c)
	return nil
}

// ListByTrade returns all corrections for a given trade ID.
func (s *InMemoryTradeCorrectionStore) ListByTrade(tradeID string) ([]types.TradeCorrection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]types.TradeCorrection, 0)
	for _, c := range s.data {
		if c.TradeID == tradeID {
			result = append(result, c)
		}
	}
	return result, nil
}

// ── TickTableStore ────────────────────────────────────────────────────────────

// TickTableStore defines the repository contract for tiered tick tables.
type TickTableStore interface {
	Get(instrumentID string) (*types.TickTable, error)
	List() ([]types.TickTable, error)
	Set(table *types.TickTable) error
	Delete(instrumentID string) error
}

// InMemoryTickTableStore is a thread-safe, in-memory implementation of TickTableStore.
type InMemoryTickTableStore struct {
	mu   sync.RWMutex
	data map[string]*types.TickTable
}

// NewInMemoryTickTableStore returns an empty InMemoryTickTableStore.
func NewInMemoryTickTableStore() *InMemoryTickTableStore {
	return &InMemoryTickTableStore{data: make(map[string]*types.TickTable)}
}

// Get retrieves a tick table by instrument ID. Returns ErrNotFound if absent.
func (s *InMemoryTickTableStore) Get(instrumentID string) (*types.TickTable, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.data[instrumentID]
	if !ok {
		return nil, ErrNotFound
	}
	// Return a deep copy so callers cannot mutate stored state.
	cp := types.TickTable{
		InstrumentID: t.InstrumentID,
		Tiers:        make([]types.TickTier, len(t.Tiers)),
	}
	copy(cp.Tiers, t.Tiers)
	return &cp, nil
}

// List returns all tick tables.
func (s *InMemoryTickTableStore) List() ([]types.TickTable, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]types.TickTable, 0, len(s.data))
	for _, t := range s.data {
		cp := types.TickTable{
			InstrumentID: t.InstrumentID,
			Tiers:        make([]types.TickTier, len(t.Tiers)),
		}
		copy(cp.Tiers, t.Tiers)
		result = append(result, cp)
	}
	return result, nil
}

// Set upserts a tick table for the given instrument.
func (s *InMemoryTickTableStore) Set(table *types.TickTable) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := types.TickTable{
		InstrumentID: table.InstrumentID,
		Tiers:        make([]types.TickTier, len(table.Tiers)),
	}
	copy(cp.Tiers, table.Tiers)
	s.data[table.InstrumentID] = &cp
	return nil
}

// Delete removes the tick table for the given instrument.
func (s *InMemoryTickTableStore) Delete(instrumentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, instrumentID)
	return nil
}

// ── AnnouncementStore ─────────────────────────────────────────────────────────

// AnnouncementStore defines the repository contract for exchange announcements.
type AnnouncementStore interface {
	Create(a *types.Announcement) error
	ListByTenant(tenantID string) ([]types.Announcement, error)
}

// InMemoryAnnouncementStore is a thread-safe, in-memory implementation of AnnouncementStore.
type InMemoryAnnouncementStore struct {
	mu   sync.RWMutex
	data []types.Announcement
}

// NewInMemoryAnnouncementStore returns an empty InMemoryAnnouncementStore.
func NewInMemoryAnnouncementStore() *InMemoryAnnouncementStore {
	return &InMemoryAnnouncementStore{}
}

// Create appends a new announcement.
func (s *InMemoryAnnouncementStore) Create(a *types.Announcement) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = append(s.data, *a)
	return nil
}

// ListByTenant returns all announcements for the given tenant.
func (s *InMemoryAnnouncementStore) ListByTenant(tenantID string) ([]types.Announcement, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]types.Announcement, 0)
	for _, a := range s.data {
		if a.TenantID == tenantID {
			result = append(result, a)
		}
	}
	return result, nil
}

// ── AuditStore ────────────────────────────────────────────────────────────────

// AuditStore defines the repository contract for audit trail entries.
type AuditStore interface {
	Log(entry types.AuditEntry) error
	List(filters types.AuditFilters) ([]types.AuditEntry, error)
}

// InMemoryAuditStore is a thread-safe, in-memory implementation of AuditStore.
type InMemoryAuditStore struct {
	mu   sync.RWMutex
	data []types.AuditEntry
}

// NewInMemoryAuditStore returns an empty InMemoryAuditStore.
func NewInMemoryAuditStore() *InMemoryAuditStore {
	return &InMemoryAuditStore{}
}

// Log appends a new audit entry.
func (s *InMemoryAuditStore) Log(entry types.AuditEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = append(s.data, entry)
	return nil
}

// List returns audit entries matching the given filters.
func (s *InMemoryAuditStore) List(filters types.AuditFilters) ([]types.AuditEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]types.AuditEntry, 0)
	for _, e := range s.data {
		if filters.EntityType != "" && e.EntityType != filters.EntityType {
			continue
		}
		if filters.EntityID != "" && e.EntityID != filters.EntityID {
			continue
		}
		if filters.ActorID != "" && e.ActorID != filters.ActorID {
			continue
		}
		if filters.StartDate != "" && e.Timestamp < filters.StartDate {
			continue
		}
		if filters.EndDate != "" && e.Timestamp > filters.EndDate {
			continue
		}
		result = append(result, e)
	}
	return result, nil
}

// ── ThrottleStore ─────────────────────────────────────────────────────────────

// ThrottleStore defines the repository contract for per-firm order rate limiting.
type ThrottleStore interface {
	// CheckAndIncrement returns (allowed bool, error). It increments the counter
	// for firmID within the current 1-second window and returns false if the count
	// would exceed maxPerSecond.
	CheckAndIncrement(firmID string, maxPerSecond int) (bool, error)
}

// throttleBucket tracks the request count within a 1-second window.
type throttleBucket struct {
	count     int64
	windowEnd time.Time
}

// InMemoryThrottleStore is a thread-safe, time-windowed, in-memory implementation
// of ThrottleStore. Each firm gets an independent 1-second tumbling window.
type InMemoryThrottleStore struct {
	mu      sync.Mutex
	buckets map[string]*throttleBucket
}

// NewInMemoryThrottleStore returns an empty InMemoryThrottleStore.
func NewInMemoryThrottleStore() *InMemoryThrottleStore {
	return &InMemoryThrottleStore{buckets: make(map[string]*throttleBucket)}
}

// CheckAndIncrement is the core rate-limit operation. It uses a tumbling 1-second
// window: if the current time has passed the bucket's windowEnd, the counter resets.
// Returns (true, nil) when the request is allowed, (false, nil) when throttled.
func (s *InMemoryThrottleStore) CheckAndIncrement(firmID string, maxPerSecond int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	bucket, ok := s.buckets[firmID]
	if !ok || now.After(bucket.windowEnd) {
		// Start a new 1-second window.
		s.buckets[firmID] = &throttleBucket{
			count:     1,
			windowEnd: now.Add(time.Second),
		}
		return true, nil
	}

	if int(bucket.count) >= maxPerSecond {
		return false, fmt.Errorf("throttle limit %d/s exceeded for firm %s", maxPerSecond, firmID)
	}
	bucket.count++
	return true, nil
}

// ── PendingChangeStore (P2c) ──────────────────────────────────────────────────

// PendingChangeStore defines the repository contract for maker/checker pending changes.
type PendingChangeStore interface {
	Create(change *types.PendingChange) error
	Get(id string) (*types.PendingChange, error)
	ListByStatus(status string) ([]types.PendingChange, error)
	Approve(id, reviewerID string) error
	Reject(id, reviewerID, comment string) error
}

// InMemoryPendingChangeStore is a thread-safe, in-memory implementation of PendingChangeStore.
type InMemoryPendingChangeStore struct {
	mu   sync.RWMutex
	data map[string]*types.PendingChange
}

// NewInMemoryPendingChangeStore returns an empty InMemoryPendingChangeStore.
func NewInMemoryPendingChangeStore() *InMemoryPendingChangeStore {
	return &InMemoryPendingChangeStore{data: make(map[string]*types.PendingChange)}
}

// Create stores a new pending change.
func (s *InMemoryPendingChangeStore) Create(change *types.PendingChange) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.data[change.ID]; exists {
		return fmt.Errorf("pending change %s already exists", change.ID)
	}
	cp := *change
	if cp.Payload != nil {
		cp.Payload = copyPayload(change.Payload)
	}
	s.data[change.ID] = &cp
	return nil
}

// Get retrieves a pending change by ID.
func (s *InMemoryPendingChangeStore) Get(id string) (*types.PendingChange, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.data[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *c
	if c.Payload != nil {
		cp.Payload = copyPayload(c.Payload)
	}
	return &cp, nil
}

// ListByStatus returns all pending changes with the given status.
// If status is empty, all records are returned.
func (s *InMemoryPendingChangeStore) ListByStatus(status string) ([]types.PendingChange, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.PendingChange, 0, len(s.data))
	for _, c := range s.data {
		if status != "" && c.Status != status {
			continue
		}
		cp := *c
		if c.Payload != nil {
			cp.Payload = copyPayload(c.Payload)
		}
		out = append(out, cp)
	}
	return out, nil
}

// Approve transitions a pending change to APPROVED status.
// Returns an error if the change is not in PENDING_APPROVAL status.
func (s *InMemoryPendingChangeStore) Approve(id, reviewerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.data[id]
	if !ok {
		return ErrNotFound
	}
	if c.Status != "PENDING_APPROVAL" {
		return fmt.Errorf("pending change %s is not in PENDING_APPROVAL status", id)
	}
	c.Status = "APPROVED"
	c.ReviewedBy = reviewerID
	c.ReviewedAt = time.Now().UTC().Format(time.RFC3339)
	return nil
}

// Reject transitions a pending change to REJECTED status.
// Returns an error if the change is not in PENDING_APPROVAL status.
func (s *InMemoryPendingChangeStore) Reject(id, reviewerID, comment string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.data[id]
	if !ok {
		return ErrNotFound
	}
	if c.Status != "PENDING_APPROVAL" {
		return fmt.Errorf("pending change %s is not in PENDING_APPROVAL status", id)
	}
	c.Status = "REJECTED"
	c.ReviewedBy = reviewerID
	c.ReviewComment = comment
	c.ReviewedAt = time.Now().UTC().Format(time.RFC3339)
	return nil
}

// copyPayload performs a shallow copy of a map[string]interface{}.
func copyPayload(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// ── ReferencePriceStore (P2c) ─────────────────────────────────────────────────

// ReferencePriceStore defines the repository contract for instrument reference prices.
type ReferencePriceStore interface {
	Get(instrumentID string) (*types.ReferencePrice, error)
	Set(rp *types.ReferencePrice) error
}

// InMemoryReferencePriceStore is a thread-safe, in-memory implementation of ReferencePriceStore.
type InMemoryReferencePriceStore struct {
	mu   sync.RWMutex
	data map[string]*types.ReferencePrice
}

// NewInMemoryReferencePriceStore returns an empty InMemoryReferencePriceStore.
func NewInMemoryReferencePriceStore() *InMemoryReferencePriceStore {
	return &InMemoryReferencePriceStore{data: make(map[string]*types.ReferencePrice)}
}

// Get retrieves the reference price for an instrument. Returns ErrNotFound if absent.
func (s *InMemoryReferencePriceStore) Get(instrumentID string) (*types.ReferencePrice, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rp, ok := s.data[instrumentID]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *rp
	return &cp, nil
}

// Set upserts the reference price for an instrument.
func (s *InMemoryReferencePriceStore) Set(rp *types.ReferencePrice) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *rp
	s.data[rp.InstrumentID] = &cp
	return nil
}

// ── SurveillanceStore ─────────────────────────────────────────────────────────

// SurveillanceAlertFilters carries optional filter parameters for listing surveillance alerts.
type SurveillanceAlertFilters struct {
	Status    types.AlertStatus
	AlertType types.AlertType
}

// SurveillanceStore defines the repository contract for market surveillance alerts and thresholds.
type SurveillanceStore interface {
	CreateAlert(alert *types.SurveillanceAlert) error
	ListAlerts(filters SurveillanceAlertFilters) ([]types.SurveillanceAlert, error)
	ResolveAlert(id, resolvedBy string) error
	SetThreshold(threshold *types.SurveillanceThreshold) error
	GetThresholds(instrumentID string) ([]types.SurveillanceThreshold, error)
}

// InMemorySurveillanceStore is a thread-safe, in-memory implementation of SurveillanceStore.
type InMemorySurveillanceStore struct {
	mu         sync.RWMutex
	alerts     map[string]*types.SurveillanceAlert
	thresholds map[string]*types.SurveillanceThreshold // key: instrumentID+":"+alertType
}

// NewInMemorySurveillanceStore returns an empty InMemorySurveillanceStore.
func NewInMemorySurveillanceStore() *InMemorySurveillanceStore {
	return &InMemorySurveillanceStore{
		alerts:     make(map[string]*types.SurveillanceAlert),
		thresholds: make(map[string]*types.SurveillanceThreshold),
	}
}

// thresholdKey builds the composite key for a threshold record.
func thresholdKey(instrumentID string, alertType types.AlertType) string {
	return instrumentID + ":" + string(alertType)
}

// CreateAlert stores a new surveillance alert.
func (s *InMemorySurveillanceStore) CreateAlert(alert *types.SurveillanceAlert) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.alerts[alert.ID]; exists {
		return fmt.Errorf("alert %s already exists", alert.ID)
	}
	cp := *alert
	s.alerts[alert.ID] = &cp
	return nil
}

// ListAlerts returns alerts matching the given filters.
func (s *InMemorySurveillanceStore) ListAlerts(filters SurveillanceAlertFilters) ([]types.SurveillanceAlert, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.SurveillanceAlert, 0, len(s.alerts))
	for _, a := range s.alerts {
		if filters.Status != "" && a.Status != filters.Status {
			continue
		}
		if filters.AlertType != "" && a.AlertType != filters.AlertType {
			continue
		}
		out = append(out, *a)
	}
	return out, nil
}

// ResolveAlert transitions an OPEN alert to RESOLVED.
func (s *InMemorySurveillanceStore) ResolveAlert(id, resolvedBy string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.alerts[id]
	if !ok {
		return ErrNotFound
	}
	if a.Status == types.AlertStatusResolved {
		return fmt.Errorf("alert %s is already resolved", id)
	}
	a.Status = types.AlertStatusResolved
	a.ResolvedBy = resolvedBy
	a.ResolvedAt = time.Now().UTC().Format(time.RFC3339)
	return nil
}

// SetThreshold upserts a surveillance threshold for an instrument and alert type.
func (s *InMemorySurveillanceStore) SetThreshold(threshold *types.SurveillanceThreshold) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *threshold
	cp.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	s.thresholds[thresholdKey(threshold.InstrumentID, threshold.AlertType)] = &cp
	return nil
}

// GetThresholds returns all thresholds for the given instrument.
func (s *InMemorySurveillanceStore) GetThresholds(instrumentID string) ([]types.SurveillanceThreshold, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	prefix := instrumentID + ":"
	var out []types.SurveillanceThreshold
	for k, v := range s.thresholds {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			cp := *v
			out = append(out, cp)
		}
	}
	return out, nil
}

// ── InstrumentGroupStore ──────────────────────────────────────────────────────

// InstrumentGroupStore defines the repository contract for instrument groups.
type InstrumentGroupStore interface {
	Create(group *types.InstrumentGroup) error
	Get(id string) (*types.InstrumentGroup, error)
	List() ([]types.InstrumentGroup, error)
	Delete(id string) error
	AddInstrument(groupID, instrumentID string) error
	RemoveInstrument(groupID, instrumentID string) error
}

// InMemoryInstrumentGroupStore is a thread-safe, in-memory implementation of InstrumentGroupStore.
type InMemoryInstrumentGroupStore struct {
	mu   sync.RWMutex
	data map[string]*types.InstrumentGroup
}

// NewInMemoryInstrumentGroupStore returns an empty InMemoryInstrumentGroupStore.
func NewInMemoryInstrumentGroupStore() *InMemoryInstrumentGroupStore {
	return &InMemoryInstrumentGroupStore{data: make(map[string]*types.InstrumentGroup)}
}

// Create stores a new instrument group.
func (s *InMemoryInstrumentGroupStore) Create(group *types.InstrumentGroup) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.data[group.ID]; exists {
		return fmt.Errorf("instrument group %s already exists", group.ID)
	}
	cp := *group
	cp.InstrumentIDs = make([]string, len(group.InstrumentIDs))
	copy(cp.InstrumentIDs, group.InstrumentIDs)
	s.data[group.ID] = &cp
	return nil
}

// Get retrieves an instrument group by ID.
func (s *InMemoryInstrumentGroupStore) Get(id string) (*types.InstrumentGroup, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g, ok := s.data[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *g
	cp.InstrumentIDs = make([]string, len(g.InstrumentIDs))
	copy(cp.InstrumentIDs, g.InstrumentIDs)
	return &cp, nil
}

// List returns all instrument groups.
func (s *InMemoryInstrumentGroupStore) List() ([]types.InstrumentGroup, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.InstrumentGroup, 0, len(s.data))
	for _, g := range s.data {
		cp := *g
		cp.InstrumentIDs = make([]string, len(g.InstrumentIDs))
		copy(cp.InstrumentIDs, g.InstrumentIDs)
		out = append(out, cp)
	}
	return out, nil
}

// Delete removes an instrument group by ID.
func (s *InMemoryInstrumentGroupStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[id]; !ok {
		return ErrNotFound
	}
	delete(s.data, id)
	return nil
}

// AddInstrument appends an instrument ID to a group (idempotent).
func (s *InMemoryInstrumentGroupStore) AddInstrument(groupID, instrumentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.data[groupID]
	if !ok {
		return ErrNotFound
	}
	for _, id := range g.InstrumentIDs {
		if id == instrumentID {
			return nil // already present — idempotent
		}
	}
	g.InstrumentIDs = append(g.InstrumentIDs, instrumentID)
	g.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return nil
}

// RemoveInstrument removes an instrument ID from a group.
func (s *InMemoryInstrumentGroupStore) RemoveInstrument(groupID, instrumentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.data[groupID]
	if !ok {
		return ErrNotFound
	}
	newIDs := g.InstrumentIDs[:0]
	for _, id := range g.InstrumentIDs {
		if id != instrumentID {
			newIDs = append(newIDs, id)
		}
	}
	g.InstrumentIDs = newIDs
	g.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return nil
}

// ── OffBookTradeStore ─────────────────────────────────────────────────────────

// OffBookTradeStore defines the repository contract for off-book trades.
type OffBookTradeStore interface {
	Create(trade *types.OffBookTrade) error
	List() ([]types.OffBookTrade, error)
	Get(id string) (*types.OffBookTrade, error)
	UpdateStatus(id string, status types.OffBookStatus) error
	Confirm(id string, confirmedBy string) error
	Reject(id string, rejectedBy string, reason string) error
}

// InMemoryOffBookTradeStore is a thread-safe, in-memory implementation of OffBookTradeStore.
type InMemoryOffBookTradeStore struct {
	mu   sync.RWMutex
	data map[string]*types.OffBookTrade
}

// NewInMemoryOffBookTradeStore returns an empty InMemoryOffBookTradeStore.
func NewInMemoryOffBookTradeStore() *InMemoryOffBookTradeStore {
	return &InMemoryOffBookTradeStore{data: make(map[string]*types.OffBookTrade)}
}

// Create stores a new off-book trade.
func (s *InMemoryOffBookTradeStore) Create(trade *types.OffBookTrade) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.data[trade.ID]; exists {
		return fmt.Errorf("off-book trade %s already exists", trade.ID)
	}
	cp := *trade
	s.data[trade.ID] = &cp
	return nil
}

// List returns all off-book trades.
func (s *InMemoryOffBookTradeStore) List() ([]types.OffBookTrade, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.OffBookTrade, 0, len(s.data))
	for _, t := range s.data {
		out = append(out, *t)
	}
	return out, nil
}

// Get retrieves an off-book trade by ID.
func (s *InMemoryOffBookTradeStore) Get(id string) (*types.OffBookTrade, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.data[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *t
	return &cp, nil
}

// UpdateStatus changes the status of an off-book trade.
func (s *InMemoryOffBookTradeStore) UpdateStatus(id string, status types.OffBookStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.data[id]
	if !ok {
		return ErrNotFound
	}
	t.Status = status
	t.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return nil
}

// Confirm marks an off-book trade as CONFIRMED, recording who confirmed it.
func (s *InMemoryOffBookTradeStore) Confirm(id string, confirmedBy string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.data[id]
	if !ok {
		return ErrNotFound
	}
	t.Status = types.OffBookConfirmed
	t.ConfirmedBy = confirmedBy
	t.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return nil
}

// Reject marks an off-book trade as REJECTED, recording who rejected it and why.
func (s *InMemoryOffBookTradeStore) Reject(id string, rejectedBy string, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.data[id]
	if !ok {
		return ErrNotFound
	}
	t.Status = types.OffBookRejected
	t.RejectedBy = rejectedBy
	t.RejectionReason = reason
	t.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return nil
}

// ── NodeStore ─────────────────────────────────────────────────────────────────

// NodeStore defines the repository contract for hierarchical organisational nodes.
type NodeStore interface {
	Create(node *types.Node) error
	Get(id string) (*types.Node, error)
	ListByFirm(firmID string) ([]types.Node, error)
	ListChildren(parentNodeID string) ([]types.Node, error)
	GetEffectivePermissions(id string) ([]string, error)
}

// InMemoryNodeStore is a thread-safe, in-memory implementation of NodeStore.
type InMemoryNodeStore struct {
	mu   sync.RWMutex
	data map[string]*types.Node
}

// NewInMemoryNodeStore returns an empty InMemoryNodeStore.
func NewInMemoryNodeStore() *InMemoryNodeStore {
	return &InMemoryNodeStore{data: make(map[string]*types.Node)}
}

// Create stores a new node.
func (s *InMemoryNodeStore) Create(node *types.Node) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.data[node.ID]; exists {
		return fmt.Errorf("node %s already exists", node.ID)
	}
	cp := *node
	cp.Permissions = make([]string, len(node.Permissions))
	copy(cp.Permissions, node.Permissions)
	s.data[node.ID] = &cp
	return nil
}

// Get retrieves a node by ID.
func (s *InMemoryNodeStore) Get(id string) (*types.Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.data[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *n
	perms := make([]string, len(n.Permissions))
	copy(perms, n.Permissions)
	cp.Permissions = perms
	return &cp, nil
}

// ListByFirm returns all nodes belonging to the given firm.
func (s *InMemoryNodeStore) ListByFirm(firmID string) ([]types.Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []types.Node
	for _, n := range s.data {
		if n.FirmID == firmID {
			cp := *n
			perms := make([]string, len(n.Permissions))
			copy(perms, n.Permissions)
			cp.Permissions = perms
			out = append(out, cp)
		}
	}
	return out, nil
}

// ListChildren returns all direct children of a parent node.
func (s *InMemoryNodeStore) ListChildren(parentNodeID string) ([]types.Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []types.Node
	for _, n := range s.data {
		if n.ParentNodeID == parentNodeID {
			cp := *n
			perms := make([]string, len(n.Permissions))
			copy(perms, n.Permissions)
			cp.Permissions = perms
			out = append(out, cp)
		}
	}
	return out, nil
}

// SetPermissions replaces the local permissions of a node in-place.
// This is not part of the NodeStore interface but is used by the handler
// via a type-assertion to avoid adding an Update method to the interface.
func (s *InMemoryNodeStore) SetPermissions(id string, perms []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.data[id]
	if !ok {
		return ErrNotFound
	}
	cp := make([]string, len(perms))
	copy(cp, perms)
	n.Permissions = cp
	return nil
}

// GetEffectivePermissions walks up the node tree, merging permissions from all
// ancestors (parent → grandparent → …) and then the node's own permissions.
// Later (more-specific) permissions win deduplication but all unique values are
// included: the result is a deduplicated union across the full ancestry chain.
func (s *InMemoryNodeStore) GetEffectivePermissions(id string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Guard: the starting node must exist.
	if _, ok := s.data[id]; !ok {
		return nil, ErrNotFound
	}

	seen := make(map[string]struct{})
	var result []string

	current := id
	// Collect the ancestry chain from root down to current node.
	var chain []*types.Node
	visited := make(map[string]struct{})
	for current != "" {
		if _, cycle := visited[current]; cycle {
			break // guard against circular references
		}
		visited[current] = struct{}{}
		n, ok := s.data[current]
		if !ok {
			break
		}
		chain = append([]*types.Node{n}, chain...) // prepend so root comes first
		current = n.ParentNodeID
	}

	// Walk from root to node, accumulating permissions.
	for _, n := range chain {
		for _, p := range n.Permissions {
			if _, already := seen[p]; !already {
				seen[p] = struct{}{}
				result = append(result, p)
			}
		}
	}

	if result == nil {
		result = []string{}
	}
	return result, nil
}

// ── P4a — LocateStore ─────────────────────────────────────────────────────────

// LocateStore defines the repository contract for short-sell locate requests.
type LocateStore interface {
	Create(req *types.LocateRequest) error
	Get(id string) (*types.LocateRequest, error)
	List(firmID string) ([]types.LocateRequest, error)
	Approve(id, lenderFirmID string) error
	Use(id string) error
}

// InMemoryLocateStore is a thread-safe, in-memory implementation of LocateStore.
type InMemoryLocateStore struct {
	mu     sync.RWMutex
	data   map[string]*types.LocateRequest
	nextID int
}

// NewInMemoryLocateStore returns an empty InMemoryLocateStore.
func NewInMemoryLocateStore() *InMemoryLocateStore {
	return &InMemoryLocateStore{data: make(map[string]*types.LocateRequest), nextID: 1}
}

// Create stores a new locate request, assigning a sequential ID.
func (s *InMemoryLocateStore) Create(req *types.LocateRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	req.ID = s.nextID
	s.nextID++
	req.Status = "PENDING"
	req.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	cp := *req
	key := fmt.Sprintf("%d", req.ID)
	s.data[key] = &cp
	return nil
}

// Get retrieves a locate request by string ID.
func (s *InMemoryLocateStore) Get(id string) (*types.LocateRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.data[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *r
	return &cp, nil
}

// List returns all locate requests where the borrower or lender matches firmID.
// If firmID is empty, all records are returned.
func (s *InMemoryLocateStore) List(firmID string) ([]types.LocateRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.LocateRequest, 0, len(s.data))
	for _, r := range s.data {
		if firmID != "" && fmt.Sprintf("%d", r.BorrowerFirmID) != firmID && fmt.Sprintf("%d", r.LenderFirmID) != firmID {
			continue
		}
		out = append(out, *r)
	}
	return out, nil
}

// Approve transitions a PENDING locate to APPROVED, setting the lender firm.
func (s *InMemoryLocateStore) Approve(id, lenderFirmID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.data[id]
	if !ok {
		return ErrNotFound
	}
	if r.Status != "PENDING" {
		return fmt.Errorf("locate %s is not in PENDING status", id)
	}
	r.Status = "APPROVED"
	return nil
}

// Use transitions an APPROVED locate to USED (consumed by a short-sell order).
func (s *InMemoryLocateStore) Use(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.data[id]
	if !ok {
		return ErrNotFound
	}
	if r.Status != "APPROVED" {
		return fmt.Errorf("locate %s is not in APPROVED status", id)
	}
	r.Status = "USED"
	return nil
}

// ── P4a — RFQStore ────────────────────────────────────────────────────────────

// RFQStore defines the repository contract for requests for quote.
type RFQStore interface {
	Create(rfq *types.RequestForQuote) error
	Get(id string) (*types.RequestForQuote, error)
	List(instrumentID, status string) ([]types.RequestForQuote, error)
	Respond(id, quoteID string) error
	Cancel(id string) error
}

// InMemoryRFQStore is a thread-safe, in-memory implementation of RFQStore.
type InMemoryRFQStore struct {
	mu     sync.RWMutex
	data   map[string]*types.RequestForQuote
	nextID int
}

// NewInMemoryRFQStore returns an empty InMemoryRFQStore.
func NewInMemoryRFQStore() *InMemoryRFQStore {
	return &InMemoryRFQStore{data: make(map[string]*types.RequestForQuote), nextID: 1}
}

// Create stores a new RFQ, assigning a sequential ID.
func (s *InMemoryRFQStore) Create(rfq *types.RequestForQuote) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rfq.ID = s.nextID
	s.nextID++
	rfq.Status = "OPEN"
	rfq.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	cp := *rfq
	key := fmt.Sprintf("%d", rfq.ID)
	s.data[key] = &cp
	return nil
}

// Get retrieves an RFQ by string ID.
func (s *InMemoryRFQStore) Get(id string) (*types.RequestForQuote, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.data[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *r
	return &cp, nil
}

// List returns RFQs filtered by instrumentID and/or status. Empty string = no filter.
func (s *InMemoryRFQStore) List(instrumentID, status string) ([]types.RequestForQuote, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.RequestForQuote, 0, len(s.data))
	for _, r := range s.data {
		if instrumentID != "" && fmt.Sprintf("%d", r.InstrumentID) != instrumentID {
			continue
		}
		if status != "" && r.Status != status {
			continue
		}
		out = append(out, *r)
	}
	return out, nil
}

// Respond transitions an OPEN RFQ to RESPONDED, recording the response quote ID.
func (s *InMemoryRFQStore) Respond(id, quoteID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.data[id]
	if !ok {
		return ErrNotFound
	}
	if r.Status != "OPEN" {
		return fmt.Errorf("RFQ %s is not in OPEN status", id)
	}
	r.Status = "RESPONDED"
	// quoteID stored as ResponseQuoteID (int field; ignore parse error for simplicity).
	fmt.Sscanf(quoteID, "%d", &r.ResponseQuoteID)
	return nil
}

// Cancel transitions an OPEN RFQ to CANCELLED.
func (s *InMemoryRFQStore) Cancel(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.data[id]
	if !ok {
		return ErrNotFound
	}
	if r.Status != "OPEN" {
		return fmt.Errorf("RFQ %s is not in OPEN status", id)
	}
	r.Status = "CANCELLED"
	return nil
}

// ── P4a — GiveUpStore ─────────────────────────────────────────────────────────

// GiveUpStore defines the repository contract for trade give-up requests.
type GiveUpStore interface {
	Create(req *types.GiveUpRequest) error
	Get(id string) (*types.GiveUpRequest, error)
	List(firmID string) ([]types.GiveUpRequest, error)
	Accept(id string) error
	Reject(id, reason string) error
}

// InMemoryGiveUpStore is a thread-safe, in-memory implementation of GiveUpStore.
type InMemoryGiveUpStore struct {
	mu     sync.RWMutex
	data   map[string]*types.GiveUpRequest
	nextID int
}

// NewInMemoryGiveUpStore returns an empty InMemoryGiveUpStore.
func NewInMemoryGiveUpStore() *InMemoryGiveUpStore {
	return &InMemoryGiveUpStore{data: make(map[string]*types.GiveUpRequest), nextID: 1}
}

// Create stores a new give-up request, assigning a sequential ID.
func (s *InMemoryGiveUpStore) Create(req *types.GiveUpRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	req.ID = s.nextID
	s.nextID++
	req.Status = "PENDING"
	req.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	cp := *req
	key := fmt.Sprintf("%d", req.ID)
	s.data[key] = &cp
	return nil
}

// Get retrieves a give-up request by string ID.
func (s *InMemoryGiveUpStore) Get(id string) (*types.GiveUpRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.data[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *r
	return &cp, nil
}

// List returns give-up requests where the from or to firm matches firmID.
// If firmID is empty, all records are returned.
func (s *InMemoryGiveUpStore) List(firmID string) ([]types.GiveUpRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]types.GiveUpRequest, 0, len(s.data))
	for _, r := range s.data {
		if firmID != "" && fmt.Sprintf("%d", r.FromFirmID) != firmID && fmt.Sprintf("%d", r.ToFirmID) != firmID {
			continue
		}
		result = append(result, *r)
	}
	return result, nil
}

// Accept transitions a PENDING give-up to ACCEPTED.
func (s *InMemoryGiveUpStore) Accept(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.data[id]
	if !ok {
		return ErrNotFound
	}
	if r.Status != "PENDING" {
		return fmt.Errorf("give-up %s is not in PENDING status", id)
	}
	r.Status = "ACCEPTED"
	r.ResolvedAt = time.Now().UTC().Format(time.RFC3339)
	return nil
}

// Reject transitions a PENDING give-up to REJECTED, recording the reason.
func (s *InMemoryGiveUpStore) Reject(id, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.data[id]
	if !ok {
		return ErrNotFound
	}
	if r.Status != "PENDING" {
		return fmt.Errorf("give-up %s is not in PENDING status", id)
	}
	r.Status = "REJECTED"
	r.Reason = reason
	r.ResolvedAt = time.Now().UTC().Format(time.RFC3339)
	return nil
}

// ── InvestigationStore ────────────────────────────────────────────────────────

// InvestigationFilters carries optional filter parameters for listing investigations.
type InvestigationFilters struct {
	Status types.InvestigationStatus
}

// InvestigationStore defines the repository contract for market surveillance investigations.
type InvestigationStore interface {
	Create(inv *types.Investigation) error
	Get(id string) (*types.Investigation, error)
	List(filters InvestigationFilters) ([]types.Investigation, error)
	Close(id, findings string) error
	AddEvidence(id, evidence string) error
}

// InMemoryInvestigationStore is a thread-safe, in-memory implementation of InvestigationStore.
type InMemoryInvestigationStore struct {
	mu   sync.RWMutex
	data map[string]*types.Investigation
}

// NewInMemoryInvestigationStore returns an empty InMemoryInvestigationStore.
func NewInMemoryInvestigationStore() *InMemoryInvestigationStore {
	return &InMemoryInvestigationStore{data: make(map[string]*types.Investigation)}
}

// Create stores a new investigation.
func (s *InMemoryInvestigationStore) Create(inv *types.Investigation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.data[inv.ID]; exists {
		return fmt.Errorf("investigation %s already exists", inv.ID)
	}
	cp := *inv
	if cp.Evidence == nil {
		cp.Evidence = []string{}
	}
	s.data[inv.ID] = &cp
	return nil
}

// Get retrieves an investigation by ID.
func (s *InMemoryInvestigationStore) Get(id string) (*types.Investigation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	inv, ok := s.data[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *inv
	ev := make([]string, len(inv.Evidence))
	copy(ev, inv.Evidence)
	cp.Evidence = ev
	return &cp, nil
}

// List returns investigations matching the given filters.
func (s *InMemoryInvestigationStore) List(filters InvestigationFilters) ([]types.Investigation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.Investigation, 0, len(s.data))
	for _, inv := range s.data {
		if filters.Status != "" && inv.Status != filters.Status {
			continue
		}
		cp := *inv
		ev := make([]string, len(inv.Evidence))
		copy(ev, inv.Evidence)
		cp.Evidence = ev
		out = append(out, cp)
	}
	return out, nil
}

// Close marks an investigation as CLOSED with findings.
func (s *InMemoryInvestigationStore) Close(id, findings string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	inv, ok := s.data[id]
	if !ok {
		return ErrNotFound
	}
	if inv.Status == types.InvestigationClosed {
		return fmt.Errorf("investigation %s is already closed", id)
	}
	inv.Status = types.InvestigationClosed
	inv.Findings = findings
	inv.ClosedAt = time.Now().UTC().Format(time.RFC3339)
	return nil
}

// AddEvidence appends an evidence reference to an investigation.
func (s *InMemoryInvestigationStore) AddEvidence(id, evidence string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	inv, ok := s.data[id]
	if !ok {
		return ErrNotFound
	}
	inv.Evidence = append(inv.Evidence, evidence)
	return nil
}

// ── ReplayStore ───────────────────────────────────────────────────────────────

// ReplayStore defines the repository contract for market replay sessions and events.
type ReplayStore interface {
	CreateSession(session *types.ReplaySession) error
	GetSession(id string) (*types.ReplaySession, error)
	ListSessions() ([]types.ReplaySession, error)
	AddEvent(event *types.ReplayEvent) error
	GetEvents(sessionID string) ([]types.ReplayEvent, error)
}

// InMemoryReplayStore is a thread-safe, in-memory implementation of ReplayStore.
type InMemoryReplayStore struct {
	mu       sync.RWMutex
	sessions map[string]*types.ReplaySession
	events   []types.ReplayEvent
}

// NewInMemoryReplayStore returns an empty InMemoryReplayStore.
func NewInMemoryReplayStore() *InMemoryReplayStore {
	return &InMemoryReplayStore{
		sessions: make(map[string]*types.ReplaySession),
	}
}

// CreateSession stores a new replay session.
func (s *InMemoryReplayStore) CreateSession(session *types.ReplaySession) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.sessions[session.ID]; exists {
		return fmt.Errorf("replay session %s already exists", session.ID)
	}
	cp := *session
	s.sessions[session.ID] = &cp
	return nil
}

// GetSession retrieves a replay session by ID.
func (s *InMemoryReplayStore) GetSession(id string) (*types.ReplaySession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *sess
	return &cp, nil
}

// ListSessions returns all replay sessions.
func (s *InMemoryReplayStore) ListSessions() ([]types.ReplaySession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.ReplaySession, 0, len(s.sessions))
	for _, sess := range s.sessions {
		out = append(out, *sess)
	}
	return out, nil
}

// AddEvent appends a replay event to the store.
func (s *InMemoryReplayStore) AddEvent(event *types.ReplayEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, *event)
	return nil
}

// GetEvents returns all events for the given session, in sequence order.
func (s *InMemoryReplayStore) GetEvents(sessionID string) ([]types.ReplayEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []types.ReplayEvent
	for _, ev := range s.events {
		if ev.SessionID == sessionID {
			out = append(out, ev)
		}
	}
	// Sort by Sequence.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Sequence < out[j-1].Sequence; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

// ── BondStore ─────────────────────────────────────────────────────────────────

// BondStore defines the repository contract for fixed-income bond instruments.
type BondStore interface {
	Create(bond *types.Bond) error
	Get(id string) (*types.Bond, error)
	List() ([]types.Bond, error)
	UpdateStatus(id string, status types.TradingStatus) error
}

// InMemoryBondStore is a thread-safe, in-memory implementation of BondStore.
type InMemoryBondStore struct {
	mu   sync.RWMutex
	data map[string]*types.Bond
}

// NewInMemoryBondStore returns an empty InMemoryBondStore.
func NewInMemoryBondStore() *InMemoryBondStore {
	return &InMemoryBondStore{data: make(map[string]*types.Bond)}
}

// Create stores a new bond.
func (s *InMemoryBondStore) Create(bond *types.Bond) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.data[bond.ID]; exists {
		return fmt.Errorf("bond %s already exists", bond.ID)
	}
	cp := *bond
	s.data[bond.ID] = &cp
	return nil
}

// Get retrieves a bond by ID.
func (s *InMemoryBondStore) Get(id string) (*types.Bond, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.data[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *b
	return &cp, nil
}

// List returns all bonds.
func (s *InMemoryBondStore) List() ([]types.Bond, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.Bond, 0, len(s.data))
	for _, b := range s.data {
		out = append(out, *b)
	}
	return out, nil
}

// UpdateStatus changes the trading status of a bond.
func (s *InMemoryBondStore) UpdateStatus(id string, status types.TradingStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.data[id]
	if !ok {
		return ErrNotFound
	}
	b.TradingStatus = status
	b.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return nil
}

// ── StrategyStore ─────────────────────────────────────────────────────────────

// StrategyStore defines the repository contract for trading strategies.
type StrategyStore interface {
	Create(strategy *types.TradingStrategy) error
	Get(id string) (*types.TradingStrategy, error)
	List(tenantID string) ([]types.TradingStrategy, error)
	Delete(id string) error
}

// InMemoryStrategyStore is a thread-safe, in-memory implementation of StrategyStore.
type InMemoryStrategyStore struct {
	mu   sync.RWMutex
	data map[string]*types.TradingStrategy
}

// NewInMemoryStrategyStore returns an empty InMemoryStrategyStore.
func NewInMemoryStrategyStore() *InMemoryStrategyStore {
	return &InMemoryStrategyStore{data: make(map[string]*types.TradingStrategy)}
}

// Create stores a new trading strategy.
func (s *InMemoryStrategyStore) Create(strategy *types.TradingStrategy) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.data[strategy.ID]; exists {
		return fmt.Errorf("strategy %s already exists", strategy.ID)
	}
	cp := *strategy
	cp.Legs = make([]types.StrategyLeg, len(strategy.Legs))
	copy(cp.Legs, strategy.Legs)
	s.data[strategy.ID] = &cp
	return nil
}

// Get retrieves a trading strategy by ID.
func (s *InMemoryStrategyStore) Get(id string) (*types.TradingStrategy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.data[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *st
	cp.Legs = make([]types.StrategyLeg, len(st.Legs))
	copy(cp.Legs, st.Legs)
	return &cp, nil
}

// List returns all strategies for the given tenant.
// If tenantID is empty, all strategies are returned.
func (s *InMemoryStrategyStore) List(tenantID string) ([]types.TradingStrategy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.TradingStrategy, 0, len(s.data))
	for _, st := range s.data {
		if tenantID != "" && st.TenantID != tenantID {
			continue
		}
		cp := *st
		cp.Legs = make([]types.StrategyLeg, len(st.Legs))
		copy(cp.Legs, st.Legs)
		out = append(out, cp)
	}
	return out, nil
}

// Delete removes a strategy by ID. Returns ErrNotFound if absent.
func (s *InMemoryStrategyStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[id]; !ok {
		return ErrNotFound
	}
	delete(s.data, id)
	return nil
}

// ── CustodyAccountStore ───────────────────────────────────────────────────────

// CustodyAccountStore defines the repository contract for CSD custody accounts.
type CustodyAccountStore interface {
	Create(account *types.CustodyAccount) error
	Get(id string) (*types.CustodyAccount, error)
	ListByFirm(firmID string) ([]types.CustodyAccount, error)
}

// InMemoryCustodyAccountStore is a thread-safe, in-memory implementation of CustodyAccountStore.
type InMemoryCustodyAccountStore struct {
	mu   sync.RWMutex
	data map[string]*types.CustodyAccount
}

// NewInMemoryCustodyAccountStore returns an empty InMemoryCustodyAccountStore.
func NewInMemoryCustodyAccountStore() *InMemoryCustodyAccountStore {
	return &InMemoryCustodyAccountStore{data: make(map[string]*types.CustodyAccount)}
}

// Create stores a new custody account.
func (s *InMemoryCustodyAccountStore) Create(account *types.CustodyAccount) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.data[account.ID]; exists {
		return fmt.Errorf("custody account %s already exists", account.ID)
	}
	cp := *account
	s.data[account.ID] = &cp
	return nil
}

// Get retrieves a custody account by ID.
func (s *InMemoryCustodyAccountStore) Get(id string) (*types.CustodyAccount, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	acc, ok := s.data[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *acc
	return &cp, nil
}

// ListByFirm returns all custody accounts for the given firm ID.
// If firmID is empty, all accounts are returned.
func (s *InMemoryCustodyAccountStore) ListByFirm(firmID string) ([]types.CustodyAccount, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.CustodyAccount, 0, len(s.data))
	for _, acc := range s.data {
		if firmID != "" && acc.FirmID != firmID {
			continue
		}
		out = append(out, *acc)
	}
	return out, nil
}

// ── CustodyBalanceStore ───────────────────────────────────────────────────────

// custodyBalanceKey builds the composite key for a balance record.
func custodyBalanceKey(accountID, instrumentID string) string {
	return accountID + ":" + instrumentID
}

// CustodyBalanceStore defines the repository contract for CSD custody balances.
type CustodyBalanceStore interface {
	GetOrUpdate(accountID, instrumentID string, deltaQty int, avgCost float64) (*types.CustodyBalance, error)
	ListByAccount(accountID string) ([]types.CustodyBalance, error)
}

// InMemoryCustodyBalanceStore is a thread-safe, in-memory implementation of CustodyBalanceStore.
type InMemoryCustodyBalanceStore struct {
	mu   sync.RWMutex
	data map[string]*types.CustodyBalance
}

// NewInMemoryCustodyBalanceStore returns an empty InMemoryCustodyBalanceStore.
func NewInMemoryCustodyBalanceStore() *InMemoryCustodyBalanceStore {
	return &InMemoryCustodyBalanceStore{data: make(map[string]*types.CustodyBalance)}
}

// GetOrUpdate retrieves the balance for (accountID, instrumentID), applies deltaQty and avgCost
// (if non-zero), and returns the updated record. Creates a zero record if absent.
func (s *InMemoryCustodyBalanceStore) GetOrUpdate(accountID, instrumentID string, deltaQty int, avgCost float64) (*types.CustodyBalance, error) {
	key := custodyBalanceKey(accountID, instrumentID)
	s.mu.Lock()
	defer s.mu.Unlock()
	bal, ok := s.data[key]
	if !ok {
		bal = &types.CustodyBalance{
			AccountID:    accountID,
			InstrumentID: instrumentID,
		}
		s.data[key] = bal
	}
	bal.Quantity += deltaQty
	if avgCost != 0 {
		bal.AvgCost = avgCost
	}
	bal.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	cp := *bal
	return &cp, nil
}

// ListByAccount returns all balances for a given custody account.
func (s *InMemoryCustodyBalanceStore) ListByAccount(accountID string) ([]types.CustodyBalance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.CustodyBalance, 0)
	for _, bal := range s.data {
		if bal.AccountID == accountID {
			out = append(out, *bal)
		}
	}
	return out, nil
}

// ── CSDTransferStore ──────────────────────────────────────────────────────────

// CSDTransferStore defines the repository contract for CSD transfer instructions.
type CSDTransferStore interface {
	Create(transfer *types.CSDTransfer) error
	Get(id string) (*types.CSDTransfer, error)
	Complete(id string) error
	Fail(id, reason string) error
}

// InMemoryCSDTransferStore is a thread-safe, in-memory implementation of CSDTransferStore.
type InMemoryCSDTransferStore struct {
	mu   sync.RWMutex
	data map[string]*types.CSDTransfer
}

// NewInMemoryCSDTransferStore returns an empty InMemoryCSDTransferStore.
func NewInMemoryCSDTransferStore() *InMemoryCSDTransferStore {
	return &InMemoryCSDTransferStore{data: make(map[string]*types.CSDTransfer)}
}

// Create stores a new CSD transfer instruction.
func (s *InMemoryCSDTransferStore) Create(transfer *types.CSDTransfer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.data[transfer.ID]; exists {
		return fmt.Errorf("CSD transfer %s already exists", transfer.ID)
	}
	cp := *transfer
	s.data[transfer.ID] = &cp
	return nil
}

// Get retrieves a CSD transfer by ID.
func (s *InMemoryCSDTransferStore) Get(id string) (*types.CSDTransfer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.data[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *t
	return &cp, nil
}

// Complete transitions a PENDING transfer to COMPLETED.
func (s *InMemoryCSDTransferStore) Complete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.data[id]
	if !ok {
		return ErrNotFound
	}
	if t.Status != types.CSDTransferPending {
		return fmt.Errorf("CSD transfer %s is not in PENDING status", id)
	}
	t.Status = types.CSDTransferCompleted
	t.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return nil
}

// Fail transitions a PENDING transfer to FAILED, recording the failure reason.
func (s *InMemoryCSDTransferStore) Fail(id, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.data[id]
	if !ok {
		return ErrNotFound
	}
	if t.Status != types.CSDTransferPending {
		return fmt.Errorf("CSD transfer %s is not in PENDING status", id)
	}
	t.Status = types.CSDTransferFailed
	t.FailReason = reason
	t.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return nil
}

// ── ThrottleConfigStore ───────────────────────────────────────────────────────

// ThrottleConfigStore defines the repository contract for per-firm throttle
// configuration. Configs are keyed by FirmID.
type ThrottleConfigStore interface {
	// Get returns the throttle config for firmID, or ErrNotFound.
	Get(firmID string) (*types.ThrottleConfig, error)
	// Set creates or replaces the throttle config for the given firm.
	Set(cfg *types.ThrottleConfig) error
	// List returns all registered throttle configs.
	List() ([]types.ThrottleConfig, error)
	// Delete removes the throttle config for firmID, or ErrNotFound.
	Delete(firmID string) error
}

// InMemoryThrottleConfigStore is a thread-safe, in-memory implementation of
// ThrottleConfigStore.
type InMemoryThrottleConfigStore struct {
	mu   sync.RWMutex
	data map[string]*types.ThrottleConfig
}

// NewInMemoryThrottleConfigStore returns an empty InMemoryThrottleConfigStore.
func NewInMemoryThrottleConfigStore() *InMemoryThrottleConfigStore {
	return &InMemoryThrottleConfigStore{data: make(map[string]*types.ThrottleConfig)}
}

// Get returns the throttle config for firmID.
func (s *InMemoryThrottleConfigStore) Get(firmID string) (*types.ThrottleConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.data[firmID]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *c
	return &cp, nil
}

// Set creates or replaces the throttle config for the given firm.
func (s *InMemoryThrottleConfigStore) Set(cfg *types.ThrottleConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *cfg
	cp.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	s.data[cfg.FirmID] = &cp
	return nil
}

// List returns all registered throttle configs.
func (s *InMemoryThrottleConfigStore) List() ([]types.ThrottleConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.ThrottleConfig, 0, len(s.data))
	for _, c := range s.data {
		out = append(out, *c)
	}
	return out, nil
}

// Delete removes the throttle config for firmID.
func (s *InMemoryThrottleConfigStore) Delete(firmID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[firmID]; !ok {
		return ErrNotFound
	}
	delete(s.data, firmID)
	return nil
}

// ── WatchListStore ────────────────────────────────────────────────────────────

// WatchListStore defines the repository contract for user watch lists.
type WatchListStore interface {
	Create(wl *types.WatchList) error
	Get(id string) (*types.WatchList, error)
	ListByOwner(ownerID string) ([]types.WatchList, error)
	Update(wl *types.WatchList) error
	Delete(id string) error
}

// InMemoryWatchListStore is a thread-safe, in-memory implementation of WatchListStore.
type InMemoryWatchListStore struct {
	mu   sync.RWMutex
	data map[string]*types.WatchList
}

// NewInMemoryWatchListStore returns an empty InMemoryWatchListStore.
func NewInMemoryWatchListStore() *InMemoryWatchListStore {
	return &InMemoryWatchListStore{data: make(map[string]*types.WatchList)}
}

// Create stores a new watch list.
func (s *InMemoryWatchListStore) Create(wl *types.WatchList) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.data[wl.ID]; exists {
		return fmt.Errorf("watch list %s already exists", wl.ID)
	}
	cp := *wl
	s.data[wl.ID] = &cp
	return nil
}

// Get retrieves a watch list by ID.
func (s *InMemoryWatchListStore) Get(id string) (*types.WatchList, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	wl, ok := s.data[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *wl
	return &cp, nil
}

// ListByOwner returns all watch lists owned by ownerID.
func (s *InMemoryWatchListStore) ListByOwner(ownerID string) ([]types.WatchList, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.WatchList, 0)
	for _, wl := range s.data {
		if wl.OwnerID == ownerID {
			out = append(out, *wl)
		}
	}
	return out, nil
}

// Update replaces an existing watch list record.
func (s *InMemoryWatchListStore) Update(wl *types.WatchList) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[wl.ID]; !ok {
		return ErrNotFound
	}
	cp := *wl
	s.data[wl.ID] = &cp
	return nil
}

// Delete removes a watch list by ID.
func (s *InMemoryWatchListStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[id]; !ok {
		return ErrNotFound
	}
	delete(s.data, id)
	return nil
}

// ── IPRestrictionStore ────────────────────────────────────────────────────────

// IPRestrictionStore defines the repository contract for participant IP allow-lists.
type IPRestrictionStore interface {
	Get(participantID string) (*types.IPRestriction, error)
	Set(r *types.IPRestriction) error
	Delete(participantID string) error
}

// InMemoryIPRestrictionStore is a thread-safe, in-memory implementation of IPRestrictionStore.
type InMemoryIPRestrictionStore struct {
	mu   sync.RWMutex
	data map[string]*types.IPRestriction
}

// NewInMemoryIPRestrictionStore returns an empty InMemoryIPRestrictionStore.
func NewInMemoryIPRestrictionStore() *InMemoryIPRestrictionStore {
	return &InMemoryIPRestrictionStore{data: make(map[string]*types.IPRestriction)}
}

// Get retrieves the IP restriction record for a participant.
func (s *InMemoryIPRestrictionStore) Get(participantID string) (*types.IPRestriction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.data[participantID]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *r
	return &cp, nil
}

// Set upserts an IP restriction record.
func (s *InMemoryIPRestrictionStore) Set(r *types.IPRestriction) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *r
	s.data[r.ParticipantID] = &cp
	return nil
}

// Delete removes the IP restriction record for a participant.
func (s *InMemoryIPRestrictionStore) Delete(participantID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[participantID]; !ok {
		return ErrNotFound
	}
	delete(s.data, participantID)
	return nil
}

// ── PasswordPolicyStore ───────────────────────────────────────────────────────

// PasswordPolicyStore defines the repository contract for per-tenant password policies.
type PasswordPolicyStore interface {
	Get(tenantID string) (*types.PasswordPolicy, error)
	Set(p *types.PasswordPolicy) error
}

// InMemoryPasswordPolicyStore is a thread-safe, in-memory implementation of PasswordPolicyStore.
type InMemoryPasswordPolicyStore struct {
	mu   sync.RWMutex
	data map[string]*types.PasswordPolicy
}

// NewInMemoryPasswordPolicyStore returns an empty InMemoryPasswordPolicyStore.
func NewInMemoryPasswordPolicyStore() *InMemoryPasswordPolicyStore {
	return &InMemoryPasswordPolicyStore{data: make(map[string]*types.PasswordPolicy)}
}

// Get retrieves the password policy for a tenant.
func (s *InMemoryPasswordPolicyStore) Get(tenantID string) (*types.PasswordPolicy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.data[tenantID]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *p
	return &cp, nil
}

// Set upserts a password policy for a tenant.
func (s *InMemoryPasswordPolicyStore) Set(p *types.PasswordPolicy) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *p
	s.data[p.TenantID] = &cp
	return nil
}
