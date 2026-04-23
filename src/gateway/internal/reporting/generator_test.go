package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garudax-platform/gateway/internal/auth"
	"github.com/garudax-platform/gateway/internal/middleware"
	"github.com/garudax-platform/gateway/internal/router"
)

// --- Generator Tests ---

func TestGenerateSettlementStatement_BasicLongPosition(t *testing.T) {
	positions := []PositionInput{
		{InstrumentID: "WHT-HRW-2026M07", Side: "long", Quantity: 100, AvgPrice: 250.0, MarkPrice: 260.0},
	}
	margin := MarginInput{
		InitialMargin:     5000,
		MaintenanceMargin: 3000,
		MarginUsed:        4000,
		ExcessMargin:      1000,
	}
	fees := FeeInput{
		TradingFees:  100,
		ClearingFees: 50,
	}

	stmt := GenerateSettlementStatement("stmt-1", "P001", "2026-03-31", positions, margin, fees)

	if stmt.ID != "stmt-1" {
		t.Errorf("ID = %q, want stmt-1", stmt.ID)
	}
	if stmt.ParticipantID != "P001" {
		t.Errorf("ParticipantID = %q, want P001", stmt.ParticipantID)
	}
	if stmt.ReportDate != "2026-03-31" {
		t.Errorf("ReportDate = %q, want 2026-03-31", stmt.ReportDate)
	}

	// Long position: unrealized = (260 - 250) * 100 = 1000
	// Total fees: 100 + 50 = 150
	// Net amount: 1000 - 150 = 850
	if stmt.NetAmount != 850.0 {
		t.Errorf("NetAmount = %v, want 850.0", stmt.NetAmount)
	}

	// Check PnL JSON
	var pnl map[string]interface{}
	if err := json.Unmarshal(stmt.PnL, &pnl); err != nil {
		t.Fatalf("Failed to unmarshal PnL: %v", err)
	}
	if pnl["total_unrealized"].(float64) != 1000.0 {
		t.Errorf("total_unrealized = %v, want 1000.0", pnl["total_unrealized"])
	}
}

func TestGenerateSettlementStatement_ShortPosition(t *testing.T) {
	positions := []PositionInput{
		{InstrumentID: "CRN-2026M09", Side: "short", Quantity: 50, AvgPrice: 300.0, MarkPrice: 280.0},
	}
	margin := MarginInput{}
	fees := FeeInput{TradingFees: 25}

	stmt := GenerateSettlementStatement("stmt-2", "P002", "2026-03-31", positions, margin, fees)

	// Short position: unrealized = (300 - 280) * 50 = 1000
	// Fees: 25
	// Net: 1000 - 25 = 975
	if stmt.NetAmount != 975.0 {
		t.Errorf("NetAmount = %v, want 975.0", stmt.NetAmount)
	}
}

func TestGenerateSettlementStatement_MultiplePositions(t *testing.T) {
	positions := []PositionInput{
		{InstrumentID: "WHT-HRW-2026M07", Side: "long", Quantity: 100, AvgPrice: 250.0, MarkPrice: 260.0},
		{InstrumentID: "CRN-2026M09", Side: "short", Quantity: 50, AvgPrice: 300.0, MarkPrice: 310.0},
	}
	margin := MarginInput{InitialMargin: 10000}
	fees := FeeInput{TradingFees: 200, ClearingFees: 100}

	stmt := GenerateSettlementStatement("stmt-3", "P003", "2026-03-31", positions, margin, fees)

	// Long: (260-250)*100 = 1000
	// Short: (300-310)*50 = -500
	// Total unrealized: 500
	// Total fees: 300
	// Net: 200
	if stmt.NetAmount != 200.0 {
		t.Errorf("NetAmount = %v, want 200.0", stmt.NetAmount)
	}
}

func TestGenerateSettlementStatement_NoPositions(t *testing.T) {
	stmt := GenerateSettlementStatement("stmt-4", "P004", "2026-03-31", nil, MarginInput{}, FeeInput{TradingFees: 10})

	// No positions, 10 in fees
	// Net = 0 - 10 = -10
	if stmt.NetAmount != -10.0 {
		t.Errorf("NetAmount = %v, want -10.0", stmt.NetAmount)
	}
}

