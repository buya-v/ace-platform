package types

import "time"

// CandleInterval represents the time interval for OHLCV candles.
type CandleInterval int

const (
	Interval1m  CandleInterval = 1
	Interval5m  CandleInterval = 2
	Interval15m CandleInterval = 3
	Interval1h  CandleInterval = 4
	Interval4h  CandleInterval = 5
	Interval1d  CandleInterval = 6
)

// IntervalDuration returns the duration for a candle interval.
func (ci CandleInterval) Duration() time.Duration {
	switch ci {
	case Interval1m:
		return time.Minute
	case Interval5m:
		return 5 * time.Minute
	case Interval15m:
		return 15 * time.Minute
	case Interval1h:
		return time.Hour
	case Interval4h:
		return 4 * time.Hour
	case Interval1d:
		return 24 * time.Hour
	default:
		return time.Minute
	}
}

// String returns the human-readable interval name.
func (ci CandleInterval) String() string {
	switch ci {
	case Interval1m:
		return "1m"
	case Interval5m:
		return "5m"
	case Interval15m:
		return "15m"
	case Interval1h:
		return "1h"
	case Interval4h:
		return "4h"
	case Interval1d:
		return "1d"
	default:
		return "unknown"
	}
}

// AllIntervals returns all supported candle intervals.
func AllIntervals() []CandleInterval {
	return []CandleInterval{Interval1m, Interval5m, Interval15m, Interval1h, Interval4h, Interval1d}
}

// Candle represents an OHLCV candle.
type Candle struct {
	InstrumentID string
	Interval     CandleInterval
	Bucket       time.Time // candle open time (bucket boundary)
	Open         Decimal
	High         Decimal
	Low          Decimal
	Close        Decimal
	Volume       uint64
	TradeCount   int32
	VWAP         Decimal // volume-weighted average price
	Turnover     Decimal // sum of trade values
	IsClosed     bool
	Timestamp    time.Time // server time
}

// BucketStart returns the bucket start time for a given timestamp and interval.
func BucketStart(t time.Time, interval CandleInterval) time.Time {
	t = t.UTC()
	switch interval {
	case Interval1m:
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, time.UTC)
	case Interval5m:
		m := t.Minute() - (t.Minute() % 5)
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), m, 0, 0, time.UTC)
	case Interval15m:
		m := t.Minute() - (t.Minute() % 15)
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), m, 0, 0, time.UTC)
	case Interval1h:
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, time.UTC)
	case Interval4h:
		h := t.Hour() - (t.Hour() % 4)
		return time.Date(t.Year(), t.Month(), t.Day(), h, 0, 0, 0, time.UTC)
	case Interval1d:
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	default:
		return t.Truncate(time.Minute)
	}
}
