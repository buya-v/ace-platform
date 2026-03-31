package surveillance

import (
	"encoding/json"
	"testing"
	"time"
)

// ---- helpers ----

func makeWashRule(windowSec, minTrades int) Rule {
	params, _ := json.Marshal(WashTradingParams{
		TimeWindowSeconds: windowSec,
		MinTrades:         minTrades,
	})
	return Rule{ID: "wash-test", RuleType: RuleWashTrading, Params: params, Enabled: true}
}

func makeSpoofRule(cancelWindowSec int, minOrderSizePct float64) Rule {
	params, _ := json.Marshal(SpoofingParams{
		CancelWindowSeconds: cancelWindowSec,
		MinOrderSizePct:     minOrderSizePct,
	})
	return Rule{ID: "spoof-test", RuleType: RuleSpoofing, Params: params, Enabled: true}
}

func makeConcentrationRule(maxPct float64) Rule {
	params, _ := json.Marshal(ConcentrationParams{MaxPositionPct: maxPct})
	return Rule{ID: "conc-test", RuleType: RuleConcentration, Params: params, Enabled: true}
}

func makeVolumeRule(stdDevThreshold float64, lookbackDays int) Rule {
	params, _ := json.Marshal(UnusualVolumeParams{
		StdDevThreshold: stdDevThreshold,
		LookbackDays:    lookbackDays,
	})
	return Rule{ID: "vol-test", RuleType: RuleUnusualVolume, Params: params, Enabled: true}
}

func newTestDetector(rules []Rule) (*TradeDetector, *MemoryAlertSink) {
	sink := NewMemoryAlertSink()
	d := NewTradeDetector(rules, sink)
	return d, sink
}

// ---- ValidRuleType ----

func TestValidRuleType(t *testing.T) {
	for _, rt := range []RuleType{RuleWashTrading, RuleSpoofing, RulePriceManipulation, RuleConcentration, RuleUnusualVolume} {
		if !ValidRuleType(rt) {
			t.Errorf("expected %s to be valid", rt)
		}
	}
	if ValidRuleType("invalid") {
		t.Error("expected 'invalid' to be invalid")
	}
}

// ---- Wash Trading Tests ----

func TestWashTrading_SelfTrade(t *testing.T) {
	d, sink := newTestDetector([]Rule{makeWashRule(60, 2)})

	now := time.Now()

	// First self-trade
	d.ProcessTrade(Trade{
		TradeID:      "t1",
		InstrumentID: "GOLD-USD",
		BuyerID:      "participant-A",
		SellerID:     "participant-A",
		Price:        1900.0,
		Quantity:     10,
		Timestamp:    now,
	})

	// Second self-trade within window - should trigger alert (min_trades=2)
	alerts := d.ProcessTrade(Trade{
		TradeID:      "t2",
		InstrumentID: "GOLD-USD",
		BuyerID:      "participant-A",
		SellerID:     "participant-A",
		Price:        1901.0,
		Quantity:     5,
		Timestamp:    now.Add(10 * time.Second),
	})

	if len(alerts) == 0 {
		t.Fatal("expected wash trading alert for self-trade")
	}
	if alerts[0].Severity != SeverityHigh {
		t.Errorf("expected HIGH severity, got %s", alerts[0].Severity)
	}
	if alerts[0].ParticipantID != "participant-A" {
		t.Errorf("expected participant-A, got %s", alerts[0].ParticipantID)
	}
	if alerts[0].InstrumentID != "GOLD-USD" {
		t.Errorf("expected GOLD-USD, got %s", alerts[0].InstrumentID)
	}
	if alerts[0].RuleID != "wash-test" {
		t.Errorf("expected wash-test rule, got %s", alerts[0].RuleID)
	}

	// Verify alert was emitted to sink
	sinkAlerts := sink.Alerts()
	if len(sinkAlerts) == 0 {
		t.Error("expected alert in sink")
	}
}