func TestGenerateSettlementStatement_AllFeeTypes(t *testing.T) {
	fees := FeeInput{
		TradingFees:  100.1234,
		ClearingFees: 50.5678,
		DataFees:     25.0001,
		OtherFees:    10.9999,
	}

	stmt := GenerateSettlementStatement("stmt-5", "P005", "2026-03-31", nil, MarginInput{}, fees)

	var feesJSON map[string]interface{}
	json.Unmarshal(stmt.Fees, &feesJSON)

	totalFees := feesJSON["total_fees"].(float64)
	expected := roundTo4(100.1234 + 50.5678 + 25.0001 + 10.9999)
	if totalFees != expected {
		t.Errorf("total_fees = %v, want %v", totalFees, expected)
	}
}

func TestGenerateSettlementStatement_PositionsJSONValid(t *testing.T) {
	positions := []PositionInput{
		{InstrumentID: "WHT-HRW-2026M07", Side: "long", Quantity: 10, AvgPrice: 100.0, MarkPrice: 105.0},
	}
	stmt := GenerateSettlementStatement("stmt-6", "P006", "2026-03-31", positions, MarginInput{}, FeeInput{})

	var parsed []PositionInput
	if err := json.Unmarshal(stmt.Positions, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal Positions JSON: %v", err)
	}
	if len(parsed) != 1 {
		t.Errorf("Positions count = %d, want 1", len(parsed))
	}
	if parsed[0].InstrumentID != "WHT-HRW-2026M07" {
		t.Errorf("InstrumentID = %q, want WHT-HRW-2026M07", parsed[0].InstrumentID)
	}
}

func TestGenerateSettlementStatement_MarginJSONValid(t *testing.T) {
	margin := MarginInput{
		InitialMargin:     5000,
		MaintenanceMargin: 3000,
		MarginUsed:        4000,
		ExcessMargin:      1000,
	}
	stmt := GenerateSettlementStatement("stmt-7", "P007", "2026-03-31", nil, margin, FeeInput{})

	var parsed MarginInput
	if err := json.Unmarshal(stmt.Margin, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal Margin JSON: %v", err)
	}
	if parsed.InitialMargin != 5000 {
		t.Errorf("InitialMargin = %v, want 5000", parsed.InitialMargin)
	}
	if parsed.ExcessMargin != 1000 {
		t.Errorf("ExcessMargin = %v, want 1000", parsed.ExcessMargin)
	}
}

// --- Market Summary Tests ---

func TestGenerateMarketSummary_BasicTrades(t *testing.T) {
	trades := []TradeInput{
		{Price: 100.0, Quantity: 10},
		{Price: 105.0, Quantity: 20},
		{Price: 98.0, Quantity: 15},
		{Price: 102.0, Quantity: 5},
	}

	ms := GenerateMarketSummary("ms-1", "WHT-HRW-2026M07", "2026-03-31", trades, 101.5, 5000.0)

	if ms.ID != "ms-1" {
		t.Errorf("ID = %q, want ms-1", ms.ID)
	}
	if ms.InstrumentID != "WHT-HRW-2026M07" {
		t.Errorf("InstrumentID = %q, want WHT-HRW-2026M07", ms.InstrumentID)
	}
	if ms.OpenPrice != 100.0 {
		t.Errorf("OpenPrice = %v, want 100.0", ms.OpenPrice)
	}
	if ms.ClosePrice != 102.0 {
		t.Errorf("ClosePrice = %v, want 102.0", ms.ClosePrice)
	}
	if ms.HighPrice != 105.0 {
		t.Errorf("HighPrice = %v, want 105.0", ms.HighPrice)
	}
	if ms.LowPrice != 98.0 {
		t.Errorf("LowPrice = %v, want 98.0", ms.LowPrice)
	}
	if ms.Volume != 50.0 {
		t.Errorf("Volume = %v, want 50.0", ms.Volume)
	}
	if ms.SettlementPrice != 101.5 {
		t.Errorf("SettlementPrice = %v, want 101.5", ms.SettlementPrice)
	}
	if ms.OpenInterest != 5000.0 {
		t.Errorf("OpenInterest = %v, want 5000.0", ms.OpenInterest)
	}
}

