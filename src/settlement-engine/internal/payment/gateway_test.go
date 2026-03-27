package payment

import (
	"testing"

	"github.com/ace-platform/settlement-engine/internal/types"
)

func TestInMemoryGatewaySuccess(t *testing.T) {
	gw := NewInMemoryGateway()

	inst := types.SettlementInstruction{
		InstructionID: "si-1",
		ParticipantID: "P1",
		Direction:     types.PayIn,
		Amount:        types.NewDecimal(500, 0),
	}

	result := gw.ProcessPayment(inst)
	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Error)
	}
	if result.Reference == "" {
		t.Error("expected non-empty payment reference")
	}
	if result.InstructionID != "si-1" {
		t.Errorf("expected instruction ID si-1, got %s", result.InstructionID)
	}
}

func TestInMemoryGatewayFailure(t *testing.T) {
	gw := NewInMemoryGateway()
	gw.SetFail("P1")

	inst := types.SettlementInstruction{
		InstructionID: "si-1",
		ParticipantID: "P1",
		Direction:     types.PayIn,
		Amount:        types.NewDecimal(500, 0),
	}

	result := gw.ProcessPayment(inst)
	if result.Success {
		t.Error("expected failure, got success")
	}
	if result.Error == "" {
		t.Error("expected error message")
	}
}

func TestInMemoryGatewayTracksPayments(t *testing.T) {
	gw := NewInMemoryGateway()

	for i := 0; i < 3; i++ {
		inst := types.SettlementInstruction{
			InstructionID: "si-" + string(rune('1'+i)),
			ParticipantID: "P1",
		}
		gw.ProcessPayment(inst)
	}

	payments := gw.GetPayments()
	if len(payments) != 3 {
		t.Errorf("expected 3 payments, got %d", len(payments))
	}
}

func TestProcessorAllSuccess(t *testing.T) {
	gw := NewInMemoryGateway()
	proc := NewProcessor(gw)

	instructions := []types.SettlementInstruction{
		{InstructionID: "si-1", ParticipantID: "P1", Direction: types.PayIn, Amount: types.NewDecimal(100, 0)},
		{InstructionID: "si-2", ParticipantID: "P2", Direction: types.PayOut, Amount: types.NewDecimal(100, 0)},
	}

	results := proc.ProcessAll(instructions)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	for _, r := range results {
		if r.Status != types.InstructionConfirmed {
			t.Errorf("expected CONFIRMED, got %s", r.Status)
		}
	}
}

func TestProcessorPartialFailure(t *testing.T) {
	gw := NewInMemoryGateway()
	gw.SetFail("P2")
	proc := NewProcessor(gw)

	instructions := []types.SettlementInstruction{
		{InstructionID: "si-1", ParticipantID: "P1", Direction: types.PayIn, Amount: types.NewDecimal(100, 0)},
		{InstructionID: "si-2", ParticipantID: "P2", Direction: types.PayOut, Amount: types.NewDecimal(100, 0)},
	}

	results := proc.ProcessAll(instructions)

	if results[0].Status != types.InstructionConfirmed {
		t.Errorf("expected P1 CONFIRMED, got %s", results[0].Status)
	}
	if results[1].Status != types.InstructionFailed {
		t.Errorf("expected P2 FAILED, got %s", results[1].Status)
	}
	if results[1].Error == "" {
		t.Error("expected error message for failed instruction")
	}
}