func TestWashTrading_CrossTrade(t *testing.T) {
	d, _ := newTestDetector([]Rule{makeWashRule(60, 2)})

	now := time.Now()

	// participant-B buys
	d.ProcessTrade(Trade{
		TradeID:      "t1",
		InstrumentID: "WHEAT-USD",
		BuyerID:      "participant-B",
		SellerID:     "participant-C",
		Price:        500.0,
		Quantity:     100,
		Timestamp:    now,
	})

	// participant-B sells same instrument within window
	alerts := d.ProcessTrade(Trade{
		TradeID:      "t2",
		InstrumentID: "WHEAT-USD",
		BuyerID:      "participant-C",
		SellerID:     "participant-B",
		Price:        501.0,
		Quantity:     100,
		Timestamp:    now.Add(30 * time.Second),
	})

	if len(alerts) == 0 {
		t.Fatal("expected wash trading alert for cross-trade pattern")
	}
	found := false
	for _, a := range alerts {
		if a.ParticipantID == "participant-B" {
			found = true
		}
	}
	if !found {
		t.Error("expected alert for participant-B")
	}
}

func TestWashTrading_NoAlertForDifferentParticipants(t *testing.T) {
	d, _ := newTestDetector([]Rule{makeWashRule(60, 2)})

	now := time.Now()

	// Two different participants, different sides - no wash trading
	d.ProcessTrade(Trade{
		TradeID:      "t1",
		InstrumentID: "GOLD-USD",
		BuyerID:      "participant-X",
		SellerID:     "participant-Y",
		Price:        1900.0,
		Quantity:     10,
		Timestamp:    now,
	})

	alerts := d.ProcessTrade(Trade{
		TradeID:      "t2",
		InstrumentID: "GOLD-USD",
		BuyerID:      "participant-Z",
		SellerID:     "participant-W",
		Price:        1901.0,
		Quantity:     5,
		Timestamp:    now.Add(10 * time.Second),
	})

	if len(alerts) > 0 {
		t.Error("did not expect alerts for trades between unrelated participants")
	}
}

func TestWashTrading_OutsideWindow(t *testing.T) {
	d, _ := newTestDetector([]Rule{makeWashRule(60, 2)})

	now := time.Now()

	d.ProcessTrade(Trade{
		TradeID:      "t1",
		InstrumentID: "GOLD-USD",
		BuyerID:      "participant-A",
		SellerID:     "participant-A",
		Price:        1900.0,
		Quantity:     10,
		Timestamp:    now,
	})

	// Second trade outside the 60-second window
	alerts := d.ProcessTrade(Trade{
		TradeID:      "t2",
		InstrumentID: "GOLD-USD",
		BuyerID:      "participant-A",
		SellerID:     "participant-A",
		Price:        1901.0,
		Quantity:     5,
		Timestamp:    now.Add(120 * time.Second),
	})

	if len(alerts) > 0 {
		t.Error("did not expect alert for trades outside the time window")
	}
}

func TestWashTrading_DisabledRule(t *testing.T) {
	rule := makeWashRule(60, 2)
	rule.Enabled = false

	d, _ := newTestDetector([]Rule{rule})

	now := time.Now()
	d.ProcessTrade(Trade{
		TradeID: "t1", InstrumentID: "GOLD-USD",
		BuyerID: "A", SellerID: "A",
		Price: 1900, Quantity: 10, Timestamp: now,
	})
	alerts := d.ProcessTrade(Trade{
		TradeID: "t2", InstrumentID: "GOLD-USD",
		BuyerID: "A", SellerID: "A",
		Price: 1901, Quantity: 5, Timestamp: now.Add(5 * time.Second),
	})

	if len(alerts) > 0 {
		t.Error("disabled rule should not generate alerts")
	}
}

// ---- Spoofing Tests ----