func TestGenerateMarketSummary_NoTrades(t *testing.T) {
	ms := GenerateMarketSummary("ms-2", "CRN-2026M09", "2026-03-31", nil, 200.0, 3000.0)

	if ms.OpenPrice != 0 {
		t.Errorf("OpenPrice = %v, want 0", ms.OpenPrice)
	}
	if ms.ClosePrice != 0 {
		t.Errorf("ClosePrice = %v, want 0", ms.ClosePrice)
	}
	if ms.HighPrice != 0 {
		t.Errorf("HighPrice = %v, want 0", ms.HighPrice)
	}
	if ms.LowPrice != 0 {
		t.Errorf("LowPrice = %v, want 0", ms.LowPrice)
	}
	if ms.Volume != 0 {
		t.Errorf("Volume = %v, want 0", ms.Volume)
	}
	if ms.SettlementPrice != 200.0 {
		t.Errorf("SettlementPrice = %v, want 200.0", ms.SettlementPrice)
	}
}

func TestGenerateMarketSummary_SingleTrade(t *testing.T) {
	trades := []TradeInput{
		{Price: 150.0, Quantity: 1},
	}

	ms := GenerateMarketSummary("ms-3", "SOY-2026M12", "2026-03-31", trades, 150.0, 1000.0)

	if ms.OpenPrice != 150.0 || ms.ClosePrice != 150.0 {
		t.Errorf("Open/Close = %v/%v, want 150.0/150.0", ms.OpenPrice, ms.ClosePrice)
	}
	if ms.HighPrice != 150.0 || ms.LowPrice != 150.0 {
		t.Errorf("High/Low = %v/%v, want 150.0/150.0", ms.HighPrice, ms.LowPrice)
	}
	if ms.Volume != 1.0 {
		t.Errorf("Volume = %v, want 1.0", ms.Volume)
	}
}

func TestGenerateMarketSummary_DecimalPrecision(t *testing.T) {
	trades := []TradeInput{
		{Price: 100.12345, Quantity: 10.5678},
	}

	ms := GenerateMarketSummary("ms-4", "WHT", "2026-03-31", trades, 99.99995, 500.123)

	// Should round to 4 decimal places
	if ms.OpenPrice != roundTo4(100.12345) {
		t.Errorf("OpenPrice = %v, want %v", ms.OpenPrice, roundTo4(100.12345))
	}
	if ms.Volume != roundTo4(10.5678) {
		t.Errorf("Volume = %v, want %v", ms.Volume, roundTo4(10.5678))
	}
}

// --- Large Trader Report Tests ---

func TestGenerateLargeTraderReport_AboveThreshold(t *testing.T) {
	positions := []PositionSnapshot{
		{ParticipantID: "P001", InstrumentID: "WHT-2026M07", LongQty: 600, ShortQty: 0},
		{ParticipantID: "P002", InstrumentID: "WHT-2026M07", LongQty: 100, ShortQty: 0},
	}
	oi := map[string]float64{"WHT-2026M07": 1000}

	// Threshold 5% — P001 has 60% of OI, P002 has 10%
	result := GenerateLargeTraderReport("2026-03-31", positions, oi, 5.0)

	if len(result) != 2 {
		t.Fatalf("Result count = %d, want 2", len(result))
	}

	// Results sorted by participant ID
	if result[0].ParticipantID != "P001" {
		t.Errorf("result[0].ParticipantID = %q, want P001", result[0].ParticipantID)
	}
	if result[0].NetPosition != 600 {
		t.Errorf("result[0].NetPosition = %v, want 600", result[0].NetPosition)
	}
	if result[0].PctOfOpenInterest != 60.0 {
		t.Errorf("result[0].PctOfOpenInterest = %v, want 60.0", result[0].PctOfOpenInterest)
	}

	if result[1].ParticipantID != "P002" {
		t.Errorf("result[1].ParticipantID = %q, want P002", result[1].ParticipantID)
	}
	if result[1].PctOfOpenInterest != 10.0 {
		t.Errorf("result[1].PctOfOpenInterest = %v, want 10.0", result[1].PctOfOpenInterest)
	}
}

