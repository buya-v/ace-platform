package netting

import (
	"testing"
	"time"

	"github.com/garudax-platform/clearing-engine/internal/types"
)

func makeObl(participant, instrument string, side types.Side, price int64, qty uint64) types.ClearingObligation {
	return types.ClearingObligation{
		ObligationID:  "obl-test",
		TradeID:       "trade-test",
		InstrumentID:  instrument,
		ParticipantID: participant,
		Side:          side,
		Price:         types.DecimalFromInt(price),
		Quantity:      qty,
		Value:         types.DecimalFromInt(price).MulUint64(qty),
		Status:        types.ClearingStatusNovated,
		CreatedAt:     time.Now(),
		NovatedAt:     time.Now(),
	}
}

func TestNetSingleObligation(t *testing.T) {
	svc := NewService()
	obligations := []types.ClearingObligation{
		makeObl("P1", "WHT", types.SideBuy, 500, 10),
	}

	results := svc.Net(obligations)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	r := results[0]
	if r.NetQuantity != 10 {
		t.Errorf("net qty = %d, want 10", r.NetQuantity)
	}
	if r.GrossLongQty != 10 {
		t.Errorf("gross long = %d, want 10", r.GrossLongQty)
	}
	if r.GrossShortQty != 0 {
		t.Errorf("gross short = %d, want 0", r.GrossShortQty)
	}
}

func TestNetOffsettingObligations(t *testing.T) {
	svc := NewService()
	obligations := []types.ClearingObligation{
		makeObl("P1", "WHT", types.SideBuy, 500, 10),
		makeObl("P1", "WHT", types.SideSell, 500, 7),
	}

	results := svc.Net(obligations)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	r := results[0]
	if r.NetQuantity != 3 {
		t.Errorf("net qty = %d, want 3", r.NetQuantity)
	}
	if r.GrossLongQty != 10 {
		t.Errorf("gross long = %d, want 10", r.GrossLongQty)
	}
	if r.GrossShortQty != 7 {
		t.Errorf("gross short = %d, want 7", r.GrossShortQty)
	}
	if r.ObligationsCount != 2 {
		t.Errorf("count = %d, want 2", r.ObligationsCount)
	}
}

func TestNetFullOffset(t *testing.T) {
	svc := NewService()
	obligations := []types.ClearingObligation{
		makeObl("P1", "WHT", types.SideBuy, 500, 10),
		makeObl("P1", "WHT", types.SideSell, 500, 10),
	}

	results := svc.Net(obligations)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	r := results[0]
	if r.NetQuantity != 0 {
		t.Errorf("net qty = %d, want 0", r.NetQuantity)
	}

	eff := r.NettingEfficiency()
	if eff != 100.0 {
		t.Errorf("efficiency = %.1f%%, want 100%%", eff)
	}
}

func TestNetMultipleParticipants(t *testing.T) {
	svc := NewService()
	obligations := []types.ClearingObligation{
		makeObl("P1", "WHT", types.SideBuy, 500, 10),
		makeObl("P2", "WHT", types.SideSell, 500, 10),
		makeObl("P1", "WHT", types.SideSell, 500, 3),
	}

	results := svc.Net(obligations)
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}

	resultMap := make(map[string]types.NettingResult)
	for _, r := range results {
		resultMap[r.ParticipantID] = r
	}

	p1 := resultMap["P1"]
	if p1.NetQuantity != 7 {
		t.Errorf("P1 net qty = %d, want 7", p1.NetQuantity)
	}

	p2 := resultMap["P2"]
	if p2.NetQuantity != -10 {
		t.Errorf("P2 net qty = %d, want -10", p2.NetQuantity)
	}
}

func TestNetMultipleInstruments(t *testing.T) {
	svc := NewService()
	obligations := []types.ClearingObligation{
		makeObl("P1", "WHT", types.SideBuy, 500, 10),
		makeObl("P1", "CORN", types.SideSell, 400, 5),
	}

	results := svc.Net(obligations)
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
}

func TestNetEmpty(t *testing.T) {
	svc := NewService()
	results := svc.Net(nil)
	if len(results) != 0 {
		t.Errorf("got %d results for nil input, want 0", len(results))
	}
}

func TestNetValue(t *testing.T) {
	svc := NewService()
	obligations := []types.ClearingObligation{
		makeObl("P1", "WHT", types.SideBuy, 500, 10),  // value = 5000
		makeObl("P1", "WHT", types.SideSell, 600, 3),   // value = -1800
	}

	results := svc.Net(obligations)
	r := results[0]
	// Net value = 5000 - 1800 = 3200
	if r.NetValue.String() != "3200" {
		t.Errorf("net value = %s, want 3200", r.NetValue.String())
	}
}

func TestNettingEfficiency(t *testing.T) {
	r := types.NettingResult{
		GrossLongQty:  100,
		GrossShortQty: 80,
		NetQuantity:   20,
	}
	eff := r.NettingEfficiency()
	// (1 - 20/180) * 100 ≈ 88.89
	if eff < 88.0 || eff > 89.0 {
		t.Errorf("efficiency = %.2f%%, expected ~88.89%%", eff)
	}
}

func TestNettingEfficiencyZeroGross(t *testing.T) {
	r := types.NettingResult{}
	if r.NettingEfficiency() != 0 {
		t.Errorf("expected 0 efficiency for zero gross")
	}
}
