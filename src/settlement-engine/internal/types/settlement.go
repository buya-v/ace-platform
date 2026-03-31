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

// Position represents a participant's net position in an instrument.
// Mirrors the clearing-engine Position type.
type Position struct {
	ParticipantID string
	InstrumentID  string
	NetQuantity   int64
	AvgEntryPrice Decimal
	UpdatedAt     time.Time
}

// SettlementCycleStatus represents the state of a daily settlement cycle.
type SettlementCycleStatus int

const (
	CycleStatusPending    SettlementCycleStatus = 0
	CycleStatusValuing    SettlementCycleStatus = 1
	CycleStatusCalculated SettlementCycleStatus = 2
	CycleStatusSettling   SettlementCycleStatus = 3
	CycleStatusCompleted  SettlementCycleStatus = 4
	CycleStatusFailed     SettlementCycleStatus = 5
)

func (s SettlementCycleStatus) String() string {
	switch s {
	case CycleStatusPending:
		return "PENDING"
	case CycleStatusValuing:
		return "VALUING"
	case CycleStatusCalculated:
		return "CALCULATED"
	case CycleStatusSettling:
		return "SETTLING"
	case CycleStatusCompleted:
		return "COMPLETED"
	case CycleStatusFailed:
		return "FAILED"
	default:
		return "UNKNOWN"
	}
}

// SettlementCycle represents a single day's end-of-day settlement run.
type SettlementCycle struct {
	CycleID       string
	SettleDate    time.Time
	Status        SettlementCycleStatus
	PnLRecords    []PnLRecord
	Instructions  []SettlementInstruction
	TotalPayIn    Decimal // Sum of all amounts owed by participants
	TotalPayOut   Decimal // Sum of all amounts owed to participants
	StartedAt     time.Time
	CompletedAt   time.Time
	Error         string
}

// SettlementPrice holds the mark/settlement price for an instrument on a given date.
type SettlementPrice struct {
	InstrumentID   string
	SettleDate     time.Time
	SettlementPrice Decimal
	PreviousPrice  Decimal
}

// PnLRecord holds the daily mark-to-market P&L for a single position.
type PnLRecord struct {
	ParticipantID  string
	InstrumentID   string
	NetQuantity    int64
	PreviousPrice  Decimal // Previous settlement price (or entry price for new positions)
	CurrentPrice   Decimal // Today's settlement price
	VariationMargin Decimal // Daily MtM P&L = (current - previous) * qty
	CalculatedAt   time.Time
}

// SettlementInstructionStatus represents the payment state.
type SettlementInstructionStatus int

const (
	InstructionPending   SettlementInstructionStatus = 0
	InstructionSubmitted SettlementInstructionStatus = 1
	InstructionConfirmed SettlementInstructionStatus = 2
	InstructionFailed    SettlementInstructionStatus = 3
)

func (s SettlementInstructionStatus) String() string {
	switch s {
	case InstructionPending:
		return "PENDING"
	case InstructionSubmitted:
		return "SUBMITTED"
	case InstructionConfirmed:
		return "CONFIRMED"
	case InstructionFailed:
		return "FAILED"
	default:
		return "UNKNOWN"
	}
}

// PayDirection indicates whether a participant pays or receives.
type PayDirection int

const (
	PayIn  PayDirection = 1 // Participant owes money (lost on MtM)
	PayOut PayDirection = 2 // Participant receives money (gained on MtM)
)

func (d PayDirection) String() string {
	if d == PayIn {
		return "PAY_IN"
	}
	return "PAY_OUT"
}

// SettlementInstruction is a net payment instruction for a participant.
// After netting all P&L across instruments, each participant has one instruction.
type SettlementInstruction struct {
	InstructionID string
	CycleID       string
	ParticipantID string
	Direction     PayDirection
	Amount        Decimal // Always positive; direction indicates pay/receive
	Status        SettlementInstructionStatus
	CreatedAt     time.Time
	SubmittedAt   time.Time
	ConfirmedAt   time.Time
	Error         string
}

// PaymentResult is returned by the payment gateway after processing.
type PaymentResult struct {
	InstructionID string
	Success       bool
	Reference     string // External payment reference
	Error         string
	ProcessedAt   time.Time
}

// IDGenerator generates unique IDs.
type IDGenerator interface {
	NewID() string
}

