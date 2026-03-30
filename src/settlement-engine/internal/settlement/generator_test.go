package settlement

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/garudax-platform/settlement-engine/internal/types"
)

type seqIDGen struct {
	counter uint64
}

func (g *seqIDGen) NewID() string {
	n := atomic.AddUint64(&g.counter, 1)
	return fmt.Sprintf("test-%d", n)
}

func TestGeneratePayInPayOut(t *testing.T) {
	gen := NewGenerator(&seqIDGen{})

	records := []types.PnLRecord{
		{ParticipantID: "P1", VariationMargin: types.NewDecimal(200, 0)},
		{ParticipantID: "P2", VariationMargin: types.NewDecimal(-200, 0)},
	}

	instructions := gen.Generate("cycle-1", records)
	if len(instructions) != 2 {
		t.Fatalf("expected 2 instructions, got %d", len(instructions))
	}

	instrByParticipant := make(map[string]types.SettlementInstruction)
	for _, inst := range instructions {
		instrByParticipant[inst.ParticipantID] = inst
	}

	p1 := instrByParticipant["P1"]
	if p1.Direction != types.PayOut {
		t.Errorf("expected P1 PayOut, got %s", p1.Direction)
	}
	if !p1.Amount.Equal(types.NewDecimal(200, 0)) {
		t.Errorf("expected P1 amount 200, got %s", p1.Amount)
	}

	p2 := instrByParticipant["P2"]
	if p2.Direction != types.PayIn {
		t.Errorf("expected P2 PayIn, got %s", p2.Direction)
	}
	if !p2.Amount.Equal(types.NewDecimal(200, 0)) {
		t.Errorf("expected P2 amount 200, got %s", p2.Amount)
	}
}

func TestGenerateSkipsZero(t *testing.T) {
	gen := NewGenerator(&seqIDGen{})

	records := []types.PnLRecord{
		{ParticipantID: "P1", VariationMargin: types.NewDecimal(100, 0)},
		{ParticipantID: "P2", VariationMargin: types.DecimalZero()},
		{ParticipantID: "P3", VariationMargin: types.NewDecimal(-100, 0)},
	}

	instructions := gen.Generate("cycle-1", records)
	if len(instructions) != 2 {
		t.Fatalf("expected 2 instructions (P2 skipped), got %d", len(instructions))
	}
}

func TestGenerateNetsMultipleInstruments(t *testing.T) {
	gen := NewGenerator(&seqIDGen{})

	// P1 gains 200 on wheat, loses 50 on corn = net +150
	records := []types.PnLRecord{
		{ParticipantID: "P1", InstrumentID: "WHEAT", VariationMargin: types.NewDecimal(200, 0)},
		{ParticipantID: "P1", InstrumentID: "CORN", VariationMargin: types.NewDecimal(-50, 0)},
		{ParticipantID: "P2", InstrumentID: "WHEAT", VariationMargin: types.NewDecimal(-200, 0)},
		{ParticipantID: "P2", InstrumentID: "CORN", VariationMargin: types.NewDecimal(50, 0)},
	}

	instructions := gen.Generate("cycle-1", records)
	if len(instructions) != 2 {
		t.Fatalf("expected 2 instructions, got %d", len(instructions))
	}

	instrByParticipant := make(map[string]types.SettlementInstruction)
	for _, inst := range instructions {
		instrByParticipant[inst.ParticipantID] = inst
	}

	p1 := instrByParticipant["P1"]
	if p1.Direction != types.PayOut {
		t.Errorf("expected P1 PayOut, got %s", p1.Direction)
	}
	if !p1.Amount.Equal(types.NewDecimal(150, 0)) {
		t.Errorf("expected P1 net amount 150, got %s", p1.Amount)
	}
}

func TestGenerateCycleIDSet(t *testing.T) {
	gen := NewGenerator(&seqIDGen{})

	records := []types.PnLRecord{
		{ParticipantID: "P1", VariationMargin: types.NewDecimal(100, 0)},
	}

	instructions := gen.Generate("cycle-42", records)
	if len(instructions) != 1 {
		t.Fatalf("expected 1 instruction, got %d", len(instructions))
	}
	if instructions[0].CycleID != "cycle-42" {
		t.Errorf("expected cycle ID cycle-42, got %s", instructions[0].CycleID)
	}
}

func TestTotals(t *testing.T) {
	instructions := []types.SettlementInstruction{
		{Direction: types.PayIn, Amount: types.NewDecimal(200, 0)},
		{Direction: types.PayOut, Amount: types.NewDecimal(150, 0)},
		{Direction: types.PayIn, Amount: types.NewDecimal(50, 0)},
		{Direction: types.PayOut, Amount: types.NewDecimal(100, 0)},
	}

	payIn, payOut := Totals(instructions)
	if !payIn.Equal(types.NewDecimal(250, 0)) {
		t.Errorf("expected pay-in 250, got %s", payIn)
	}
	if !payOut.Equal(types.NewDecimal(250, 0)) {
		t.Errorf("expected pay-out 250, got %s", payOut)
	}
}

func TestTotalsEmpty(t *testing.T) {
	payIn, payOut := Totals(nil)
	if !payIn.IsZero() {
		t.Errorf("expected zero pay-in, got %s", payIn)
	}
	if !payOut.IsZero() {
		t.Errorf("expected zero pay-out, got %s", payOut)
	}
}
