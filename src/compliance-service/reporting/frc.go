// Package reporting builds the external regulatory reporting pipeline that
// sends formatted data to the Financial Regulatory Commission (FRC) of Mongolia.
//
// The FRC requires a fixed set of reports from the mse-equities flagship venue
// (see docs/platform-architecture.md §10.5). This package turns platform data
// into FRC-formatted reports and publishes each one to its delivery targets:
//
//	Kafka topic: <tenant>.compliance.frc-report-generated
//	S3 object:   s3://garudax-<tenant>-reports/<report_type>/<date>/<report_id>.<ext>
//
// Per the GarudaX multi-tenant directive, tenant ID is a first-class,
// non-optional input — every report must carry a non-empty TenantID. FRC is the
// regulator for the mse-equities venue; the reporting surface is tenant-aware so
// other venues (e.g. ace-commodities → MCGA) can reuse the same pipeline shape
// with a different regulator and report set.
//
// The package is zero-dependency (standard library only), matching the
// platform's winning "zero-dep Go module per service" pattern.
package reporting

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/garudax-platform/decimal"
)

// Regulator is the regulatory body these reports are addressed to.
const Regulator = "FRC"

// ReportType identifies one of the FRC-mandated report kinds.
type ReportType string

const (
	// ReportDailyTradingSummary — volume, value, trades by instrument, top movers. Daily 14:00.
	ReportDailyTradingSummary ReportType = "DAILY_TRADING_SUMMARY"
	// ReportLargeTrader — participant position size and % of outstanding. On threshold breach.
	ReportLargeTrader ReportType = "LARGE_TRADER_REPORT"
	// ReportSettlementFails — failed obligations, penalties, buy-in status. Daily 17:00.
	ReportSettlementFails ReportType = "SETTLEMENT_FAILS_REPORT"
	// ReportSuspiciousTrading — surveillance patterns (wash trading, spoofing, layering). Real-time.
	ReportSuspiciousTrading ReportType = "SUSPICIOUS_TRADING_ALERT"
	// ReportQuarterlyCompliance — KYC status, participant count, complaint summary. Quarterly.
	ReportQuarterlyCompliance ReportType = "QUARTERLY_COMPLIANCE_REPORT"
)

// ValidReportType reports whether t is one of the known FRC report types.
func ValidReportType(t ReportType) bool {
	switch t {
	case ReportDailyTradingSummary, ReportLargeTrader, ReportSettlementFails,
		ReportSuspiciousTrading, ReportQuarterlyCompliance:
		return true
	}
	return false
}

// Format is the serialization format of a rendered report.
type Format string

const (
	FormatJSON Format = "JSON"
	FormatCSV  Format = "CSV"
)

// formatAllowed reports whether the requested format is permitted for a report
// type, per the FRC format column in docs/platform-architecture.md §10.5.
// CSV is only meaningful for the tabular Daily Trading Summary; everything else
// is JSON (Quarterly Compliance is PDF/JSON — PDF rendering is out of scope, so
// JSON is the supported machine format).
func formatAllowed(t ReportType, f Format) bool {
	if f == FormatJSON {
		return true
	}
	if f == FormatCSV {
		return t == ReportDailyTradingSummary
	}
	return false
}

// Sentinel errors returned by the package.
var (
	ErrMissingTenant    = fmt.Errorf("reporting: tenant_id is required")
	ErrInvalidType      = fmt.Errorf("reporting: invalid report type")
	ErrFormatNotAllowed = fmt.Errorf("reporting: format not allowed for this report type")
	ErrNoData           = fmt.Errorf("reporting: report has no data")
)

// ── Report bodies ─────────────────────────────────────────────────────────────

// InstrumentVolume is a per-instrument row of the daily trading summary.
type InstrumentVolume struct {
	InstrumentID string          `json:"instrument_id"`
	Symbol       string          `json:"symbol,omitempty"`
	Trades       int64           `json:"trades"`
	Volume       int64           `json:"volume"`       // total quantity
	Value        decimal.Decimal `json:"value"`        // total notional
	PriceChange  float64         `json:"price_change"` // session close vs prior close, signed
}

// DailyTradingSummary is the body of a Daily Trading Summary report.
type DailyTradingSummary struct {
	SessionDate string             `json:"session_date"` // YYYY-MM-DD
	TotalTrades int64              `json:"total_trades"`
	TotalVolume int64              `json:"total_volume"`
	TotalValue  decimal.Decimal    `json:"total_value"`
	Instruments []InstrumentVolume `json:"instruments"`
	TopMovers   []InstrumentVolume `json:"top_movers"`
}