// InstrumentType distinguishes between physical delivery and cash-settled instruments.
type InstrumentType int

const (
	// InstrumentCashSettled instruments settle only via cash payment (variation margin).
	InstrumentCashSettled InstrumentType = 0
	// InstrumentPhysicalDelivery instruments require commodity delivery alongside cash payment.
	InstrumentPhysicalDelivery InstrumentType = 1
)

func (t InstrumentType) String() string {
	if t == InstrumentPhysicalDelivery {
		return "PHYSICAL_DELIVERY"
	}
	return "CASH_SETTLED"
}

// InstrumentConfig holds settlement configuration for an instrument.
type InstrumentConfig struct {
	InstrumentID   string
	Type           InstrumentType
	ContractUnit   string  // e.g. "MT" (metric tons), "BBL" (barrels)
	ContractSize   int64   // units per contract, e.g. 100 MT per lot
}

// DeliveryReceiptStatus tracks the lifecycle of a commodity delivery.
type DeliveryReceiptStatus int

const (
	DeliveryPending   DeliveryReceiptStatus = 0
	DeliveryConfirmed DeliveryReceiptStatus = 1
	DeliveryFailed    DeliveryReceiptStatus = 2
)

func (s DeliveryReceiptStatus) String() string {
	switch s {
	case DeliveryPending:
		return "PENDING"
	case DeliveryConfirmed:
		return "CONFIRMED"
	case DeliveryFailed:
		return "FAILED"
	default:
		return "UNKNOWN"
	}
}

// DeliveryReceipt represents proof that physical commodity was delivered.
type DeliveryReceipt struct {
	ReceiptID     string
	InstrumentID  string
	SellerID      string
	BuyerID       string
	Quantity      int64   // in contract units
	WarehouseID   string
	Status        DeliveryReceiptStatus
	IssuedAt      time.Time
	ConfirmedAt   time.Time
	Error         string
}

// DVPStep tracks each step of the DVP coordination sequence.
type DVPStep int

const (
	DVPStepValidateDelivery DVPStep = 1
	DVPStepLockPayment      DVPStep = 2
	DVPStepConfirmDelivery  DVPStep = 3
	DVPStepReleasePayment   DVPStep = 4
)

func (s DVPStep) String() string {
	switch s {
	case DVPStepValidateDelivery:
		return "VALIDATE_DELIVERY"
	case DVPStepLockPayment:
		return "LOCK_PAYMENT"
	case DVPStepConfirmDelivery:
		return "CONFIRM_DELIVERY"
	case DVPStepReleasePayment:
		return "RELEASE_PAYMENT"
	default:
		return "UNKNOWN"
	}
}

// DVPResultStatus is the outcome of a DVP coordination.
type DVPResultStatus int

const (
	DVPSucceeded  DVPResultStatus = 0
	DVPFailed     DVPResultStatus = 1
	DVPRolledBack DVPResultStatus = 2
)

func (s DVPResultStatus) String() string {
	switch s {
	case DVPSucceeded:
		return "SUCCEEDED"
	case DVPFailed:
		return "FAILED"
	case DVPRolledBack:
		return "ROLLED_BACK"
	default:
		return "UNKNOWN"
	}
}

// DVPResult is the outcome of a delivery-vs-payment coordination.
type DVPResult struct {
	InstrumentID     string
	Status           DVPResultStatus
	DeliveryReceipts []DeliveryReceipt
	Instructions     []SettlementInstruction
	FailedAtStep     DVPStep
	Error            string
	CompletedAt      time.Time
}

// MultiInstrumentResult holds aggregated settlement results across all instruments.
type MultiInstrumentResult struct {
	CycleID              string
	InstrumentResults    map[string]*InstrumentSettlementResult
	AggregatedPayIn      Decimal
	AggregatedPayOut     Decimal
	NetParticipantAmounts map[string]Decimal // participant -> net amount (positive = receive, negative = owe)
}

// InstrumentSettlementResult holds settlement results for a single instrument within a cycle.
type InstrumentSettlementResult struct {
	InstrumentID   string
	InstrumentType InstrumentType
	PnLRecords     []PnLRecord
	DVPResult      *DVPResult // nil for cash-settled instruments
	PayIn          Decimal
	PayOut         Decimal
	Error          string
}
