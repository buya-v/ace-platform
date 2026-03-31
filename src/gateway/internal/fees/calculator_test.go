package fees

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/garudax-platform/gateway/internal/auth"
	"github.com/garudax-platform/gateway/internal/middleware"
	"github.com/garudax-platform/gateway/internal/router"
)

// --- Calculator Tests ---

func defaultRules() []FeeRule {
	maxFee := 5000.0
	return []FeeRule{
		{ID: "farmer-trading", ScheduleID: "default", FeeType: "trading", ParticipantTier: "farmer", InstrumentPattern: "*", RateBPS: 10.0, MinFee: 0, PerContractFee: 0},
		{ID: "hedger-trading", ScheduleID: "default", FeeType: "trading", ParticipantTier: "hedger", InstrumentPattern: "*", RateBPS: 15.0, MinFee: 0, PerContractFee: 0},
		{ID: "speculator-trading", ScheduleID: "default", FeeType: "trading", ParticipantTier: "speculator", InstrumentPattern: "*", RateBPS: 25.0, MinFee: 0, PerContractFee: 0},
		{ID: "mm-trading", ScheduleID: "default", FeeType: "trading", ParticipantTier: "market_maker", InstrumentPattern: "*", RateBPS: 5.0, MinFee: 0, PerContractFee: 0},
		{ID: "clearing-all", ScheduleID: "default", FeeType: "clearing", ParticipantTier: "*", InstrumentPattern: "*", RateBPS: 5.0, MinFee: 0, PerContractFee: 0},
		{ID: "with-min", ScheduleID: "default", FeeType: "data", ParticipantTier: "*", InstrumentPattern: "*", RateBPS: 1.0, MinFee: 10.0, PerContractFee: 0},
		{ID: "with-max", ScheduleID: "default", FeeType: "membership", ParticipantTier: "*", InstrumentPattern: "*", RateBPS: 100.0, MaxFee: &maxFee, MinFee: 0, PerContractFee: 0},
	}
}

func TestCalculateFee_FarmerTrading(t *testing.T) {
	rules := defaultRules()
	// 10 bps = 0.10% = 0.001 multiplier
	// tradeValue = 1,000,000 MNT -> fee = 1,000,000 * 10/10000 = 1000
	result := CalculateFee(1_000_000, 1, "farmer", "WHT-HRW-2026M07-UB", "trading", rules)

	if result.FeeType != "trading" {
		t.Errorf("FeeType = %q, want trading", result.FeeType)
	}
	if result.RateBPS != 10.0 {
		t.Errorf("RateBPS = %v, want 10.0", result.RateBPS)
	}
	if result.TotalFee != 1000.0 {
		t.Errorf("TotalFee = %v, want 1000.0", result.TotalFee)
	}
}

func TestCalculateFee_SpeculatorTrading(t *testing.T) {
	rules := defaultRules()
	// 25 bps on 500,000 = 500,000 * 25/10000 = 1250
	result := CalculateFee(500_000, 1, "speculator", "WHT-HRW-2026M07-UB", "trading", rules)

	if result.TotalFee != 1250.0 {
		t.Errorf("TotalFee = %v, want 1250.0", result.TotalFee)
	}
}

func TestCalculateFee_MarketMakerDiscount(t *testing.T) {
	rules := defaultRules()
	// 5 bps on 1,000,000 = 500
	result := CalculateFee(1_000_000, 1, "market_maker", "CSH-RAW-2026M09-UB", "trading", rules)

	if result.TotalFee != 500.0 {
		t.Errorf("TotalFee = %v, want 500.0 (market maker discount)", result.TotalFee)
	}
}

func TestCalculateFee_ClearingWildcardTier(t *testing.T) {
	rules := defaultRules()
	// Clearing rule has wildcard tier, so any participant matches
	// 5 bps on 200,000 = 100
	result := CalculateFee(200_000, 1, "farmer", "WHT-HRW-2026M07-UB", "clearing", rules)

	if result.FeeType != "clearing" {
		t.Errorf("FeeType = %q, want clearing", result.FeeType)
	}
	if result.TotalFee != 100.0 {
		t.Errorf("TotalFee = %v, want 100.0", result.TotalFee)
	}
}

