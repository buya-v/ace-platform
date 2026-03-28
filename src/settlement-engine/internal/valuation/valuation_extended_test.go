package valuation

import (
	"testing"
	"time"

	"github.com/ace-platform/settlement-engine/internal/types"
)

func TestValuePositionTableDriven(t *testing.T) {
	tests := []struct {
		name      string
		quantity  int64
		markPrice types.Decimal
		want      types.Decimal
	}{
		{
			name:      "long position",
			quantity:  10,
			markPrice: types.NewDecimal(1500, 0),
			want:      types.NewDecimal(15000, 0),
		},
		{
			name:      "short position uses absolute quantity",
			quantity:  -10,
			markPrice: types.NewDecimal(1500, 0),
			want:      types.NewDecimal(15000, 0),
		},
		{
			name:      "flat position",
			quantity:  0,
			markPrice: types.NewDecimal(1500, 0),
			want:      types.DecimalZero(),
		},
		{
			name:      "single contract",
			quantity:  1,
			markPrice: types.NewDecimal(100, 0),
			want:      types.NewDecimal(100, 0),
		},
		{
			name:      "zero mark price",
			quantity:  100,
			markPrice: types.DecimalZero(),
			want:      types.DecimalZero(),
		},
		{
			name:      "fractional mark price",
			quantity:  10,
			markPrice: types.NewDecimal(75, 5000), // 75.5
			want:      types.NewDecimal(755, 0),   // 75.5 * 10
		},
		{
			name:      "large position",
			quantity:  100000,
			markPrice: types.NewDecimal(1, 0),
			want:      types.NewDecimal(100000, 0),
		},
		{
			name:      "short single contract",
			quantity:  -1,
			markPrice: types.NewDecimal(250, 0),
			want:      types.NewDecimal(250, 0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := types.Position{
				ParticipantID: "P1",
				InstrumentID:  "INST",
				NetQuantity:   tt.quantity,
			}
			got := ValuePosition(pos, tt.markPrice)
			if !got.Equal(tt.want) {
				t.Errorf("ValuePosition(qty=%d, price=%s) = %s, want %s",
					tt.quantity, tt.markPrice, got, tt.want)
			}
		})
	}
}

func TestStoreMultipleInstruments(t *testing.T) {
	store := NewStore()
	date := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	store.SetSettlementPrice("WHEAT", date, types.NewDecimal(1500, 0))
	store.SetSettlementPrice("CORN", date, types.NewDecimal(800, 0))
	store.SetSettlementPrice("SOYBEAN", date, types.NewDecimal(1200, 0))

	instruments := []struct {
		id    string
		price types.Decimal
	}{
		{"WHEAT", types.NewDecimal(1500, 0)},
		{"CORN", types.NewDecimal(800, 0)},
		{"SOYBEAN", types.NewDecimal(1200, 0)},
	}

	for _, inst := range instruments {
		sp, err := store.GetSettlementPrice(inst.id, date)
		if err != nil {
			t.Fatalf("unexpected error for %s: %v", inst.id, err)
		}
		if !sp.SettlementPrice.Equal(inst.price) {
			t.Errorf("%s: expected %s, got %s", inst.id, inst.price, sp.SettlementPrice)
		}
	}
}

func TestStoreOverwritePrice(t *testing.T) {
	store := NewStore()
	date := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	store.SetSettlementPrice("WHEAT", date, types.NewDecimal(1500, 0))
	store.SetSettlementPrice("WHEAT", date, types.NewDecimal(1520, 0))

	sp, err := store.GetSettlementPrice("WHEAT", date)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sp.SettlementPrice.Equal(types.NewDecimal(1520, 0)) {
		t.Errorf("expected overwritten price 1520, got %s", sp.SettlementPrice)
	}
}

