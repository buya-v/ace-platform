package streaming

import (
	"testing"
	"time"

	"github.com/ace-platform/market-data-service/internal/types"
)

func TestHub_SubscribeAndPublishCandle(t *testing.T) {
	hub := NewHub()
	sub := hub.SubscribeCandles("inst-1", 10)
	defer sub.Close()

	d, _ := types.ParseDecimal("100")
	candle := types.Candle{
		InstrumentID: "inst-1",
		Interval:     types.Interval1m,
		Open:         d,
		Close:        d,
	}

	hub.PublishCandle(candle)

	select {
	case c := <-sub.Ch:
		if c.InstrumentID != "inst-1" {
			t.Errorf("instrument = %s, want inst-1", c.InstrumentID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for candle")
	}
}

func TestHub_SubscribeAndPublishTrade(t *testing.T) {
	hub := NewHub()
	sub := hub.SubscribeTrades("inst-1", 10)
	defer sub.Close()

	d, _ := types.ParseDecimal("100")
	trade := types.Trade{
		TradeID:      "t1",
		InstrumentID: "inst-1",
		Price:        d,
	}

	hub.PublishTrade(trade)

	select {
	case tr := <-sub.Ch:
		if tr.TradeID != "t1" {
			t.Errorf("trade_id = %s, want t1", tr.TradeID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for trade")
	}
}

func TestHub_DifferentInstruments(t *testing.T) {
	hub := NewHub()
	sub1 := hub.SubscribeCandles("inst-1", 10)
	sub2 := hub.SubscribeCandles("inst-2", 10)
	defer sub1.Close()
	defer sub2.Close()

	d, _ := types.ParseDecimal("100")
	hub.PublishCandle(types.Candle{InstrumentID: "inst-1", Open: d})

	select {
	case <-sub1.Ch:
		// expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout on sub1")
	}

	select {
	case <-sub2.Ch:
		t.Fatal("sub2 should not receive inst-1 candle")
	case <-time.After(50 * time.Millisecond):
		// expected
	}
}

func TestHub_SlowSubscriberDoesNotBlock(t *testing.T) {
	hub := NewHub()
	// Buffer of 1
	sub := hub.SubscribeCandles("inst-1", 1)
	defer sub.Close()

	d, _ := types.ParseDecimal("100")
	// Publish more than buffer size
	for i := 0; i < 5; i++ {
		hub.PublishCandle(types.Candle{InstrumentID: "inst-1", Open: d})
	}

	// Should not have blocked — just drops messages
	// Drain what we can
	count := 0
	for {
		select {
		case <-sub.Ch:
			count++
		default:
			goto done
		}
	}
done:
	if count == 0 {
		t.Error("expected at least 1 message")
	}
	if count > 1 {
		// With buffer 1, we might get exactly 1
		t.Logf("received %d messages with buffer 1", count)
	}
}