func TestSpoofing_DetectsLargeCancelBeforeTrade(t *testing.T) {
	d, sink := newTestDetector([]Rule{makeSpoofRule(5, 5.0)})

	now := time.Now()

	// Record a large order cancellation
	d.RecordOrderCancel(OrderCancel{
		OrderID:       "order-1",
		InstrumentID:  "GOLD-USD",
		ParticipantID: "spoofer",
		Side:          "BUY",
		Price:         1900.0,
		Quantity:      1000,
		SubmittedAt:   now.Add(-3 * time.Second),
		CancelledAt:   now.Add(-1 * time.Second),
	})

	// Trade happens shortly after cancellation
	alerts := d.ProcessTrade(Trade{
		TradeID:      "t1",
		InstrumentID: "GOLD-USD",
		BuyerID:      "spoofer",
		SellerID:     "victim",
		Price:        1899.0,
		Quantity:     10,
		Timestamp:    now,
	})

	if len(alerts) == 0 {
		t.Fatal("expected spoofing alert")
	}
	if alerts[0].Severity != SeverityMedium {
		t.Errorf("expected MEDIUM severity, got %s", alerts[0].Severity)
	}
	if alerts[0].RuleID != "spoof-test" {
		t.Errorf("expected spoof-test rule, got %s", alerts[0].RuleID)
	}

	sinkAlerts := sink.Alerts()
	if len(sinkAlerts) == 0 {
		t.Error("expected alert in sink")
	}
}

func TestSpoofing_NoCancelNoAlert(t *testing.T) {
	d, _ := newTestDetector([]Rule{makeSpoofRule(5, 5.0)})

	alerts := d.ProcessTrade(Trade{
		TradeID:      "t1",
		InstrumentID: "GOLD-USD",
		BuyerID:      "normal-buyer",
		SellerID:     "normal-seller",
		Price:        1900.0,
		Quantity:     10,
		Timestamp:    time.Now(),
	})

	if len(alerts) > 0 {
		t.Error("no spoofing alert expected when there are no cancellations")
	}
}

func TestSpoofing_SmallCancelNoAlert(t *testing.T) {
	d, _ := newTestDetector([]Rule{makeSpoofRule(5, 50.0)}) // 50% threshold

	now := time.Now()

	// Record a small cancellation (quantity = 1, trade quantity = 100)
	d.RecordOrderCancel(OrderCancel{
		OrderID:       "order-1",
		InstrumentID:  "GOLD-USD",
		ParticipantID: "trader",
		Quantity:      1,
		CancelledAt:   now.Add(-1 * time.Second),
	})

	alerts := d.ProcessTrade(Trade{
		TradeID:      "t1",
		InstrumentID: "GOLD-USD",
		BuyerID:      "trader",
		SellerID:     "other",
		Quantity:     100,
		Timestamp:    now,
	})

	if len(alerts) > 0 {
		t.Error("small cancel should not trigger spoofing alert")
	}
}

func TestSpoofing_CancelOutsideWindow(t *testing.T) {
	d, _ := newTestDetector([]Rule{makeSpoofRule(5, 5.0)})

	now := time.Now()

	// Cancel happened 30 seconds ago, outside the 5-second window
	d.RecordOrderCancel(OrderCancel{
		OrderID:       "order-1",
		InstrumentID:  "GOLD-USD",
		ParticipantID: "trader",
		Quantity:      1000,
		CancelledAt:   now.Add(-30 * time.Second),
	})

	alerts := d.ProcessTrade(Trade{
		TradeID:      "t1",
		InstrumentID: "GOLD-USD",
		BuyerID:      "trader",
		SellerID:     "other",
		Quantity:     10,
		Timestamp:    now,
	})

	if len(alerts) > 0 {
		t.Error("cancel outside window should not trigger spoofing alert")
	}
}

// ---- Concentration Tests ----

func TestConcentration_ExceedsThreshold(t *testing.T) {
	d, sink := newTestDetector([]Rule{makeConcentrationRule(25.0)})

	rule := makeConcentrationRule(25.0)
	alert := d.CheckConcentration(PositionSnapshot{
		ParticipantID:   "big-player",
		InstrumentID:    "GOLD-USD",
		PositionSize:    3000,
		TotalMarketSize: 10000,
	}, rule)

	if alert == nil {
		t.Fatal("expected concentration alert for 30% position")
	}
	if alert.Severity != SeverityMedium {
		t.Errorf("expected MEDIUM severity for 30%% (threshold 25%%), got %s", alert.Severity)
	}
	if alert.ParticipantID != "big-player" {
		t.Errorf("expected big-player, got %s", alert.ParticipantID)
	}

	sinkAlerts := sink.Alerts()
	if len(sinkAlerts) == 0 {
		t.Error("expected alert in sink")
	}
}

