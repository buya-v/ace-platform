// Package engine_test provides tests for tiered tick-size validation.
package engine_test

import (
	"strings"
	"testing"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/types"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// tickTable builds a TickTable with the given (min, max, tickSize) triples.
func tickTable(instrumentID string, tiers ...float64) *types.TickTable {
	if len(tiers)%3 != 0 {
		panic("tickTable: tiers must be multiples of 3 (min, max, tickSize)")
	}
	t := &types.TickTable{InstrumentID: instrumentID}
	for i := 0; i < len(tiers); i += 3 {
		t.Tiers = append(t.Tiers, types.TickTier{
			MinPrice: tiers[i],
			MaxPrice: tiers[i+1],
			TickSize: tiers[i+2],
		})
	}
	return t
}

// ── ValidateTickSize tests ────────────────────────────────────────────────────

// TestTieredTick_ValidPrice verifies that a price exactly on a tier boundary
// that is a clean multiple of the tier's tick size returns nil.
func TestTieredTick_ValidPrice(t *testing.T) {
	// Single tier: 0–100, tick 0.5. Price 50.0 is 100 * 0.5.
	table := tickTable("INST-A", 0, 100, 0.5)

	if err := engine.ValidateTickSize(50.0, table, 0.01); err != nil {
		t.Errorf("expected nil for valid price 50.0 in tier [0,100) tick 0.5, got: %v", err)
	}
}

// TestTieredTick_InvalidPrice verifies that a price that does NOT align with
// the tier's tick size returns an error containing tier boundary information.
func TestTieredTick_InvalidPrice(t *testing.T) {
	// Single tier: 0–100, tick 0.5. Price 50.3 is NOT a multiple of 0.5.
	table := tickTable("INST-B", 0, 100, 0.5)

	err := engine.ValidateTickSize(50.3, table, 0.01)
	if err == nil {
		t.Fatal("expected error for price 50.3 not aligned to tick 0.5, got nil")
	}

	// Error message should contain tier information.
	msg := err.Error()
	if !strings.Contains(msg, "tick_size") {
		t.Errorf("error message %q does not mention tick_size", msg)
	}
	// The error message format is: "price %.2f must be a multiple of tick_size %.4f (tier: %.2f-%.2f)"
	if !strings.Contains(msg, "tier") {
		t.Errorf("error message %q does not contain tier boundary info", msg)
	}
}

// TestTieredTick_MultipleTiers verifies correct tier selection across 3 tiers:
// - [0, 10)   tick 0.01
// - [10, 100) tick 0.1
// - [100, 1000) tick 1.0
// A valid price in each tier must return nil; an invalid one must return an error.
func TestTieredTick_MultipleTiers(t *testing.T) {
	table := tickTable("INST-C",
		0, 10, 0.01,
		10, 100, 0.1,
		100, 1000, 1.0,
	)

	tests := []struct {
		name    string
		price   float64
		wantErr bool
	}{
		// Tier 1: [0, 10) tick 0.01
		{"tier1_valid_0.05", 0.05, false},
		{"tier1_invalid_0.005", 0.005, true},

		// Tier 2: [10, 100) tick 0.1
		{"tier2_valid_55.0", 55.0, false},
		{"tier2_valid_10.5", 10.5, false},
		{"tier2_invalid_55.05", 55.05, true},

		// Tier 3: [100, 1000) tick 1.0
		{"tier3_valid_500.0", 500.0, false},
		{"tier3_invalid_500.5", 500.5, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := engine.ValidateTickSize(tc.price, table, 0.01)
			if tc.wantErr && err == nil {
				t.Errorf("price %v: expected error, got nil", tc.price)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("price %v: expected nil, got %v", tc.price, err)
			}
		})
	}
}

// TestTieredTick_NoTable verifies that when tickTable is nil, the function falls
// back to defaultTickSize. A price that aligns with defaultTickSize must return nil;
// one that does not must return an error.
func TestTieredTick_NoTable(t *testing.T) {
	const defaultTick = 0.25

	t.Run("valid_price_aligns_default", func(t *testing.T) {
		if err := engine.ValidateTickSize(50.75, nil, defaultTick); err != nil {
			t.Errorf("expected nil for 50.75 with default tick 0.25, got: %v", err)
		}
	})

	t.Run("invalid_price_does_not_align_default", func(t *testing.T) {
		err := engine.ValidateTickSize(50.10, nil, defaultTick)
		if err == nil {
			t.Error("expected error for 50.10 with default tick 0.25, got nil")
		}
		if !strings.Contains(err.Error(), "tick_size") {
			t.Errorf("error %q should mention tick_size", err.Error())
		}
	})

	t.Run("zero_default_no_validation", func(t *testing.T) {
		// defaultTickSize <= 0 means no validation at all.
		if err := engine.ValidateTickSize(99.99, nil, 0); err != nil {
			t.Errorf("expected nil when defaultTickSize=0 (no validation), got: %v", err)
		}
	})

	t.Run("negative_default_no_validation", func(t *testing.T) {
		if err := engine.ValidateTickSize(123.456, nil, -1); err != nil {
			t.Errorf("expected nil when defaultTickSize<0, got: %v", err)
		}
	})
}

