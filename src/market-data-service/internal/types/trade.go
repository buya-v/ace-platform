package types

import "time"

// Trade represents a trade event ingested from the matching engine.
type Trade struct {
	TradeID        string
	InstrumentID   string
	Price          Decimal
	Quantity       uint64
	TradeValue     Decimal // price * quantity (pre-computed)
	AggressorSide  string  // "BUY" or "SELL"
	TradeType      string  // "CONTINUOUS", "AUCTION", "BLOCK"
	SequenceNumber uint64
	ExecutedAt     time.Time
}
