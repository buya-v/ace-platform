package params

import (
	"testing"

	"github.com/garudax-platform/margin-engine/internal/types"
)

func TestDefaultScenariosCount(t *testing.T) {
	psr := types.DecimalFromInt(300) // $3.00 price scan range scaled up
	vsr := types.DecimalFromInt(50)
	scenarios := DefaultScenarios(psr, vsr)
	if len(scenarios) != 16 {
		t.Errorf("expected 16 scenarios, got %d", len(scenarios))
	}
}

func TestDefaultScenariosSymmetry(t *testing.T) {
	psr := types.DecimalFromInt(100)
	vsr := types.DecimalFromInt(10)
	scenarios := DefaultScenarios(psr, vsr)

	// First 14 scenarios should have weight 1.0
	one := types.DecimalFromInt(1)
	for i := 0; i < 14; i++ {
		if !scenarios[i].Weight.Equal(one) {
			t.Errorf("scenario %d: expected weight 1, got %s", i, scenarios[i].Weight.String())
		}
	}

	// Last 2 scenarios are extreme moves with reduced weight
	for i := 14; i < 16; i++ {
		if scenarios[i].Weight.Equal(one) {
			t.Errorf("scenario %d: expected reduced weight, got %s", i, scenarios[i].Weight.String())
		}
	}

	// Extreme scenarios should have opposite price moves
	if !scenarios[14].PriceMove.Add(scenarios[15].PriceMove).IsZero() {
		t.Error("extreme scenarios should have symmetric price moves")
	}
}

func TestStoreSetAndGet(t *testing.T) {
	store := NewStore()

	store.Set(InstrumentParams{
		InstrumentID:   "CORN-2026-07",
		PriceScanRange: types.DecimalFromInt(300),
		VolScanRange:   types.DecimalFromInt(50),
		SpotPrice:      types.DecimalFromInt(450),
		ContractSize:   5000,
	})

	p, err := store.Get("CORN-2026-07")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ContractSize != 5000 {
		t.Errorf("expected contract size 5000, got %d", p.ContractSize)
	}
	if len(p.Scenarios) != 16 {
		t.Errorf("expected 16 default scenarios, got %d", len(p.Scenarios))
	}
}

func TestStoreGetMissing(t *testing.T) {
	store := NewStore()
	_, err := store.Get("NONEXISTENT")
	if err == nil {
		t.Error("expected error for missing instrument")
	}
}

func TestStoreUpdateSpotPrice(t *testing.T) {
	store := NewStore()
	store.Set(InstrumentParams{
		InstrumentID:   "WHEAT-2026-09",
		PriceScanRange: types.DecimalFromInt(200),
		SpotPrice:      types.DecimalFromInt(600),
		ContractSize:   5000,
	})

	newPrice := types.DecimalFromInt(650)
	if err := store.UpdateSpotPrice("WHEAT-2026-09", newPrice); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p, _ := store.Get("WHEAT-2026-09")
	if !p.SpotPrice.Equal(newPrice) {
		t.Errorf("expected spot price %s, got %s", newPrice.String(), p.SpotPrice.String())
	}
}

func TestStoreUpdateSpotPriceMissing(t *testing.T) {
	store := NewStore()
	if err := store.UpdateSpotPrice("NONEXISTENT", types.DecimalFromInt(100)); err == nil {
		t.Error("expected error for missing instrument")
	}
}

func TestStoreAll(t *testing.T) {
	store := NewStore()
	store.Set(InstrumentParams{InstrumentID: "A", PriceScanRange: types.DecimalFromInt(100), ContractSize: 1000})
	store.Set(InstrumentParams{InstrumentID: "B", PriceScanRange: types.DecimalFromInt(200), ContractSize: 2000})

	all := store.All()
	if len(all) != 2 {
		t.Errorf("expected 2 instruments, got %d", len(all))
	}
}