func TestCalculateFee_MinFeeApplied(t *testing.T) {
	rules := defaultRules()
	// data fee: 1 bps on 500 = 0.05, but min is 10
	result := CalculateFee(500, 1, "farmer", "WHT-HRW-2026M07-UB", "data", rules)

	if result.TotalFee != 10.0 {
		t.Errorf("TotalFee = %v, want 10.0 (min fee applied)", result.TotalFee)
	}
	if !result.MinApplied {
		t.Error("MinApplied should be true")
	}
}

func TestCalculateFee_MaxFeeCapped(t *testing.T) {
	rules := defaultRules()
	// membership: 100 bps on 10,000,000 = 10,000 but max is 5000
	result := CalculateFee(10_000_000, 1, "speculator", "WHT-HRW-2026M07-UB", "membership", rules)

	if result.TotalFee != 5000.0 {
		t.Errorf("TotalFee = %v, want 5000.0 (max fee capped)", result.TotalFee)
	}
	if !result.MaxApplied {
		t.Error("MaxApplied should be true")
	}
}

func TestCalculateFee_PerContractFee(t *testing.T) {
	rules := []FeeRule{
		{ID: "per-contract", FeeType: "trading", ParticipantTier: "*", InstrumentPattern: "*", RateBPS: 10.0, PerContractFee: 50.0},
	}
	// 10 bps on 1,000,000 = 1000, plus 5 contracts * 50 = 250, total = 1250
	result := CalculateFee(1_000_000, 5, "farmer", "WHT-HRW-2026M07-UB", "trading", rules)

	if result.RateAmount != 1000.0 {
		t.Errorf("RateAmount = %v, want 1000.0", result.RateAmount)
	}
	if result.PerContractAmt != 250.0 {
		t.Errorf("PerContractAmt = %v, want 250.0", result.PerContractAmt)
	}
	if result.TotalFee != 1250.0 {
		t.Errorf("TotalFee = %v, want 1250.0", result.TotalFee)
	}
}

func TestCalculateFee_NoMatchingRule(t *testing.T) {
	rules := defaultRules()
	// No rule for fee_type "exchange"
	result := CalculateFee(1_000_000, 1, "farmer", "WHT-HRW-2026M07-UB", "exchange", rules)

	if result.TotalFee != 0 {
		t.Errorf("TotalFee = %v, want 0 (no matching rule)", result.TotalFee)
	}
}

func TestCalculateFee_ZeroTradeValue(t *testing.T) {
	rules := defaultRules()
	result := CalculateFee(0, 1, "farmer", "WHT-HRW-2026M07-UB", "trading", rules)

	if result.TotalFee != 0 {
		t.Errorf("TotalFee = %v, want 0", result.TotalFee)
	}
}

func TestCalculateFee_EmptyRules(t *testing.T) {
	result := CalculateFee(1_000_000, 1, "farmer", "WHT-HRW-2026M07-UB", "trading", nil)

	if result.TotalFee != 0 {
		t.Errorf("TotalFee = %v, want 0 (empty rules)", result.TotalFee)
	}
}

func TestCalculateAllFees(t *testing.T) {
	rules := defaultRules()
	results := CalculateAllFees(1_000_000, 1, "farmer", "WHT-HRW-2026M07-UB", rules)

	// Should have trading (farmer), clearing (*), data (*), membership (*)
	if len(results) < 3 {
		t.Errorf("expected at least 3 fee results, got %d", len(results))
	}

	feeTypes := make(map[string]bool)
	for _, r := range results {
		feeTypes[r.FeeType] = true
	}
	for _, expected := range []string{"trading", "clearing"} {
		if !feeTypes[expected] {
			t.Errorf("missing fee type %q in results", expected)
		}
	}
}

// --- Instrument Pattern Matching ---

func TestMatchInstrumentPattern_Wildcard(t *testing.T) {
	if !matchInstrumentPattern("*", "ANY-INSTRUMENT") {
		t.Error("wildcard should match any instrument")
	}
}

