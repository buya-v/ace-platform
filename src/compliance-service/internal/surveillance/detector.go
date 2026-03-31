package surveillance

import (
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

var alertCounter uint64

func newAlertID() string {
	n := atomic.AddUint64(&alertCounter, 1)
	return fmt.Sprintf("surv-alert-%d", n)
}

// AlertSink receives generated alerts. Implementations may persist to a
// database, forward to Kafka, or simply collect in memory (for tests).
type AlertSink interface {
	EmitAlert(alert *Alert) error
}

// TradeDetector checks incoming trades against all enabled surveillance rules
// and emits alerts when patterns are detected.
type TradeDetector struct {
	mu    sync.RWMutex
	rules []Rule
	sink  AlertSink

	// Recent trade history keyed by participant ID, used for wash trading detection.
	recentTrades map[string][]Trade

	// Recent order cancellations keyed by participant ID, used for spoofing detection.
	recentCancels map[string][]OrderCancel
}

// NewTradeDetector creates a detector with the given rules and alert sink.
func NewTradeDetector(rules []Rule, sink AlertSink) *TradeDetector {
	return &TradeDetector{
		rules:         rules,
		sink:          sink,
		recentTrades:  make(map[string][]Trade),
		recentCancels: make(map[string][]OrderCancel),
	}
}

// ProcessTrade evaluates a trade against all enabled rules and emits alerts
// for any violations detected.
func (d *TradeDetector) ProcessTrade(trade Trade) []Alert {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Record the trade for both buyer and seller (deduplicate self-trades)
	d.recentTrades[trade.BuyerID] = append(d.recentTrades[trade.BuyerID], trade)
	if trade.SellerID != trade.BuyerID {
		d.recentTrades[trade.SellerID] = append(d.recentTrades[trade.SellerID], trade)
	}

	var alerts []Alert

	for _, rule := range d.rules {
		if !rule.Enabled {
			continue
		}

		var ruleAlerts []Alert

		switch rule.RuleType {
		case RuleWashTrading:
			ruleAlerts = d.checkWashTrading(trade, rule)
		case RuleSpoofing:
			ruleAlerts = d.checkSpoofing(trade, rule)
		case RuleConcentration:
			// Concentration checks require position snapshot data;
			// use CheckConcentration directly with position data.
			continue
		case RuleUnusualVolume:
			// Unusual volume checks require aggregated volume data;
			// use CheckUnusualVolume directly with volume snapshots.
			continue
		}

		for i := range ruleAlerts {
			if d.sink != nil {
				_ = d.sink.EmitAlert(&ruleAlerts[i])
			}
		}
		alerts = append(alerts, ruleAlerts...)
	}

	return alerts
}

// checkWashTrading detects when the same participant appears as both buyer and
// seller within a configurable time window, meeting a minimum trade count.
func (d *TradeDetector) checkWashTrading(trade Trade, rule Rule) []Alert {
	if trade.BuyerID != trade.SellerID {
		// Not a self-trade on this individual trade. Check history for
		// the same participant appearing on both sides within the window.
		return d.checkCrossTradePairWash(trade, rule)
	}

	// Direct self-trade: buyer == seller
	var params WashTradingParams
	if err := json.Unmarshal(rule.Params, &params); err != nil {
		return nil
	}

	window := time.Duration(params.TimeWindowSeconds) * time.Second
	participantTrades := d.recentTrades[trade.BuyerID]

	// Count trades by this participant within the window
	count := 0
	for _, t := range participantTrades {
		if trade.Timestamp.Sub(t.Timestamp) <= window {
			count++
		}
	}

	if count >= params.MinTrades {
		alert := Alert{
			ID:            newAlertID(),
			RuleID:        rule.ID,
			ParticipantID: trade.BuyerID,
			InstrumentID:  trade.InstrumentID,
			Severity:      SeverityHigh,
			Details: fmt.Sprintf(
				"Self-trade detected: participant %s is both buyer and seller on instrument %s. "+
					"%d trades within %d second window.",
				trade.BuyerID, trade.InstrumentID, count, params.TimeWindowSeconds),
			Status:     AlertStatusOpen,
			DetectedAt: time.Now().UTC(),
		}
		return []Alert{alert}
	}
	return nil
}

// checkCrossTradePairWash checks if a participant has been on both the buy side
// and sell side of trades in the same instrument within the time window.
func (d *TradeDetector) checkCrossTradePairWash(trade Trade, rule Rule) []Alert {
	var params WashTradingParams
	if err := json.Unmarshal(rule.Params, &params); err != nil {
		return nil
	}
	window := time.Duration(params.TimeWindowSeconds) * time.Second

	var alerts []Alert

	// Check buyer: was this buyer also a seller recently on the same instrument?
	alerts = append(alerts, d.detectCrossWash(trade.BuyerID, trade.InstrumentID, "buyer", trade.Timestamp, window, params.MinTrades, rule)...)

	// Check seller: was this seller also a buyer recently on the same instrument?
	alerts = append(alerts, d.detectCrossWash(trade.SellerID, trade.InstrumentID, "seller", trade.Timestamp, window, params.MinTrades, rule)...)

	return alerts
}

func (d *TradeDetector) detectCrossWash(participantID, instrumentID, currentSide string, now time.Time, window time.Duration, minTrades int, rule Rule) []Alert {
	trades := d.recentTrades[participantID]

	buyCount := 0
	sellCount := 0

	for _, t := range trades {
		if t.InstrumentID != instrumentID {
			continue
		}
		if now.Sub(t.Timestamp) > window {
			continue
		}
		if t.BuyerID == participantID {
			buyCount++
		}
		if t.SellerID == participantID {
			sellCount++
		}
	}

	if buyCount >= 1 && sellCount >= 1 && (buyCount+sellCount) >= minTrades {
		alert := Alert{
			ID:            newAlertID(),
			RuleID:        rule.ID,
			ParticipantID: participantID,
			InstrumentID:  instrumentID,
			Severity:      SeverityHigh,
			Details: fmt.Sprintf(
				"Potential wash trading: participant %s appeared as buyer %d times and seller %d times "+
					"on instrument %s within %s window.",
				participantID, buyCount, sellCount, instrumentID, window),
			Status:     AlertStatusOpen,
			DetectedAt: time.Now().UTC(),
		}
		return []Alert{alert}
	}
	return nil
}

// checkSpoofing detects when a participant places a large order and then
// cancels it shortly after. This requires order cancel events to have been
// recorded via RecordOrderCancel.
//
// NOTE: Full spoofing detection requires order submission + cancellation event
// streams. This implementation checks if any recent cancellation by the trade's
// participants happened within the spoofing window. The trade itself acts as
// the trigger to check for preceding cancel patterns.
func (d *TradeDetector) checkSpoofing(trade Trade, rule Rule) []Alert {
	var params SpoofingParams
	if err := json.Unmarshal(rule.Params, &params); err != nil {
		return nil
	}

	cancelWindow := time.Duration(params.CancelWindowSeconds) * time.Second
	var alerts []Alert

	// Check both buyer and seller
	for _, pid := range []string{trade.BuyerID, trade.SellerID} {
		cancels := d.recentCancels[pid]
		for _, cancel := range cancels {
			if cancel.InstrumentID != trade.InstrumentID {
				continue
			}
			// Was this cancel within the spoofing window?
			timeSinceCancel := trade.Timestamp.Sub(cancel.CancelledAt)
			if timeSinceCancel < 0 {
				timeSinceCancel = -timeSinceCancel
			}
			if timeSinceCancel > cancelWindow {
				continue
			}
			// Was the cancelled order large relative to the trade?
			if trade.Quantity > 0 {
				cancelPct := (cancel.Quantity / trade.Quantity) * 100
				if cancelPct >= params.MinOrderSizePct {
					alert := Alert{
						ID:            newAlertID(),
						RuleID:        rule.ID,
						ParticipantID: pid,
						InstrumentID:  trade.InstrumentID,
						Severity:      SeverityMedium,
						Details: fmt.Sprintf(
							"Potential spoofing: participant %s cancelled order %s (qty %.2f) "+
								"within %d seconds of trade on instrument %s. Cancel size %.1f%% of trade quantity.",
							pid, cancel.OrderID, cancel.Quantity,
							params.CancelWindowSeconds, trade.InstrumentID, cancelPct),
						Status:     AlertStatusOpen,
						DetectedAt: time.Now().UTC(),
					}
					alerts = append(alerts, alert)
				}
			}
		}
	}

	return alerts
}

// RecordOrderCancel records an order cancellation event for spoofing detection.
// Call this when order cancel events arrive from the matching engine.
func (d *TradeDetector) RecordOrderCancel(cancel OrderCancel) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.recentCancels[cancel.ParticipantID] = append(d.recentCancels[cancel.ParticipantID], cancel)
}