func TestConcentration_HighSeverity(t *testing.T) {
	d, _ := newTestDetector(nil)

	rule := makeConcentrationRule(25.0)
	// 40% > 25% * 1.5 = 37.5%, so HIGH severity
	alert := d.CheckConcentration(PositionSnapshot{
		ParticipantID:   "big-player",
		InstrumentID:    "GOLD-USD",
		PositionSize:    4000,
		TotalMarketSize: 10000,
	}, rule)

	if alert == nil {
		t.Fatal("expected alert")
	}
	if alert.Severity != SeverityHigh {
		t.Errorf("expected HIGH severity for 40%% position, got %s", alert.Severity)
	}
}

func TestConcentration_CriticalSeverity(t *testing.T) {
	d, _ := newTestDetector(nil)

	rule := makeConcentrationRule(25.0)
	// 55% > 25% * 2 = 50%, so CRITICAL severity
	alert := d.CheckConcentration(PositionSnapshot{
		ParticipantID:   "dominant",
		InstrumentID:    "GOLD-USD",
		PositionSize:    5500,
		TotalMarketSize: 10000,
	}, rule)

	if alert == nil {
		t.Fatal("expected alert")
	}
	if alert.Severity != SeverityCritical {
		t.Errorf("expected CRITICAL severity for 55%% position, got %s", alert.Severity)
	}
}

func TestConcentration_BelowThreshold(t *testing.T) {
	d, _ := newTestDetector(nil)

	rule := makeConcentrationRule(25.0)
	alert := d.CheckConcentration(PositionSnapshot{
		ParticipantID:   "small-player",
		InstrumentID:    "GOLD-USD",
		PositionSize:    1000,
		TotalMarketSize: 10000,
	}, rule)

	if alert != nil {
		t.Error("no alert expected for 10% position (threshold 25%)")
	}
}

func TestConcentration_ZeroMarketSize(t *testing.T) {
	d, _ := newTestDetector(nil)

	rule := makeConcentrationRule(25.0)
	alert := d.CheckConcentration(PositionSnapshot{
		ParticipantID:   "player",
		InstrumentID:    "GOLD-USD",
		PositionSize:    1000,
		TotalMarketSize: 0,
	}, rule)

	if alert != nil {
		t.Error("no alert expected when total market size is zero")
	}
}

func TestConcentration_DisabledRule(t *testing.T) {
	d, _ := newTestDetector(nil)

	rule := makeConcentrationRule(25.0)
	rule.Enabled = false

	alert := d.CheckConcentration(PositionSnapshot{
		ParticipantID:   "player",
		InstrumentID:    "GOLD-USD",
		PositionSize:    5000,
		TotalMarketSize: 10000,
	}, rule)

	if alert != nil {
		t.Error("disabled rule should not generate alert")
	}
}

// ---- Unusual Volume Tests ----

func TestUnusualVolume_ExceedsThreshold(t *testing.T) {
	d, sink := newTestDetector(nil)

	rule := makeVolumeRule(3.0, 30)
	alert := d.CheckUnusualVolume(VolumeSnapshot{
		InstrumentID:  "WHEAT-USD",
		DailyVolume:   10000,
		RollingMean:   2000,
		RollingStdDev: 500,
	}, rule, "active-trader")

	// z-score = (10000 - 2000) / 500 = 16.0, well above 3.0
	if alert == nil {
		t.Fatal("expected unusual volume alert")
	}
	if alert.InstrumentID != "WHEAT-USD" {
		t.Errorf("expected WHEAT-USD, got %s", alert.InstrumentID)
	}
	if alert.ParticipantID != "active-trader" {
		t.Errorf("expected active-trader, got %s", alert.ParticipantID)
	}

	sinkAlerts := sink.Alerts()
	if len(sinkAlerts) == 0 {
		t.Error("expected alert in sink")
	}
}

