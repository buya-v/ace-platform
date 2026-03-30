// Package retention enforces data retention policies for market data.
package retention

import (
	"time"

	"github.com/garudax-platform/market-data-service/internal/store"
	"github.com/garudax-platform/market-data-service/internal/types"
)

// Rule defines a retention rule for a candle interval.
type Rule struct {
	Interval types.CandleInterval
	MaxAge   time.Duration // 0 means keep indefinitely
}

// Policy holds all retention rules.
type Policy struct {
	Rules []Rule
}

// DefaultPolicy returns the default data retention policy per the T035 spec:
// - 1m candles: 1 year
// - 5m/15m candles: 1 year
// - 1h candles: 2 years
// - 4h/1d candles: indefinite
func DefaultPolicy() *Policy {
	return &Policy{
		Rules: []Rule{
			{Interval: types.Interval1m, MaxAge: 365 * 24 * time.Hour},
			{Interval: types.Interval5m, MaxAge: 365 * 24 * time.Hour},
			{Interval: types.Interval15m, MaxAge: 365 * 24 * time.Hour},
			{Interval: types.Interval1h, MaxAge: 2 * 365 * 24 * time.Hour},
			{Interval: types.Interval4h, MaxAge: 0}, // indefinite
			{Interval: types.Interval1d, MaxAge: 0}, // indefinite
		},
	}
}

// Enforce deletes candles that exceed their retention period.
func (p *Policy) Enforce(cs *store.CandleStore) int {
	now := time.Now().UTC()
	total := 0
	for _, rule := range p.Rules {
		if rule.MaxAge == 0 {
			continue
		}
		cutoff := now.Add(-rule.MaxAge)
		total += cs.DeleteBefore(rule.Interval, cutoff)
	}
	return total
}
