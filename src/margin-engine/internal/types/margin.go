package types

import "time"

// Side represents a position side.
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

// Position represents a participant's net position in an instrument,
// mirroring the clearing-engine Position type for cross-service compatibility.
type Position struct {
	ParticipantID string
	InstrumentID  string
	NetQuantity   int64   // Positive = net long, Negative = net short
	AvgEntryPrice Decimal // Volume-weighted average entry price
	UpdatedAt     time.Time
}

// MarginCallStatus represents the state of a margin call.
type MarginCallStatus int

const (
	MarginCallPending    MarginCallStatus = 0
	MarginCallIssued     MarginCallStatus = 1
	MarginCallSatisfied  MarginCallStatus = 2
	MarginCallBreached   MarginCallStatus = 3
)

func (s MarginCallStatus) String() string {
	switch s {
	case MarginCallPending:
		return "PENDING"
	case MarginCallIssued:
		return "ISSUED"
	case MarginCallSatisfied:
		return "SATISFIED"
	case MarginCallBreached:
		return "BREACHED"
	default:
		return "UNKNOWN"
	}
}

// MarginRequirement represents the calculated margin for a single position.
type MarginRequirement struct {
	ParticipantID string
	InstrumentID  string
	NetQuantity   int64
	ScanRisk      Decimal // SPAN scanning risk (worst-case loss across scenarios)
	InterMonth    Decimal // Inter-month spread charge (zero for single instrument)
	DeliveryMonth Decimal // Delivery month charge
	ShortOption   Decimal // Short option minimum (zero for futures)
	InitialMargin Decimal // Max(ScanRisk + InterMonth + DeliveryMonth, ShortOption)
	MarkToMarket  Decimal // Unrealized P&L based on mark price
	TotalRequired Decimal // InitialMargin - MarkToMarket (if MtM is negative, adds to margin)
	CalculatedAt  time.Time
}

// PortfolioMargin represents the total margin requirement for a participant.
type PortfolioMargin struct {
	ParticipantID    string
	Requirements     []MarginRequirement
	TotalInitial     Decimal // Sum of all initial margin
	TotalMtM         Decimal // Sum of all mark-to-market
	TotalRequired    Decimal // Total margin required
	CollateralOnHand Decimal // Current collateral deposited
	ExcessDeficit    Decimal // Collateral - Required (negative = margin call)
	CalculatedAt     time.Time
}

// MarginCall represents a margin call issued to a participant.
type MarginCall struct {
	CallID        string
	ParticipantID string
	Required      Decimal // Total margin required
	OnHand        Decimal // Collateral when call was issued
	Deficit       Decimal // Amount that must be deposited
	Deadline      time.Time
	Status        MarginCallStatus
	IssuedAt      time.Time
	ResolvedAt    time.Time
}