// LargeTraderPosition is the body of a Large Trader report (one breach).
type LargeTraderPosition struct {
	ParticipantID      string  `json:"participant_id"`
	InstrumentID       string  `json:"instrument_id"`
	PositionSize       int64   `json:"position_size"`
	OutstandingShares  int64   `json:"outstanding_shares"`
	PercentOutstanding float64 `json:"percent_outstanding"`
	ThresholdPercent   float64 `json:"threshold_percent"`
	AsOf               string  `json:"as_of"`
}

// SettlementFail is one failed obligation in a Settlement Fails report.
type SettlementFail struct {
	ObligationID   string          `json:"obligation_id"`
	ParticipantID  string          `json:"participant_id"`
	InstrumentID   string          `json:"instrument_id"`
	Quantity       int64           `json:"quantity"`
	FailValue      decimal.Decimal `json:"fail_value"`
	PenaltyAmount  decimal.Decimal `json:"penalty_amount"`
	BuyInStatus    string          `json:"buy_in_status"` // NONE | INITIATED | EXECUTED
	SettlementDate string          `json:"settlement_date"`
}

// SettlementFailsReport is the body of a Settlement Fails report.
type SettlementFailsReport struct {
	SessionDate  string           `json:"session_date"`
	Fails        []SettlementFail `json:"fails"`
	TotalFails   int              `json:"total_fails"`
	TotalPenalty decimal.Decimal  `json:"total_penalty"`
}

// SuspiciousTradingAlert is the body of a real-time Suspicious Trading Alert.
type SuspiciousTradingAlert struct {
	AlertID       string   `json:"alert_id"`
	Pattern       string   `json:"pattern"` // WASH_TRADING | SPOOFING | LAYERING | ...
	ParticipantID string   `json:"participant_id"`
	InstrumentID  string   `json:"instrument_id,omitempty"`
	Description   string   `json:"description"`
	Severity      string   `json:"severity"` // LOW | MEDIUM | HIGH
	DetectedAt    string   `json:"detected_at"`
	OrderIDs      []string `json:"order_ids,omitempty"`
}

// QuarterlyComplianceReport is the body of a Quarterly Compliance report.
type QuarterlyComplianceReport struct {
	Quarter            string `json:"quarter"` // e.g. 2026-Q2
	TotalParticipants  int    `json:"total_participants"`
	KYCApproved        int    `json:"kyc_approved"`
	KYCPending         int    `json:"kyc_pending"`
	KYCRejected        int    `json:"kyc_rejected"`
	SuspendedAccounts  int    `json:"suspended_accounts"`
	SARsFiled          int    `json:"sars_filed"`
	Complaints         int    `json:"complaints"`
	ComplaintsResolved int    `json:"complaints_resolved"`
}

// ── Report envelope ───────────────────────────────────────────────────────────

// Report is a generated, ready-to-deliver FRC report. The Body field holds the
// typed report payload; Render serializes the full envelope in the requested
// Format.
type Report struct {
	ReportID    string      `json:"report_id"`
	TenantID    string      `json:"tenant_id"`
	Regulator   string      `json:"regulator"`
	Type        ReportType  `json:"report_type"`
	Format      Format      `json:"format"`
	PeriodStart time.Time   `json:"period_start,omitempty"`
	PeriodEnd   time.Time   `json:"period_end,omitempty"`
	GeneratedAt time.Time   `json:"generated_at"`
	Body        interface{} `json:"payload"`
}

// KafkaTopic returns the tenant-scoped Kafka topic the report is published to.
func (r *Report) KafkaTopic() string {
	return r.TenantID + ".compliance.frc-report-generated"
}

// fileExt returns the object extension for the report's format.
func (r *Report) fileExt() string {
	if r.Format == FormatCSV {
		return "csv"
	}
	return "json"
}

// S3Key returns the S3 destination object key for the report.
func (r *Report) S3Key() string {
	date := r.GeneratedAt.UTC().Format("2006-01-02")
	return fmt.Sprintf("s3://garudax-%s-reports/%s/%s/%s.%s",
		r.TenantID, r.Type, date, r.ReportID, r.fileExt())
}

// Render serializes the report in its Format.
func (r *Report) Render() ([]byte, error) {
	switch r.Format {
	case FormatCSV:
		return r.renderCSV()
	case FormatJSON, "":
		return json.MarshalIndent(r, "", "  ")
	default:
		return nil, ErrFormatNotAllowed
	}
}