func TestUnusualVolume_BelowThreshold(t *testing.T) {
	d, _ := newTestDetector(nil)

	rule := makeVolumeRule(3.0, 30)
	alert := d.CheckUnusualVolume(VolumeSnapshot{
		InstrumentID:  "WHEAT-USD",
		DailyVolume:   2500,
		RollingMean:   2000,
		RollingStdDev: 500,
	}, rule, "normal-trader")

	// z-score = (2500 - 2000) / 500 = 1.0, below 3.0
	if alert != nil {
		t.Error("no alert expected for z-score below threshold")
	}
}

func TestUnusualVolume_ZeroStdDev(t *testing.T) {
	d, _ := newTestDetector(nil)

	rule := makeVolumeRule(3.0, 30)
	alert := d.CheckUnusualVolume(VolumeSnapshot{
		InstrumentID:  "WHEAT-USD",
		DailyVolume:   10000,
		RollingMean:   2000,
		RollingStdDev: 0,
	}, rule, "trader")

	if alert != nil {
		t.Error("no alert expected when stddev is zero")
	}
}

func TestUnusualVolume_SeverityLevels(t *testing.T) {
	d, _ := newTestDetector(nil)

	rule := makeVolumeRule(3.0, 30)

	// z-score = 3.5 -> LOW severity (above threshold but below 1.5x)
	alert := d.CheckUnusualVolume(VolumeSnapshot{
		InstrumentID:  "X",
		DailyVolume:   3750,
		RollingMean:   2000,
		RollingStdDev: 500,
	}, rule, "p")
	if alert == nil {
		t.Fatal("expected alert")
	}
	if alert.Severity != SeverityLow {
		t.Errorf("expected LOW severity for z=3.5, got %s", alert.Severity)
	}

	// z-score = 5.0 -> MEDIUM severity (above 1.5x threshold = 4.5)
	alert = d.CheckUnusualVolume(VolumeSnapshot{
		InstrumentID:  "X",
		DailyVolume:   4500,
		RollingMean:   2000,
		RollingStdDev: 500,
	}, rule, "p")
	if alert == nil {
		t.Fatal("expected alert")
	}
	if alert.Severity != SeverityMedium {
		t.Errorf("expected MEDIUM severity for z=5.0, got %s", alert.Severity)
	}

	// z-score = 7.0 -> HIGH severity (above 2x threshold = 6.0)
	alert = d.CheckUnusualVolume(VolumeSnapshot{
		InstrumentID:  "X",
		DailyVolume:   5500,
		RollingMean:   2000,
		RollingStdDev: 500,
	}, rule, "p")
	if alert == nil {
		t.Fatal("expected alert")
	}
	if alert.Severity != SeverityHigh {
		t.Errorf("expected HIGH severity for z=7.0, got %s", alert.Severity)
	}
}

func TestUnusualVolume_DisabledRule(t *testing.T) {
	d, _ := newTestDetector(nil)

	rule := makeVolumeRule(3.0, 30)
	rule.Enabled = false

	alert := d.CheckUnusualVolume(VolumeSnapshot{
		InstrumentID:  "X",
		DailyVolume:   10000,
		RollingMean:   2000,
		RollingStdDev: 500,
	}, rule, "p")

	if alert != nil {
		t.Error("disabled rule should not generate alert")
	}
}

// ---- PurgeOldData ----

func TestPurgeOldData(t *testing.T) {
	d, _ := newTestDetector([]Rule{makeWashRule(60, 2)})

	old := time.Now().Add(-2 * time.Hour)
	recent := time.Now().Add(-10 * time.Second)

	d.ProcessTrade(Trade{
		TradeID: "old", InstrumentID: "GOLD", BuyerID: "A", SellerID: "B",
		Quantity: 10, Timestamp: old,
	})
	d.ProcessTrade(Trade{
		TradeID: "recent", InstrumentID: "GOLD", BuyerID: "A", SellerID: "B",
		Quantity: 10, Timestamp: recent,
	})
	d.RecordOrderCancel(OrderCancel{
		OrderID: "old-cancel", InstrumentID: "GOLD", ParticipantID: "A",
		Quantity: 100, CancelledAt: old,
	})

	d.PurgeOldData(1 * time.Hour)

	d.mu.RLock()
	defer d.mu.RUnlock()

	// Old trade should be purged, recent should remain
	for _, trades := range d.recentTrades {
		for _, tr := range trades {
			if tr.TradeID == "old" {
				t.Error("old trade should have been purged")
			}
		}
	}

	// Old cancel should be purged
	for _, cancels := range d.recentCancels {
		for _, c := range cancels {
			if c.OrderID == "old-cancel" {
				t.Error("old cancel should have been purged")
			}
		}
	}
}

