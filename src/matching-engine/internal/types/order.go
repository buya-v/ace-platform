package types

import (
	"time"
)

// Side represents the order side.
type Side int

const (
	SideUnspecified Side = 0
	SideBuy         Side = 1
	SideSell        Side = 2
)

func (s Side) String() string {
	switch s {
	case SideBuy:
		return "BUY"
	case SideSell:
		return "SELL"
	default:
		return "UNSPECIFIED"
	}
}

// Opposite returns the opposite side.
func (s Side) Opposite() Side {
	if s == SideBuy {
		return SideSell
	}
	return SideBuy
}

// OrderType represents the type of order.
type OrderType int

const (
	OrderTypeUnspecified OrderType = 0
	OrderTypeLimit      OrderType = 1
	OrderTypeMarket     OrderType = 2
	OrderTypeStopLimit  OrderType = 3
	OrderTypeStopMarket OrderType = 4
)

// TimeInForce represents how long an order remains active.
type TimeInForce int

const (
	TIFUnspecified TimeInForce = 0
	TIFDay         TimeInForce = 1
	TIFGTC         TimeInForce = 2
	TIFGTD         TimeInForce = 3
	TIFIOC         TimeInForce = 4
	TIFFOK         TimeInForce = 5
)

// OrderStatus represents the current state of an order.
type OrderStatus int

const (
	OrderStatusUnspecified     OrderStatus = 0
	OrderStatusNew             OrderStatus = 1
	OrderStatusPartiallyFilled OrderStatus = 2
	OrderStatusFilled          OrderStatus = 3
	OrderStatusCancelled       OrderStatus = 4
	OrderStatusRejected        OrderStatus = 5
	OrderStatusExpired         OrderStatus = 6
	OrderStatusPendingNew      OrderStatus = 7
)

func (s OrderStatus) String() string {
	switch s {
	case OrderStatusNew:
		return "NEW"
	case OrderStatusPartiallyFilled:
		return "PARTIALLY_FILLED"
	case OrderStatusFilled:
		return "FILLED"
	case OrderStatusCancelled:
		return "CANCELLED"
	case OrderStatusRejected:
		return "REJECTED"
	case OrderStatusExpired:
		return "EXPIRED"
	default:
		return "UNSPECIFIED"
	}
}

// ExecType represents the type of execution report.
type ExecType int

const (
	ExecTypeUnspecified  ExecType = 0
	ExecTypeNew         ExecType = 1
	ExecTypePartialFill ExecType = 2
	ExecTypeFill        ExecType = 3
	ExecTypeCancelled   ExecType = 4
	ExecTypeRejected    ExecType = 5
	ExecTypeExpired     ExecType = 6
)

// TradeType represents how the trade was generated.
type TradeType int

const (
	TradeTypeUnspecified TradeType = 0
	TradeTypeContinuous TradeType = 1
	TradeTypeAuction    TradeType = 2
)

// BookState represents the state of an order book.
type BookState int

const (
	BookStateUnspecified BookState = 0
	BookStatePreOpen     BookState = 1
	BookStateAuction     BookState = 2
	BookStateContinuous  BookState = 3
	BookStateHalted      BookState = 4
	BookStateClosed      BookState = 5
)

// STPMode represents self-trade prevention mode.
type STPMode int

const (
	STPModeUnspecified   STPMode = 0
	STPModeCancelNewest  STPMode = 1
	STPModeCancelOldest  STPMode = 2
	STPModeCancelBoth    STPMode = 3
)

// Order represents an order in the matching engine.
type Order struct {
	OrderID        string
	ClientOrderID  string
	InstrumentID   string
	AccountID      string
	ParticipantID  string
	Side           Side
	OrderType      OrderType
	TimeInForce    TimeInForce
	Price          Decimal // Zero for MARKET orders
	StopPrice      Decimal // Zero unless STOP_*
	Quantity       uint64  // Original quantity in lots
	RemainingQty   uint64
	FilledQty      uint64
	Status         OrderStatus
	STPMode        STPMode
	ExpireAt       time.Time // Zero for GTC
	CreatedAt      time.Time
	SequenceNumber uint64 // Global sequence assigned on acceptance
}

// IsFilled returns true if the order is fully filled.
func (o *Order) IsFilled() bool {
	return o.RemainingQty == 0
}

// Fill reduces the remaining quantity and increases filled quantity.
func (o *Order) Fill(qty uint64) {
	o.FilledQty += qty
	o.RemainingQty -= qty
	if o.RemainingQty == 0 {
		o.Status = OrderStatusFilled
	} else {
		o.Status = OrderStatusPartiallyFilled
	}
}

// Trade represents a matched trade.
type Trade struct {
	TradeID              string
	InstrumentID         string
	BuyOrderID           string
	SellOrderID          string
	BuyerParticipantID   string
	SellerParticipantID  string
	Price                Decimal
	Quantity             uint64
	TradeValue           Decimal // price * quantity
	AggressorSide        Side
	TradeType            TradeType
	SequenceNumber       uint64
	ExecutedAt           time.Time
}

// ExecutionReport represents a report of an order state change.
type ExecutionReport struct {
	ExecID        string
	OrderID       string
	ClientOrderID string
	ExecType      ExecType
	OrderStatus   OrderStatus
	Side          Side
	InstrumentID  string
	Price         Decimal // Order price
	Quantity      uint64  // Order quantity
	LastQty       uint64  // Fill quantity (0 if not a fill)
	LastPrice     Decimal // Fill price
	CumulativeQty uint64  // Total filled so far
	LeavesQty     uint64  // Remaining quantity
	TradeID       string  // Set only for fills
	TransactTime  time.Time
	RejectReason  string // Set only for REJECTED
	AccountID     string
}