// renderCSV renders the tabular reports (currently only Daily Trading Summary)
// as CSV with a metadata header.
func (r *Report) renderCSV() ([]byte, error) {
	summary, ok := r.Body.(DailyTradingSummary)
	if !ok {
		if p, okp := r.Body.(*DailyTradingSummary); okp {
			summary, ok = *p, true
		}
	}
	if !ok {
		return nil, ErrFormatNotAllowed
	}

	var buf bytes.Buffer
	// Metadata preamble as comment-style rows keeps the CSV self-describing.
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"# report_id", r.ReportID})
	_ = w.Write([]string{"# tenant_id", r.TenantID})
	_ = w.Write([]string{"# regulator", r.Regulator})
	_ = w.Write([]string{"# report_type", string(r.Type)})
	_ = w.Write([]string{"# session_date", summary.SessionDate})
	_ = w.Write([]string{"# generated_at", r.GeneratedAt.UTC().Format(time.RFC3339)})
	_ = w.Write([]string{"instrument_id", "symbol", "trades", "volume", "value", "price_change"})
	for _, iv := range summary.Instruments {
		_ = w.Write([]string{
			iv.InstrumentID,
			iv.Symbol,
			strconv.FormatInt(iv.Trades, 10),
			strconv.FormatInt(iv.Volume, 10),
			strconv.FormatFloat(iv.Value.Float64(), 'f', 2, 64),
			strconv.FormatFloat(iv.PriceChange, 'f', 4, 64),
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ── Publisher ─────────────────────────────────────────────────────────────────

// Delivery is the rendered report plus its delivery targets, handed to a Publisher.
type Delivery struct {
	TenantID   string
	ReportID   string
	Type       ReportType
	Format     Format
	KafkaTopic string
	S3Key      string
	Body       []byte
}

// Publisher delivers a rendered report to its external targets (Kafka + S3).
// Implementations must be safe for concurrent use.
type Publisher interface {
	Publish(ctx context.Context, d Delivery) error
}

// NoopPublisher discards deliveries; the default when no publisher is wired.
type NoopPublisher struct{}

// Publish implements Publisher and does nothing.
func (NoopPublisher) Publish(context.Context, Delivery) error { return nil }

// RecordingPublisher captures deliveries in memory. Useful for tests, demos, and
// the in-memory (no-broker) deployment mode. Safe for concurrent use.
type RecordingPublisher struct {
	mu         sync.Mutex
	deliveries []Delivery
}

// Publish records the delivery.
func (p *RecordingPublisher) Publish(_ context.Context, d Delivery) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deliveries = append(p.deliveries, d)
	return nil
}

// Deliveries returns a copy of all recorded deliveries.
func (p *RecordingPublisher) Deliveries() []Delivery {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Delivery, len(p.deliveries))
	copy(out, p.deliveries)
	return out
}

// Count returns the number of recorded deliveries.
func (p *RecordingPublisher) Count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.deliveries)
}

// ── Reporter ──────────────────────────────────────────────────────────────────

var idCounter uint64

func newReportID() string {
	n := atomic.AddUint64(&idCounter, 1)
	return fmt.Sprintf("frc-%s-%d", time.Now().UTC().Format("20060102"), n)
}

// Reporter builds and publishes FRC reports for a single tenant venue.
type Reporter struct {
	tenantID  string
	publisher Publisher
	now       func() time.Time
}

// NewReporter creates a Reporter for tenantID. If publisher is nil, a
// NoopPublisher is used. Returns ErrMissingTenant if tenantID is empty, honoring
// the platform invariant that tenant ID is never optional.
func NewReporter(tenantID string, publisher Publisher) (*Reporter, error) {
	if tenantID == "" {
		return nil, ErrMissingTenant
	}
	if publisher == nil {
		publisher = NoopPublisher{}
	}
	return &Reporter{
		tenantID:  tenantID,
		publisher: publisher,
		now:       func() time.Time { return time.Now().UTC() },
	}, nil
}

// TenantID returns the tenant this reporter is scoped to.
func (r *Reporter) TenantID() string { return r.tenantID }

// newReport builds the common report envelope, validating type/format.
func (r *Reporter) newReport(t ReportType, format Format, body interface{}) (*Report, error) {
	if !ValidReportType(t) {
		return nil, ErrInvalidType
	}
	if format == "" {
		format = FormatJSON
	}
	if !formatAllowed(t, format) {
		return nil, ErrFormatNotAllowed
	}
	return &Report{
		ReportID:    newReportID(),
		TenantID:    r.tenantID,
		Regulator:   Regulator,
		Type:        t,
		Format:      format,
		GeneratedAt: r.now(),
		Body:        body,
	}, nil
}