func TestGenerateLargeTraderReport_BelowThreshold(t *testing.T) {
	positions := []PositionSnapshot{
		{ParticipantID: "P001", InstrumentID: "WHT-2026M07", LongQty: 10, ShortQty: 0},
	}
	oi := map[string]float64{"WHT-2026M07": 10000}

	// 10/10000 = 0.1%, threshold is 5%
	result := GenerateLargeTraderReport("2026-03-31", positions, oi, 5.0)

	if len(result) != 0 {
		t.Errorf("Result count = %d, want 0 (below threshold)", len(result))
	}
}

func TestGenerateLargeTraderReport_NetShortPosition(t *testing.T) {
	positions := []PositionSnapshot{
		{ParticipantID: "P001", InstrumentID: "CRN-2026M09", LongQty: 100, ShortQty: 600},
	}
	oi := map[string]float64{"CRN-2026M09": 1000}

	result := GenerateLargeTraderReport("2026-03-31", positions, oi, 5.0)

	if len(result) != 1 {
		t.Fatalf("Result count = %d, want 1", len(result))
	}
	// Net = 100 - 600 = -500; abs(-500)/1000 = 50%
	if result[0].NetPosition != -500 {
		t.Errorf("NetPosition = %v, want -500", result[0].NetPosition)
	}
	if result[0].GrossPosition != 700 {
		t.Errorf("GrossPosition = %v, want 700", result[0].GrossPosition)
	}
	if result[0].PctOfOpenInterest != 50.0 {
		t.Errorf("PctOfOpenInterest = %v, want 50.0", result[0].PctOfOpenInterest)
	}
}

func TestGenerateLargeTraderReport_ZeroOpenInterest(t *testing.T) {
	positions := []PositionSnapshot{
		{ParticipantID: "P001", InstrumentID: "WHT-2026M07", LongQty: 100, ShortQty: 0},
	}
	oi := map[string]float64{"WHT-2026M07": 0}

	result := GenerateLargeTraderReport("2026-03-31", positions, oi, 5.0)

	if len(result) != 0 {
		t.Errorf("Result count = %d, want 0 (zero OI)", len(result))
	}
}

func TestGenerateLargeTraderReport_MissingOpenInterest(t *testing.T) {
	positions := []PositionSnapshot{
		{ParticipantID: "P001", InstrumentID: "WHT-2026M07", LongQty: 100, ShortQty: 0},
	}
	oi := map[string]float64{} // instrument not in map

	result := GenerateLargeTraderReport("2026-03-31", positions, oi, 5.0)

	if len(result) != 0 {
		t.Errorf("Result count = %d, want 0 (missing OI)", len(result))
	}
}

func TestGenerateLargeTraderReport_MultipleInstruments(t *testing.T) {
	positions := []PositionSnapshot{
		{ParticipantID: "P001", InstrumentID: "WHT-2026M07", LongQty: 600, ShortQty: 0},
		{ParticipantID: "P001", InstrumentID: "CRN-2026M09", LongQty: 10, ShortQty: 0},
	}
	oi := map[string]float64{
		"WHT-2026M07": 1000,
		"CRN-2026M09": 10000,
	}

	// P001 has 60% of WHT (above 5%) but only 0.1% of CRN (below 5%)
	result := GenerateLargeTraderReport("2026-03-31", positions, oi, 5.0)

	if len(result) != 1 {
		t.Fatalf("Result count = %d, want 1", len(result))
	}
	if result[0].InstrumentID != "WHT-2026M07" {
		t.Errorf("InstrumentID = %q, want WHT-2026M07", result[0].InstrumentID)
	}
}