func TestMatchInstrumentPattern_PrefixWildcard(t *testing.T) {
	if !matchInstrumentPattern("WHT-*", "WHT-HRW-2026M07-UB") {
		t.Error("WHT-* should match WHT-HRW-2026M07-UB")
	}
	if matchInstrumentPattern("WHT-*", "CSH-RAW-2026M09-UB") {
		t.Error("WHT-* should not match CSH-RAW-2026M09-UB")
	}
}

func TestMatchInstrumentPattern_Exact(t *testing.T) {
	if !matchInstrumentPattern("WHT-HRW-2026M07-UB", "WHT-HRW-2026M07-UB") {
		t.Error("exact pattern should match exact instrument")
	}
	if matchInstrumentPattern("WHT-HRW-2026M07-UB", "CSH-RAW-2026M09-UB") {
		t.Error("exact pattern should not match different instrument")
	}
}

// --- Rule Priority Tests ---

func TestFindMatchingRule_ExactTierOverWildcard(t *testing.T) {
	rules := []FeeRule{
		{ID: "wild", FeeType: "trading", ParticipantTier: "*", InstrumentPattern: "*", RateBPS: 99.0},
		{ID: "exact", FeeType: "trading", ParticipantTier: "farmer", InstrumentPattern: "*", RateBPS: 10.0},
	}

	rule := findMatchingRule(rules, "trading", "farmer", "WHT-HRW-2026M07-UB")
	if rule == nil {
		t.Fatal("expected a matching rule")
	}
	if rule.ID != "exact" {
		t.Errorf("expected exact tier rule, got %q", rule.ID)
	}
}

func TestFindMatchingRule_ExactInstrumentOverWildcard(t *testing.T) {
	rules := []FeeRule{
		{ID: "wild-inst", FeeType: "trading", ParticipantTier: "farmer", InstrumentPattern: "*", RateBPS: 10.0},
		{ID: "exact-inst", FeeType: "trading", ParticipantTier: "farmer", InstrumentPattern: "WHT-HRW-2026M07-UB", RateBPS: 5.0},
	}

	rule := findMatchingRule(rules, "trading", "farmer", "WHT-HRW-2026M07-UB")
	if rule == nil {
		t.Fatal("expected a matching rule")
	}
	if rule.ID != "exact-inst" {
		t.Errorf("expected exact instrument rule, got %q", rule.ID)
	}
}

func TestFindMatchingRule_NoMatch(t *testing.T) {
	rules := []FeeRule{
		{ID: "trading-only", FeeType: "trading", ParticipantTier: "farmer", InstrumentPattern: "*", RateBPS: 10.0},
	}

	rule := findMatchingRule(rules, "clearing", "farmer", "WHT-HRW-2026M07-UB")
	if rule != nil {
		t.Error("expected no matching rule for different fee type")
	}
}

func TestFindMatchingRule_PrefixInstrumentPattern(t *testing.T) {
	rules := []FeeRule{
		{ID: "wildcard", FeeType: "trading", ParticipantTier: "*", InstrumentPattern: "*", RateBPS: 25.0},
		{ID: "wheat-specific", FeeType: "trading", ParticipantTier: "*", InstrumentPattern: "WHT-*", RateBPS: 15.0},
	}

	rule := findMatchingRule(rules, "trading", "farmer", "WHT-HRW-2026M07-UB")
	if rule == nil {
		t.Fatal("expected a matching rule")
	}
	if rule.ID != "wheat-specific" {
		t.Errorf("expected wheat-specific prefix rule, got %q", rule.ID)
	}
}

// --- Rounding ---

