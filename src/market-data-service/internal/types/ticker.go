package types

import "time"

// Ticker represents a summary of 24h market data for an instrument.
type Ticker struct {
	InstrumentID    string
	Symbol          string
	LastPrice       Decimal
	PriceChange24h  Decimal
	PriceChangePct  Decimal // percentage e.g. 2.5000
	High24h         Decimal
	Low24h          Decimal
	Volume24h       uint64
	Turnover24h     Decimal
	BestBid         Decimal
	BestAsk         Decimal
	OpenInterest    int64
	LastTradeAt     time.Time
	Timestamp       time.Time
}
