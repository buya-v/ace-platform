// Package candle provides in-memory OHLCV candle aggregation from trade events.
package candle

import (
	"sync"
	"time"

	"github.com/garudax-platform/market-data-service/internal/types"
)

// candleKey uniquely identifies an in-progress candle.
type candleKey struct {
	InstrumentID string
	Interval     types.CandleInterval
	Bucket       time.Time
}

// Builder aggregates trade events into OHLCV candles across all intervals.
// It maintains one in-progress candle per (instrument, interval) pair.
// Completed candles are emitted via the OnCandle callback.
type Builder struct {
	mu       sync.RWMutex
	candles  map[candleKey]*types.Candle
	onCandle func(types.Candle) // callback when a candle updates
}

// NewBuilder creates a new candle builder.
// onCandle is called (under lock) every time a candle is updated by a trade.
func NewBuilder(onCandle func(types.Candle)) *Builder {
	if onCandle == nil {
		onCandle = func(types.Candle) {}
	}
	return &Builder{
		candles:  make(map[candleKey]*types.Candle),
		onCandle: onCandle,
	}
}

// IngestTrade processes a trade and updates all interval candles for that instrument.
func (b *Builder) IngestTrade(trade types.Trade) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now().UTC()

	for _, interval := range types.AllIntervals() {
		bucket := types.BucketStart(trade.ExecutedAt, interval)
		key := candleKey{
			InstrumentID: trade.InstrumentID,
			Interval:     interval,
			Bucket:       bucket,
		}

		c, exists := b.candles[key]
		if !exists {
			// Close any previous candle for this instrument+interval
			b.closePreviousCandle(trade.InstrumentID, interval, bucket)

			c = &types.Candle{
				InstrumentID: trade.InstrumentID,
				Interval:     interval,
				Bucket:       bucket,
				Open:         trade.Price,
				High:         trade.Price,
				Low:          trade.Price,
				Close:        trade.Price,
				Volume:       0,
				TradeCount:   0,
				Turnover:     types.DecimalZero(),
				IsClosed:     false,
			}
			b.candles[key] = c
		}

		// Update OHLCV
		if trade.Price.GreaterThan(c.High) {
			c.High = trade.Price
		}
		if trade.Price.LessThan(c.Low) {
			c.Low = trade.Price
		}
		c.Close = trade.Price
		c.Volume += trade.Quantity
		c.TradeCount++
		c.Turnover = c.Turnover.Add(trade.Price.MulUint64(trade.Quantity))

		// Compute VWAP = turnover / volume
		if c.Volume > 0 {
			c.VWAP = c.Turnover.DivInt(int64(c.Volume))
		}

		c.Timestamp = now

		// Emit update
		b.onCandle(*c)
	}
}

// closePreviousCandle marks the previous candle for the given instrument+interval as closed.
func (b *Builder) closePreviousCandle(instrumentID string, interval types.CandleInterval, currentBucket time.Time) {
	// Scan for a candle with the same instrument+interval but older bucket
	for key, c := range b.candles {
		if key.InstrumentID == instrumentID && key.Interval == interval && key.Bucket.Before(currentBucket) {
			c.IsClosed = true
			c.Timestamp = time.Now().UTC()
			b.onCandle(*c)
			delete(b.candles, key)
		}
	}
}

// GetCandle returns the current in-progress candle for a given instrument and interval.
func (b *Builder) GetCandle(instrumentID string, interval types.CandleInterval) (types.Candle, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for key, c := range b.candles {
		if key.InstrumentID == instrumentID && key.Interval == interval {
			return *c, true
		}
	}
	return types.Candle{}, false
}

// GetAllCandles returns all in-progress candles for a given instrument.
func (b *Builder) GetAllCandles(instrumentID string) []types.Candle {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var result []types.Candle
	for key, c := range b.candles {
		if key.InstrumentID == instrumentID {
			_ = key
			result = append(result, *c)
		}
	}
	return result
}

// FlushClosed removes and returns all candles whose bucket end time has passed.
func (b *Builder) FlushClosed(now time.Time) []types.Candle {
	b.mu.Lock()
	defer b.mu.Unlock()

	var flushed []types.Candle
	for key, c := range b.candles {
		bucketEnd := key.Bucket.Add(key.Interval.Duration())
		if now.After(bucketEnd) || now.Equal(bucketEnd) {
			c.IsClosed = true
			c.Timestamp = now
			flushed = append(flushed, *c)
			delete(b.candles, key)
		}
	}
	return flushed
}
