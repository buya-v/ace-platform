package server

import (
	"testing"
	"time"

	"github.com/ace-platform/market-data-service/internal/types"
)

func mustDecimal(s string) types.Decimal {
	d, _ := types.ParseDecimal(s)
	return d
}

func makeTrade(id, instrumentID, price string, qty uint64, seq uint64, execAt time.Time) types.Trade {
	return types.Trade{
		TradeID:        id,
		InstrumentID:   instrumentID,
		Price:          mustDecimal(price),
		Quantity:       qty,
		TradeValue:     mustDecimal(price).MulUint64(qty),
		AggressorSide:  "BUY",
		TradeType:      "CONTINUOUS",
		SequenceNumber: seq,
		ExecutedAt:     execAt,
	}
}

func TestServer_IngestAndGetCandles(t *testing.T) {
	srv := NewServer(DefaultConfig())
	base := time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)

	srv.IngestTrade(makeTrade("t1", "inst-1", "100", 5, 1, base.Add(10*time.Second)))
	srv.IngestTrade(makeTrade("t2", "inst-1", "105", 3, 2, base.Add(20*time.Second)))

	candles := srv.GetCandles("inst-1", types.Interval1m, base, base.Add(time.Minute), 500)
	if len(candles) == 0 {
		t.Fatal("expected at least 1 candle")
	}

	c := candles[0]
	if !c.Open.Equal(mustDecimal("100")) {
		t.Errorf("open = %s, want 100", c.Open)
	}
	if !c.High.Equal(mustDecimal("105")) {
		t.Errorf("high = %s, want 105", c.High)
	}
	if c.Volume != 8 {
		t.Errorf("volume = %d, want 8", c.Volume)
	}
}

func TestServer_IngestAndGetTicker(t *testing.T) {
	srv := NewServer(DefaultConfig())
	srv.SetSymbol("inst-1", "WHT-HRW")

	now := time.Now().UTC()
	srv.IngestTrade(makeTrade("t1", "inst-1", "100", 5, 1, now))
	srv.IngestTrade(makeTrade("t2", "inst-1", "110", 3, 2, now.Add(time.Second)))

	ticker, ok := srv.GetTicker("inst-1")
	if !ok {
		t.Fatal("expected ticker")
	}
	if ticker.Symbol != "WHT-HRW" {
		t.Errorf("symbol = %s, want WHT-HRW", ticker.Symbol)
	}
	if !ticker.LastPrice.Equal(mustDecimal("110")) {
		t.Errorf("last_price = %s, want 110", ticker.LastPrice)
	}
}

func TestServer_IngestAndGetTrades(t *testing.T) {
	srv := NewServer(DefaultConfig())
	now := time.Now().UTC()

	srv.IngestTrade(makeTrade("t1", "inst-1", "100", 5, 1, now))
	srv.IngestTrade(makeTrade("t2", "inst-1", "101", 3, 2, now.Add(time.Second)))
	srv.IngestTrade(makeTrade("t3", "inst-1", "102", 7, 3, now.Add(2*time.Second)))

	// Last N
	trades := srv.GetTrades("inst-1", 2, 0, time.Time{}, time.Time{})
	if len(trades) != 2 {
		t.Fatalf("expected 2 trades, got %d", len(trades))
	}
	if trades[0].TradeID != "t3" {
		t.Errorf("first trade = %s, want t3 (newest first)", trades[0].TradeID)
	}

	// Since sequence
	trades = srv.GetTrades("inst-1", 100, 1, time.Time{}, time.Time{})
	if len(trades) != 2 {
		t.Fatalf("expected 2 trades since seq 1, got %d", len(trades))
	}
}

func TestServer_GetTickers(t *testing.T) {
	srv := NewServer(DefaultConfig())
	now := time.Now().UTC()

	srv.IngestTrade(makeTrade("t1", "inst-1", "100", 5, 1, now))
	srv.IngestTrade(makeTrade("t2", "inst-2", "200", 3, 1, now))

	tickers := srv.GetTickers(nil)
	if len(tickers) != 2 {
		t.Errorf("expected 2 tickers, got %d", len(tickers))
	}
}

func TestServer_StreamingIntegration(t *testing.T) {
	srv := NewServer(DefaultConfig())

	sub := srv.SubscribeTrades("inst-1")
	defer sub.Close()

	now := time.Now().UTC()
	srv.IngestTrade(makeTrade("t1", "inst-1", "100", 5, 1, now))

	select {
	case trade := <-sub.Ch:
		if trade.TradeID != "t1" {
			t.Errorf("trade_id = %s, want t1", trade.TradeID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for streamed trade")
	}
}

func TestServer_ReadyState(t *testing.T) {
	srv := NewServer(DefaultConfig())
	if srv.IsReady() {
		t.Error("should not be ready initially")
	}
	srv.SetReady()
	if !srv.IsReady() {
		t.Error("should be ready after SetReady")
	}
}

func TestServer_GetCandles_LimitClamping(t *testing.T) {
	srv := NewServer(DefaultConfig())
	base := time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)
	srv.IngestTrade(makeTrade("t1", "inst-1", "100", 5, 1, base))

	// Limit > 5000 should be clamped
	candles := srv.GetCandles("inst-1", types.Interval1m, base, base.Add(time.Hour), 10000)
	_ = candles // Just ensure no panic
}
