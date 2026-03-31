// Package store provides storage interfaces and implementations for market data.
package store

import (
	"time"

	"github.com/garudax-platform/market-data-service/internal/types"
)

// TradeRepository defines the interface for trade storage operations.
type TradeRepository interface {
	// Append adds a trade to the store.
	Append(trade types.Trade)
	// LastN returns the last N trades for an instrument, newest first.
	LastN(instrumentID string, n int) []types.Trade
	// SinceSequence returns trades with sequence number > sinceSequence.
	SinceSequence(instrumentID string, sinceSequence uint64) []types.Trade
	// InTimeRange returns trades in [start, end) for an instrument.
	InTimeRange(instrumentID string, start, end time.Time, limit int) []types.Trade
	// LastTrade returns the most recent trade for an instrument.
	LastTrade(instrumentID string) (types.Trade, bool)
	// AllInstruments returns all instrument IDs that have trades.
	AllInstruments() []string
	// Len returns the number of trades for an instrument.
	Len(instrumentID string) int
}

// CandleRepository defines the interface for candle storage operations.
type CandleRepository interface {
	// Store persists a candle (upsert by key).
	Store(c types.Candle)
	// Query returns candles for an instrument and interval within [start, end),
	// ordered by bucket ascending.
	Query(instrumentID string, interval types.CandleInterval, start, end time.Time, limit int) []types.Candle
	// DeleteBefore removes candles older than the given time for a specific interval.
	DeleteBefore(interval types.CandleInterval, before time.Time) int
}

// TickerRepository defines the interface for ticker storage operations.
type TickerRepository interface {
	// Upsert inserts or updates a ticker for an instrument.
	Upsert(t types.Ticker)
	// Get returns the ticker for an instrument.
	Get(instrumentID string) (types.Ticker, bool)
	// GetAll returns tickers for the specified instruments (empty = all).
	GetAll(instrumentIDs []string) []types.Ticker
}
