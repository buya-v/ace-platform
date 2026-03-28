// Package ticker computes 24h ticker summaries from trade data.
package ticker

import (
	"sync"
	"time"

	"github.com/ace-platform/market-data-service/internal/types"
)

// Engine computes and caches ticker summaries per instrument.
type Engine struct {
	mu      sync.RWMutex
	tickers map[string]*tickerState
	symbols map[string]string // instrumentID -> symbol
}

type tickerState struct {
	trades    []types.Trade // rolling 24h window
	lastTrade types.Trade
}

// NewEngine creates a new ticker computation engine.
func NewEngine() *Engine {
	return &Engine{
		tickers: make(map[string]*tickerState),
		symbols: make(map[string]string),
	}
}

// SetSymbol registers a symbol name for an instrument.
func (e *Engine) SetSymbol(instrumentID, symbol string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.symbols[instrumentID] = symbol
}

// IngestTrade adds a trade and updates the ticker state.
func (e *Engine) IngestTrade(trade types.Trade) {
	e.mu.Lock()
	defer e.mu.Unlock()

	state, ok := e.tickers[trade.InstrumentID]
	if !ok {
		state = &tickerState{}
		e.tickers[trade.InstrumentID] = state
	}

	state.trades = append(state.trades, trade)
	state.lastTrade = trade
}

// GetTicker computes the current ticker for an instrument.
func (e *Engine) GetTicker(instrumentID string) (types.Ticker, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	state, ok := e.tickers[instrumentID]
	if !ok || len(state.trades) == 0 {
		return types.Ticker{}, false
	}

	now := time.Now().UTC()
	cutoff := now.Add(-24 * time.Hour)

	return e.computeTicker(instrumentID, state, cutoff, now), true
}

// GetTickers computes tickers for multiple instruments. Empty list = all.
func (e *Engine) GetTickers(instrumentIDs []string) []types.Ticker {
	e.mu.RLock()
	defer e.mu.RUnlock()

	now := time.Now().UTC()
	cutoff := now.Add(-24 * time.Hour)

	if len(instrumentIDs) == 0 {
		instrumentIDs = make([]string, 0, len(e.tickers))
		for id := range e.tickers {
			instrumentIDs = append(instrumentIDs, id)
		}
	}

	var result []types.Ticker
	for _, id := range instrumentIDs {
		state, ok := e.tickers[id]
		if !ok || len(state.trades) == 0 {
			continue
		}
		result = append(result, e.computeTicker(id, state, cutoff, now))
	}
	return result
}

func (e *Engine) computeTicker(instrumentID string, state *tickerState, cutoff, now time.Time) types.Ticker {
	var (
		high24h   types.Decimal
		low24h    types.Decimal
		volume24h uint64
		turnover  types.Decimal
		first24h  types.Decimal
		hasFirst  bool
	)

	for _, t := range state.trades {
		if t.ExecutedAt.Before(cutoff) {
			continue
		}
		if !hasFirst {
			first24h = t.Price
			high24h = t.Price
			low24h = t.Price
			hasFirst = true
		}
		if t.Price.GreaterThan(high24h) {
			high24h = t.Price
		}
		if t.Price.LessThan(low24h) {
			low24h = t.Price
		}
		volume24h += t.Quantity
		turnover = turnover.Add(t.Price.MulUint64(t.Quantity))
	}

	lastPrice := state.lastTrade.Price
	change := lastPrice.Sub(first24h)
	var changePct types.Decimal
	if !first24h.IsZero() {
		// changePct = (change / first24h) * 100, using fixed-point: (change * 100 * scale) / first24h
		changePct = types.DecimalFromRaw(change.Raw() * 100 * 10000 / first24h.Raw())
	}

	ticker := types.Ticker{
		InstrumentID:   instrumentID,
		Symbol:         e.symbols[instrumentID],
		LastPrice:      lastPrice,
		PriceChange24h: change,
		PriceChangePct: changePct,
		High24h:        high24h,
		Low24h:         low24h,
		Volume24h:      volume24h,
		Turnover24h:    turnover,
		LastTradeAt:    state.lastTrade.ExecutedAt,
		Timestamp:      now,
	}
	return ticker
}

// PruneBefore removes trades older than the given time from the rolling window.
func (e *Engine) PruneBefore(cutoff time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, state := range e.tickers {
		pruned := state.trades[:0]
		for _, t := range state.trades {
			if !t.ExecutedAt.Before(cutoff) {
				pruned = append(pruned, t)
			}
		}
		state.trades = pruned
	}
}