// ---- DefaultRules ----

func TestDefaultRules(t *testing.T) {
	rules := DefaultRules()
	if len(rules) != 4 {
		t.Fatalf("expected 4 default rules, got %d", len(rules))
	}

	expectedTypes := map[RuleType]bool{
		RuleWashTrading:   false,
		RuleSpoofing:      false,
		RuleConcentration: false,
		RuleUnusualVolume: false,
	}
	for _, r := range rules {
		if !r.Enabled {
			t.Errorf("default rule %s should be enabled", r.ID)
		}
		if _, ok := expectedTypes[r.RuleType]; !ok {
			t.Errorf("unexpected rule type %s", r.RuleType)
		}
		expectedTypes[r.RuleType] = true
	}
	for rt, found := range expectedTypes {
		if !found {
			t.Errorf("missing default rule for type %s", rt)
		}
	}
}

// ---- EnabledRules ----

func TestEnabledRules(t *testing.T) {
	washRule := makeWashRule(60, 2)
	disabledRule := makeSpoofRule(5, 5.0)
	disabledRule.Enabled = false

	d, _ := newTestDetector([]Rule{washRule, disabledRule})

	enabled := d.EnabledRules()
	if len(enabled) != 1 {
		t.Fatalf("expected 1 enabled rule, got %d", len(enabled))
	}
	if enabled[0].RuleType != RuleWashTrading {
		t.Errorf("expected wash_trading rule, got %s", enabled[0].RuleType)
	}
}

// ---- MemoryAlertSink ----

func TestMemoryAlertSink_Reset(t *testing.T) {
	sink := NewMemoryAlertSink()
	sink.EmitAlert(&Alert{ID: "a1", Severity: SeverityLow})
	sink.EmitAlert(&Alert{ID: "a2", Severity: SeverityHigh})

	if len(sink.Alerts()) != 2 {
		t.Fatalf("expected 2 alerts, got %d", len(sink.Alerts()))
	}

	sink.Reset()
	if len(sink.Alerts()) != 0 {
		t.Errorf("expected 0 alerts after reset, got %d", len(sink.Alerts()))
	}
}

// ---- Math helpers ----

func TestZScore(t *testing.T) {
	z := ZScore(10, 5, 2.5)
	if z != 2.0 {
		t.Errorf("expected z-score 2.0, got %f", z)
	}

	z = ZScore(10, 5, 0)
	if z != 0 {
		t.Errorf("expected 0 for zero stddev, got %f", z)
	}
}

func TestStdDev(t *testing.T) {
	values := []float64{2, 4, 4, 4, 5, 5, 7, 9}
	sd := StdDev(values)
	// Expected: ~2.0
	if sd < 1.9 || sd > 2.1 {
		t.Errorf("expected stddev ~2.0, got %f", sd)
	}

	sd = StdDev(nil)
	if sd != 0 {
		t.Errorf("expected 0 for empty slice, got %f", sd)
	}
}

func TestMean(t *testing.T) {
	m := Mean([]float64{10, 20, 30})
	if m != 20 {
		t.Errorf("expected mean 20, got %f", m)
	}

	m = Mean(nil)
	if m != 0 {
		t.Errorf("expected 0 for empty slice, got %f", m)
	}
}

// ---- AlertCount ----

