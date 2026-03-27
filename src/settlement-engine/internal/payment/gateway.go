package payment

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ace-platform/settlement-engine/internal/types"
)

// Gateway defines the interface for processing settlement payments.
// Production implementations will integrate with banking/payment rails.
type Gateway interface {
	ProcessPayment(instruction types.SettlementInstruction) types.PaymentResult
}

// Processor handles submitting settlement instructions to the payment gateway
// and tracking their status.
type Processor struct {
	gateway Gateway
}

func NewProcessor(gateway Gateway) *Processor {
	return &Processor{gateway: gateway}
}

// ProcessAll submits all instructions to the payment gateway and returns updated instructions.
func (p *Processor) ProcessAll(instructions []types.SettlementInstruction) []types.SettlementInstruction {
	results := make([]types.SettlementInstruction, len(instructions))
	for i, inst := range instructions {
		inst.Status = types.InstructionSubmitted
		inst.SubmittedAt = time.Now()

		result := p.gateway.ProcessPayment(inst)
		if result.Success {
			inst.Status = types.InstructionConfirmed
			inst.ConfirmedAt = result.ProcessedAt
		} else {
			inst.Status = types.InstructionFailed
			inst.Error = result.Error
		}
		results[i] = inst
	}
	return results
}

// InMemoryGateway is a simple in-memory payment gateway for development and testing.
// All payments succeed by default. Can be configured to fail specific participants.
type InMemoryGateway struct {
	mu          sync.Mutex
	counter     uint64
	payments    []types.PaymentResult
	failSet     map[string]bool // participants that should fail
}

func NewInMemoryGateway() *InMemoryGateway {
	return &InMemoryGateway{
		payments: make([]types.PaymentResult, 0),
		failSet:  make(map[string]bool),
	}
}

// SetFail configures a participant ID to fail payment processing.
func (g *InMemoryGateway) SetFail(participantID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.failSet[participantID] = true
}

func (g *InMemoryGateway) ProcessPayment(inst types.SettlementInstruction) types.PaymentResult {
	g.mu.Lock()
	defer g.mu.Unlock()

	n := atomic.AddUint64(&g.counter, 1)
	now := time.Now()

	if g.failSet[inst.ParticipantID] {
		result := types.PaymentResult{
			InstructionID: inst.InstructionID,
			Success:       false,
			Error:         fmt.Sprintf("payment failed for participant %s", inst.ParticipantID),
			ProcessedAt:   now,
		}
		g.payments = append(g.payments, result)
		return result
	}

	result := types.PaymentResult{
		InstructionID: inst.InstructionID,
		Success:       true,
		Reference:     fmt.Sprintf("PAY-%06d", n),
		ProcessedAt:   now,
	}
	g.payments = append(g.payments, result)
	return result
}

// GetPayments returns all processed payments.
func (g *InMemoryGateway) GetPayments() []types.PaymentResult {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]types.PaymentResult, len(g.payments))
	copy(out, g.payments)
	return out
}
