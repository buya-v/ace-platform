package netting

import (
	"testing"
	"time"

	"github.com/garudax-platform/clearing-engine/internal/types"
)

func makeOblWithID(id, participant, instrument string, side types.Side, price int64, qty uint64) types.ClearingObligation {
	return types.ClearingObligation{
		ObligationID:  id,
		TradeID:       "trade-" + id,
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

func TestNetBilateralNetting(t *testing.T) {
	// Two participants with exactly offsetting positions
	svc := NewService()
	obligations := []types.ClearingObligation{
		makeObl("P1", "WHT", types.SideBuy, 500, 100),
		makeObl("P2", "WHT", types.SideSell, 500, 100),
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
	if p1.NetQuantity != 100 {
		t.Errorf("P1 net qty = %d, want 100", p1.NetQuantity)
	}
	p2 := resultMap["P2"]
	if p2.NetQuantity != -100 {
		t.Errorf("P2 net qty = %d, want -100", p2.NetQuantity)
	}
}

func TestNetMultilateralNetting(t *testing.T) {
	// Three participants in a ring: P1→P2, P2→P3, P3→P1
	svc := NewService()
	obligations := []types.ClearingObligation{
		makeOblWithID("o1", "P1", "WHT", types.SideBuy, 500, 50),
		makeOblWithID("o2", "P1", "WHT", types.SideSell, 500, 30),
		makeOblWithID("o3", "P2", "WHT", types.SideBuy, 500, 30),
		makeOblWithID("o4", "P2", "WHT", types.SideSell, 500, 50),
		makeOblWithID("o5", "P3", "WHT", types.SideBuy, 500, 20),
		makeOblWithID("o6", "P3", "WHT", types.SideSell, 500, 20),
	}

	results := svc.Net(obligations)
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}

	resultMap := make(map[string]types.NettingResult)
	for _, r := range results {
		resultMap[r.ParticipantID] = r
	}

	// P1: bought 50, sold 30 → net +20
	if resultMap["P1"].NetQuantity != 20 {
		t.Errorf("P1 net = %d, want 20", resultMap["P1"].NetQuantity)
	}
	// P2: bought 30, sold 50 → net -20
	if resultMap["P2"].NetQuantity != -20 {
		t.Errorf("P2 net = %d, want -20", resultMap["P2"].NetQuantity)
	}
	// P3: bought 20, sold 20 → net 0 (fully netted)
	if resultMap["P3"].NetQuantity != 0 {
		t.Errorf("P3 net = %d, want 0", resultMap["P3"].NetQuantity)
	}
	// P3 should have 100% efficiency
	p3 := resultMap["P3"]
	eff := p3.NettingEfficiency()
	if eff != 100.0 {
		t.Errorf("P3 efficiency = %.1f%%, want 100%%", eff)
	}
}

func TestNetMixedBuySellPositions(t *testing.T) {
	svc := NewService()
	obligations := []types.ClearingObligation{
		makeObl("P1", "WHT", types.SideBuy, 500, 10),
		makeObl("P1", "WHT", types.SideBuy, 520, 5),
		makeObl("P1", "WHT", types.SideSell, 510, 8),
		makeObl("P1", "WHT", types.SideSell, 530, 2),
	}

	results := svc.Net(obligations)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	r := results[0]
	// Net qty: +10 +5 -8 -2 = 5
	if r.NetQuantity != 5 {
		t.Errorf("net qty = %d, want 5", r.NetQuantity)
	}
	if r.GrossLongQty != 15 {
		t.Errorf("gross long = %d, want 15", r.GrossLongQty)
	}
	if r.GrossShortQty != 10 {
		t.Errorf("gross short = %d, want 10", r.GrossShortQty)
	}
	if r.ObligationsCount != 4 {
		t.Errorf("count = %d, want 4", r.ObligationsCount)
	}

	// Efficiency: (1 - 5/25) * 100 = 80%
	eff := r.NettingEfficiency()
	if eff < 79.0 || eff > 81.0 {
		t.Errorf("efficiency = %.2f%%, want ~80%%", eff)
	}
}

func TestNetSingleSellPosition(t *testing.T) {
	svc := NewService()
	obligations := []types.ClearingObligation{
		makeObl("P1", "CORN", types.SideSell, 400, 25),
	}

	results := svc.Net(obligations)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	r := results[0]
	if r.NetQuantity != -25 {
		t.Errorf("net qty = %d, want -25", r.NetQuantity)
	}
	if r.GrossLongQty != 0 {
		t.Errorf("gross long = %d, want 0", r.GrossLongQty)
	}
	if r.GrossShortQty != 25 {
		t.Errorf("gross short = %d, want 25", r.GrossShortQty)
	}
	// Efficiency: (1 - 25/25) * 100 = 0%
	if r.NettingEfficiency() != 0 {
		t.Errorf("efficiency = %.2f%%, want 0%%", r.NettingEfficiency())
	}
}

func TestNetMultipleInstrumentsSameParticipant(t *testing.T) {
	svc := NewService()
	obligations := []types.ClearingObligation{
		makeObl("P1", "WHT", types.SideBuy, 500, 10),
		makeObl("P1", "CORN", types.SideSell, 400, 5),
		makeObl("P1", "WHT", types.SideSell, 500, 3),
		makeObl("P1", "CORN", types.SideBuy, 400, 5),
	}

	results := svc.Net(obligations)
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}

	resultMap := make(map[string]types.NettingResult)
	for _, r := range results {
		resultMap[r.InstrumentID] = r
	}

	// WHT: +10 -3 = +7
	if resultMap["WHT"].NetQuantity != 7 {
		t.Errorf("WHT net = %d, want 7", resultMap["WHT"].NetQuantity)
	}
	// CORN: -5 +5 = 0
	if resultMap["CORN"].NetQuantity != 0 {
		t.Errorf("CORN net = %d, want 0", resultMap["CORN"].NetQuantity)
	}
}

func TestNetValueAccumulation(t *testing.T) {
	svc := NewService()
	obligations := []types.ClearingObligation{
		makeObl("P1", "WHT", types.SideBuy, 500, 10),  // value = 5000
		makeObl("P1", "WHT", types.SideBuy, 600, 5),   // value = 3000
		makeObl("P1", "WHT", types.SideSell, 550, 8),  // value = -4400
	}

	results := svc.Net(obligations)
	r := results[0]

	// Net value = 5000 + 3000 - 4400 = 3600
	if r.NetValue.String() != "3600" {
		t.Errorf("net value = %s, want 3600", r.NetValue.String())
	}
}

func TestNetLargeNumberOfObligations(t *testing.T) {
	svc := NewService()
	var obligations []types.ClearingObligation

	// 100 buy obligations, 99 sell obligations for P1
	for i := 0; i < 100; i++ {
		obligations = append(obligations, makeObl("P1", "WHT", types.SideBuy, 500, 1))
	}
	for i := 0; i < 99; i++ {
		obligations = append(obligations, makeObl("P1", "WHT", types.SideSell, 500, 1))
	}

	results := svc.Net(obligations)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	r := results[0]
	if r.NetQuantity != 1 {
		t.Errorf("net qty = %d, want 1", r.NetQuantity)
	}
	if r.ObligationsCount != 199 {
		t.Errorf("count = %d, want 199", r.ObligationsCount)
	}
}

func TestNetNettedAtTimestamp(t *testing.T) {
	svc := NewService()
	before := time.Now()
	results := svc.Net([]types.ClearingObligation{
		makeObl("P1", "WHT", types.SideBuy, 500, 10),
	})
	after := time.Now()

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	nettedAt := results[0].NettedAt
	if nettedAt.Before(before) || nettedAt.After(after) {
		t.Errorf("NettedAt %v not between %v and %v", nettedAt, before, after)
	}
}

// Test types methods used by netting
func TestDecimalFromRawRoundtrip(t *testing.T) {
	d := types.DecimalFromInt(500)
	raw := d.Raw()
	d2 := types.DecimalFromRaw(raw)
	if !d.Equal(d2) {
		t.Errorf("%s != %s after raw roundtrip", d.String(), d2.String())
	}
}

func TestDecimalSub(t *testing.T) {
	a := types.DecimalFromInt(1000)
	b := types.DecimalFromInt(600)
	diff := a.Sub(b)
	if diff.String() != "400" {
		t.Errorf("1000 - 600 = %s, want 400", diff.String())
	}
}

func TestDecimalNegate(t *testing.T) {
	d := types.DecimalFromInt(100)
	neg := d.Negate()
	if neg.String() != "-100" {
		t.Errorf("Negate(100) = %s, want -100", neg.String())
	}
	// Double negate
	if !neg.Negate().Equal(d) {
		t.Error("double negate should return original")
	}
}
