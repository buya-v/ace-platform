// Package engine provides a self-contained matching engine and clearing engine
// for load testing. Types and logic are duplicated from src/matching-engine and
// src/clearing-engine to avoid Go internal package import restrictions.
//
// This follows the zero-dep duplication pattern used across the GarudaX platform.
package engine

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// ---------------------------------------------------------------------------
// Decimal — fixed-point Decimal(18,4)
// ---------------------------------------------------------------------------

type Decimal struct {
	value int64
}

const decimalScale = 10000

func NewDecimal(integer int64, fraction int64) Decimal {
	return Decimal{value: integer*decimalScale + fraction}
}

func DecimalFromInt(v int64) Decimal {
	return Decimal{value: v * decimalScale}
}

func DecimalZero() Decimal { return Decimal{} }

func DecimalFromRaw(raw int64) Decimal {
	return Decimal{value: raw}
}

func ParseDecimal(s string) (Decimal, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return DecimalZero(), nil
	}
	negative := false
	if strings.HasPrefix(s, "-") {
		negative = true
		s = s[1:]
	}
	parts := strings.SplitN(s, ".", 2)
	intPart, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return DecimalZero(), fmt.Errorf("invalid decimal %q: %w", s, err)
	}
	var fracPart int64
	if len(parts) == 2 {
		fracStr := parts[1]
		if len(fracStr) > 4 {
			fracStr = fracStr[:4]
		}
		for len(fracStr) < 4 {
			fracStr += "0"
		}
		fracPart, err = strconv.ParseInt(fracStr, 10, 64)
		if err != nil {
			return DecimalZero(), fmt.Errorf("invalid decimal fraction %q: %w", s, err)
		}
	}
	val := intPart*decimalScale + fracPart
	if negative {
		val = -val
	}
	return Decimal{value: val}, nil
}

func MustParseDecimal(s string) Decimal {
	d, err := ParseDecimal(s)
	if err != nil {
		panic(err)
	}
	return d
}

func (d Decimal) Raw() int64               { return d.value }
func (d Decimal) IsZero() bool             { return d.value == 0 }
func (d Decimal) Equal(o Decimal) bool     { return d.value == o.value }
func (d Decimal) LessThan(o Decimal) bool  { return d.value < o.value }
func (d Decimal) GreaterThan(o Decimal) bool { return d.value > o.value }
func (d Decimal) GreaterThanOrEqual(o Decimal) bool { return d.value >= o.value }
func (d Decimal) LessThanOrEqual(o Decimal) bool    { return d.value <= o.value }
func (d Decimal) MulUint64(qty uint64) Decimal      { return Decimal{value: d.value * int64(qty)} }
func (d Decimal) Sub(o Decimal) Decimal              { return Decimal{value: d.value - o.value} }
func (d Decimal) Add(o Decimal) Decimal              { return Decimal{value: d.value + o.value} }
func (d Decimal) Abs() Decimal {
	if d.value < 0 {
		return Decimal{value: -d.value}
	}
	return d
}

func (d Decimal) String() string {
	negative := d.value < 0
	v := d.value
	if negative {
		v = -v
	}
	intPart := v / decimalScale
	fracPart := v % decimalScale
	sign := ""
	if negative {
		sign = "-"
	}
	if fracPart == 0 {
		return fmt.Sprintf("%s%d", sign, intPart)
	}
	fracStr := fmt.Sprintf("%04d", fracPart)
	fracStr = strings.TrimRight(fracStr, "0")
	return fmt.Sprintf("%s%d.%s", sign, intPart, fracStr)
}

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

type Side int

const (
	SideUnspecified Side = 0
	SideBuy         Side = 1
	SideSell        Side = 2
)

type OrderType int

const (
	OrderTypeLimit  OrderType = 1
	OrderTypeMarket OrderType = 2
)

type TimeInForce int

