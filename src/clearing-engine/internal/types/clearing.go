package types

import "time"

// Side represents a trade/position side.
type Side int

const (
	SideBuy  Side = 1
	SideSell Side = 2
)

func (s Side) String() string {
	if s == SideBuy {
		return "BUY"
	}
	return "SELL"
}

func (s Side) Opposite() Side {
	if s == SideBuy {
		return SideSell
	}
	return SideBuy
}

// Trade represents an incoming trade from the matching engine.
// This mirrors the matching-engine's Trade type.
type Trade struct {
	TradeID             string
	InstrumentID        string
	BuyOrderID          string
	SellOrderID         string
	BuyerParticipantID  string
	SellerParticipantID string
	Price               Decimal
	Quantity            uint64
	TradeValue          Decimal
	AggressorSide       Side
	SequenceNumber      uint64
	ExecutedAt          time.Time
}

// ClearingStatus represents the lifecycle state of a clearing obligation.
type ClearingStatus int

const (
	ClearingStatusPending   ClearingStatus = 0
	ClearingStatusNovated   ClearingStatus = 1
	ClearingStatusNetted    ClearingStatus = 2
	ClearingStatusSettled   ClearingStatus = 3
	ClearingStatusRejected  ClearingStatus = 4
)

func (s ClearingStatus) String() string {
	switch s {
	case ClearingStatusPending:
		return "PENDING"
	case ClearingStatusNovated:
		return "NOVATED"
	case ClearingStatusNetted:
		return "NETTED"
	case ClearingStatusSettled:
		return "SETTLED"
	case ClearingStatusRejected:
		return "REJECTED"
	default:
		return "UNKNOWN"
	}
}

// ClearingObligation represents a novated obligation between a participant and the CCP.
// Trade novation splits one bilateral trade into two CCP-intermediated obligations.
type ClearingObligation struct {
	ObligationID  string
	TradeID       string
	InstrumentID  string
	ParticipantID string
	Side          Side
	Price         Decimal
	Quantity      uint64
	Value         Decimal // price * quantity
	Status        ClearingStatus
	CreatedAt     time.Time
	NovatedAt     time.Time
}

// Position represents a participant's net position in an instrument.
type Position struct {
	ParticipantID string
	InstrumentID  string
	NetQuantity   int64   // Positive = net long, Negative = net short
	AvgEntryPrice Decimal // Volume-weighted average entry price
	TotalBuyQty   uint64
	TotalSellQty  uint64
	RealizedPnL   Decimal // Cumulative realized P&L from netted trades
	UpdatedAt     time.Time
}

// IsLong returns true if net position is long.
func (p *Position) IsLong() bool { return p.NetQuantity > 0 }

// IsShort returns true if net position is short.
func (p *Position) IsShort() bool { return p.NetQuantity < 0 }

// IsFlat returns true if position is flat (zero).
func (p *Position) IsFlat() bool { return p.NetQuantity == 0 }

// NettingResult represents the output of multilateral netting for a participant.
type NettingResult struct {
	ParticipantID    string
	InstrumentID     string
	NetQuantity      int64   // Final net obligation quantity
	NetValue         Decimal // Final net obligation value
	GrossLongQty     uint64  // Total long before netting
	GrossShortQty    uint64  // Total short before netting
	ObligationsCount int     // Number of obligations netted
	NettedAt         time.Time
}

// NettingEfficiency returns the percentage reduction from gross to net.
// A higher value means more offsetting occurred.
func (n *NettingResult) NettingEfficiency() float64 {
	gross := float64(n.GrossLongQty + n.GrossShortQty)
	if gross == 0 {
		return 0
	}
	net := float64(abs64(n.NetQuantity))
	return (1.0 - net/gross) * 100.0
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// IDGenerator generates unique IDs for clearing obligations.
type IDGenerator interface {
	NewID() string
}
