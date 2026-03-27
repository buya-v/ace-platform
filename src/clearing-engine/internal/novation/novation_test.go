package novation

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ace-platform/clearing-engine/internal/types"
)

type testIDGen struct{ counter uint64 }

func (g *testIDGen) NewID() string {
	n := atomic.AddUint64(&g.counter, 1)
	return fmt.Sprintf("obl-%d", n)
}

func makeTrade(id string, buyer, seller string, price int64, qty uint64) types.Trade {
	return types.Trade{
		TradeID:             id,
		InstrumentID:        "WHT-HRW-2026M07-UB",
		BuyOrderID:          "buy-order-1",
		SellOrderID:         "sell-order-1",
		BuyerParticipantID:  buyer,
		SellerParticipantID: seller,
		Price:               types.DecimalFromInt(price),
		Quantity:            qty,
		TradeValue:          types.DecimalFromInt(price).MulUint64(qty),
		AggressorSide:       types.SideBuy,
		SequenceNumber:      1,
		ExecutedAt:          time.Now(),
	}
}

func TestNovateCreatestwoCCPObligations(t *testing.T) {
	svc := NewService(&testIDGen{})
	trade := makeTrade("t-1", "buyer-1", "seller-1", 500, 10)

	result, err := svc.Novate(trade)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Buyer obligation
	bo := result.BuyerObligation
	if bo.ParticipantID != "buyer-1" {
		t.Errorf("buyer participant = %s, want buyer-1", bo.ParticipantID)
	}
	if bo.Side != types.SideBuy {
		t.Errorf("buyer side = %v, want BUY", bo.Side)
	}
	if bo.TradeID != "t-1" {
		t.Errorf("buyer tradeID = %s, want t-1", bo.TradeID)
	}
	if bo.Quantity != 10 {
		t.Errorf("buyer qty = %d, want 10", bo.Quantity)
	}
	if bo.Status != types.ClearingStatusNovated {
		t.Errorf("buyer status = %v, want NOVATED", bo.Status)
	}
	if bo.Value.String() != "5000" {
		t.Errorf("buyer value = %s, want 5000", bo.Value.String())
	}

	// Seller obligation
	so := result.SellerObligation
	if so.ParticipantID != "seller-1" {
		t.Errorf("seller participant = %s, want seller-1", so.ParticipantID)
	}
	if so.Side != types.SideSell {
		t.Errorf("seller side = %v, want SELL", so.Side)
	}
	if so.Quantity != 10 {
		t.Errorf("seller qty = %d, want 10", so.Quantity)
	}

	// Both should have different obligation IDs
	if bo.ObligationID == so.ObligationID {
		t.Error("buyer and seller obligations should have different IDs")
	}
}

func TestNovateRejectsEmptyTradeID(t *testing.T) {
	svc := NewService(&testIDGen{})
	trade := makeTrade("", "buyer-1", "seller-1", 500, 10)

	_, err := svc.Novate(trade)
	if err == nil {
		t.Fatal("expected error for empty trade ID")
	}
}

func TestNovateRejectsZeroQuantity(t *testing.T) {
	svc := NewService(&testIDGen{})
	trade := makeTrade("t-1", "buyer-1", "seller-1", 500, 0)

	_, err := svc.Novate(trade)
	if err == nil {
		t.Fatal("expected error for zero quantity")
	}
}

func TestNovateRejectsMissingParticipants(t *testing.T) {
	svc := NewService(&testIDGen{})

	trade := makeTrade("t-1", "", "seller-1", 500, 10)
	_, err := svc.Novate(trade)
	if err == nil {
		t.Fatal("expected error for empty buyer")
	}

	trade = makeTrade("t-1", "buyer-1", "", 500, 10)
	_, err = svc.Novate(trade)
	if err == nil {
		t.Fatal("expected error for empty seller")
	}
}

func TestNovatePreservesInstrument(t *testing.T) {
	svc := NewService(&testIDGen{})
	trade := makeTrade("t-1", "b", "s", 100, 5)
	trade.InstrumentID = "CORN-2026M12-CH"

	result, err := svc.Novate(trade)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.BuyerObligation.InstrumentID != "CORN-2026M12-CH" {
		t.Errorf("instrument mismatch: %s", result.BuyerObligation.InstrumentID)
	}
	if result.SellerObligation.InstrumentID != "CORN-2026M12-CH" {
		t.Errorf("instrument mismatch: %s", result.SellerObligation.InstrumentID)
	}
}