func TestAlertCount(t *testing.T) {
	d, _ := newTestDetector([]Rule{makeWashRule(60, 2)})

	now := time.Now()
	d.ProcessTrade(Trade{
		TradeID: "t1", InstrumentID: "GOLD", BuyerID: "A", SellerID: "A",
		Quantity: 10, Timestamp: now,
	})
	d.ProcessTrade(Trade{
		TradeID: "t2", InstrumentID: "GOLD", BuyerID: "A", SellerID: "A",
		Quantity: 10, Timestamp: now.Add(5 * time.Second),
	})

	count := d.AlertCount()
	if count < 1 {
		t.Errorf("expected at least 1 alert, got %d", count)
	}
}

// ---- Integration: multiple rules at once ----

func TestMultipleRulesProcessTrade(t *testing.T) {
	rules := []Rule{
		makeWashRule(60, 2),
		makeSpoofRule(5, 5.0),
	}
	d, sink := newTestDetector(rules)

	now := time.Now()

	// Record a cancel for spoofing detection
	d.RecordOrderCancel(OrderCancel{
		OrderID: "o1", InstrumentID: "GOLD", ParticipantID: "A",
		Quantity: 500, CancelledAt: now.Add(-2 * time.Second),
	})

	// Self-trade that also matches spoofing
	d.ProcessTrade(Trade{
		TradeID: "t1", InstrumentID: "GOLD", BuyerID: "A", SellerID: "A",
		Quantity: 10, Price: 1900, Timestamp: now.Add(-1 * time.Second),
	})
	alerts := d.ProcessTrade(Trade{
		TradeID: "t2", InstrumentID: "GOLD", BuyerID: "A", SellerID: "A",
		Quantity: 10, Price: 1900, Timestamp: now,
	})

	// Should have alerts from both wash trading and spoofing rules
	hasWash := false
	hasSpoof := false
	for _, a := range alerts {
		if a.RuleID == "wash-test" {
			hasWash = true
		}
		if a.RuleID == "spoof-test" {
			hasSpoof = true
		}
	}
	if !hasWash {
		t.Error("expected wash trading alert")
	}
	if !hasSpoof {
		t.Error("expected spoofing alert")
	}

	// All alerts should be in the sink too
	if len(sink.Alerts()) < 2 {
		t.Errorf("expected at least 2 alerts in sink, got %d", len(sink.Alerts()))
	}
}

// ---- Edge case: bad JSON params ----

func TestBadRuleParams(t *testing.T) {
	badRule := Rule{
		ID:       "bad",
		RuleType: RuleWashTrading,
		Params:   json.RawMessage(`{invalid json`),
		Enabled:  true,
	}
	d, _ := newTestDetector([]Rule{badRule})

	now := time.Now()
	d.ProcessTrade(Trade{
		TradeID: "t1", InstrumentID: "GOLD", BuyerID: "A", SellerID: "A",
		Quantity: 10, Timestamp: now,
	})
	alerts := d.ProcessTrade(Trade{
		TradeID: "t2", InstrumentID: "GOLD", BuyerID: "A", SellerID: "A",
		Quantity: 10, Timestamp: now.Add(5 * time.Second),
	})

	// Bad params should not cause panic, just no alerts
	if len(alerts) > 0 {
		t.Error("bad rule params should not generate alerts")
	}
}

func TestConcentration_BadParams(t *testing.T) {
	d, _ := newTestDetector(nil)
	badRule := Rule{
		ID: "bad", RuleType: RuleConcentration,
		Params: json.RawMessage(`{invalid`), Enabled: true,
	}
	alert := d.CheckConcentration(PositionSnapshot{
		ParticipantID: "p", PositionSize: 5000, TotalMarketSize: 10000,
	}, badRule)
	if alert != nil {
		t.Error("bad params should not generate alert")
	}
}

func TestUnusualVolume_BadParams(t *testing.T) {
	d, _ := newTestDetector(nil)
	badRule := Rule{
		ID: "bad", RuleType: RuleUnusualVolume,
		Params: json.RawMessage(`{invalid`), Enabled: true,
	}
	alert := d.CheckUnusualVolume(VolumeSnapshot{
		DailyVolume: 10000, RollingMean: 2000, RollingStdDev: 500,
	}, badRule, "p")
	if alert != nil {
		t.Error("bad params should not generate alert")
	}
}