// TestTieredTick_EmptyTiers verifies that a TickTable with an empty Tiers slice
// falls back to defaultTickSize, just like a nil table.
func TestTieredTick_EmptyTiers(t *testing.T) {
	emptyTable := &types.TickTable{
		InstrumentID: "INST-EMPTY",
		Tiers:        []types.TickTier{}, // explicitly empty — not nil table, but no tiers
	}
	const defaultTick = 0.5

	t.Run("valid_price_aligns_default", func(t *testing.T) {
		if err := engine.ValidateTickSize(10.0, emptyTable, defaultTick); err != nil {
			t.Errorf("expected nil for 10.0 with default tick 0.5, got: %v", err)
		}
	})

	t.Run("invalid_price_does_not_align_default", func(t *testing.T) {
		err := engine.ValidateTickSize(10.3, emptyTable, defaultTick)
		if err == nil {
			t.Error("expected error for 10.3 with default tick 0.5, got nil")
		}
	})
}

// TestTieredTick_PriceAboveAllTiers verifies that a price that exceeds all
// tier MaxPrice values uses the last tier's tick size.
func TestTieredTick_PriceAboveAllTiers(t *testing.T) {
	// Two tiers: [0, 50) tick 0.1, [50, 100) tick 0.5.
	// Price 150 exceeds both MaxPrices → should use last tier (tick 0.5).
	table := tickTable("INST-D",
		0, 50, 0.1,
		50, 100, 0.5,
	)

	t.Run("valid_above_max", func(t *testing.T) {
		// 150.0 is a multiple of 0.5 (last tier tick).
		if err := engine.ValidateTickSize(150.0, table, 0.01); err != nil {
			t.Errorf("expected nil for 150.0 using last tier tick 0.5, got: %v", err)
		}
	})

	t.Run("invalid_above_max", func(t *testing.T) {
		// 150.3 is NOT a multiple of 0.5.
		if err := engine.ValidateTickSize(150.3, table, 0.01); err == nil {
			t.Error("expected error for 150.3 using last tier tick 0.5, got nil")
		}
	})
}

// TestTieredTick_BoundaryExact verifies that the tier boundary condition
// (price >= MinPrice && price < MaxPrice) is respected at exact boundary values.
func TestTieredTick_BoundaryExact(t *testing.T) {
	// Tier 1: [0, 10) tick 1.0; Tier 2: [10, 100) tick 5.0
	table := tickTable("INST-E",
		0, 10, 1.0,
		10, 100, 5.0,
	)

	t.Run("price_at_min_of_first_tier", func(t *testing.T) {
		// 0.0 falls in [0, 10) → tick 1.0. 0.0 is a multiple of 1.0.
		if err := engine.ValidateTickSize(0.0, table, 0.01); err != nil {
			t.Errorf("expected nil for 0.0 in first tier, got: %v", err)
		}
	})

	t.Run("price_equal_to_max_of_first_tier_is_in_second_tier", func(t *testing.T) {
		// 10.0 is NOT < 10, so it falls in [10, 100) → tick 5.0.
		// 10.0 is a multiple of 5.0.
		if err := engine.ValidateTickSize(10.0, table, 0.01); err != nil {
			t.Errorf("expected nil for 10.0 in second tier (tick 5.0), got: %v", err)
		}
	})

	t.Run("price_9_invalid_for_tick_1_is_valid_at_1", func(t *testing.T) {
		// 9.0 is in [0, 10) → tick 1.0. Valid.
		if err := engine.ValidateTickSize(9.0, table, 0.01); err != nil {
			t.Errorf("expected nil for 9.0 in first tier, got: %v", err)
		}
	})

	t.Run("price_9_invalid_for_second_tier_tick", func(t *testing.T) {
		// 9.5 is in [0, 10) → tick 1.0. NOT a multiple of 1.0.
		if err := engine.ValidateTickSize(9.5, table, 0.01); err == nil {
			t.Error("expected error for 9.5 in first tier (tick 1.0), got nil")
		}
	})
}
