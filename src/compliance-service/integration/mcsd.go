// Package integration provides the adapter that integrates the GarudaX
// settlement pipeline directly with the Mongolian Central Securities Depository
// (MCSD) for securities custody and book-entry transfer.
//
//	GarudaX Settlement Engine → CSD Adapter → MCSD API (ISO 20022)
//
// MCSD is the depository for the mse-equities flagship venue only
// (docs/platform-architecture.md §10.6). This package fixes the CSDAdapter
// interface boundary and ships an in-memory StubAdapter — the initial
// implementation where operations succeed immediately (book-entry movements are
// applied locally). A production adapter swaps in behind the same interface,
// speaking ISO 20022 XML over HTTPS, without touching callers.
//
// Per the GarudaX multi-tenant directive, tenant ID is a first-class,
// non-optional input: every account, transfer instruction, and corporate-action
// notification carries a TenantID, and the adapter enforces tenant isolation
// (a transfer may never cross tenants). The package is zero-dependency
// (standard library only).
package integration

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// ISO 20022 message type identifiers used on the MCSD wire (documented here so
// the production adapter and the stub agree on the message contract).
const (
	// MsgSettlementInstruction — Securities Settlement Transaction Instruction.
	MsgSettlementInstruction = "sese.023.001.09"
	// MsgSettlementStatus — Securities Settlement Transaction Status Advice.
	MsgSettlementStatus = "sese.024.001.09"
	// MsgIntraPositionMovement — Intra-Position Movement Instruction (FoP).
	MsgIntraPositionMovement = "semt.013.001.06"
	// MsgCorporateActionNotification — Corporate Action Notification.
	MsgCorporateActionNotification = "seev.031.001.13"
)

// TransferType distinguishes delivery-versus-payment from free-of-payment.
type TransferType string

const (
	// TransferDvP — Delivery versus Payment: securities move against cash.
	TransferDvP TransferType = "DVP"
	// TransferFoP — Free of Payment: securities move with no cash leg.
	TransferFoP TransferType = "FOP"
)

// TransferState is the lifecycle state of a settlement transfer at MCSD.
type TransferState string

const (
	StatePending   TransferState = "PENDING"   // instructed, awaiting matching/affirmation
	StateAffirmed  TransferState = "AFFIRMED"  // matched at MCSD, awaiting settlement
	StateSettled   TransferState = "SETTLED"   // book-entry complete
	StateFailed    TransferState = "FAILED"    // settlement failed (e.g. insufficient holdings)
	StateCancelled TransferState = "CANCELLED" // recalled before settlement
)

// Sentinel errors.
var (
	ErrMissingTenant        = fmt.Errorf("mcsd: tenant_id is required")
	ErrTenantMismatch       = fmt.Errorf("mcsd: accounts belong to different tenants")
	ErrAccountNotFound      = fmt.Errorf("mcsd: custody account not found")
	ErrTransferNotFound     = fmt.Errorf("mcsd: transfer not found")
	ErrInvalidQuantity      = fmt.Errorf("mcsd: quantity must be positive")
	ErrInvalidAmount        = fmt.Errorf("mcsd: DvP settlement amount must be positive")
	ErrInsufficientHoldings = fmt.Errorf("mcsd: insufficient holdings in delivering account")
	ErrInvalidState         = fmt.Errorf("mcsd: transfer not in a state that permits this operation")
	ErrMissingFields        = fmt.Errorf("mcsd: required fields are missing")
)

// ── Domain types ──────────────────────────────────────────────────────────────

// CreateAccountRequest requests creation of a custody account at MCSD.
type CreateAccountRequest struct {
	TenantID string `json:"tenant_id"`
	OwnerID  string `json:"owner_id"` // participant / firm that owns the account
	Name     string `json:"name"`
	Currency string `json:"currency"`
	BIC      string `json:"bic,omitempty"` // MCSD participant BIC, if known
}

