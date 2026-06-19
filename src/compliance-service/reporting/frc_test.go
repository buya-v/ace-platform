package reporting

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

const testTenant = "mse-equities"

func newTestReporter(t *testing.T) (*Reporter, *RecordingPublisher) {
	t.Helper()
	pub := &RecordingPublisher{}
	r, err := NewReporter(testTenant, pub)
	if err != nil {
		t.Fatalf("NewReporter: %v", err)
	}
	return r, pub
}

func TestNewReporterRequiresTenant(t *testing.T) {
	if _, err := NewReporter("", nil); err != ErrMissingTenant {
		t.Fatalf("expected ErrMissingTenant, got %v", err)
	}
	r, err := NewReporter(testTenant, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.TenantID() != testTenant {
		t.Fatalf("TenantID = %q", r.TenantID())
	}
}

func TestDailyTradingSummaryTotalsAndMovers(t *testing.T) {
	r, _ := newTestReporter(t)
	instruments := []InstrumentVolume{
		{InstrumentID: "MSE:APU", Symbol: "APU", Trades: 10, Volume: 1000, Value: 5000, PriceChange: 1.5},
		{InstrumentID: "MSE:GOV", Symbol: "GOV", Trades: 5, Volume: 500, Value: 2500, PriceChange: -3.2},
		{InstrumentID: "MSE:TDB", Symbol: "TDB", Trades: 2, Volume: 200, Value: 800, PriceChange: 0.1},
	}
	rep, err := r.GenerateDailyTradingSummary("2026-06-19", instruments, FormatJSON)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	body := rep.Body.(DailyTradingSummary)
	if body.TotalTrades != 17 || body.TotalVolume != 1700 || body.TotalValue != 8300 {
		t.Fatalf("totals wrong: %+v", body)
	}
	// Top mover should be the largest absolute change (GOV at -3.2).
	if body.TopMovers[0].InstrumentID != "MSE:GOV" {
		t.Fatalf("top mover = %s, want MSE:GOV", body.TopMovers[0].InstrumentID)
	}
	if rep.Regulator != Regulator || rep.TenantID != testTenant {
		t.Fatalf("envelope metadata wrong: %+v", rep)
	}
}

func TestDailyTradingSummaryEmptyIsError(t *testing.T) {
	r, _ := newTestReporter(t)
	if _, err := r.GenerateDailyTradingSummary("2026-06-19", nil, FormatJSON); err != ErrNoData {
		t.Fatalf("expected ErrNoData, got %v", err)
	}
}

func TestRenderCSVOnlyForDailySummary(t *testing.T) {
	r, _ := newTestReporter(t)
	// CSV allowed for daily summary.
	rep, err := r.GenerateDailyTradingSummary("2026-06-19",
		[]InstrumentVolume{{InstrumentID: "MSE:APU", Symbol: "APU", Trades: 1, Volume: 10, Value: 50, PriceChange: 0.5}},
		FormatCSV)
	if err != nil {
		t.Fatalf("generate csv: %v", err)
	}
	out, err := rep.Render()
	if err != nil {
		t.Fatalf("render csv: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "instrument_id,symbol,trades,volume,value,price_change") {
		t.Fatalf("missing CSV header:\n%s", s)
	}
	if !strings.Contains(s, "MSE:APU,APU,1,10,50.00,0.5000") {
		t.Fatalf("missing data row:\n%s", s)
	}
	if rep.fileExt() != "csv" {
		t.Fatalf("fileExt = %s", rep.fileExt())
	}

	// CSV NOT allowed for a non-tabular report.
	if _, err := r.GenerateSuspiciousTradingAlert(SuspiciousTradingAlert{
		Pattern: "WASH_TRADING", ParticipantID: "p1",
	}); err != nil {
		t.Fatalf("alert generate: %v", err)
	}
	bad := &Report{TenantID: testTenant, Type: ReportSuspiciousTrading, Format: FormatCSV, Body: SuspiciousTradingAlert{}}
	if _, err := bad.Render(); err != ErrFormatNotAllowed {
		t.Fatalf("expected ErrFormatNotAllowed rendering non-tabular CSV, got %v", err)
	}
}

func TestLargeTraderPercentComputed(t *testing.T) {
	r, _ := newTestReporter(t)
	rep, err := r.GenerateLargeTraderReport(LargeTraderPosition{
		ParticipantID: "p1", InstrumentID: "MSE:APU",
		PositionSize: 150000, OutstandingShares: 1000000, ThresholdPercent: 10,
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	body := rep.Body.(LargeTraderPosition)
	if body.PercentOutstanding != 15 {
		t.Fatalf("percent = %v, want 15", body.PercentOutstanding)
	}
	if _, err := r.GenerateLargeTraderReport(LargeTraderPosition{InstrumentID: "x"}); err == nil {
		t.Fatal("expected error for missing participant_id")
	}
}

func TestSettlementFailsReportTotals(t *testing.T) {
	r, _ := newTestReporter(t)
	fails := []SettlementFail{
		{ObligationID: "o1", ParticipantID: "p1", InstrumentID: "i1", Quantity: 100, FailValue: 1000, PenaltyAmount: 12.34, BuyInStatus: "NONE"},
		{ObligationID: "o2", ParticipantID: "p2", InstrumentID: "i2", Quantity: 50, FailValue: 500, PenaltyAmount: 7.66, BuyInStatus: "INITIATED"},
	}
	rep, err := r.GenerateSettlementFailsReport("2026-06-19", fails)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	body := rep.Body.(SettlementFailsReport)
	if body.TotalFails != 2 || body.TotalPenalty != 20 {
		t.Fatalf("totals wrong: %+v", body)
	}
	// Empty fails is a valid clean report with a non-nil slice.
	clean, err := r.GenerateSettlementFailsReport("2026-06-19", nil)
	if err != nil {
		t.Fatalf("clean generate: %v", err)
	}
	cb := clean.Body.(SettlementFailsReport)
	if cb.TotalFails != 0 || cb.Fails == nil {
		t.Fatalf("clean report wrong: %+v", cb)
	}
}

func TestSuspiciousAndQuarterlyValidation(t *testing.T) {
	r, _ := newTestReporter(t)
	if _, err := r.GenerateSuspiciousTradingAlert(SuspiciousTradingAlert{Pattern: "SPOOFING"}); err == nil {
		t.Fatal("expected error: missing participant_id")
	}
	alert, err := r.GenerateSuspiciousTradingAlert(SuspiciousTradingAlert{Pattern: "LAYERING", ParticipantID: "p1"})
	if err != nil {
		t.Fatalf("alert: %v", err)
	}
	ab := alert.Body.(SuspiciousTradingAlert)
	if ab.Severity != "MEDIUM" || ab.DetectedAt == "" {
		t.Fatalf("defaults not applied: %+v", ab)
	}
	if _, err := r.GenerateQuarterlyComplianceReport(QuarterlyComplianceReport{}); err == nil {
		t.Fatal("expected error: missing quarter")
	}
	if _, err := r.GenerateQuarterlyComplianceReport(QuarterlyComplianceReport{Quarter: "2026-Q2", TotalParticipants: 42}); err != nil {
		t.Fatalf("quarterly: %v", err)
	}
}

func TestPublishComputesTargetsAndDelivers(t *testing.T) {
	r, pub := newTestReporter(t)
	rep, err := r.GenerateSettlementFailsReport("2026-06-19", nil)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	d, err := r.Publish(context.Background(), rep)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	wantTopic := "mse-equities.compliance.frc-report-generated"
	if d.KafkaTopic != wantTopic {
		t.Fatalf("topic = %s, want %s", d.KafkaTopic, wantTopic)
	}
	if !strings.HasPrefix(d.S3Key, "s3://garudax-mse-equities-reports/SETTLEMENT_FAILS_REPORT/") {
		t.Fatalf("s3 key = %s", d.S3Key)
	}
	if !strings.HasSuffix(d.S3Key, ".json") {
		t.Fatalf("expected .json key, got %s", d.S3Key)
	}
	if pub.Count() != 1 {
		t.Fatalf("publisher count = %d", pub.Count())
	}
	// Rendered body must be valid JSON carrying tenant_id and regulator.
	var env map[string]interface{}
	if err := json.Unmarshal(d.Body, &env); err != nil {
		t.Fatalf("body not valid JSON: %v", err)
	}
	if env["tenant_id"] != testTenant || env["regulator"] != Regulator {
		t.Fatalf("rendered envelope wrong: %v", env)
	}
}

func TestPublishRejectsMissingTenant(t *testing.T) {
	r, _ := newTestReporter(t)
	if _, err := r.Publish(context.Background(), &Report{Type: ReportSettlementFails, Format: FormatJSON}); err != ErrMissingTenant {
		t.Fatalf("expected ErrMissingTenant, got %v", err)
	}
	if _, err := r.Publish(context.Background(), nil); err != ErrNoData {
		t.Fatalf("expected ErrNoData for nil report, got %v", err)
	}
}

type failingPublisher struct{}

func (failingPublisher) Publish(context.Context, Delivery) error {
	return context.DeadlineExceeded
}

func TestPublishWrapsPublisherError(t *testing.T) {
	r, err := NewReporter(testTenant, failingPublisher{})
	if err != nil {
		t.Fatalf("NewReporter: %v", err)
	}
	rep, _ := r.GenerateSettlementFailsReport("2026-06-19", nil)
	if _, err := r.Publish(context.Background(), rep); err == nil {
		t.Fatal("expected publish error")
	}
}

func TestValidReportTypeAndFormatAllowed(t *testing.T) {
	if ValidReportType("NOPE") {
		t.Fatal("NOPE should be invalid")
	}
	if !ValidReportType(ReportDailyTradingSummary) {
		t.Fatal("daily summary should be valid")
	}
	if formatAllowed(ReportLargeTrader, FormatCSV) {
		t.Fatal("CSV not allowed for large trader")
	}
	if !formatAllowed(ReportDailyTradingSummary, FormatCSV) {
		t.Fatal("CSV allowed for daily summary")
	}
	// Invalid type / format paths through newReport.
	r, _ := newTestReporter(t)
	if _, err := r.newReport("BOGUS", FormatJSON, nil); err != ErrInvalidType {
		t.Fatalf("expected ErrInvalidType, got %v", err)
	}
	if _, err := r.newReport(ReportLargeTrader, FormatCSV, nil); err != ErrFormatNotAllowed {
		t.Fatalf("expected ErrFormatNotAllowed, got %v", err)
	}
}
