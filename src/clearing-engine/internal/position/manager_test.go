package position

import (
	"testing"
	"time"

	"github.com/ace-platform/clearing-engine/internal/types"
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

func TestNewBuyPosition(t *testing.T) {
	mgr := NewManager()
	obl := makeObl("P1", "WHT", types.SideBuy, 500, 10)

	pos, err := mgr.Apply(obl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pos.NetQuantity != 10 {
		t.Errorf("net qty = %d, want 10", pos.NetQuantity)
	}
	if pos.AvgEntryPrice.String() != "500" {
		t.Errorf("avg price = %s, want 500", pos.AvgEntryPrice.String())
	}
	if pos.TotalBuyQty != 10 {
		t.Errorf("total buy = %d, want 10", pos.TotalBuyQty)
	}
	if !pos.IsLong() {
		t.Error("expected long position")
	}
}

func TestNewSellPosition(t *testing.T) {
	mgr := NewManager()
	obl := makeObl("P1", "WHT", types.SideSell, 500, 10)

	pos, err := mgr.Apply(obl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pos.NetQuantity != -10 {
		t.Errorf("net qty = %d, want -10", pos.NetQuantity)
	}
	if !pos.IsShort() {
		t.Error("expected short position")
	}
}

func TestAddToLongPosition(t *testing.T) {
	mgr := NewManager()

	mgr.Apply(makeObl("P1", "WHT", types.SideBuy, 500, 10))
	pos, _ := mgr.Apply(makeObl("P1", "WHT", types.SideBuy, 600, 10))

	if pos.NetQuantity != 20 {
		t.Errorf("net qty = %d, want 20", pos.NetQuantity)
	}
	// VWAP = (500*10 + 600*10) / 20 = 550
	if pos.AvgEntryPrice.String() != "550" {
		t.Errorf("avg price = %s, want 550", pos.AvgEntryPrice.String())
	}
}

func TestPartialCloseWithPnL(t *testing.T) {
	mgr := NewManager()

	// Open long 10 @ 500
	mgr.Apply(makeObl("P1", "WHT", types.SideBuy, 500, 10))
	// Close 5 @ 600 (sell 5)
	pos, _ := mgr.Apply(makeObl("P1", "WHT", types.SideSell, 600, 5))

	if pos.NetQuantity != 5 {
		t.Errorf("net qty = %d, want 5", pos.NetQuantity)
	}
	// Realized P&L = (600 - 500) * 5 = 500
	if pos.RealizedPnL.String() != "500" {
		t.Errorf("realized PnL = %s, want 500", pos.RealizedPnL.String())
	}
	// Avg entry price should remain 500 (partial close doesn't change avg)
	if pos.AvgEntryPrice.String() != "500" {
		t.Errorf("avg price = %s, want 500", pos.AvgEntryPrice.String())
	}
}

func TestFullCloseGoesFlat(t *testing.T) {
	mgr := NewManager()

	mgr.Apply(makeObl("P1", "WHT", types.SideBuy, 500, 10))
	pos, _ := mgr.Apply(makeObl("P1", "WHT", types.SideSell, 500, 10))

	if !pos.IsFlat() {
		t.Errorf("expected flat, got net=%d", pos.NetQuantity)
	}
	if !pos.AvgEntryPrice.IsZero() {
		t.Errorf("expected zero avg price for flat position, got %s", pos.AvgEntryPrice.String())
	}
}

func TestPositionFlip(t *testing.T) {
	mgr := NewManager()

	// Open long 10 @ 500
	mgr.Apply(makeObl("P1", "WHT", types.SideBuy, 500, 10))
	// Sell 15 @ 600 — close 10, open short 5
	pos, _ := mgr.Apply(makeObl("P1", "WHT", types.SideSell, 600, 15))

	if pos.NetQuantity != -5 {
		t.Errorf("net qty = %d, want -5", pos.NetQuantity)
	}
	// Realized P&L on closed 10: (600-500)*10 = 1000
	if pos.RealizedPnL.String() != "1000" {
		t.Errorf("realized PnL = %s, want 1000", pos.RealizedPnL.String())
	}
	// New avg price should be 600 (the flip price)
	if pos.AvgEntryPrice.String() != "600" {
		t.Errorf("avg price = %s, want 600", pos.AvgEntryPrice.String())
	}
}

func TestShortPositionPnL(t *testing.T) {
	mgr := NewManager()

	// Open short 10 @ 600
	mgr.Apply(makeObl("P1", "WHT", types.SideSell, 600, 10))
	// Close 5 @ 500 (buy 5 to cover)
	pos, _ := mgr.Apply(makeObl("P1", "WHT", types.SideBuy, 500, 5))

	if pos.NetQuantity != -5 {
		t.Errorf("net qty = %d, want -5", pos.NetQuantity)
	}
	// Short P&L: -(500-600)*5 = 500 (profit because price went down)
	if pos.RealizedPnL.String() != "500" {
		t.Errorf("realized PnL = %s, want 500", pos.RealizedPnL.String())
	}
}

func TestGetPositionNotFound(t *testing.T) {
	mgr := NewManager()
	_, ok := mgr.Get("unknown", "unknown")
	if ok {
		t.Error("expected not found")
	}
}

func TestGetAll(t *testing.T) {
	mgr := NewManager()
	mgr.Apply(makeObl("P1", "WHT", types.SideBuy, 500, 10))
	mgr.Apply(makeObl("P1", "CORN", types.SideBuy, 400, 5))
	mgr.Apply(makeObl("P2", "WHT", types.SideSell, 500, 10))

	p1 := mgr.GetAll("P1")
	if len(p1) != 2 {
		t.Errorf("P1 positions = %d, want 2", len(p1))
	}

	p2 := mgr.GetAll("P2")
	if len(p2) != 1 {
		t.Errorf("P2 positions = %d, want 1", len(p2))
	}
}

func TestGetByInstrument(t *testing.T) {
	mgr := NewManager()
	mgr.Apply(makeObl("P1", "WHT", types.SideBuy, 500, 10))
	mgr.Apply(makeObl("P2", "WHT", types.SideSell, 500, 10))
	mgr.Apply(makeObl("P3", "CORN", types.SideBuy, 400, 5))

	wht := mgr.GetByInstrument("WHT")
	if len(wht) != 2 {
		t.Errorf("WHT positions = %d, want 2", len(wht))
	}
}

func TestRejectsEmptyParticipant(t *testing.T) {
	mgr := NewManager()
	obl := makeObl("", "WHT", types.SideBuy, 500, 10)
	_, err := mgr.Apply(obl)
	if err == nil {
		t.Error("expected error for empty participant")
	}
}

func TestRejectsZeroQuantity(t *testing.T) {
	mgr := NewManager()
	obl := makeObl("P1", "WHT", types.SideBuy, 500, 0)
	_, err := mgr.Apply(obl)
	if err == nil {
		t.Error("expected error for zero quantity")
	}
}
