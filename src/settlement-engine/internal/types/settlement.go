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
