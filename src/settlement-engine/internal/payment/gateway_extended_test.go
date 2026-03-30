package payment

import (
	"strings"
	"testing"

	"github.com/garudax-platform/settlement-engine/internal/types"
)

func TestProcessorEmptyInstructions(t *testing.T) {
	gw := NewInMemoryGateway()
	proc := NewProcessor(gw)

	results := proc.ProcessAll(nil)
	if len(results) != 0 {
		t.Errorf("expected 0 results for nil input, got %d", len(results))
	}

	results = proc.ProcessAll([]types.SettlementInstruction{})
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty input, got %d", len(results))
	}
}

func TestProcessorAllFailed(t *testing.T) {
	gw := NewInMemoryGateway()
	gw.SetFail("P1")
	gw.SetFail("P2")
	gw.SetFail("P3")
	proc := NewProcessor(gw)

	instructions := []types.SettlementInstruction{
		{InstructionID: "si-1", ParticipantID: "P1", Amount: types.NewDecimal(100, 0)},
		{InstructionID: "si-2", ParticipantID: "P2", Amount: types.NewDecimal(200, 0)},
		{InstructionID: "si-3", ParticipantID: "P3", Amount: types.NewDecimal(300, 0)},
	}

	results := proc.ProcessAll(instructions)
	for i, r := range results {
		if r.Status != types.InstructionFailed {
			t.Errorf("result[%d]: expected FAILED, got %s", i, r.Status)
		}
		if r.Error == "" {
			t.Errorf("result[%d]: expected error message", i)
		}
	}
}

func TestProcessorConfirmedHasTimestamp(t *testing.T) {
	gw := NewInMemoryGateway()
	proc := NewProcessor(gw)

	instructions := []types.SettlementInstruction{
		{InstructionID: "si-1", ParticipantID: "P1", Amount: types.NewDecimal(100, 0)},
	}

	results := proc.ProcessAll(instructions)
	if results[0].ConfirmedAt.IsZero() {
		t.Error("expected non-zero ConfirmedAt for confirmed instruction")
	}
	if !results[0].SubmittedAt.IsZero() {
		// SubmittedAt is set before gateway call
		if results[0].SubmittedAt.After(results[0].ConfirmedAt) {
			t.Error("SubmittedAt should not be after ConfirmedAt")
		}
	}
}

func TestProcessorFailedHasNoConfirmedAt(t *testing.T) {
	gw := NewInMemoryGateway()
	gw.SetFail("P1")
	proc := NewProcessor(gw)

	instructions := []types.SettlementInstruction{
		{InstructionID: "si-1", ParticipantID: "P1", Amount: types.NewDecimal(100, 0)},
	}

	results := proc.ProcessAll(instructions)
	if !results[0].ConfirmedAt.IsZero() {
		t.Error("expected zero ConfirmedAt for failed instruction")
	}
}

func TestInMemoryGatewaySequentialReferences(t *testing.T) {
	gw := NewInMemoryGateway()

	for i := 0; i < 5; i++ {
		inst := types.SettlementInstruction{
			InstructionID: "si",
			ParticipantID: "P1",
		}
		result := gw.ProcessPayment(inst)
		if !strings.HasPrefix(result.Reference, "PAY-") {
			t.Errorf("expected PAY- prefix, got %s", result.Reference)
		}
	}

	payments := gw.GetPayments()
	// Verify all references are unique
	refs := make(map[string]bool)
	for _, p := range payments {
		if refs[p.Reference] {
			t.Errorf("duplicate reference: %s", p.Reference)
		}
		refs[p.Reference] = true
	}
}

func TestInMemoryGatewayFailErrorMessage(t *testing.T) {
	gw := NewInMemoryGateway()
	gw.SetFail("ACME-CORP")

	inst := types.SettlementInstruction{
		InstructionID: "si-1",
		ParticipantID: "ACME-CORP",
	}

	result := gw.ProcessPayment(inst)
	if !strings.Contains(result.Error, "ACME-CORP") {
		t.Errorf("error message should contain participant ID, got: %s", result.Error)
	}
}

func TestInMemoryGatewayMixedSuccessAndFailure(t *testing.T) {
	gw := NewInMemoryGateway()
	gw.SetFail("P2")

	participants := []string{"P1", "P2", "P3", "P2", "P1"}
	expectedSuccess := []bool{true, false, true, false, true}

	for i, pid := range participants {
		inst := types.SettlementInstruction{
			InstructionID: "si",
			ParticipantID: pid,
		}
		result := gw.ProcessPayment(inst)
		if result.Success != expectedSuccess[i] {
			t.Errorf("payment %d (participant %s): expected success=%v, got %v",
				i, pid, expectedSuccess[i], result.Success)
		}
	}

	payments := gw.GetPayments()
	if len(payments) != 5 {
		t.Errorf("expected 5 tracked payments, got %d", len(payments))
	}
}

func TestProcessorMultiCurrencySettlement(t *testing.T) {
	// Simulates settlement across multiple instruments with different amounts
	gw := NewInMemoryGateway()
	proc := NewProcessor(gw)

	instructions := []types.SettlementInstruction{
		{InstructionID: "si-1", ParticipantID: "P1", Direction: types.PayIn, Amount: types.NewDecimal(1000, 5000)},
		{InstructionID: "si-2", ParticipantID: "P2", Direction: types.PayOut, Amount: types.NewDecimal(500, 2500)},
		{InstructionID: "si-3", ParticipantID: "P3", Direction: types.PayIn, Amount: types.NewDecimal(0, 1)},  // tiny amount
		{InstructionID: "si-4", ParticipantID: "P4", Direction: types.PayOut, Amount: types.NewDecimal(999999, 9999)},
	}

	results := proc.ProcessAll(instructions)
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}

	for i, r := range results {
		if r.Status != types.InstructionConfirmed {
			t.Errorf("result[%d]: expected CONFIRMED, got %s", i, r.Status)
		}
	}
}

func TestProcessorLargeSettlement(t *testing.T) {
	gw := NewInMemoryGateway()
	proc := NewProcessor(gw)

	n := 100
	instructions := make([]types.SettlementInstruction, n)
	for i := 0; i < n; i++ {
		instructions[i] = types.SettlementInstruction{
			InstructionID: "si",
			ParticipantID: "P1",
			Direction:     types.PayIn,
			Amount:        types.NewDecimal(int64(i+1), 0),
		}
	}

	results := proc.ProcessAll(instructions)
	if len(results) != n {
		t.Fatalf("expected %d results, got %d", n, len(results))
	}

	confirmed := 0
	for _, r := range results {
		if r.Status == types.InstructionConfirmed {
			confirmed++
		}
	}
	if confirmed != n {
		t.Errorf("expected all %d confirmed, got %d", n, confirmed)
	}
}

func TestInMemoryGatewayGetPaymentsReturnsCopy(t *testing.T) {
	gw := NewInMemoryGateway()
	inst := types.SettlementInstruction{InstructionID: "si-1", ParticipantID: "P1"}
	gw.ProcessPayment(inst)

	payments1 := gw.GetPayments()
	payments2 := gw.GetPayments()

	// Modifying one slice should not affect the other
	if len(payments1) != 1 || len(payments2) != 1 {
		t.Fatal("expected 1 payment in each copy")
	}
	payments1[0].Reference = "MODIFIED"
	if payments2[0].Reference == "MODIFIED" {
		t.Error("GetPayments should return a copy, not a reference")
	}
}