// CheckConcentration evaluates whether a participant's position exceeds the
// maximum allowed percentage of total market size for a given instrument.
func (d *TradeDetector) CheckConcentration(pos PositionSnapshot, rule Rule) *Alert {
	if !rule.Enabled {
		return nil
	}

	var params ConcentrationParams
	if err := json.Unmarshal(rule.Params, &params); err != nil {
		return nil
	}

	if pos.TotalMarketSize <= 0 {
		return nil
	}

	concentrationPct := (pos.PositionSize / pos.TotalMarketSize) * 100

	if concentrationPct > params.MaxPositionPct {
		severity := SeverityMedium
		if concentrationPct > params.MaxPositionPct*2 {
			severity = SeverityCritical
		} else if concentrationPct > params.MaxPositionPct*1.5 {
			severity = SeverityHigh
		}

		alert := &Alert{
			ID:            newAlertID(),
			RuleID:        rule.ID,
			ParticipantID: pos.ParticipantID,
			InstrumentID:  pos.InstrumentID,
			Severity:      severity,
			Details: fmt.Sprintf(
				"Position concentration alert: participant %s holds %.1f%% of instrument %s "+
					"(threshold: %.1f%%). Position: %.2f / Total: %.2f.",
				pos.ParticipantID, concentrationPct, pos.InstrumentID,
				params.MaxPositionPct, pos.PositionSize, pos.TotalMarketSize),
			Status:     AlertStatusOpen,
			DetectedAt: time.Now().UTC(),
		}

		if d.sink != nil {
			_ = d.sink.EmitAlert(alert)
		}

		return alert
	}

	return nil
}

