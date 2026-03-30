package candle

import (
	"sync"
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

func makeTrade(instrumentID, price string, qty uint64, seq uint64, execAt time.Time) types.Trade {
	return types.Trade{
		TradeID:        "t-" + price,
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

func TestBuilder_SingleTrade(t *testing.T) {
	var updates []types.Candle
	var mu sync.Mutex
	b := NewBuilder(func(c types.Candle) {
		mu.Lock()
		updates = append(updates, c)
		mu.Unlock()
	})

	now := time.Date(2026, 3, 28, 10, 3, 15, 0, time.UTC)
	trade := makeTrade("inst-1", "100.50", 10, 1, now)
	b.IngestTrade(trade)

	// Should create candles for all 6 intervals
	mu.Lock()
	if len(updates) != 6 {
		t.Fatalf("expected 6 candle updates, got %d", len(updates))
	}
	mu.Unlock()

	// Check 1m candle
	c, ok := b.GetCandle("inst-1", types.Interval1m)
	if !ok {
		t.Fatal("expected 1m candle")
	}
	if !c.Open.Equal(mustDecimal("100.50")) {
		t.Errorf("open = %s, want 100.50", c.Open)
	}
	if !c.High.Equal(mustDecimal("100.50")) {
		t.Errorf("high = %s, want 100.50", c.High)
	}
	if !c.Low.Equal(mustDecimal("100.50")) {
		t.Errorf("low = %s, want 100.50", c.Low)
	}
	if !c.Close.Equal(mustDecimal("100.50")) {
		t.Errorf("close = %s, want 100.50", c.Close)
	}
	if c.Volume != 10 {
		t.Errorf("volume = %d, want 10", c.Volume)
	}
	if c.TradeCount != 1 {
		t.Errorf("trade_count = %d, want 1", c.TradeCount)
	}
	if c.IsClosed {
		t.Error("candle should not be closed")
	}

	// Bucket should be start of minute
	expectedBucket := time.Date(2026, 3, 28, 10, 3, 0, 0, time.UTC)
	if !c.Bucket.Equal(expectedBucket) {
		t.Errorf("bucket = %v, want %v", c.Bucket, expectedBucket)
	}
}

func TestBuilder_MultipleTrades_OHLCV(t *testing.T) {
	b := NewBuilder(nil)
	base := time.Date(2026, 3, 28, 10, 5, 0, 0, time.UTC)

	// Three trades within the same 1-minute bucket
	b.IngestTrade(makeTrade("inst-1", "100", 5, 1, base.Add(1*time.Second)))
	b.IngestTrade(makeTrade("inst-1", "105", 3, 2, base.Add(2*time.Second)))
	b.IngestTrade(makeTrade("inst-1", "98", 7, 3, base.Add(3*time.Second)))

	c, ok := b.GetCandle("inst-1", types.Interval1m)
	if !ok {
		t.Fatal("expected 1m candle")
	}

	if !c.Open.Equal(mustDecimal("100")) {
		t.Errorf("open = %s, want 100", c.Open)
	}
	if !c.High.Equal(mustDecimal("105")) {
		t.Errorf("high = %s, want 105", c.High)
	}
	if !c.Low.Equal(mustDecimal("98")) {
		t.Errorf("low = %s, want 98", c.Low)
	}
	if !c.Close.Equal(mustDecimal("98")) {
		t.Errorf("close = %s, want 98", c.Close)
	}
	if c.Volume != 15 {
		t.Errorf("volume = %d, want 15", c.Volume)
	}
	if c.TradeCount != 3 {
		t.Errorf("trade_count = %d, want 3", c.TradeCount)
	}

	// Turnover = 100*5 + 105*3 + 98*7 = 500 + 315 + 686 = 1501
	expectedTurnover := mustDecimal("1501")
	if !c.Turnover.Equal(expectedTurnover) {
		t.Errorf("turnover = %s, want %s", c.Turnover, expectedTurnover)
	}

	// VWAP = 1501 / 15 = 100.0666... truncated to 100.0666
	// With Decimal(18,4), 1501*10000/15 = 10006666 -> 1000.6666
	// Actually: turnover raw = 1501 * 10000 = 15010000, DivInt(15) = 1000666
	// That represents 100.0666
	if c.VWAP.IsZero() {
		t.Error("vwap should not be zero")
	}
}

func TestBuilder_NewBucket_ClosesPrevious(t *testing.T) {
	var closedCandles []types.Candle
	var mu sync.Mutex
	b := NewBuilder(func(c types.Candle) {
		mu.Lock()
		if c.IsClosed {
			closedCandles = append(closedCandles, c)
		}
		mu.Unlock()
	})

	// Trade at 10:05:30
	t1 := time.Date(2026, 3, 28, 10, 5, 30, 0, time.UTC)
	b.IngestTrade(makeTrade("inst-1", "100", 5, 1, t1))

	// Trade at 10:06:10 — new 1m bucket
	t2 := time.Date(2026, 3, 28, 10, 6, 10, 0, time.UTC)
	b.IngestTrade(makeTrade("inst-1", "101", 3, 2, t2))

	mu.Lock()
	// Should have closed the 10:05 1m candle (and potentially others depending on interval boundaries)
	found := false
	for _, c := range closedCandles {
		if c.Interval == types.Interval1m && c.Bucket.Equal(time.Date(2026, 3, 28, 10, 5, 0, 0, time.UTC)) {
			found = true
			if !c.IsClosed {
				t.Error("previous candle should be closed")
			}
		}
	}
	mu.Unlock()
	if !found {
		t.Error("expected previous 1m candle to be closed")
	}
}

func TestBuilder_MultipleInstruments(t *testing.T) {
	b := NewBuilder(nil)
	now := time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)

	b.IngestTrade(makeTrade("inst-1", "100", 5, 1, now))
	b.IngestTrade(makeTrade("inst-2", "200", 3, 1, now))

	c1, ok := b.GetCandle("inst-1", types.Interval1m)
	if !ok {
		t.Fatal("expected inst-1 candle")
	}
	if !c1.Open.Equal(mustDecimal("100")) {
		t.Errorf("inst-1 open = %s, want 100", c1.Open)
	}

	c2, ok := b.GetCandle("inst-2", types.Interval1m)
	if !ok {
		t.Fatal("expected inst-2 candle")
	}
	if !c2.Open.Equal(mustDecimal("200")) {
		t.Errorf("inst-2 open = %s, want 200", c2.Open)
	}
}

func TestBuilder_GetAllCandles(t *testing.T) {
	b := NewBuilder(nil)
	now := time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)
	b.IngestTrade(makeTrade("inst-1", "100", 5, 1, now))

	all := b.GetAllCandles("inst-1")
	if len(all) != 6 {
		t.Errorf("expected 6 candles (one per interval), got %d", len(all))
	}
}

func TestBuilder_FlushClosed(t *testing.T) {
	b := NewBuilder(nil)
	// Trade at 10:00:30
	t1 := time.Date(2026, 3, 28, 10, 0, 30, 0, time.UTC)
	b.IngestTrade(makeTrade("inst-1", "100", 5, 1, t1))

	// Flush at 10:02 — the 1m candle (bucket 10:00) should be flushed
	flushTime := time.Date(2026, 3, 28, 10, 2, 0, 0, time.UTC)
	flushed := b.FlushClosed(flushTime)

	// At minimum the 1m candle should be flushed
	foundMinute := false
	for _, c := range flushed {
		if c.Interval == types.Interval1m {
			foundMinute = true
			if !c.IsClosed {
				t.Error("flushed 1m candle should be closed")
			}
		}
	}
	if !foundMinute {
		t.Error("expected 1m candle in flushed set")
	}

	// The flushed candle should no longer be in the builder
	_, ok := b.GetCandle("inst-1", types.Interval1m)
	if ok {
		t.Error("flushed candle should not be in builder")
	}
}

func TestBuilder_GetCandle_NotFound(t *testing.T) {
	b := NewBuilder(nil)
	_, ok := b.GetCandle("nonexistent", types.Interval1m)
	if ok {
		t.Error("expected not found for nonexistent instrument")
	}
}

func TestBucketStart(t *testing.T) {
	ts := time.Date(2026, 3, 28, 10, 37, 42, 0, time.UTC)

	tests := []struct {
		interval types.CandleInterval
		expected time.Time
	}{
		{types.Interval1m, time.Date(2026, 3, 28, 10, 37, 0, 0, time.UTC)},
		{types.Interval5m, time.Date(2026, 3, 28, 10, 35, 0, 0, time.UTC)},
		{types.Interval15m, time.Date(2026, 3, 28, 10, 30, 0, 0, time.UTC)},
		{types.Interval1h, time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)},
		{types.Interval4h, time.Date(2026, 3, 28, 8, 0, 0, 0, time.UTC)},
		{types.Interval1d, time.Date(2026, 3, 28, 0, 0, 0, 0, time.UTC)},
	}

	for _, tt := range tests {
		got := types.BucketStart(ts, tt.interval)
		if !got.Equal(tt.expected) {
			t.Errorf("BucketStart(%v, %s) = %v, want %v", ts, tt.interval, got, tt.expected)
		}
	}
}
