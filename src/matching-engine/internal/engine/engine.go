package engine

import (
	"fmt"
	"sync"

	"github.com/ace-platform/matching-engine/internal/orderbook"
	"github.com/ace-platform/matching-engine/internal/types"
)

// TradeHandler is called for each trade produced by the engine.
// Implementations should persist trades (append-only) and publish events.
type TradeHandler func(trade types.Trade)

// ExecReportHandler is called for each execution report.
type ExecReportHandler func(report types.ExecutionReport)

// Engine manages multiple order books, one per instrument.
// Each order book is single-threaded; the engine serializes access per instrument.
type Engine struct {
	books     map[string]*bookEntry
	mu        sync.RWMutex
	globalSeq uint64

	idGen          orderbook.IDGenerator
	tradeHandler   TradeHandler
	execHandler    ExecReportHandler
}

type bookEntry struct {
	book *orderbook.OrderBook
	mu   sync.Mutex // Per-instrument lock for single-threaded matching
}

// NewEngine creates a new matching engine.
func NewEngine(idGen orderbook.IDGenerator) *Engine {
	return &Engine{
		books: make(map[string]*bookEntry),
		idGen: idGen,
	}
}

// SetTradeHandler sets the callback for trade events.
func (e *Engine) SetTradeHandler(h TradeHandler) {
	e.tradeHandler = h
}

// SetExecReportHandler sets the callback for execution report events.
func (e *Engine) SetExecReportHandler(h ExecReportHandler) {
	e.execHandler = h
}

// RegisterInstrument creates an order book for an instrument.
func (e *Engine) RegisterInstrument(instrumentID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.books[instrumentID]; exists {
		return fmt.Errorf("instrument %s already registered", instrumentID)
	}

	e.books[instrumentID] = &bookEntry{
		book: orderbook.NewOrderBook(instrumentID, e.idGen, &e.globalSeq),
	}
	return nil
}

// SubmitOrder submits an order to the matching engine.
func (e *Engine) SubmitOrder(order *types.Order) (orderbook.MatchResult, error) {
	entry, err := e.getBook(order.InstrumentID)
	if err != nil {
		return orderbook.MatchResult{}, err
	}

	entry.mu.Lock()
	result := entry.book.SubmitOrder(order)
	entry.mu.Unlock()

	e.dispatchResults(result)
	return result, nil
}

// CancelOrder cancels an order.
func (e *Engine) CancelOrder(instrumentID, orderID string) (types.ExecutionReport, error) {
	entry, err := e.getBook(instrumentID)
	if err != nil {
		return types.ExecutionReport{}, err
	}

	entry.mu.Lock()
	report, err := entry.book.CancelOrder(orderID)
	entry.mu.Unlock()

	if err != nil {
		return types.ExecutionReport{}, err
	}

	if e.execHandler != nil {
		e.execHandler(report)
	}
	return report, nil
}

// CancelAll cancels all orders for an account on a specific instrument.
func (e *Engine) CancelAll(instrumentID, accountID string, side types.Side) (uint32, []string, error) {
	entry, err := e.getBook(instrumentID)
	if err != nil {
		return 0, nil, err
	}

	entry.mu.Lock()
	count, ids := entry.book.CancelAll(accountID, side)
	entry.mu.Unlock()

	return count, ids, nil
}

// ModifyOrder modifies an existing order (cancel-replace semantics).
func (e *Engine) ModifyOrder(instrumentID, orderID, accountID string, newPrice types.Decimal, newQty uint64) (orderbook.MatchResult, error) {
	entry, err := e.getBook(instrumentID)
	if err != nil {
		return orderbook.MatchResult{}, err
	}

	entry.mu.Lock()
	result, err := entry.book.ModifyOrder(orderID, accountID, newPrice, newQty)
	entry.mu.Unlock()

	if err != nil {
		return orderbook.MatchResult{}, err
	}

	e.dispatchResults(result)
	return result, nil
}

// GetOrder retrieves an order by ID from a specific instrument book.
func (e *Engine) GetOrder(instrumentID, orderID string) (*types.Order, error) {
	entry, err := e.getBook(instrumentID)
	if err != nil {
		return nil, err
	}

	entry.mu.Lock()
	order, ok := entry.book.GetOrder(orderID)
	entry.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("order %s not found", orderID)
	}
	return order, nil
}

// GetOrderBook returns the order book for an instrument (for market data).
func (e *Engine) GetOrderBook(instrumentID string) (*orderbook.OrderBook, error) {
	entry, err := e.getBook(instrumentID)
	if err != nil {
		return nil, err
	}
	return entry.book, nil
}

func (e *Engine) getBook(instrumentID string) (*bookEntry, error) {
	e.mu.RLock()
	entry, ok := e.books[instrumentID]
	e.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("instrument %s not found", instrumentID)
	}
	return entry, nil
}

// CircuitBreakerStatus describes the circuit breaker state for an instrument.
type CircuitBreakerStatus struct {
	InstrumentID string `json:"instrument_id"`
	State        string `json:"state"`
	Halted       bool   `json:"halted"`
}

// GetCircuitBreakers returns circuit breaker status for all instruments.
func (e *Engine) GetCircuitBreakers() []CircuitBreakerStatus {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]CircuitBreakerStatus, 0, len(e.books))
	for id, entry := range e.books {
		state := "CONTINUOUS"
		halted := false
		switch entry.book.State {
		case types.BookStateHalted:
			state = "HALTED"
			halted = true
		case types.BookStateAuction:
			state = "AUCTION"
		}
		result = append(result, CircuitBreakerStatus{
			InstrumentID: id,
			State:        state,
			Halted:       halted,
		})
	}
	return result
}

// ListInstruments returns the IDs of all registered instruments.
func (e *Engine) ListInstruments() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	ids := make([]string, 0, len(e.books))
	for id := range e.books {
		ids = append(ids, id)
	}
	return ids
}

func (e *Engine) dispatchResults(result orderbook.MatchResult) {
	if e.tradeHandler != nil {
		for _, t := range result.Trades {
			e.tradeHandler(t)
		}
	}
	if e.execHandler != nil {
		for _, r := range result.ExecutionReports {
			e.execHandler(r)
		}
	}
}