const (
	TIFDay TimeInForce = 1
	TIFGTC TimeInForce = 2
	TIFIOC TimeInForce = 4
	TIFFOK TimeInForce = 5
)

type OrderStatus int

const (
	OrderStatusNew             OrderStatus = 1
	OrderStatusPartiallyFilled OrderStatus = 2
	OrderStatusFilled          OrderStatus = 3
	OrderStatusCancelled       OrderStatus = 4
	OrderStatusRejected        OrderStatus = 5
)

type ExecType int

const (
	ExecTypeNew         ExecType = 1
	ExecTypePartialFill ExecType = 2
	ExecTypeFill        ExecType = 3
	ExecTypeCancelled   ExecType = 4
	ExecTypeRejected    ExecType = 5
)

type TradeType int

const (
	TradeTypeContinuous TradeType = 1
)

type BookState int

const (
	BookStateContinuous BookState = 3
)

type STPMode int

const (
	STPModeUnspecified  STPMode = 0
	STPModeCancelNewest STPMode = 1
)

type ClearingStatus int

const (
	ClearingStatusNovated ClearingStatus = 1
)

// ---------------------------------------------------------------------------
// Order / Trade / ExecutionReport
// ---------------------------------------------------------------------------

type Order struct {
	OrderID        string
	ClientOrderID  string
	InstrumentID   string
	AccountID      string
	ParticipantID  string
	Side           Side
	OrderType      OrderType
	TimeInForce    TimeInForce
	Price          Decimal
	StopPrice      Decimal
	Quantity       uint64
	RemainingQty   uint64
	FilledQty      uint64
	Status         OrderStatus
	STPMode        STPMode
	ExpireAt       time.Time
	CreatedAt      time.Time
	SequenceNumber uint64
}

func (o *Order) Fill(qty uint64) {
	o.FilledQty += qty
	o.RemainingQty -= qty
	if o.RemainingQty == 0 {
		o.Status = OrderStatusFilled
	} else {
		o.Status = OrderStatusPartiallyFilled
	}
}

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
	TradeType           TradeType
	SequenceNumber      uint64
	ExecutedAt          time.Time
}

type ExecutionReport struct {
	ExecID        string
	OrderID       string
	ClientOrderID string
	ExecType      ExecType
	OrderStatus   OrderStatus
	Side          Side
	InstrumentID  string
	Price         Decimal
	Quantity      uint64
	LastQty       uint64
	LastPrice     Decimal
	CumulativeQty uint64
	LeavesQty     uint64
	TradeID       string
	TransactTime  time.Time
	RejectReason  string
	AccountID     string
}

// ---------------------------------------------------------------------------
// IDGenerator
// ---------------------------------------------------------------------------

type IDGenerator interface {
	NewID() string
}

type SeqIDGen struct {
	Counter uint64
}

func (g *SeqIDGen) NewID() string {
	n := atomic.AddUint64(&g.Counter, 1)
	return fmt.Sprintf("id-%d", n)
}

// ---------------------------------------------------------------------------
// Clearing types
// ---------------------------------------------------------------------------

type ClearingObligation struct {
	ObligationID  string
	TradeID       string
	InstrumentID  string
	ParticipantID string
	Side          Side
	Price         Decimal
	Quantity      uint64
	Value         Decimal
	Status        ClearingStatus
	CreatedAt     time.Time
	NovatedAt     time.Time
}

type NettingResult struct {
	ParticipantID    string
	InstrumentID     string
	NetQuantity      int64
	NetValue         Decimal
	GrossLongQty     uint64
	GrossShortQty    uint64
	ObligationsCount int
	NettedAt         time.Time
}

func (n *NettingResult) NettingEfficiency() float64 {
	gross := float64(n.GrossLongQty + n.GrossShortQty)
	if gross == 0 {
		return 0
	}
	net := float64(n.NetQuantity)
	if net < 0 {
		net = -net
	}
	return (1.0 - net/gross) * 100.0
}