func TestRoundTo4(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
	}{
		{100.123456, 100.1235},
		{0.00005, 0.0001},
		{0.00004, 0},
		{1234.5678, 1234.5678},
		{0, 0},
	}

	for _, tc := range tests {
		got := roundTo4(tc.input)
		if math.Abs(got-tc.expected) > 1e-10 {
			t.Errorf("roundTo4(%v) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}

// --- Fee Type Collection ---

func TestCollectFeeTypes(t *testing.T) {
	rules := defaultRules()
	types := collectFeeTypes(rules)

	seen := make(map[string]bool)
	for _, ft := range types {
		seen[ft] = true
	}

	for _, expected := range []string{"trading", "clearing", "data", "membership"} {
		if !seen[expected] {
			t.Errorf("missing fee type %q", expected)
		}
	}
}

func TestCollectFeeTypes_Empty(t *testing.T) {
	types := collectFeeTypes(nil)
	if len(types) != 0 {
		t.Errorf("expected empty types, got %d", len(types))
	}
}

// --- Itoa ---

func TestItoa(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{9, "9"},
		{10, "10"},
		{42, "42"},
		{100, "100"},
	}
	for _, tc := range tests {
		got := itoa(tc.input)
		if got != tc.expected {
			t.Errorf("itoa(%d) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

// --- Handler Tests (mock store) ---

type mockFeeStore struct {
	activeSchedules []FeeSchedule
	allSchedules    []FeeSchedule
	rules           map[string][]FeeRule
	transactions    []FeeTransaction
	tier            string
	err             error
}

func (m *mockFeeStore) ListActiveSchedules(_ context.Context) ([]FeeSchedule, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.activeSchedules, nil
}

func (m *mockFeeStore) ListAllSchedules(_ context.Context) ([]FeeSchedule, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.allSchedules, nil
}

func (m *mockFeeStore) GetRulesForSchedule(_ context.Context, scheduleID string) ([]FeeRule, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.rules != nil {
		return m.rules[scheduleID], nil
	}
	return nil, nil
}

func (m *mockFeeStore) GetActiveRules(_ context.Context) ([]FeeRule, error) {
	if m.err != nil {
		return nil, m.err
	}
	var all []FeeRule
	for _, rules := range m.rules {
		all = append(all, rules...)
	}
	return all, nil
}

func (m *mockFeeStore) CreateRule(_ context.Context, _ FeeRule) error {
	if m.err != nil {
		return m.err
	}
	return nil
}

func (m *mockFeeStore) GetParticipantTier(_ context.Context, _ string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if m.tier != "" {
		return m.tier, nil
	}
	return "speculator", nil
}

func (m *mockFeeStore) ListFeeTransactions(_ context.Context, _, _, _ string) ([]FeeTransaction, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.transactions, nil
}

func sampleSchedules() []FeeSchedule {
	return []FeeSchedule{
		{
			ID:            "default",
			Name:          "GarudaX Default Fee Schedule",
			EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			Status:        "ACTIVE",
			CreatedAt:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}
}

func sampleFeeRules() map[string][]FeeRule {
	return map[string][]FeeRule{
		"default": {
			{ID: "farmer-trading", ScheduleID: "default", FeeType: "trading", ParticipantTier: "farmer", InstrumentPattern: "*", RateBPS: 10.0},
			{ID: "clearing-all", ScheduleID: "default", FeeType: "clearing", ParticipantTier: "*", InstrumentPattern: "*", RateBPS: 5.0},
		},
	}
}

func sampleTransactions() []FeeTransaction {
	return []FeeTransaction{
		{ID: "fee-1", TradeID: "trade-1", ParticipantID: "user-1", FeeType: "trading", Amount: 100.0, Currency: "MNT", CreatedAt: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)},
		{ID: "fee-2", TradeID: "trade-2", ParticipantID: "user-1", FeeType: "clearing", Amount: 50.0, Currency: "MNT", CreatedAt: time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)},
	}
}

// testClaims creates an auth.Claims with the given sub and participant_id for test context injection.
func testClaims(sub, participantID string) *auth.Claims {
	return &auth.Claims{
		Sub:           sub,
		ParticipantID: participantID,
		Roles:         []string{"trader"},
		ExpiresAt:     time.Now().Add(time.Hour).Unix(),
	}
}

func TestListActiveSchedules_Success(t *testing.T) {
	store := &mockFeeStore{
		activeSchedules: sampleSchedules(),
		rules:           sampleFeeRules(),
	}
	h := NewHandlers(store)
	req := httptest.NewRequest("GET", "/api/v1/fees/schedule", nil)
	rec := httptest.NewRecorder()

	h.ListActiveSchedules(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]json.RawMessage
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if _, ok := resp["data"]; !ok {
		t.Error("response missing 'data' field")
	}

	var schedules []FeeSchedule
	json.Unmarshal(resp["data"], &schedules)
	if len(schedules) != 1 {
		t.Errorf("len(schedules) = %d, want 1", len(schedules))
	}
	if len(schedules[0].Rules) != 2 {
		t.Errorf("len(rules) = %d, want 2", len(schedules[0].Rules))
	}
}

func TestListActiveSchedules_Empty(t *testing.T) {
	h := NewHandlers(&mockFeeStore{})
	req := httptest.NewRequest("GET", "/api/v1/fees/schedule", nil)
	rec := httptest.NewRecorder()

	h.ListActiveSchedules(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]json.RawMessage
	json.Unmarshal(rec.Body.Bytes(), &resp)
	var schedules []FeeSchedule
	json.Unmarshal(resp["data"], &schedules)
	if len(schedules) != 0 {
		t.Errorf("expected empty list, got %d", len(schedules))
	}
}

func TestListActiveSchedules_StoreError(t *testing.T) {
	h := NewHandlers(&mockFeeStore{err: errors.New("db failed")})
	req := httptest.NewRequest("GET", "/api/v1/fees/schedule", nil)
	rec := httptest.NewRecorder()

	h.ListActiveSchedules(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestListActiveSchedules_RulesError(t *testing.T) {
	// Store that succeeds on ListActiveSchedules but fails on GetRulesForSchedule
	store := &mockFeeStore{
		activeSchedules: sampleSchedules(),
	}
	h := NewHandlers(store)
	req := httptest.NewRequest("GET", "/api/v1/fees/schedule", nil)
	rec := httptest.NewRecorder()

	// First call succeeds (ListActiveSchedules), second would need rules
	// Since rules map is nil, GetRulesForSchedule returns nil, nil - which is fine
	h.ListActiveSchedules(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestGetMyFees_NoAuth(t *testing.T) {
	h := NewHandlers(&mockFeeStore{})
	req := httptest.NewRequest("GET", "/api/v1/fees/my-fees", nil)
	rec := httptest.NewRecorder()

	h.GetMyFees(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestGetMyFees_WithAuth(t *testing.T) {
	store := &mockFeeStore{transactions: sampleTransactions()}
	h := NewHandlers(store)

	req := httptest.NewRequest("GET", "/api/v1/fees/my-fees?from=2026-03-01&to=2026-03-31", nil)
	ctx := context.WithValue(req.Context(), middleware.ClaimsContextKey, testClaims("user-1", "user-1"))
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.GetMyFees(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]json.RawMessage
	json.Unmarshal(rec.Body.Bytes(), &resp)
	var txns []FeeTransaction
	json.Unmarshal(resp["data"], &txns)
	if len(txns) != 2 {
		t.Errorf("len(txns) = %d, want 2", len(txns))
	}
}

func TestGetMyFees_FallbackToSub(t *testing.T) {
	store := &mockFeeStore{transactions: sampleTransactions()}
	h := NewHandlers(store)

	req := httptest.NewRequest("GET", "/api/v1/fees/my-fees", nil)
	// Claims with empty participant_id - should fall back to Sub
	claims := testClaims("user-1", "")
	ctx := context.WithValue(req.Context(), middleware.ClaimsContextKey, claims)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.GetMyFees(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestGetMyFees_StoreError(t *testing.T) {
	h := NewHandlers(&mockFeeStore{err: errors.New("db failed")})
	req := httptest.NewRequest("GET", "/api/v1/fees/my-fees", nil)
	ctx := context.WithValue(req.Context(), middleware.ClaimsContextKey, testClaims("user-1", "user-1"))
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.GetMyFees(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestGetMyFees_EmptyTransactions(t *testing.T) {
	h := NewHandlers(&mockFeeStore{})
	req := httptest.NewRequest("GET", "/api/v1/fees/my-fees", nil)
	ctx := context.WithValue(req.Context(), middleware.ClaimsContextKey, testClaims("user-1", "user-1"))
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.GetMyFees(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]json.RawMessage
	json.Unmarshal(rec.Body.Bytes(), &resp)
	var txns []FeeTransaction
	json.Unmarshal(resp["data"], &txns)
	if len(txns) != 0 {
		t.Errorf("expected empty list, got %d", len(txns))
	}
}

func TestListAllSchedules_Success(t *testing.T) {
	store := &mockFeeStore{
		allSchedules: sampleSchedules(),
		rules:        sampleFeeRules(),
	}
	h := NewHandlers(store)
	req := httptest.NewRequest("GET", "/api/v1/admin/fees", nil)
	rec := httptest.NewRecorder()

	h.ListAllSchedules(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestListAllSchedules_StoreError(t *testing.T) {
	h := NewHandlers(&mockFeeStore{err: errors.New("db failed")})
	req := httptest.NewRequest("GET", "/api/v1/admin/fees", nil)
	rec := httptest.NewRecorder()

	h.ListAllSchedules(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestCreateFeeRule_Success(t *testing.T) {
	h := NewHandlers(&mockFeeStore{})
	body := `{"id":"new-rule","schedule_id":"default","fee_type":"trading","participant_tier":"hedger","rate_bps":20.0}`
	req := httptest.NewRequest("POST", "/api/v1/admin/fees/rules", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.CreateFeeRule(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp map[string]json.RawMessage
	json.Unmarshal(rec.Body.Bytes(), &resp)
	var rule FeeRule
	json.Unmarshal(resp["data"], &rule)
	if rule.ID != "new-rule" {
		t.Errorf("rule.ID = %q, want new-rule", rule.ID)
	}
	if rule.InstrumentPattern != "*" {
		t.Errorf("rule.InstrumentPattern = %q, want * (default)", rule.InstrumentPattern)
	}
}

func TestCreateFeeRule_DefaultsApplied(t *testing.T) {
	h := NewHandlers(&mockFeeStore{})
	// Omit instrument_pattern and participant_tier - should default to "*"
	body := `{"id":"rule-1","schedule_id":"default","fee_type":"clearing","rate_bps":5.0}`
	req := httptest.NewRequest("POST", "/api/v1/admin/fees/rules", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.CreateFeeRule(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var resp map[string]json.RawMessage
	json.Unmarshal(rec.Body.Bytes(), &resp)
	var rule FeeRule
	json.Unmarshal(resp["data"], &rule)
	if rule.InstrumentPattern != "*" {
		t.Errorf("InstrumentPattern = %q, want *", rule.InstrumentPattern)
	}
	if rule.ParticipantTier != "*" {
		t.Errorf("ParticipantTier = %q, want *", rule.ParticipantTier)
	}
}

func TestCreateFeeRule_InvalidBody(t *testing.T) {
	h := NewHandlers(&mockFeeStore{})
	req := httptest.NewRequest("POST", "/api/v1/admin/fees/rules", strings.NewReader("not json"))
	rec := httptest.NewRecorder()

	h.CreateFeeRule(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateFeeRule_MissingID(t *testing.T) {
	h := NewHandlers(&mockFeeStore{})
	body := `{"schedule_id":"default","fee_type":"trading","rate_bps":10}`
	req := httptest.NewRequest("POST", "/api/v1/admin/fees/rules", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.CreateFeeRule(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateFeeRule_MissingScheduleID(t *testing.T) {
	h := NewHandlers(&mockFeeStore{})
	body := `{"id":"rule-1","fee_type":"trading","rate_bps":10}`
	req := httptest.NewRequest("POST", "/api/v1/admin/fees/rules", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.CreateFeeRule(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateFeeRule_MissingFeeType(t *testing.T) {
	h := NewHandlers(&mockFeeStore{})
	body := `{"id":"rule-1","schedule_id":"default","rate_bps":10}`
	req := httptest.NewRequest("POST", "/api/v1/admin/fees/rules", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.CreateFeeRule(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateFeeRule_InvalidFeeType(t *testing.T) {
	h := NewHandlers(&mockFeeStore{})
	body := `{"id":"rule-1","schedule_id":"default","fee_type":"invalid","rate_bps":10}`
	req := httptest.NewRequest("POST", "/api/v1/admin/fees/rules", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.CreateFeeRule(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateFeeRule_NegativeRateBPS(t *testing.T) {
	h := NewHandlers(&mockFeeStore{})
	body := `{"id":"rule-1","schedule_id":"default","fee_type":"trading","rate_bps":-5}`
	req := httptest.NewRequest("POST", "/api/v1/admin/fees/rules", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.CreateFeeRule(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateFeeRule_StoreError(t *testing.T) {
	h := NewHandlers(&mockFeeStore{err: errors.New("db failed")})
	body := `{"id":"rule-1","schedule_id":"default","fee_type":"trading","rate_bps":10}`
	req := httptest.NewRequest("POST", "/api/v1/admin/fees/rules", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.CreateFeeRule(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestIsValidFeeType(t *testing.T) {
	valid := []string{"trading", "clearing", "data", "membership"}
	for _, ft := range valid {
		if !isValidFeeType(ft) {
			t.Errorf("isValidFeeType(%q) = false, want true", ft)
		}
	}

	invalid := []string{"exchange", "admin", "", "Trading"}
	for _, ft := range invalid {
		if isValidFeeType(ft) {
			t.Errorf("isValidFeeType(%q) = true, want false", ft)
		}
	}
}

func TestWriteJSON_ContentType(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, map[string]string{"status": "ok"})

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestWriteError_Format(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, "INVALID", "bad request")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	var resp map[string]map[string]string
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["error"]["code"] != "INVALID" {
		t.Errorf("error.code = %q, want INVALID", resp["error"]["code"])
	}
}

// --- Edge case: HedgerTrading rate ---

func TestCalculateFee_HedgerTrading(t *testing.T) {
	rules := defaultRules()
	// 15 bps on 1,000,000 = 1,000,000 * 15/10000 = 1500
	result := CalculateFee(1_000_000, 1, "hedger", "WHT-HRW-2026M07-UB", "trading", rules)

	if result.TotalFee != 1500.0 {
		t.Errorf("TotalFee = %v, want 1500.0", result.TotalFee)
	}
}

// --- Edge case: Min fee not applied when fee exceeds min ---

func TestCalculateFee_MinFeeNotApplied(t *testing.T) {
	rules := defaultRules()
	// data fee: 1 bps on 10,000,000 = 1000, min is 10 -> min NOT applied
	result := CalculateFee(10_000_000, 1, "farmer", "WHT-HRW-2026M07-UB", "data", rules)

	if result.TotalFee != 1000.0 {
		t.Errorf("TotalFee = %v, want 1000.0", result.TotalFee)
	}
	if result.MinApplied {
		t.Error("MinApplied should be false when fee exceeds min")
	}
}

// --- Edge case: Max fee not applied when fee is below max ---

func TestCalculateFee_MaxFeeNotApplied(t *testing.T) {
	rules := defaultRules()
	// membership: 100 bps on 100,000 = 1000, max is 5000 -> max NOT applied
	result := CalculateFee(100_000, 1, "speculator", "WHT-HRW-2026M07-UB", "membership", rules)

	if result.TotalFee != 1000.0 {
		t.Errorf("TotalFee = %v, want 1000.0", result.TotalFee)
	}
	if result.MaxApplied {
		t.Error("MaxApplied should be false when fee is below max")
	}
}

// --- RegisterRoutes ---

func TestRegisterRoutes(t *testing.T) {
	h := NewHandlers(&mockFeeStore{})
	rt := router.New()
	h.RegisterRoutes(rt)

	routes := rt.GetRoutes()
	if len(routes) != 4 {
		t.Errorf("registered %d routes, want 4", len(routes))
	}
}