// CheckUnusualVolume evaluates whether the current daily volume for an
// instrument is an unusual number of standard deviations above the rolling
// average. Returns alerts for every participant who traded if volume is unusual.
func (d *TradeDetector) CheckUnusualVolume(snap VolumeSnapshot, rule Rule, participantID string) *Alert {
	if !rule.Enabled {
		return nil
	}

	var params UnusualVolumeParams
	if err := json.Unmarshal(rule.Params, &params); err != nil {
		return nil
	}

	if snap.RollingStdDev <= 0 {
		return nil
	}

	zScore := (snap.DailyVolume - snap.RollingMean) / snap.RollingStdDev

	if zScore > params.StdDevThreshold {
		severity := SeverityLow
		if zScore > params.StdDevThreshold*2 {
			severity = SeverityHigh
		} else if zScore > params.StdDevThreshold*1.5 {
			severity = SeverityMedium
		}

		alert := &Alert{
			ID:            newAlertID(),
			RuleID:        rule.ID,
			ParticipantID: participantID,
			InstrumentID:  snap.InstrumentID,
			Severity:      severity,
			Details: fmt.Sprintf(
				"Unusual volume detected on instrument %s: daily volume %.2f is %.1f standard "+
					"deviations above the %d-day rolling average (mean: %.2f, stddev: %.2f, threshold: %.1f).",
				snap.InstrumentID, snap.DailyVolume, zScore,
				params.LookbackDays, snap.RollingMean, snap.RollingStdDev, params.StdDevThreshold),
			Status:     AlertStatusOpen,
			DetectedAt: time.Now().UTC(),
		}

		if d.sink != nil {
			_ = d.sink.EmitAlert(alert)
		}

		return alert
	}

	return nil
}