func TestGenerateLargeTraderReport_EmptyPositions(t *testing.T) {
	result := GenerateLargeTraderReport("2026-03-31", nil, map[string]float64{}, 5.0)

	if len(result) != 0 {
		t.Errorf("Result count = %d, want 0", len(result))
	}
}

func TestGenerateLargeTraderReport_IDFormat(t *testing.T) {
	positions := []PositionSnapshot{
		{ParticipantID: "P001", InstrumentID: "WHT", LongQty: 100, ShortQty: 0},
	}
	oi := map[string]float64{"WHT": 100}

	result := GenerateLargeTraderReport("2026-03-31", positions, oi, 5.0)

	if len(result) != 1 {
		t.Fatalf("Result count = %d, want 1", len(result))
	}
	expected := "ltp-P001-WHT-2026-03-31"
	if result[0].ID != expected {
		t.Errorf("ID = %q, want %q", result[0].ID, expected)
	}
}

// --- Date Validation Tests ---

func TestIsValidDate(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"2026-03-31", true},
		{"2026-01-01", true},
		{"2026-12-31", true},
		{"20260331", false},
		{"2026/03/31", false},
		{"2026-3-31", false},
		{"", false},
		{"not-a-date", false},
		{"2026-13-01", true}, // format check only, not semantic
	}

	for _, tt := range tests {
		got := isValidDate(tt.input)
		if got != tt.want {
			t.Errorf("isValidDate(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// --- roundTo4 Tests ---

func TestRoundTo4(t *testing.T) {
	tests := []struct {
		input float64
		want  float64
	}{
		{1.23456789, 1.2346},
		{1.0, 1.0},
		{0.00005, 0.0001},
		{0.00004, 0.0},
		{-1.23456, -1.2346},
		{999999.99999, 1000000.0},
	}

	for _, tt := range tests {
		got := roundTo4(tt.input)
		if math.Abs(got-tt.want) > 0.00001 {
			t.Errorf("roundTo4(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// --- Handler Tests ---

// mockStore is a test double for the Store interface.
type mockStore struct {
	getDailyStatementFn      func(ctx context.Context, pid, date string) (*DailyStatement, error)
	listMarketSummariesFn    func(ctx context.Context, date string) ([]MarketSummary, error)
	listLargeTradersFn       func(ctx context.Context, date string) ([]LargeTraderPosition, error)
	listTradesFn             func(ctx context.Context, pid, from, to string) ([]json.RawMessage, error)
}

func (m *mockStore) SaveDailyStatement(ctx context.Context, stmt DailyStatement) error { return nil }
func (m *mockStore) GetDailyStatement(ctx context.Context, pid, date string) (*DailyStatement, error) {
	if m.getDailyStatementFn != nil {
		return m.getDailyStatementFn(ctx, pid, date)
	}
	return nil, nil
}
func (m *mockStore) SaveMarketSummary(ctx context.Context, ms MarketSummary) error { return nil }
func (m *mockStore) ListMarketSummaries(ctx context.Context, date string) ([]MarketSummary, error) {
	if m.listMarketSummariesFn != nil {
		return m.listMarketSummariesFn(ctx, date)
	}
	return nil, nil
}
func (m *mockStore) SaveLargeTraderPosition(ctx context.Context, ltp LargeTraderPosition) error {
	return nil
}
func (m *mockStore) ListLargeTraderPositions(ctx context.Context, date string) ([]LargeTraderPosition, error) {
	if m.listLargeTradersFn != nil {
		return m.listLargeTradersFn(ctx, date)
	}
	return nil, nil
}
func (m *mockStore) ListTradesForParticipant(ctx context.Context, pid, from, to string) ([]json.RawMessage, error) {
	if m.listTradesFn != nil {
		return m.listTradesFn(ctx, pid, from, to)
	}
	return nil, nil
}

func withClaims(r *http.Request, participantID string) *http.Request {
	claims := &auth.Claims{
		Sub:           "user-1",
		ParticipantID: participantID,
		Roles:         []string{"trader"},
	}
	ctx := context.WithValue(r.Context(), middleware.ClaimsContextKey, claims)
	return r.WithContext(ctx)
}

func TestHandler_GetSettlementStatement_Success(t *testing.T) {
	store := &mockStore{
		getDailyStatementFn: func(ctx context.Context, pid, date string) (*DailyStatement, error) {
			return &DailyStatement{
				ID:            "stmt-1",
				ParticipantID: pid,
				ReportDate:    date,
				NetAmount:     1000.0,
				Positions:     json.RawMessage(`[]`),
				Margin:        json.RawMessage(`{}`),
				PnL:           json.RawMessage(`{}`),
				Fees:          json.RawMessage(`{}`),
			}, nil
		},
	}

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	req := httptest.NewRequest("GET", "/api/v1/reports/settlement-statement?date=2026-03-31", nil)
	req = withClaims(req, "P001")
	w := httptest.NewRecorder()
	rt.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	data := resp["data"].(map[string]interface{})
	if data["participant_id"] != "P001" {
		t.Errorf("participant_id = %v, want P001", data["participant_id"])
	}
}

func TestHandler_GetSettlementStatement_MissingDate(t *testing.T) {
	h := NewHandlers(&mockStore{})
	rt := router.New()
	h.RegisterRoutes(rt)

	req := httptest.NewRequest("GET", "/api/v1/reports/settlement-statement", nil)
	req = withClaims(req, "P001")
	w := httptest.NewRecorder()
	rt.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandler_GetSettlementStatement_InvalidDate(t *testing.T) {
	h := NewHandlers(&mockStore{})
	rt := router.New()
	h.RegisterRoutes(rt)

	req := httptest.NewRequest("GET", "/api/v1/reports/settlement-statement?date=20260331", nil)
	req = withClaims(req, "P001")
	w := httptest.NewRecorder()
	rt.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandler_GetSettlementStatement_NotFound(t *testing.T) {
	store := &mockStore{
		getDailyStatementFn: func(ctx context.Context, pid, date string) (*DailyStatement, error) {
			return nil, nil
		},
	}

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	req := httptest.NewRequest("GET", "/api/v1/reports/settlement-statement?date=2026-03-31", nil)
	req = withClaims(req, "P001")
	w := httptest.NewRecorder()
	rt.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandler_GetSettlementStatement_NoAuth(t *testing.T) {
	h := NewHandlers(&mockStore{})
	rt := router.New()
	h.RegisterRoutes(rt)

	req := httptest.NewRequest("GET", "/api/v1/reports/settlement-statement?date=2026-03-31", nil)
	// No claims added
	w := httptest.NewRecorder()
	rt.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandler_GetSettlementStatement_StoreError(t *testing.T) {
	store := &mockStore{
		getDailyStatementFn: func(ctx context.Context, pid, date string) (*DailyStatement, error) {
			return nil, errors.New("db connection lost")
		},
	}

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	req := httptest.NewRequest("GET", "/api/v1/reports/settlement-statement?date=2026-03-31", nil)
	req = withClaims(req, "P001")
	w := httptest.NewRecorder()
	rt.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandler_GetTradeSummary_Success(t *testing.T) {
	store := &mockStore{
		listTradesFn: func(ctx context.Context, pid, from, to string) ([]json.RawMessage, error) {
			return []json.RawMessage{
				json.RawMessage(`{"trade_id":"T1","price":100}`),
				json.RawMessage(`{"trade_id":"T2","price":105}`),
			}, nil
		},
	}

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	req := httptest.NewRequest("GET", "/api/v1/reports/trade-summary?from=2026-03-01&to=2026-03-31", nil)
	req = withClaims(req, "P001")
	w := httptest.NewRecorder()
	rt.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	count := resp["count"].(float64)
	if count != 2 {
		t.Errorf("count = %v, want 2", count)
	}
}

func TestHandler_GetTradeSummary_MissingParams(t *testing.T) {
	h := NewHandlers(&mockStore{})
	rt := router.New()
	h.RegisterRoutes(rt)

	// Missing 'to'
	req := httptest.NewRequest("GET", "/api/v1/reports/trade-summary?from=2026-03-01", nil)
	req = withClaims(req, "P001")
	w := httptest.NewRecorder()
	rt.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandler_GetTradeSummary_InvalidDate(t *testing.T) {
	h := NewHandlers(&mockStore{})
	rt := router.New()
	h.RegisterRoutes(rt)

	req := httptest.NewRequest("GET", "/api/v1/reports/trade-summary?from=20260301&to=20260331", nil)
	req = withClaims(req, "P001")
	w := httptest.NewRecorder()
	rt.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandler_GetTradeSummary_NoAuth(t *testing.T) {
	h := NewHandlers(&mockStore{})
	rt := router.New()
	h.RegisterRoutes(rt)

	req := httptest.NewRequest("GET", "/api/v1/reports/trade-summary?from=2026-03-01&to=2026-03-31", nil)
	w := httptest.NewRecorder()
	rt.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandler_GetTradeSummary_StoreError(t *testing.T) {
	store := &mockStore{
		listTradesFn: func(ctx context.Context, pid, from, to string) ([]json.RawMessage, error) {
			return nil, errors.New("timeout")
		},
	}

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	req := httptest.NewRequest("GET", "/api/v1/reports/trade-summary?from=2026-03-01&to=2026-03-31", nil)
	req = withClaims(req, "P001")
	w := httptest.NewRecorder()
	rt.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandler_GetTradeSummary_EmptyResult(t *testing.T) {
	store := &mockStore{
		listTradesFn: func(ctx context.Context, pid, from, to string) ([]json.RawMessage, error) {
			return nil, nil
		},
	}

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	req := httptest.NewRequest("GET", "/api/v1/reports/trade-summary?from=2026-03-01&to=2026-03-31", nil)
	req = withClaims(req, "P001")
	w := httptest.NewRecorder()
	rt.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	count := resp["count"].(float64)
	if count != 0 {
		t.Errorf("count = %v, want 0", count)
	}
}

func TestHandler_GetMarketSummary_Success(t *testing.T) {
	store := &mockStore{
		listMarketSummariesFn: func(ctx context.Context, date string) ([]MarketSummary, error) {
			return []MarketSummary{
				{ID: "ms-1", InstrumentID: "WHT-2026M07", ReportDate: date, OpenPrice: 100, HighPrice: 110, LowPrice: 95, ClosePrice: 105, Volume: 500},
			}, nil
		},
	}

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	req := httptest.NewRequest("GET", "/api/v1/admin/reports/market-summary?date=2026-03-31", nil)
	w := httptest.NewRecorder()
	rt.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	count := resp["count"].(float64)
	if count != 1 {
		t.Errorf("count = %v, want 1", count)
	}
}

func TestHandler_GetMarketSummary_MissingDate(t *testing.T) {
	h := NewHandlers(&mockStore{})
	rt := router.New()
	h.RegisterRoutes(rt)

	req := httptest.NewRequest("GET", "/api/v1/admin/reports/market-summary", nil)
	w := httptest.NewRecorder()
	rt.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandler_GetMarketSummary_StoreError(t *testing.T) {
	store := &mockStore{
		listMarketSummariesFn: func(ctx context.Context, date string) ([]MarketSummary, error) {
			return nil, errors.New("db error")
		},
	}

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	req := httptest.NewRequest("GET", "/api/v1/admin/reports/market-summary?date=2026-03-31", nil)
	w := httptest.NewRecorder()
	rt.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandler_GetMarketSummary_EmptyResult(t *testing.T) {
	store := &mockStore{
		listMarketSummariesFn: func(ctx context.Context, date string) ([]MarketSummary, error) {
			return nil, nil
		},
	}

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	req := httptest.NewRequest("GET", "/api/v1/admin/reports/market-summary?date=2026-03-31", nil)
	w := httptest.NewRecorder()
	rt.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	data := resp["data"].([]interface{})
	if len(data) != 0 {
		t.Errorf("data length = %d, want 0", len(data))
	}
}

func TestHandler_GetLargeTraders_Success(t *testing.T) {
	store := &mockStore{
		listLargeTradersFn: func(ctx context.Context, date string) ([]LargeTraderPosition, error) {
			return []LargeTraderPosition{
				{ID: "ltp-1", ParticipantID: "P001", InstrumentID: "WHT", ReportDate: date, NetPosition: 600, PctOfOpenInterest: 60.0},
			}, nil
		},
	}

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	req := httptest.NewRequest("GET", "/api/v1/admin/reports/large-traders?date=2026-03-31", nil)
	w := httptest.NewRecorder()
	rt.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	count := resp["count"].(float64)
	if count != 1 {
		t.Errorf("count = %v, want 1", count)
	}
}

func TestHandler_GetLargeTraders_MissingDate(t *testing.T) {
	h := NewHandlers(&mockStore{})
	rt := router.New()
	h.RegisterRoutes(rt)

	req := httptest.NewRequest("GET", "/api/v1/admin/reports/large-traders", nil)
	w := httptest.NewRecorder()
	rt.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandler_GetLargeTraders_StoreError(t *testing.T) {
	store := &mockStore{
		listLargeTradersFn: func(ctx context.Context, date string) ([]LargeTraderPosition, error) {
			return nil, errors.New("db error")
		},
	}

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	req := httptest.NewRequest("GET", "/api/v1/admin/reports/large-traders?date=2026-03-31", nil)
	w := httptest.NewRecorder()
	rt.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandler_GetLargeTraders_EmptyResult(t *testing.T) {
	store := &mockStore{
		listLargeTradersFn: func(ctx context.Context, date string) ([]LargeTraderPosition, error) {
			return nil, nil
		},
	}

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	req := httptest.NewRequest("GET", "/api/v1/admin/reports/large-traders?date=2026-03-31", nil)
	w := httptest.NewRecorder()
	rt.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	data := resp["data"].([]interface{})
	if len(data) != 0 {
		t.Errorf("data length = %d, want 0", len(data))
	}
}

func TestHandler_RegisterRoutes(t *testing.T) {
	h := NewHandlers(&mockStore{})
	rt := router.New()
	h.RegisterRoutes(rt)

	routes := rt.GetRoutes()
	expected := map[string]string{
		"GET /api/v1/reports/settlement-statement": "",
		"GET /api/v1/reports/trade-summary":        "",
		"GET /api/v1/admin/reports/market-summary": "",
		"GET /api/v1/admin/reports/large-traders":  "",
	}

	for _, r := range routes {
		key := r.Method + " " + r.Pattern
		delete(expected, key)
	}

	for k := range expected {
		t.Errorf("Missing route: %s", k)
	}
}

func TestHandler_GetSettlementStatement_FallbackToSub(t *testing.T) {
	store := &mockStore{
		getDailyStatementFn: func(ctx context.Context, pid, date string) (*DailyStatement, error) {
			if pid != "user-1" {
				t.Errorf("Expected fallback to Sub, got pid = %q", pid)
			}
			return &DailyStatement{
				ID:            "stmt-1",
				ParticipantID: pid,
				ReportDate:    date,
				NetAmount:     0,
				Positions:     json.RawMessage(`[]`),
				Margin:        json.RawMessage(`{}`),
				PnL:           json.RawMessage(`{}`),
				Fees:          json.RawMessage(`{}`),
			}, nil
		},
	}

	h := NewHandlers(store)
	rt := router.New()
	h.RegisterRoutes(rt)

	// Claims with empty ParticipantID — should fall back to Sub
	req := httptest.NewRequest("GET", "/api/v1/reports/settlement-statement?date=2026-03-31", nil)
	req = withClaims(req, "") // empty participant ID
	w := httptest.NewRecorder()
	rt.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}
}
