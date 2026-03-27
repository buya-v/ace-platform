package settlement

import (
	"time"

	"github.com/ace-platform/settlement-engine/internal/types"
)

// Generator creates settlement instructions from P&L records.
type Generator struct {
	idGen types.IDGenerator
}

func NewGenerator(idGen types.IDGenerator) *Generator {
	return &Generator{idGen: idGen}
}

// Generate creates net settlement instructions for a settlement cycle.
// Each participant receives one instruction: either PAY_IN (lost money) or PAY_OUT (gained money).
// Flat participants (net zero P&L) are excluded.
func (g *Generator) Generate(cycleID string, pnlRecords []types.PnLRecord) []types.SettlementInstruction {
	// Net P&L by participant
	nets := make(map[string]types.Decimal)
	for _, r := range pnlRecords {
		current, ok := nets[r.ParticipantID]
		if !ok {
			current = types.DecimalZero()
		}
		nets[r.ParticipantID] = current.Add(r.VariationMargin)
	}

	now := time.Now()
	instructions := make([]types.SettlementInstruction, 0, len(nets))

	for participantID, netAmount := range nets {
		if netAmount.IsZero() {
			continue
		}

		var direction types.PayDirection
		var amount types.Decimal
		if netAmount.IsNeg() {
			// Participant lost money — must pay in
			direction = types.PayIn
			amount = netAmount.Negate()
		} else {
			// Participant gained money — receives payout
			direction = types.PayOut
			amount = netAmount
		}

		instructions = append(instructions, types.SettlementInstruction{
			InstructionID: g.idGen.NewID(),
			CycleID:       cycleID,
			ParticipantID: participantID,
			Direction:     direction,
			Amount:        amount,
			Status:        types.InstructionPending,
			CreatedAt:     now,
		})
	}

	return instructions
}

// Totals calculates the total pay-in and pay-out amounts.
// In a balanced settlement, TotalPayIn == TotalPayOut (zero-sum).
func Totals(instructions []types.SettlementInstruction) (payIn, payOut types.Decimal) {
	payIn = types.DecimalZero()
	payOut = types.DecimalZero()
	for _, inst := range instructions {
		switch inst.Direction {
		case types.PayIn:
			payIn = payIn.Add(inst.Amount)
		case types.PayOut:
			payOut = payOut.Add(inst.Amount)
		}
	}
	return payIn, payOut
}
