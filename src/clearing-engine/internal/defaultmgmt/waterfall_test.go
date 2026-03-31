package defaultmgmt

import (
	"testing"

	"github.com/garudax-platform/clearing-engine/internal/types"
)

// setupFund creates a DefaultFundManager with standard test data:
//   - P001: 1000, P002: 2000, P003: 3000 (total: 6000)
//   - CCP skin-in-the-game: 500
//   - CCP additional capital: 1000
func setupFund() *DefaultFundManager {
	fm := NewDefaultFundManager()
	fm.AddContribution("P001", types.DecimalFromInt(1000))
	fm.AddContribution("P002", types.DecimalFromInt(2000))
	fm.AddContribution("P003", types.DecimalFromInt(3000))
	fm.SetCCPSkinInTheGame(types.DecimalFromInt(500))
	fm.SetCCPAdditionalCapital(types.DecimalFromInt(1000))
	return fm
}

func TestWaterfall_LossCoveredByDefaulterMargin(t *testing.T) {
	fm := setupFund()
	wf := NewDefaultWaterfall(fm)

	// P001 defaults with loss=800, margin=1000
	result, err := wf.ExecuteWaterfall("P001", types.DecimalFromInt(800), types.DecimalFromInt(1000))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.FullyCovered {
		t.Error("expected loss to be fully covered")
	}
	if !result.RemainingLoss.IsZero() {
		t.Errorf("expected zero remaining, got %s", result.RemainingLoss.String())
	}
	if !result.TotalAbsorbed.Equal(types.DecimalFromInt(800)) {
		t.Errorf("expected total absorbed 800, got %s", result.TotalAbsorbed.String())
	}

	// Only layer 1 should have absorbed anything
	if len(result.Layers) < 1 {
		t.Fatal("expected at least 1 layer")
	}
	if !result.Layers[0].Absorbed.Equal(types.DecimalFromInt(800)) {
		t.Errorf("layer 0 absorbed: expected 800, got %s", result.Layers[0].Absorbed.String())
	}
	// All subsequent layers should be zero
	for i := 1; i < len(result.Layers); i++ {
		if !result.Layers[i].Absorbed.IsZero() {
			t.Errorf("layer %d should have absorbed 0, got %s", i, result.Layers[i].Absorbed.String())
		}
	}
}

func TestWaterfall_LossReachesDefaultFund(t *testing.T) {
	fm := setupFund()
	wf := NewDefaultWaterfall(fm)

	// P001 defaults with loss=1500, margin=800, P001 fund contribution=1000
	// Layer 1: margin absorbs 800, remaining=700
	// Layer 2: P001 fund contribution absorbs 700, remaining=0
	result, err := wf.ExecuteWaterfall("P001", types.DecimalFromInt(1500), types.DecimalFromInt(800))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.FullyCovered {
		t.Error("expected loss to be fully covered")
	}
	if !result.TotalAbsorbed.Equal(types.DecimalFromInt(1500)) {
		t.Errorf("expected total absorbed 1500, got %s", result.TotalAbsorbed.String())
	}

	assertLayerAbsorbed(t, result, LayerDefaulterMargin, 800)
	assertLayerAbsorbed(t, result, LayerDefaulterFundContribution, 700)
}

func TestWaterfall_LossReachesCCPCapital(t *testing.T) {
	fm := setupFund()
	wf := NewDefaultWaterfall(fm)

	// P001 defaults with loss=2200, margin=500, P001 fund=1000
	// Layer 1: margin absorbs 500, remaining=1700
	// Layer 2: P001 fund absorbs 1000, remaining=700
	// Layer 3: CCP skin-in-the-game absorbs 500, remaining=200
	// Layer 4: non-defaulting (P002:2000, P003:3000 = 5000), absorbs 200 pro-rata
	result, err := wf.ExecuteWaterfall("P001", types.DecimalFromInt(2200), types.DecimalFromInt(500))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.FullyCovered {
		t.Error("expected loss to be fully covered")
	}

	assertLayerAbsorbed(t, result, LayerDefaulterMargin, 500)
	assertLayerAbsorbed(t, result, LayerDefaulterFundContribution, 1000)
	assertLayerAbsorbed(t, result, LayerCCPSkinInTheGame, 500)
	assertLayerAbsorbed(t, result, LayerNonDefaultingContributions, 200)
}