// PurgeOldData removes trade and cancel records older than maxAge.
// Call periodically to prevent unbounded memory growth.
func (d *TradeDetector) PurgeOldData(maxAge time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)

	for pid, trades := range d.recentTrades {
		var kept []Trade
		for _, t := range trades {
			if t.Timestamp.After(cutoff) {
				kept = append(kept, t)
			}
		}
		if len(kept) == 0 {
			delete(d.recentTrades, pid)
		} else {
			d.recentTrades[pid] = kept
		}
	}

	for pid, cancels := range d.recentCancels {
		var kept []OrderCancel
		for _, c := range cancels {
			if c.CancelledAt.After(cutoff) {
				kept = append(kept, c)
			}
		}
		if len(kept) == 0 {
			delete(d.recentCancels, pid)
		} else {
			d.recentCancels[pid] = kept
		}
	}
}

// EnabledRules returns the list of enabled rules.
func (d *TradeDetector) EnabledRules() []Rule {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var enabled []Rule
	for _, r := range d.rules {
		if r.Enabled {
			enabled = append(enabled, r)
		}
	}
	return enabled
}

// AlertCount returns the total number of alerts generated (for testing).
func (d *TradeDetector) AlertCount() int {
	if ms, ok := d.sink.(*MemoryAlertSink); ok {
		return len(ms.Alerts())
	}
	return -1
}

// ---- MemoryAlertSink for testing ----

// MemoryAlertSink collects alerts in memory. Useful for tests.
type MemoryAlertSink struct {
	mu     sync.Mutex
	alerts []Alert
}

// NewMemoryAlertSink creates a new in-memory alert sink.
func NewMemoryAlertSink() *MemoryAlertSink {
	return &MemoryAlertSink{}
}

// EmitAlert stores the alert.
func (s *MemoryAlertSink) EmitAlert(alert *Alert) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.alerts = append(s.alerts, *alert)
	return nil
}

// Alerts returns all collected alerts.
func (s *MemoryAlertSink) Alerts() []Alert {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]Alert, len(s.alerts))
	copy(cp, s.alerts)
	return cp
}

// Reset clears all collected alerts.
func (s *MemoryAlertSink) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.alerts = nil
}

// ---- Helper: default rules matching the V20 migration seed data ----

// DefaultRules returns the default surveillance rules matching the V20 migration.
func DefaultRules() []Rule {
	return []Rule{
		{
			ID:       "wash-1",
			RuleType: RuleWashTrading,
			Params:   json.RawMessage(`{"time_window_seconds": 60, "min_trades": 2}`),
			Enabled:  true,
		},
		{
			ID:       "spoof-1",
			RuleType: RuleSpoofing,
			Params:   json.RawMessage(`{"cancel_window_seconds": 5, "min_order_size_pct": 5.0}`),
			Enabled:  true,
		},
		{
			ID:       "concentration-1",
			RuleType: RuleConcentration,
			Params:   json.RawMessage(`{"max_position_pct": 25.0}`),
			Enabled:  true,
		},
		{
			ID:       "unusual-vol-1",
			RuleType: RuleUnusualVolume,
			Params:   json.RawMessage(`{"std_dev_threshold": 3.0, "lookback_days": 30}`),
			Enabled:  true,
		},
	}
}

// ---- Helpers for z-score calculation (exported for testing) ----

// ZScore computes how many standard deviations value is above mean.
func ZScore(value, mean, stddev float64) float64 {
	if stddev <= 0 {
		return 0
	}
	return (value - mean) / stddev
}

// StdDev computes the population standard deviation of a slice of float64.
func StdDev(values []float64) float64 {
	n := float64(len(values))
	if n == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean := sum / n
	var variance float64
	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}
	return math.Sqrt(variance / n)
}

// Mean computes the arithmetic mean of a slice of float64.
func Mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}
