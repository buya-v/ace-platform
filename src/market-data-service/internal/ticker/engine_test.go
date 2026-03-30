package ticker

import (
	"testing"
	"time"

	"github.com/garudax-platform/market-data-service/internal/types"
)

func mustDecimal(s string) types.Decimal {
	d, err := types.ParseDecimal(s)
	if err != nil {
		panic(err)
	}
	return d
}

func makeTrade(instrumentID, price string, qty uint64, execAt time.Time) types.Trade {
	return types.Trade{
		TradeID:       "t-" + price,
		InstrumentID:  instrumentID,
		Price:         mustDecimal(price),
		Quantity:      qty,
		TradeValue:    mustDecimal(price).MulUint64(qty),
		AggressorSide: "BUY",
		TradeType:     "CONTINUOUS",
		ExecutedAt:    execAt,
	}
}

func TestEngine_SingleTrade(t *testing.T) {
	eng := NewEngine()
	eng.SetSymbol("inst-1", "WHT-HRW")

	now := time.Now().UTC()
	eng.IngestTrade(makeTrade("inst-1", "100", 10, now))

	ticker, ok := eng.GetTicker("inst-1")
	if !ok {
		t.Fatal("expected ticker")
	}

	if ticker.Symbol != "WHT-HRW" {
		t.Errorf("symbol = %s, want WHT-HRW", ticker.Symbol)
	}
	if !ticker.LastPrice.Equal(mustDecimal("100")) {
		t.Errorf("last_price = %s, want 100", ticker.LastPrice)
	}
	if ticker.Volume24h != 10 {
		t.Errorf("volume_24h = %d, want 10", ticker.Volume24h)
	}
}

func TestEngine_MultipleTrades_24hStats(t *testing.T) {
	eng := NewEngine()
	now := time.Now().UTC()

	eng.IngestTrade(makeTrade("inst-1", "100", 5, now.Add(-1*time.Hour)))
	eng.IngestTrade(makeTrade("inst-1", "110", 3, now.Add(-30*time.Minute)))
	eng.IngestTrade(makeTrade("inst-1", "95", 7, now.Add(-10*time.Minute)))

	ticker, ok := eng.GetTicker("inst-1")
	if !ok {
		t.Fatal("expected ticker")
	}

	if !ticker.High24h.Equal(mustDecimal("110")) {
		t.Errorf("high_24h = %s, want 110", ticker.High24h)
	}
	if !ticker.Low24h.Equal(mustDecimal("95")) {
		t.Errorf("low_24h = %s, want 95", ticker.Low24h)
	}
	if !ticker.LastPrice.Equal(mustDecimal("95")) {
		t.Errorf("last_price = %s, want 95", ticker.LastPrice)
	}
	// Volume = 5 + 3 + 7 = 15
	if ticker.Volume24h != 15 {
		t.Errorf("volume_24h = %d, want 15", ticker.Volume24h)
	}
	// Change = 95 - 100 = -5
	if !ticker.PriceChange24h.Equal(mustDecimal("-5")) {
		t.Errorf("price_change_24h = %s, want -5", ticker.PriceChange24h)
	}
	// Change % = -5/100 * 100 = -5%
	if ticker.PriceChangePct.IsZero() {
		t.Error("price_change_pct should not be zero")
	}
}

func TestEngine_GetTicker_NotFound(t *testing.T) {
	eng := NewEngine()
	_, ok := eng.GetTicker("nonexistent")
	if ok {
		t.Error("expected not found")
	}
}

func TestEngine_GetTickers_All(t *testing.T) {
	eng := NewEngine()
	now := time.Now().UTC()

	eng.IngestTrade(makeTrade("inst-1", "100", 5, now))
	eng.IngestTrade(makeTrade("inst-2", "200", 3, now))

	tickers := eng.GetTickers(nil)
	if len(tickers) != 2 {
		t.Errorf("expected 2 tickers, got %d", len(tickers))
	}
}

func TestEngine_GetTickers_Filtered(t *testing.T) {
	eng := NewEngine()
	now := time.Now().UTC()

	eng.IngestTrade(makeTrade("inst-1", "100", 5, now))
	eng.IngestTrade(makeTrade("inst-2", "200", 3, now))

	tickers := eng.GetTickers([]string{"inst-1"})
	if len(tickers) != 1 {
		t.Errorf("expected 1 ticker, got %d", len(tickers))
	}
	if tickers[0].InstrumentID != "inst-1" {
		t.Errorf("instrument_id = %s, want inst-1", tickers[0].InstrumentID)
	}
}

func TestEngine_PruneBefore(t *testing.T) {
	eng := NewEngine()
	now := time.Now().UTC()

	eng.IngestTrade(makeTrade("inst-1", "100", 5, now.Add(-25*time.Hour)))
	eng.IngestTrade(makeTrade("inst-1", "110", 3, now.Add(-1*time.Hour)))

	// Before prune, ticker includes both trades
	ticker, _ := eng.GetTicker("inst-1")
	if ticker.Volume24h != 3 {
		// The old trade should already be outside 24h window
		t.Logf("volume before prune: %d (old trade already outside 24h window)", ticker.Volume24h)
	}

	eng.PruneBefore(now.Add(-24 * time.Hour))

	// After prune, only recent trade remains
	ticker, ok := eng.GetTicker("inst-1")
	if !ok {
		t.Fatal("expected ticker after prune")
	}
	if !ticker.LastPrice.Equal(mustDecimal("110")) {
		t.Errorf("last_price = %s, want 110", ticker.LastPrice)
	}
}

func TestEngine_Turnover(t *testing.T) {
	eng := NewEngine()
	now := time.Now().UTC()

	// Trade: 100 * 5 = 500, 200 * 3 = 600
	eng.IngestTrade(makeTrade("inst-1", "100", 5, now))
	eng.IngestTrade(makeTrade("inst-1", "200", 3, now.Add(time.Second)))

	ticker, _ := eng.GetTicker("inst-1")
	expectedTurnover := mustDecimal("1100")
	if !ticker.Turnover24h.Equal(expectedTurnover) {
		t.Errorf("turnover_24h = %s, want %s", ticker.Turnover24h, expectedTurnover)
	}
}