func TestWaterfall_FullExhaustion(t *testing.T) {
	fm := setupFund()
	wf := NewDefaultWaterfall(fm)

	// Total available resources:
	// Layer 1 (margin): 300
	// Layer 2 (P001 fund): 1000
	// Layer 3 (CCP skin): 500
	// Layer 4 (non-defaulting P002+P003): 5000
	// Layer 5 (CCP additional): 1000
	// Total: 7800
	//
	// Set loss = 10000, so 2200 remains uncovered
	result, err := wf.ExecuteWaterfall("P001", types.DecimalFromInt(10000), types.DecimalFromInt(300))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.FullyCovered {
		t.Error("expected loss NOT to be fully covered")
	}
	if !result.RemainingLoss.Equal(types.DecimalFromInt(2200)) {
		t.Errorf("expected remaining 2200, got %s", result.RemainingLoss.String())
	}

	assertLayerAbsorbed(t, result, LayerDefaulterMargin, 300)
	assertLayerAbsorbed(t, result, LayerDefaulterFundContribution, 1000)
	assertLayerAbsorbed(t, result, LayerCCPSkinInTheGame, 500)
	assertLayerAbsorbed(t, result, LayerNonDefaultingContributions, 5000)
	assertLayerAbsorbed(t, result, LayerCCPAdditionalCapital, 1000)
}

func TestWaterfall_ProRataAllocation(t *testing.T) {
	fm := NewDefaultFundManager()
	fm.AddContribution("P001", types.DecimalFromInt(1000))
	fm.AddContribution("P002", types.DecimalFromInt(2000))
	fm.AddContribution("P003", types.DecimalFromInt(2000))
	fm.SetCCPSkinInTheGame(types.DecimalZero())
	fm.SetCCPAdditionalCapital(types.DecimalZero())

	wf := NewDefaultWaterfall(fm)

	// P001 defaults. Non-defaulting: P002=2000, P003=2000, total=4000
	// Loss=2000, margin=0, P001 fund=1000
	// Layer 1: margin absorbs 0, remaining=2000
	// Layer 2: P001 fund absorbs 1000, remaining=1000
	// Layer 3: CCP skin=0, remaining=1000
	// Layer 4: pro-rata from 4000 pool: P002 absorbs 500, P003 absorbs 500
	result, err := wf.ExecuteWaterfall("P001", types.DecimalFromInt(2000), types.DecimalZero())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.FullyCovered {
		t.Error("expected loss to be fully covered")
	}

	// Check pro-rata details
	layer4 := findLayer(result, LayerNonDefaultingContributions)
	if layer4 == nil {
		t.Fatal("expected non-defaulting contributions layer")
	}
	if !layer4.Absorbed.Equal(types.DecimalFromInt(1000)) {
		t.Errorf("expected layer 4 absorbed 1000, got %s", layer4.Absorbed.String())
	}
	if layer4.ProRataDetails == nil {
		t.Fatal("expected pro-rata details")
	}
	// Equal contributions => equal shares
	p2Share := layer4.ProRataDetails["P002"]
	p3Share := layer4.ProRataDetails["P003"]
	if !p2Share.Equal(types.DecimalFromInt(500)) {
		t.Errorf("P002 pro-rata: expected 500, got %s", p2Share.String())
	}
	if !p3Share.Equal(types.DecimalFromInt(500)) {
		t.Errorf("P003 pro-rata: expected 500, got %s", p3Share.String())
	}
}

