// Package engine — short-selling compliance checks.
//
// ShortSellEngine centralises the rules that gate a SHORT_SELL order:
//  1. the instrument must not be flagged short-sell restricted,
//  2. the order must carry a locate_id, and
//  3. that locate must be approved, unexpired and cover the order (delegated to
//     the LocateEngine).
//
// All rule failures surface as *ShortSellError, which carries a stable error
// code and an HTTP status so callers (HTTP handlers, the matching engine) can
// translate a single error type into a consistent client response.
package engine

import (
	"net/http"
	"strconv"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// Stable error codes returned via ShortSellError.Code. They match the codes the
// securities HTTP handlers already emit so the surface contract is unchanged.
const (
	CodeShortSellRestricted = "SHORT_SELL_RESTRICTED"
	CodeLocateRequired      = "LOCATE_REQUIRED"
	CodeInvalidLocate       = "INVALID_LOCATE"
	CodeMissingField        = "MISSING_FIELD"
	CodeInvalidField        = "INVALID_FIELD"
)

// ShortSellError is a compliance rejection with a machine-readable code.
type ShortSellError struct {
	Code    string
	Message string
}

// Error implements the error interface.
func (e *ShortSellError) Error() string { return e.Code + ": " + e.Message }

// HTTPStatus maps the error code to an HTTP status. Every short-sell compliance
// rejection is a 422 (the request was well-formed but violates a business rule).
func (e *ShortSellError) HTTPStatus() int { return http.StatusUnprocessableEntity }

func newShortSellError(code, msg string) *ShortSellError {
	return &ShortSellError{Code: code, Message: msg}
}

// Sentinel locate-validity errors. These all map to CodeInvalidLocate at the
// HTTP boundary but stay distinct so internal callers can branch on the cause.
var (
	// ErrLocateNotFound indicates the referenced locate does not exist.
	ErrLocateNotFound = newShortSellError(CodeInvalidLocate, "locate not found")
	// ErrLocateExpired indicates the locate's expiry has passed.
	ErrLocateExpired = newShortSellError(CodeInvalidLocate, "locate has expired")
	// ErrLocateInstrumentMismatch indicates the locate is for a different instrument.
	ErrLocateInstrumentMismatch = newShortSellError(CodeInvalidLocate, "locate is for a different instrument")
	// ErrLocateBorrowerMismatch indicates the locate belongs to a different borrower firm.
	ErrLocateBorrowerMismatch = newShortSellError(CodeInvalidLocate, "locate belongs to a different borrower firm")
	// ErrLocateInsufficientQty indicates the locate does not cover the order quantity.
	ErrLocateInsufficientQty = newShortSellError(CodeInvalidLocate, "locate quantity does not cover the order")
)

func errLocateStoreUnavailable() *ShortSellError {
	return newShortSellError(CodeInvalidLocate, "locate store not configured")
}

// ShortSellEngine evaluates short-sell compliance for orders.
type ShortSellEngine struct {
	instruments store.InstrumentStore
	locates     *LocateEngine
}

// NewShortSellEngine builds a ShortSellEngine. The locate store may be nil; when
// it is, locate existence/state cannot be verified and only the instrument
// restriction and locate-presence rules are enforced (matching the service's
// behaviour when no locate store is wired).
func NewShortSellEngine(inst store.InstrumentStore, loc store.LocateStore) *ShortSellEngine {
	return &ShortSellEngine{instruments: inst, locates: NewLocateEngine(loc)}
}

// EvaluateOrder enforces short-sell compliance for the given order and, on
// success, consumes the backing locate (marking it USED).
//
// It is a no-op returning nil for non-SHORT_SELL orders, so callers may invoke
// it unconditionally. On a rule violation it returns a *ShortSellError. The
// locate is consumed only when a locate store is configured; this preserves the
// existing service behaviour where presence of locate_id is required even if the
// locate cannot be looked up.
func (e *ShortSellEngine) EvaluateOrder(order *types.SecurityOrder, inst *types.Instrument) error {
	if order == nil || order.Side != types.OrderSideShortSell {
		return nil
	}
	if inst != nil && inst.ShortSellRestricted {
		return newShortSellError(CodeShortSellRestricted,
			"short selling is restricted for this instrument")
	}
	if order.LocateID == "" {
		return newShortSellError(CodeLocateRequired,
			"locate_id is required for short sell orders")
	}
	// Without a locate store the presence check above is the strongest guarantee
	// we can offer; accept and let downstream validation proceed.
	if e.locates == nil || e.locates.store == nil {
		return nil
	}
	instCtx := atoiSafe(order.InstrumentID)
	firmCtx := atoiSafe(order.FirmID)
	return e.locates.Consume(order.LocateID, instCtx, firmCtx, order.Quantity)
}

// LocateEngine exposes the underlying locate workflow engine so callers can
// drive locate request/approve/validate operations through the same instance.
func (e *ShortSellEngine) LocateEngine() *LocateEngine { return e.locates }

// atoiSafe parses s to an int, returning 0 when s is empty or non-numeric. A
// zero result signals "unknown" to LocateEngine.Validate, which then skips the
// corresponding cross-check.
func atoiSafe(s string) int {
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}
