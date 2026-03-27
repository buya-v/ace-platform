package valuation

import (
	"testing"
	"time"

	"github.com/ace-platform/settlement-engine/internal/types"
)

func TestSetAndGetSettlementPrice(t *testing.T) {
	store := NewStore()
	date := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)
	price := types.NewDecimal(1500, 0)

	store.SetSettlementPrice("WHEAT-MAY26", date, price)

	sp, err := store.GetSettlementPrice("WHEAT-MAY26", date)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sp.SettlementPrice.Equal(price) {
		t.Errorf("expected price %s, got %s", price, sp.SettlementPrice)
	}
	if sp.InstrumentID != "WHEAT-MAY26" {
		t.Errorf("expected instrument WHEAT-MAY26, got %s", sp.InstrumentID)
	}
}

func TestGetSettlementPriceMissing(t *testing.T) {
	store := NewStore()
	date := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	_, err := store.GetSettlementPrice("WHEAT-MAY26", date)
	if err == nil {
		t.Fatal("expected error for missing price")
	}
}

func TestPreviousPriceTracking(t *testing.T) {
	store := NewStore()
	day1 := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	store.SetSettlementPrice("WHEAT-MAY26", day1, types.NewDecimal(1500, 0))
	store.SetSettlementPrice("WHEAT-MAY26", day2, types.NewDecimal(1520, 0))

	sp, err := store.GetSettlementPrice("WHEAT-MAY26", day2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sp.PreviousPrice.Equal(types.NewDecimal(1500, 0)) {
		t.Errorf("expected previous price 1500, got %s", sp.PreviousPrice)
	}
	if !sp.SettlementPrice.Equal(types.NewDecimal(1520, 0)) {
		t.Errorf("expected settlement price 1520, got %s", sp.SettlementPrice)
	}
}

func TestHasPreviousPrice(t *testing.T) {
	store := NewStore()
	day1 := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	if store.HasPreviousPrice("WHEAT-MAY26", day2) {
		t.Error("expected no previous price")
	}

	store.SetSettlementPrice("WHEAT-MAY26", day1, types.NewDecimal(1500, 0))
	if !store.HasPreviousPrice("WHEAT-MAY26", day2) {
		t.Error("expected previous price to exist")
	}
}

func TestValuePositionLong(t *testing.T) {
	pos := types.Position{
		ParticipantID: "P1",
		InstrumentID:  "WHEAT-MAY26",
		NetQuantity:   10,
	}
	markPrice := types.NewDecimal(1500, 0)
	val := ValuePosition(pos, markPrice)

	expected := types.NewDecimal(15000, 0)
	if !val.Equal(expected) {
		t.Errorf("expected value %s, got %s", expected, val)
	}
}

func TestValuePositionShort(t *testing.T) {
	pos := types.Position{
		ParticipantID: "P1",
		InstrumentID:  "WHEAT-MAY26",
		NetQuantity:   -5,
	}
	markPrice := types.NewDecimal(1500, 0)
	val := ValuePosition(pos, markPrice)

	expected := types.NewDecimal(7500, 0)
	if !val.Equal(expected) {
		t.Errorf("expected value %s, got %s", expected, val)
	}
}

func TestValuePositionFlat(t *testing.T) {
	pos := types.Position{
		ParticipantID: "P1",
		InstrumentID:  "WHEAT-MAY26",
		NetQuantity:   0,
	}
	val := ValuePosition(pos, types.NewDecimal(1500, 0))
	if !val.IsZero() {
		t.Errorf("expected zero value, got %s", val)
	}
}