func TestWaterfall_ProRataUnequalContributions(t *testing.T) {
	fm := NewDefaultFundManager()
	fm.AddContribution("P001", types.DecimalFromInt(100))
	fm.AddContribution("P002", types.DecimalFromInt(1000)) // 25% of non-defaulting pool
	fm.AddContribution("P003", types.DecimalFromInt(3000)) // 75% of non-defaulting pool

	wf := NewDefaultWaterfall(fm)

	// P001 defaults with loss=500, margin=0, P001 fund=100
	// Layer 2: P001 fund absorbs 100, remaining=400
	// Layer 4: pro-rata from P002(1000)+P003(3000)=4000
	// P002 share = 400 * (1000/4000) = 100
	// P003 share = 400 * (3000/4000) = 300
	result, err := wf.ExecuteWaterfall("P001", types.DecimalFromInt(500), types.DecimalZero())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.FullyCovered {
		t.Error("expected loss to be fully covered")
	}

	layer4 := findLayer(result, LayerNonDefaultingContributions)
	if layer4 == nil {
		t.Fatal("expected non-defaulting contributions layer")
	}
	p2Share := layer4.ProRataDetails["P002"]
	p3Share := layer4.ProRataDetails["P003"]
	if !p2Share.Equal(types.DecimalFromInt(100)) {
		t.Errorf("P002 pro-rata: expected 100, got %s", p2Share.String())
	}
	if !p3Share.Equal(types.DecimalFromInt(300)) {
		t.Errorf("P003 pro-rata: expected 300, got %s", p3Share.String())
	}
}

func TestWaterfall_NoNonDefaultingMembers(t *testing.T) {
	fm := NewDefaultFundManager()
	fm.AddContribution("P001", types.DecimalFromInt(500))
	fm.SetCCPSkinInTheGame(types.DecimalFromInt(200))
	fm.SetCCPAdditionalCapital(types.DecimalFromInt(300))

	wf := NewDefaultWaterfall(fm)

	// P001 is the only member and defaults. No non-defaulting pool.
	// Loss=1500, margin=100
	// Layer 1: 100, remaining=1400
	// Layer 2: 500, remaining=900
	// Layer 3: 200, remaining=700
	// Layer 4: 0 (no non-defaulting members)
	// Layer 5: 300, remaining=400
	result, err := wf.ExecuteWaterfall("P001", types.DecimalFromInt(1500), types.DecimalFromInt(100))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.FullyCovered {
		t.Error("expected loss NOT to be fully covered")
	}
	if !result.RemainingLoss.Equal(types.DecimalFromInt(400)) {
		t.Errorf("expected remaining 400, got %s", result.RemainingLoss.String())
	}
	assertLayerAbsorbed(t, result, LayerNonDefaultingContributions, 0)
}

func TestWaterfall_ZeroMarginStillWorks(t *testing.T) {
	fm := setupFund()
	wf := NewDefaultWaterfall(fm)

	result, err := wf.ExecuteWaterfall("P001", types.DecimalFromInt(100), types.DecimalZero())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.FullyCovered {
		t.Error("expected loss to be fully covered")
	}
	assertLayerAbsorbed(t, result, LayerDefaulterMargin, 0)
	assertLayerAbsorbed(t, result, LayerDefaulterFundContribution, 100)
}

func TestWaterfall_InvalidInputs(t *testing.T) {
	fm := setupFund()
	wf := NewDefaultWaterfall(fm)

	// Negative loss
	_, err := wf.ExecuteWaterfall("P001", types.DecimalFromInt(-100), types.DecimalFromInt(50))
	if err == nil {
		t.Error("expected error for negative loss")
	}

	// Zero loss
	_, err = wf.ExecuteWaterfall("P001", types.DecimalZero(), types.DecimalFromInt(50))
	if err == nil {
		t.Error("expected error for zero loss")
	}

	// Negative margin
	_, err = wf.ExecuteWaterfall("P001", types.DecimalFromInt(100), types.DecimalFromInt(-50))
	if err == nil {
		t.Error("expected error for negative margin")
	}
}