// GenerateDailyTradingSummary builds a Daily Trading Summary report from
// per-instrument rows. Totals and the top-5 movers (by absolute price change)
// are computed automatically. format may be JSON or CSV.
func (r *Reporter) GenerateDailyTradingSummary(sessionDate string, instruments []InstrumentVolume, format Format) (*Report, error) {
	if len(instruments) == 0 {
		return nil, ErrNoData
	}
	summary := DailyTradingSummary{
		SessionDate: sessionDate,
		Instruments: instruments,
	}
	for _, iv := range instruments {
		summary.TotalTrades += iv.Trades
		summary.TotalVolume += iv.Volume
		summary.TotalValue = summary.TotalValue.Add(iv.Value)
	}
	summary.TopMovers = topMovers(instruments, 5)

	rep, err := r.newReport(ReportDailyTradingSummary, format, summary)
	if err != nil {
		return nil, err
	}
	return rep, nil
}

// GenerateLargeTraderReport builds a Large Trader report for a single threshold
// breach. percentOutstanding is computed from position/outstanding when
// outstanding > 0.
func (r *Reporter) GenerateLargeTraderReport(pos LargeTraderPosition) (*Report, error) {
	if pos.ParticipantID == "" || pos.InstrumentID == "" {
		return nil, fmt.Errorf("reporting: participant_id and instrument_id are required")
	}
	if pos.OutstandingShares > 0 {
		// PercentOutstanding is a ratio (percentage), not money — float is fine.
		// Round to 2 dp, half away from zero, matching the prior behaviour.
		pct := float64(pos.PositionSize) / float64(pos.OutstandingShares) * 100
		pos.PercentOutstanding = math.Round(pct*100) / 100
	}
	if pos.AsOf == "" {
		pos.AsOf = r.now().Format("2006-01-02")
	}
	return r.newReport(ReportLargeTrader, FormatJSON, pos)
}

// GenerateSettlementFailsReport builds a Settlement Fails report. The fails
// slice may be empty (a clean "no fails" report is valid and still filed).
func (r *Reporter) GenerateSettlementFailsReport(sessionDate string, fails []SettlementFail) (*Report, error) {
	body := SettlementFailsReport{
		SessionDate: sessionDate,
		Fails:       fails,
		TotalFails:  len(fails),
	}
	if body.Fails == nil {
		body.Fails = []SettlementFail{}
	}
	for _, f := range fails {
		body.TotalPenalty = body.TotalPenalty.Add(f.PenaltyAmount)
	}
	return r.newReport(ReportSettlementFails, FormatJSON, body)
}

// GenerateSuspiciousTradingAlert builds a real-time Suspicious Trading Alert.
func (r *Reporter) GenerateSuspiciousTradingAlert(alert SuspiciousTradingAlert) (*Report, error) {
	if alert.Pattern == "" || alert.ParticipantID == "" {
		return nil, fmt.Errorf("reporting: pattern and participant_id are required")
	}
	if alert.DetectedAt == "" {
		alert.DetectedAt = r.now().Format(time.RFC3339)
	}
	if alert.Severity == "" {
		alert.Severity = "MEDIUM"
	}
	return r.newReport(ReportSuspiciousTrading, FormatJSON, alert)
}

// GenerateQuarterlyComplianceReport builds a Quarterly Compliance report.
func (r *Reporter) GenerateQuarterlyComplianceReport(body QuarterlyComplianceReport) (*Report, error) {
	if body.Quarter == "" {
		return nil, fmt.Errorf("reporting: quarter is required")
	}
	return r.newReport(ReportQuarterlyCompliance, FormatJSON, body)
}

// Publish renders the report and delivers it to the configured publisher,
// returning the Delivery (with computed Kafka topic and S3 key) on success.
func (r *Reporter) Publish(ctx context.Context, rep *Report) (Delivery, error) {
	if rep == nil {
		return Delivery{}, ErrNoData
	}
	if rep.TenantID == "" {
		return Delivery{}, ErrMissingTenant
	}
	body, err := rep.Render()
	if err != nil {
		return Delivery{}, err
	}
	d := Delivery{
		TenantID:   rep.TenantID,
		ReportID:   rep.ReportID,
		Type:       rep.Type,
		Format:     rep.Format,
		KafkaTopic: rep.KafkaTopic(),
		S3Key:      rep.S3Key(),
		Body:       body,
	}
	if err := r.publisher.Publish(ctx, d); err != nil {
		return Delivery{}, fmt.Errorf("reporting: publish failed: %w", err)
	}
	return d, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// topMovers returns up to n instruments ranked by absolute price change (desc).
func topMovers(instruments []InstrumentVolume, n int) []InstrumentVolume {
	movers := make([]InstrumentVolume, len(instruments))
	copy(movers, instruments)
	sort.SliceStable(movers, func(i, j int) bool {
		return abs(movers[i].PriceChange) > abs(movers[j].PriceChange)
	})
	if len(movers) > n {
		movers = movers[:n]
	}
	return movers
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
