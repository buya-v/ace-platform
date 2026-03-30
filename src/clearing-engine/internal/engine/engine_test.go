package engine

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garudax-platform/clearing-engine/internal/novation"
	"github.com/garudax-platform/clearing-engine/internal/store"
	"github.com/garudax-platform/clearing-engine/internal/types"
)

type testIDGen struct{ counter uint64 }

func (g *testIDGen) NewID() string {
	n := atomic.AddUint64(&g.counter, 1)
	return fmt.Sprintf("obl-%d", n)
}

func makeTrade(id, buyer, seller string, price int64, qty uint64) types.Trade {
	return types.Trade{
		TradeID:             id,
		InstrumentID:        "WHT-HRW-2026M07-UB",
		BuyOrderID:          "buy-" + id,
		SellOrderID:         "sell-" + id,
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

func newTestEngine() *Engine {
	return NewEngine(&testIDGen{}, store.NewInMemoryObligationStore())
}

func TestClearTradeBasic(t *testing.T) {
	eng := newTestEngine()
	trade := makeTrade("t-1", "buyer-1", "seller-1", 500, 10)

	result, err := eng.ClearTrade(trade)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.BuyerPosition.NetQuantity != 10 {
		t.Errorf("buyer net = %d, want 10", result.BuyerPosition.NetQuantity)
	}
	if result.SellerPosition.NetQuantity != -10 {
		t.Errorf("seller net = %d, want -10", result.SellerPosition.NetQuantity)
	}
}

func TestClearTradeIdempotency(t *testing.T) {
	eng := newTestEngine()
	trade := makeTrade("t-1", "buyer-1", "seller-1", 500, 10)

	_, err := eng.ClearTrade(trade)
	if err != nil {
		t.Fatalf("first clear failed: %v", err)
	}

	_, err = eng.ClearTrade(trade)
	if err == nil {
		t.Fatal("expected error on duplicate trade")
	}
}

func TestClearMultipleTradesUpdatesPositions(t *testing.T) {
	eng := newTestEngine()

	// Trade 1: buyer-1 buys 10 from seller-1 @ 500
	eng.ClearTrade(makeTrade("t-1", "buyer-1", "seller-1", 500, 10))
	// Trade 2: buyer-1 buys 5 from seller-2 @ 600
	result, _ := eng.ClearTrade(makeTrade("t-2", "buyer-1", "seller-2", 600, 5))

	if result.BuyerPosition.NetQuantity != 15 {
		t.Errorf("buyer net = %d, want 15", result.BuyerPosition.NetQuantity)
	}
	if result.BuyerPosition.TotalBuyQty != 15 {
		t.Errorf("total buy = %d, want 15", result.BuyerPosition.TotalBuyQty)
	}
}

func TestClearTradeWithHandler(t *testing.T) {
	eng := newTestEngine()

	var handled bool
	eng.SetTradeHandler(func(trade types.Trade, result novation.NovationResult) {
		handled = true
		if trade.TradeID != "t-1" {
			t.Errorf("handler trade ID = %s, want t-1", trade.TradeID)
		}
	})

	eng.ClearTrade(makeTrade("t-1", "buyer-1", "seller-1", 500, 10))
	if !handled {
		t.Error("trade handler was not called")
	}
}

func TestGetObligations(t *testing.T) {
	eng := newTestEngine()
	eng.ClearTrade(makeTrade("t-1", "buyer-1", "seller-1", 500, 10))

	obls := eng.GetObligations("t-1")
	if len(obls) != 2 {
		t.Fatalf("got %d obligations, want 2", len(obls))
	}
}

func TestGetPosition(t *testing.T) {
	eng := newTestEngine()
	eng.ClearTrade(makeTrade("t-1", "buyer-1", "seller-1", 500, 10))

	pos, ok := eng.GetPosition("buyer-1", "WHT-HRW-2026M07-UB")
	if !ok {
		t.Fatal("position not found")
	}
	if pos.NetQuantity != 10 {
		t.Errorf("net = %d, want 10", pos.NetQuantity)
	}
}

func TestGetPositionNotFound(t *testing.T) {
	eng := newTestEngine()
	_, ok := eng.GetPosition("unknown", "unknown")
	if ok {
		t.Error("expected not found")
	}
}

func TestNetObligations(t *testing.T) {
	eng := newTestEngine()

	// P1 buys 10, then sells 3
	eng.ClearTrade(makeTrade("t-1", "P1", "P2", 500, 10))
	eng.ClearTrade(makeTrade("t-2", "P2", "P1", 500, 3))

	results := eng.NetObligations()

	resultMap := make(map[string]types.NettingResult)
	for _, r := range results {
		resultMap[r.ParticipantID] = r
	}

	p1 := resultMap["P1"]
	if p1.NetQuantity != 7 { // bought 10, sold 3
		t.Errorf("P1 net = %d, want 7", p1.NetQuantity)
	}

	p2 := resultMap["P2"]
	if p2.NetQuantity != -7 { // sold 10, bought 3
		t.Errorf("P2 net = %d, want -7", p2.NetQuantity)
	}
}

func TestNetObligationsByInstrument(t *testing.T) {
	eng := newTestEngine()

	trade1 := makeTrade("t-1", "P1", "P2", 500, 10)
	trade2 := makeTrade("t-2", "P1", "P2", 400, 5)
	trade2.InstrumentID = "CORN-2026M12-CH"

	eng.ClearTrade(trade1)
	eng.ClearTrade(trade2)

	results := eng.NetObligationsByInstrument("WHT-HRW-2026M07-UB")
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2 (one per participant)", len(results))
	}
}

func TestConcurrentClearing(t *testing.T) {
	eng := newTestEngine()

	var wg sync.WaitGroup
	errs := make(chan error, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			trade := makeTrade(
				fmt.Sprintf("t-%d", idx),
				fmt.Sprintf("buyer-%d", idx%10),
				fmt.Sprintf("seller-%d", idx%10),
				500,
				uint64(idx+1),
			)
			_, err := eng.ClearTrade(trade)
			if err != nil {
				errs <- err
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent clear error: %v", err)
	}
}