func TestWaterfall_ResultMetadata(t *testing.T) {
	fm := setupFund()
	wf := NewDefaultWaterfall(fm)

	result, err := wf.ExecuteWaterfall("P001", types.DecimalFromInt(500), types.DecimalFromInt(500))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.DefaultingParticipantID != "P001" {
		t.Errorf("expected defaulting participant P001, got %s", result.DefaultingParticipantID)
	}
	if !result.TotalLoss.Equal(types.DecimalFromInt(500)) {
		t.Errorf("expected total loss 500, got %s", result.TotalLoss.String())
	}
	if result.ExecutedAt.IsZero() {
		t.Error("expected non-zero executed_at timestamp")
	}
	if len(result.Layers) != 1 {
		t.Errorf("expected 1 layer (early exit), got %d", len(result.Layers))
	}
}

func TestWaterfall_ExactMarginCoverage(t *testing.T) {
	fm := setupFund()
	wf := NewDefaultWaterfall(fm)

	// Loss exactly equals margin
	result, err := wf.ExecuteWaterfall("P001", types.DecimalFromInt(1000), types.DecimalFromInt(1000))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.FullyCovered {
		t.Error("expected fully covered")
	}
	if len(result.Layers) != 1 {
		t.Errorf("expected 1 layer, got %d", len(result.Layers))
	}
	assertLayerAbsorbed(t, result, LayerDefaulterMargin, 1000)
}

func TestWaterfall_FractionalAmounts(t *testing.T) {
	fm := NewDefaultFundManager()
	fm.AddContribution("P001", types.NewDecimal(100, 5000)) // 100.5
	fm.AddContribution("P002", types.NewDecimal(200, 2500)) // 200.25
	fm.AddContribution("P003", types.NewDecimal(300, 7500)) // 300.75

	wf := NewDefaultWaterfall(fm)

	// Loss = 150.75, margin = 25.25
	result, err := wf.ExecuteWaterfall("P001",
		types.NewDecimal(150, 7500),
		types.NewDecimal(25, 2500))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.FullyCovered {
		t.Error("expected fully covered")
	}
	// Layer 1: margin absorbs 25.25, remaining=125.50
	// Layer 2: P001 fund absorbs 100.50, remaining=25.00
	// Layer 3: CCP skin=0, remaining=25.00
	// Layer 4: pro-rata from P002(200.25)+P003(300.75)=501.00, absorbs 25.00
	if !result.TotalAbsorbed.Equal(types.NewDecimal(150, 7500)) {
		t.Errorf("expected total absorbed 150.75, got %s", result.TotalAbsorbed.String())
	}
}

func TestWaterfallLayerString(t *testing.T) {
	tests := []struct {
		layer    WaterfallLayer
		expected string
	}{
		{LayerDefaulterMargin, "defaulter_margin"},
		{LayerDefaulterFundContribution, "defaulter_fund_contribution"},
		{LayerCCPSkinInTheGame, "ccp_skin_in_the_game"},
		{LayerNonDefaultingContributions, "non_defaulting_contributions"},
		{LayerCCPAdditionalCapital, "ccp_additional_capital"},
		{WaterfallLayer(99), "unknown"},
	}
	for _, tc := range tests {
		if got := tc.layer.String(); got != tc.expected {
			t.Errorf("layer %d: expected %q, got %q", tc.layer, tc.expected, got)
		}
	}
}

// --- helpers ---

func assertLayerAbsorbed(t *testing.T, result *WaterfallResult, layer WaterfallLayer, expectedInt int64) {
	t.Helper()
	lr := findLayer(result, layer)
	if lr == nil {
		t.Fatalf("layer %s not found in result", layer.String())
	}
	expected := types.DecimalFromInt(expectedInt)
	if !lr.Absorbed.Equal(expected) {
		t.Errorf("layer %s: expected absorbed %s, got %s", layer.String(), expected.String(), lr.Absorbed.String())
	}
}

func findLayer(result *WaterfallResult, layer WaterfallLayer) *LayerResult {
	for i := range result.Layers {
		if result.Layers[i].Layer == layer {
			return &result.Layers[i]
		}
	}
	return nil
}