func TestStorePreviousPriceChaining(t *testing.T) {
	store := NewStore()
	day1 := time.Date(2026, 3, 25, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	day3 := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	store.SetSettlementPrice("WHEAT", day1, types.NewDecimal(1480, 0))
	store.SetSettlementPrice("WHEAT", day2, types.NewDecimal(1500, 0))
	store.SetSettlementPrice("WHEAT", day3, types.NewDecimal(1520, 0))

	// Day 1 has no previous
	sp1, _ := store.GetSettlementPrice("WHEAT", day1)
	if !sp1.PreviousPrice.IsZero() {
		t.Errorf("day1 should have no previous price, got %s", sp1.PreviousPrice)
	}

	// Day 2 previous = day 1
	sp2, _ := store.GetSettlementPrice("WHEAT", day2)
	if !sp2.PreviousPrice.Equal(types.NewDecimal(1480, 0)) {
		t.Errorf("day2 previous should be 1480, got %s", sp2.PreviousPrice)
	}

	// Day 3 previous = day 2
	sp3, _ := store.GetSettlementPrice("WHEAT", day3)
	if !sp3.PreviousPrice.Equal(types.NewDecimal(1500, 0)) {
		t.Errorf("day3 previous should be 1500, got %s", sp3.PreviousPrice)
	}
}

func TestStoreNonConsecutiveDays(t *testing.T) {
	store := NewStore()
	monday := time.Date(2026, 3, 23, 0, 0, 0, 0, time.UTC)
	wednesday := time.Date(2026, 3, 25, 0, 0, 0, 0, time.UTC) // skip Tuesday

	store.SetSettlementPrice("WHEAT", monday, types.NewDecimal(1500, 0))
	store.SetSettlementPrice("WHEAT", wednesday, types.NewDecimal(1520, 0))

	// Wednesday won't find Monday as previous (only checks day-1)
	sp, _ := store.GetSettlementPrice("WHEAT", wednesday)
	if !sp.PreviousPrice.IsZero() {
		t.Errorf("non-consecutive day should have no previous, got %s", sp.PreviousPrice)
	}
}

func TestGetSettlementPriceWrongInstrument(t *testing.T) {
	store := NewStore()
	date := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	store.SetSettlementPrice("WHEAT", date, types.NewDecimal(1500, 0))

	_, err := store.GetSettlementPrice("CORN", date)
	if err == nil {
		t.Fatal("expected error for wrong instrument")
	}
}

func TestGetSettlementPriceWrongDate(t *testing.T) {
	store := NewStore()
	date := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)
	otherDate := time.Date(2026, 3, 28, 0, 0, 0, 0, time.UTC)

	store.SetSettlementPrice("WHEAT", date, types.NewDecimal(1500, 0))

	_, err := store.GetSettlementPrice("WHEAT", otherDate)
	if err == nil {
		t.Fatal("expected error for wrong date")
	}
}

func TestHasPreviousPriceMultipleInstruments(t *testing.T) {
	store := NewStore()
	day1 := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	store.SetSettlementPrice("WHEAT", day1, types.NewDecimal(1500, 0))
	// CORN has no day1 price

	if !store.HasPreviousPrice("WHEAT", day2) {
		t.Error("WHEAT should have previous price")
	}
	if store.HasPreviousPrice("CORN", day2) {
		t.Error("CORN should not have previous price")
	}
}

func TestStoreSettleDatePreserved(t *testing.T) {
	store := NewStore()
	date := time.Date(2026, 3, 27, 14, 30, 0, 0, time.UTC)

	store.SetSettlementPrice("WHEAT", date, types.NewDecimal(1500, 0))

	sp, err := store.GetSettlementPrice("WHEAT", date)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sp.SettleDate != date {
		t.Errorf("expected settle date %v, got %v", date, sp.SettleDate)
	}
}

func TestStoreInstrumentIDPreserved(t *testing.T) {
	store := NewStore()
	date := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	store.SetSettlementPrice("WHEAT-MAY26-FUT", date, types.NewDecimal(1500, 0))

	sp, err := store.GetSettlementPrice("WHEAT-MAY26-FUT", date)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sp.InstrumentID != "WHEAT-MAY26-FUT" {
		t.Errorf("expected instrument WHEAT-MAY26-FUT, got %s", sp.InstrumentID)
	}
}