// CustodyAccount is a securities custody account held at MCSD.
type CustodyAccount struct {
	AccountID string    `json:"account_id"`
	TenantID  string    `json:"tenant_id"`
	OwnerID   string    `json:"owner_id"`
	Name      string    `json:"name"`
	Currency  string    `json:"currency"`
	BIC       string    `json:"bic,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Balance is the holding of a single instrument in a custody account.
type Balance struct {
	AccountID    string `json:"account_id"`
	InstrumentID string `json:"instrument_id"`
	Quantity     int64  `json:"quantity"` // settled position
	Pending      int64  `json:"pending"`  // net pending deliveries (can be negative)
}

// DvPInstruction instructs a delivery-versus-payment settlement.
type DvPInstruction struct {
	TenantID         string  `json:"tenant_id"`
	FromAccountID    string  `json:"from_account_id"`
	ToAccountID      string  `json:"to_account_id"`
	InstrumentID     string  `json:"instrument_id"`
	Quantity         int64   `json:"quantity"`
	SettlementAmount float64 `json:"settlement_amount"`
	Currency         string  `json:"currency,omitempty"`
	SettlementDate   string  `json:"settlement_date,omitempty"` // YYYY-MM-DD; T+2 etc.
	Reference        string  `json:"reference,omitempty"`
}

// FoPInstruction instructs a free-of-payment securities movement.
type FoPInstruction struct {
	TenantID       string `json:"tenant_id"`
	FromAccountID  string `json:"from_account_id"`
	ToAccountID    string `json:"to_account_id"`
	InstrumentID   string `json:"instrument_id"`
	Quantity       int64  `json:"quantity"`
	SettlementDate string `json:"settlement_date,omitempty"`
	Reference      string `json:"reference,omitempty"`
}

// TransferResponse acknowledges a settlement instruction.
type TransferResponse struct {
	TransferID string        `json:"transfer_id"`
	TenantID   string        `json:"tenant_id"`
	State      TransferState `json:"state"`
	MessageID  string        `json:"message_id"` // ISO 20022 message type used
	AcceptedAt time.Time     `json:"accepted_at"`
}

// TransferStatus is the full status record of a transfer.
type TransferStatus struct {
	TransferID       string        `json:"transfer_id"`
	TenantID         string        `json:"tenant_id"`
	Type             TransferType  `json:"type"`
	FromAccountID    string        `json:"from_account_id"`
	ToAccountID      string        `json:"to_account_id"`
	InstrumentID     string        `json:"instrument_id"`
	Quantity         int64         `json:"quantity"`
	SettlementAmount float64       `json:"settlement_amount,omitempty"`
	State            TransferState `json:"state"`
	Reason           string        `json:"reason,omitempty"`
	SettlementDate   string        `json:"settlement_date,omitempty"`
	InstructedAt     time.Time     `json:"instructed_at"`
	SettledAt        time.Time     `json:"settled_at,omitempty"`
}

// CorporateAction is a corporate-action notification sent to MCSD.
type CorporateAction struct {
	ActionID      string  `json:"action_id"`
	TenantID      string  `json:"tenant_id"`
	InstrumentID  string  `json:"instrument_id"`
	Type          string  `json:"type"` // DIVIDEND | STOCK_SPLIT | RIGHTS_ISSUE | ...
	RecordDate    string  `json:"record_date"`
	PaymentDate   string  `json:"payment_date,omitempty"`
	RatioOrAmount float64 `json:"ratio_or_amount,omitempty"`
}

// Entitlement is a per-holder entitlement computed for a corporate action.
type Entitlement struct {
	ActionID         string  `json:"action_id"`
	TenantID         string  `json:"tenant_id"`
	AccountID        string  `json:"account_id"`
	OwnerID          string  `json:"owner_id"`
	HeldQty          int64   `json:"held_qty"`
	CashEntitlement  float64 `json:"cash_entitlement"`
	ShareEntitlement int64   `json:"share_entitlement"`
}

// ── Adapter interface ─────────────────────────────────────────────────────────

// CSDAdapter is the boundary between the GarudaX settlement pipeline and MCSD.
// Implementations must be safe for concurrent use.
type CSDAdapter interface {
	// Account management
	CreateCustodyAccount(ctx context.Context, req CreateAccountRequest) (*CustodyAccount, error)
	GetBalance(ctx context.Context, accountID, instrumentID string) (*Balance, error)

	// Transfers
	InstructDvP(ctx context.Context, req DvPInstruction) (*TransferResponse, error)
	InstructFoP(ctx context.Context, req FoPInstruction) (*TransferResponse, error)
	GetTransferStatus(ctx context.Context, transferID string) (*TransferStatus, error)

	// Corporate actions
	NotifyCorporateAction(ctx context.Context, action CorporateAction) error
	GetEntitlements(ctx context.Context, actionID string) ([]Entitlement, error)
}

// ── StubAdapter ───────────────────────────────────────────────────────────────

// StubAdapter is the in-memory MCSD adapter. Transfers settle immediately when
// AutoSettle is true (the default, matching "all operations succeed
// immediately"); when false they stay PENDING until Settle or Fail is called,
// modelling the MCSD affirmation handshake for tests. Safe for concurrent use.
type StubAdapter struct {
	mu           sync.Mutex
	accounts     map[string]*CustodyAccount     // accountID → account
	balances     map[string]map[string]*Balance // accountID → instrumentID → balance
	transfers    map[string]*TransferStatus     // transferID → status
	corpActions  map[string]CorporateAction     // actionID → action
	entitlements map[string][]Entitlement       // actionID → entitlements
	seq          uint64

	// AutoSettle controls whether InstructDvP/InstructFoP settle synchronously.
	AutoSettle bool
}

// NewStubAdapter returns an in-memory CSDAdapter with auto-settlement enabled.
func NewStubAdapter() *StubAdapter {
	return &StubAdapter{
		accounts:     make(map[string]*CustodyAccount),
		balances:     make(map[string]map[string]*Balance),
		transfers:    make(map[string]*TransferStatus),
		corpActions:  make(map[string]CorporateAction),
		entitlements: make(map[string][]Entitlement),
		AutoSettle:   true,
	}
}

var _ CSDAdapter = (*StubAdapter)(nil)

func (s *StubAdapter) nextID(prefix string) string {
	n := atomic.AddUint64(&s.seq, 1)
	return fmt.Sprintf("%s-%s-%d", prefix, time.Now().UTC().Format("20060102"), n)
}

// CreateCustodyAccount creates a custody account. TenantID and OwnerID are required.
func (s *StubAdapter) CreateCustodyAccount(_ context.Context, req CreateAccountRequest) (*CustodyAccount, error) {
	if req.TenantID == "" {
		return nil, ErrMissingTenant
	}
	if req.OwnerID == "" || req.Name == "" {
		return nil, ErrMissingFields
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	acct := &CustodyAccount{
		AccountID: s.nextID("csd-acct"),
		TenantID:  req.TenantID,
		OwnerID:   req.OwnerID,
		Name:      req.Name,
		Currency:  req.Currency,
		BIC:       req.BIC,
		CreatedAt: time.Now().UTC(),
	}
	s.accounts[acct.AccountID] = acct
	s.balances[acct.AccountID] = make(map[string]*Balance)
	return acct, nil
}

// Credit adds a settled holding to an account, creating balances as needed. This
// seeds opening positions (e.g. from CSD reconciliation) outside of a transfer.
func (s *StubAdapter) Credit(accountID, instrumentID string, quantity int64) error {
	if quantity <= 0 {
		return ErrInvalidQuantity
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.accounts[accountID]; !ok {
		return ErrAccountNotFound
	}
	s.adjustLocked(accountID, instrumentID, quantity, 0)
	return nil
}

// GetBalance returns the holding of an instrument in an account. A zero balance
// is returned for a known account with no holding of that instrument.
func (s *StubAdapter) GetBalance(_ context.Context, accountID, instrumentID string) (*Balance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.accounts[accountID]; !ok {
		return nil, ErrAccountNotFound
	}
	if b, ok := s.balances[accountID][instrumentID]; ok {
		cp := *b
		return &cp, nil
	}
	return &Balance{AccountID: accountID, InstrumentID: instrumentID}, nil
}

// InstructDvP instructs a delivery-versus-payment settlement.
func (s *StubAdapter) InstructDvP(_ context.Context, req DvPInstruction) (*TransferResponse, error) {
	if req.TenantID == "" {
		return nil, ErrMissingTenant
	}
	if req.SettlementAmount <= 0 {
		return nil, ErrInvalidAmount
	}
	return s.instruct(req.TenantID, req.FromAccountID, req.ToAccountID, req.InstrumentID,
		req.Quantity, req.SettlementAmount, TransferDvP, req.SettlementDate, MsgSettlementInstruction)
}

// InstructFoP instructs a free-of-payment securities movement.
func (s *StubAdapter) InstructFoP(_ context.Context, req FoPInstruction) (*TransferResponse, error) {
	if req.TenantID == "" {
		return nil, ErrMissingTenant
	}
	return s.instruct(req.TenantID, req.FromAccountID, req.ToAccountID, req.InstrumentID,
		req.Quantity, 0, TransferFoP, req.SettlementDate, MsgIntraPositionMovement)
}

// instruct creates a transfer, validates accounts/tenant/holdings, and either
// settles immediately (AutoSettle) or leaves it PENDING.
func (s *StubAdapter) instruct(tenantID, from, to, instrument string, qty int64, amount float64,
	tt TransferType, settlementDate, msgID string) (*TransferResponse, error) {
	if from == "" || to == "" || instrument == "" {
		return nil, ErrMissingFields
	}
	if qty <= 0 {
		return nil, ErrInvalidQuantity
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	fromAcct, ok := s.accounts[from]
	if !ok {
		return nil, fmt.Errorf("%w: from %s", ErrAccountNotFound, from)
	}
	toAcct, ok := s.accounts[to]
	if !ok {
		return nil, fmt.Errorf("%w: to %s", ErrAccountNotFound, to)
	}
	// Tenant isolation: instruction and both accounts must share the tenant.
	if fromAcct.TenantID != tenantID || toAcct.TenantID != tenantID {
		return nil, ErrTenantMismatch
	}

	now := time.Now().UTC()
	st := &TransferStatus{
		TransferID:       s.nextID("csd-xfer"),
		TenantID:         tenantID,
		Type:             tt,
		FromAccountID:    from,
		ToAccountID:      to,
		InstrumentID:     instrument,
		Quantity:         qty,
		SettlementAmount: amount,
		State:            StatePending,
		SettlementDate:   settlementDate,
		InstructedAt:     now,
	}

	if s.AutoSettle {
		if err := s.settleLocked(st); err != nil {
			// Record the failed transfer so its status is queryable.
			st.State = StateFailed
			st.Reason = err.Error()
			s.transfers[st.TransferID] = st
			return nil, err
		}
	} else {
		// Reserve the delivering holding as pending so concurrent instructions
		// see the encumbrance.
		s.adjustLocked(from, instrument, 0, -qty)
		s.adjustLocked(to, instrument, 0, qty)
	}

	s.transfers[st.TransferID] = st
	return &TransferResponse{
		TransferID: st.TransferID,
		TenantID:   tenantID,
		State:      st.State,
		MessageID:  msgID,
		AcceptedAt: now,
	}, nil
}

// settleLocked applies book-entry movements for a transfer. Caller holds s.mu.
func (s *StubAdapter) settleLocked(st *TransferStatus) error {
	held := s.quantityLocked(st.FromAccountID, st.InstrumentID)
	if held < st.Quantity {
		return ErrInsufficientHoldings
	}
	s.adjustLocked(st.FromAccountID, st.InstrumentID, -st.Quantity, 0)
	s.adjustLocked(st.ToAccountID, st.InstrumentID, st.Quantity, 0)
	st.State = StateSettled
	st.SettledAt = time.Now().UTC()
	return nil
}

// Settle drives a PENDING/AFFIRMED transfer to SETTLED (used when AutoSettle is
// off — mirrors the MCSD affirmation → settlement handshake).
func (s *StubAdapter) Settle(transferID string) (*TransferStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.transfers[transferID]
	if !ok {
		return nil, ErrTransferNotFound
	}
	if st.State != StatePending && st.State != StateAffirmed {
		return nil, ErrInvalidState
	}
	// Release the pending reservation, then apply the settled movement.
	s.adjustLocked(st.FromAccountID, st.InstrumentID, 0, st.Quantity)
	s.adjustLocked(st.ToAccountID, st.InstrumentID, 0, -st.Quantity)
	if err := s.settleLocked(st); err != nil {
		// Re-reserve so state is consistent with a still-pending transfer.
		s.adjustLocked(st.FromAccountID, st.InstrumentID, 0, -st.Quantity)
		s.adjustLocked(st.ToAccountID, st.InstrumentID, 0, st.Quantity)
		return nil, err
	}
	cp := *st
	return &cp, nil
}

// Affirm moves a PENDING transfer to AFFIRMED (MCSD matched it).
func (s *StubAdapter) Affirm(transferID string) (*TransferStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.transfers[transferID]
	if !ok {
		return nil, ErrTransferNotFound
	}
	if st.State != StatePending {
		return nil, ErrInvalidState
	}
	st.State = StateAffirmed
	cp := *st
	return &cp, nil
}

// Fail marks a PENDING/AFFIRMED transfer FAILED and releases its reservation.
func (s *StubAdapter) Fail(transferID, reason string) (*TransferStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.transfers[transferID]
	if !ok {
		return nil, ErrTransferNotFound
	}
	if st.State != StatePending && st.State != StateAffirmed {
		return nil, ErrInvalidState
	}
	// Release the pending reservation made at instruction time.
	s.adjustLocked(st.FromAccountID, st.InstrumentID, 0, st.Quantity)
	s.adjustLocked(st.ToAccountID, st.InstrumentID, 0, -st.Quantity)
	st.State = StateFailed
	st.Reason = reason
	cp := *st
	return &cp, nil
}

// GetTransferStatus returns the status of a transfer.
func (s *StubAdapter) GetTransferStatus(_ context.Context, transferID string) (*TransferStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.transfers[transferID]
	if !ok {
		return nil, ErrTransferNotFound
	}
	cp := *st
	return &cp, nil
}

// NotifyCorporateAction records a corporate-action notification at MCSD and
// computes per-holder entitlements from current settled holdings (record-date
// snapshot). Cash entitlement applies to DIVIDEND; share entitlement to splits
// and rights issues, using RatioOrAmount.
func (s *StubAdapter) NotifyCorporateAction(_ context.Context, action CorporateAction) error {
	if action.TenantID == "" {
		return ErrMissingTenant
	}
	if action.ActionID == "" || action.InstrumentID == "" || action.Type == "" {
		return ErrMissingFields
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.corpActions[action.ActionID] = action
	var ents []Entitlement
	for accountID, byInstrument := range s.balances {
		bal, ok := byInstrument[action.InstrumentID]
		if !ok || bal.Quantity <= 0 {
			continue
		}
		acct := s.accounts[accountID]
		if acct == nil || acct.TenantID != action.TenantID {
			continue
		}
		ent := Entitlement{
			ActionID:  action.ActionID,
			TenantID:  action.TenantID,
			AccountID: accountID,
			OwnerID:   acct.OwnerID,
			HeldQty:   bal.Quantity,
		}
		switch action.Type {
		case "DIVIDEND":
			ent.CashEntitlement = round2(float64(bal.Quantity) * action.RatioOrAmount)
		case "STOCK_SPLIT", "RIGHTS_ISSUE", "STOCK_DIVIDEND", "REVERSE_SPLIT":
			ent.ShareEntitlement = int64(float64(bal.Quantity) * action.RatioOrAmount)
		}
		ents = append(ents, ent)
	}
	// Deterministic order for stable reads.
	sort.Slice(ents, func(i, j int) bool { return ents[i].AccountID < ents[j].AccountID })
	s.entitlements[action.ActionID] = ents
	return nil
}

// GetEntitlements returns the entitlements computed for a corporate action.
func (s *StubAdapter) GetEntitlements(_ context.Context, actionID string) ([]Entitlement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.corpActions[actionID]; !ok {
		return nil, fmt.Errorf("mcsd: corporate action %s not found", actionID)
	}
	ents := s.entitlements[actionID]
	out := make([]Entitlement, len(ents))
	copy(out, ents)
	return out, nil
}

// ── balance helpers (caller holds s.mu) ─────────────────────────────────────────

func (s *StubAdapter) quantityLocked(accountID, instrumentID string) int64 {
	if byInstrument, ok := s.balances[accountID]; ok {
		if b, ok := byInstrument[instrumentID]; ok {
			return b.Quantity
		}
	}
	return 0
}

// adjustLocked applies deltas to settled quantity and pending for an account's
// instrument balance, creating the record if absent.
func (s *StubAdapter) adjustLocked(accountID, instrumentID string, qtyDelta, pendingDelta int64) {
	byInstrument, ok := s.balances[accountID]
	if !ok {
		byInstrument = make(map[string]*Balance)
		s.balances[accountID] = byInstrument
	}
	b, ok := byInstrument[instrumentID]
	if !ok {
		b = &Balance{AccountID: accountID, InstrumentID: instrumentID}
		byInstrument[instrumentID] = b
	}
	b.Quantity += qtyDelta
	b.Pending += pendingDelta
}

// round2 rounds to 2 decimal places, half away from zero.
func round2(f float64) float64 {
	if f >= 0 {
		return float64(int64(f*100+0.5)) / 100
	}
	return float64(int64(f*100-0.5)) / 100
}
