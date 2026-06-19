// Package engine — pre-borrow / locate workflow for short selling.
//
// A "locate" is a borrower firm's request to source shares from a lender so
// that a SHORT_SELL order can be backed by borrowable inventory. The lifecycle
// is: PENDING → APPROVED → USED, with EXPIRED/REJECTED as terminal off-ramps.
// This engine encapsulates the workflow transitions and the validity checks
// (status, expiry, instrument match, borrower match, quantity coverage) that
// guard whether a locate may be consumed by a short-sell order.
package engine

import (
	"strconv"
	"time"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// Locate status constants mirror the values persisted by the LocateStore.
const (
	LocateStatusPending  = "PENDING"
	LocateStatusApproved = "APPROVED"
	LocateStatusRejected = "REJECTED"
	LocateStatusExpired  = "EXPIRED"
	LocateStatusUsed     = "USED"
)

// defaultLocateTTL is applied when a locate request omits an explicit expiry.
const defaultLocateTTL = 24 * time.Hour

// LocateEngine coordinates the pre-borrow / locate workflow on top of a
// LocateStore. It is safe to construct per request — it holds no mutable state
// beyond its (immutable) dependencies.
type LocateEngine struct {
	store store.LocateStore
	now   func() time.Time
}

// NewLocateEngine returns a LocateEngine backed by the given store. The store
// may be nil; store-dependent operations then report ErrLocateStoreUnavailable.
func NewLocateEngine(s store.LocateStore) *LocateEngine {
	return &LocateEngine{store: s, now: time.Now}
}

// WithClock overrides the time source. Intended for tests so expiry behaviour
// is deterministic. Returns the receiver for chaining.
func (e *LocateEngine) WithClock(now func() time.Time) *LocateEngine {
	if now != nil {
		e.now = now
	}
	return e
}

// Request validates and persists a new locate request. On success the supplied
// request is populated with the assigned ID, PENDING status and timestamps by
// the underlying store. A default 24h expiry is applied when none is given.
func (e *LocateEngine) Request(req *types.LocateRequest) error {
	if e.store == nil {
		return errLocateStoreUnavailable()
	}
	if req == nil {
		return newShortSellError(CodeMissingField, "locate request body is required")
	}
	if req.InstrumentID == 0 {
		return newShortSellError(CodeMissingField, "instrument_id is required")
	}
	if req.BorrowerFirmID == 0 {
		return newShortSellError(CodeMissingField, "borrower_firm_id is required")
	}
	if req.Quantity <= 0 {
		return newShortSellError(CodeInvalidField, "quantity must be greater than 0")
	}
	if req.ExpiresAt == "" {
		req.ExpiresAt = e.now().UTC().Add(defaultLocateTTL).Format(time.RFC3339)
	}
	if err := e.store.Create(req); err != nil {
		return err
	}
	return nil
}

// Approve transitions a PENDING locate to APPROVED, recording the lender firm.
func (e *LocateEngine) Approve(id, lenderFirmID string) error {
	if e.store == nil {
		return errLocateStoreUnavailable()
	}
	return e.store.Approve(id, lenderFirmID)
}

// Validate checks (without mutating) that loc may back a short-sell order in
// the given context. A zero instrumentID or borrowerFirmID skips the
// corresponding cross-check, which keeps the engine usable when the caller
// cannot resolve those identifiers to integers. Returns nil when usable.
func (e *LocateEngine) Validate(loc *types.LocateRequest, instrumentID, borrowerFirmID, quantity int) error {
	if loc == nil {
		return ErrLocateNotFound
	}
	if loc.Status != LocateStatusApproved {
		return newShortSellError(CodeInvalidLocate,
			"locate "+strconv.Itoa(loc.ID)+" is not in APPROVED status (current: "+loc.Status+")")
	}
	if e.IsExpired(loc) {
		return ErrLocateExpired
	}
	if instrumentID != 0 && loc.InstrumentID != 0 && loc.InstrumentID != instrumentID {
		return ErrLocateInstrumentMismatch
	}
	if borrowerFirmID != 0 && loc.BorrowerFirmID != 0 && loc.BorrowerFirmID != borrowerFirmID {
		return ErrLocateBorrowerMismatch
	}
	if quantity > 0 && loc.Quantity > 0 && quantity > loc.Quantity {
		return ErrLocateInsufficientQty
	}
	return nil
}

// Consume validates the locate identified by id for the given context and, if
// usable, marks it USED so it cannot back another order. The validity rules are
// those of Validate.
func (e *LocateEngine) Consume(id string, instrumentID, borrowerFirmID, quantity int) error {
	if e.store == nil {
		return errLocateStoreUnavailable()
	}
	loc, err := e.store.Get(id)
	if err != nil {
		if err == store.ErrNotFound {
			return ErrLocateNotFound
		}
		return err
	}
	if err := e.Validate(loc, instrumentID, borrowerFirmID, quantity); err != nil {
		return err
	}
	return e.store.Use(id)
}

// IsExpired reports whether the locate's ExpiresAt is set and in the past
// relative to the engine clock. An unparseable or empty expiry is treated as
// non-expiring (false) so malformed data never silently blocks trading.
func (e *LocateEngine) IsExpired(loc *types.LocateRequest) bool {
	if loc == nil || loc.ExpiresAt == "" {
		return false
	}
	exp, err := time.Parse(time.RFC3339, loc.ExpiresAt)
	if err != nil {
		return false
	}
	return e.now().After(exp)
}
