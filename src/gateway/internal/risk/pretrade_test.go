package risk

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/garudax-platform/decimal"
)

// d parses a decimal literal for tests (panicking on bad input).
func d(s string) decimal.Decimal { return decimal.MustParse(s) }

// --- Mock Store ---

type mockStore struct {
	limits    map[string]*OrderLimits
	allLimits []OrderLimits
	err       error
}

func (m *mockStore) GetOrderLimits(_ context.Context, instrumentID string) (*OrderLimits, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.limits[instrumentID], nil
}

func (m *mockStore) ListOrderLimits(_ context.Context) ([]OrderLimits, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.allLimits, nil
}

func (m *mockStore) UpsertOrderLimits(_ context.Context, limits *OrderLimits) error {
	if m.err != nil {
		return m.err
	}
	m.limits[limits.InstrumentID] = limits
	return nil
}

// --- ParseOrderRequest Tests ---

func TestParseOrderRequest(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		participantID string
		wantErr       bool
		wantQty       decimal.Decimal
		wantPrice     decimal.Decimal
		wantInst      string
	}{
		{
			name:          "valid order with numeric quantity",
			body:          `{"instrument_id":"WHT-HRW-2026M07-UB","side":"buy","price":"250.50","quantity":10}`,
			participantID: "part-1",
			wantQty:       d("10"),
			wantPrice:     d("250.50"),
			wantInst:      "WHT-HRW-2026M07-UB",
		},
		{
			name:          "valid order with string quantity",
			body:          `{"instrument_id":"CRN-YEL","side":"sell","price":"100.00","quantity":"25"}`,
			participantID: "part-2",
			wantQty:       d("25"),
			wantPrice:     d("100.00"),
			wantInst:      "CRN-YEL",
		},
		{
			name:          "fractional numeric quantity preserved",
			body:          `{"instrument_id":"WHT","side":"buy","price":"12.3456","quantity":2.5}`,
			participantID: "part-frac",
			wantQty:       d("2.5"),
			wantPrice:     d("12.3456"),
			wantInst:      "WHT",
		},
		{
			name:          "market order no price",
			body:          `{"instrument_id":"WHT","side":"buy","price":"","quantity":5}`,
			participantID: "part-3",
			wantQty:       d("5"),
			wantPrice:     decimal.Zero(),
			wantInst:      "WHT",
		},
		{
			name:    "empty body",
			body:    "",
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			body:    `{bad json`,
			wantErr: true,
		},
		{
			name:    "invalid price",
			body:    `{"instrument_id":"X","side":"buy","price":"abc","quantity":1}`,
			wantErr: true,
		},
		{
			name:    "invalid quantity",
			body:    `{"instrument_id":"X","side":"buy","price":"10","quantity":"abc"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body json.RawMessage
			if tt.body != "" {
				body = json.RawMessage(tt.body)
			}
			order, err := ParseOrderRequest(body, tt.participantID)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if order.InstrumentID != tt.wantInst {
				t.Errorf("instrument_id = %q, want %q", order.InstrumentID, tt.wantInst)
			}
			if !order.Quantity.Equal(tt.wantQty) {
				t.Errorf("quantity = %v, want %v", order.Quantity, tt.wantQty)
			}
			if !order.Price.Equal(tt.wantPrice) {
				t.Errorf("price = %v, want %v", order.Price, tt.wantPrice)
			}
			if order.ParticipantID != tt.participantID {
				t.Errorf("participant_id = %q, want %q", order.ParticipantID, tt.participantID)
			}
		})
	}
}

// --- CheckOrderSize Tests ---

func TestCheckOrderSize(t *testing.T) {
	limits := &OrderLimits{
		InstrumentID:  "WHT",
		MaxOrderQty:   d("100"),
		MaxOrderValue: d("50000"),
		PriceBandPct:  10.0,
	}

	tests := []struct {
		name     string
		order    *OrderRequest
		wantCode string
	}{
		{
			name:     "within limits",
			order:    &OrderRequest{InstrumentID: "WHT", Quantity: d("50"), Price: d("200")},
			wantCode: "",
		},
		{
			name:     "quantity at limit",
			order:    &OrderRequest{InstrumentID: "WHT", Quantity: d("100"), Price: d("200")},
			wantCode: "",
		},
		{
			name:     "quantity exceeds limit",
			order:    &OrderRequest{InstrumentID: "WHT", Quantity: d("101"), Price: d("200")},
			wantCode: "ORDER_QTY_EXCEEDED",
		},
		{
			name:     "value at limit",
			order:    &OrderRequest{InstrumentID: "WHT", Quantity: d("100"), Price: d("500")},
			wantCode: "",
		},
		{
			name:     "value exceeds limit",
			order:    &OrderRequest{InstrumentID: "WHT", Quantity: d("100"), Price: d("501")},
			wantCode: "ORDER_VALUE_EXCEEDED",
		},
		{
			name:     "market order no value check",
			order:    &OrderRequest{InstrumentID: "WHT", Quantity: d("50"), Price: decimal.Zero()},
			wantCode: "",
		},
		{
			name:     "large quantity small price within value",
			order:    &OrderRequest{InstrumentID: "WHT", Quantity: d("90"), Price: d("100")},
			wantCode: "",
		},
		{
			name:     "fractional notional just over limit",
			order:    &OrderRequest{InstrumentID: "WHT", Quantity: d("100"), Price: d("500.01")},
			wantCode: "ORDER_VALUE_EXCEEDED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rErr := CheckOrderSize(tt.order, limits)
			if tt.wantCode == "" {
				if rErr != nil {
					t.Errorf("expected no error, got %v", rErr)
				}
			} else {
				if rErr == nil {
					t.Fatal("expected error, got nil")
				}
				if rErr.Code != tt.wantCode {
					t.Errorf("code = %q, want %q", rErr.Code, tt.wantCode)
				}
			}
		})
	}
}

// --- CheckPriceBand Tests ---

func TestCheckPriceBand(t *testing.T) {
	limits := &OrderLimits{
		PriceBandPct: 10.0, // 10% band
	}

	tests := []struct {
		name      string
		price     decimal.Decimal
		lastPrice decimal.Decimal
		wantCode  string
	}{
		{
			name:      "within band",
			price:     d("105"),
			lastPrice: d("100"),
			wantCode:  "",
		},
		{
			name:      "at band boundary",
			price:     d("110"),
			lastPrice: d("100"),
			wantCode:  "",
		},
		{
			name:      "exceeds band above",
			price:     d("111"),
			lastPrice: d("100"),
			wantCode:  "PRICE_BAND_EXCEEDED",
		},
		{
			name:      "exceeds band below",
			price:     d("89"),
			lastPrice: d("100"),
			wantCode:  "PRICE_BAND_EXCEEDED",
		},
		{
			name:      "fractional within band",
			price:     d("110.00"),
			lastPrice: d("100"),
			wantCode:  "",
		},
		{
			name:      "fractional just over band",
			price:     d("110.01"),
			lastPrice: d("100"),
			wantCode:  "PRICE_BAND_EXCEEDED",
		},
		{
			name:      "no last price skips check",
			price:     d("500"),
			lastPrice: decimal.Zero(),
			wantCode:  "",
		},
		{
			name:      "market order skips check",
			price:     decimal.Zero(),
			lastPrice: d("100"),
			wantCode:  "",
		},
		{
			name:      "exact match",
			price:     d("100"),
			lastPrice: d("100"),
			wantCode:  "",
		},
		{
			name:      "negative last price skips",
			price:     d("100"),
			lastPrice: d("-1"),
			wantCode:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order := &OrderRequest{Price: tt.price}
			rErr := CheckPriceBand(order, tt.lastPrice, limits)
			if tt.wantCode == "" {
				if rErr != nil {
					t.Errorf("expected no error, got %v", rErr)
				}
			} else {
				if rErr == nil {
					t.Fatal("expected error, got nil")
				}
				if rErr.Code != tt.wantCode {
					t.Errorf("code = %q, want %q", rErr.Code, tt.wantCode)
				}
			}
		})
	}
}

// --- CheckParticipantStatus Tests ---

func TestCheckParticipantStatus(t *testing.T) {
	suspended := map[string]bool{
		"part-bad": true,
	}

	tests := []struct {
		name          string
		participantID string
		wantCode      string
	}{
		{
			name:          "active participant",
			participantID: "part-good",
			wantCode:      "",
		},
		{
			name:          "suspended participant",
			participantID: "part-bad",
			wantCode:      "PARTICIPANT_SUSPENDED",
		},
		{
			name:          "nil map",
			participantID: "anyone",
			wantCode:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var susp map[string]bool
			if tt.name != "nil map" {
				susp = suspended
			}
			rErr := CheckParticipantStatus(tt.participantID, susp)
			if tt.wantCode == "" {
				if rErr != nil {
					t.Errorf("expected no error, got %v", rErr)
				}
			} else {
				if rErr == nil {
					t.Fatal("expected error, got nil")
				}
				if rErr.Code != tt.wantCode {
					t.Errorf("code = %q, want %q", rErr.Code, tt.wantCode)
				}
			}
		})
	}
}

// --- PreTradeChecker Integration Tests ---

func TestPreTradeChecker_NilStore(t *testing.T) {
	checker := NewPreTradeChecker(nil)
	order := &OrderRequest{InstrumentID: "WHT", Quantity: d("99999"), Price: d("99999")}
	rErr := checker.CheckOrder(context.Background(), order, d("100"))
	if rErr != nil {
		t.Errorf("nil store should fail-open, got %v", rErr)
	}
}

func TestPreTradeChecker_StoreError(t *testing.T) {
	store := &mockStore{err: errors.New("db connection refused")}
	checker := NewPreTradeChecker(store)
	order := &OrderRequest{InstrumentID: "WHT", Quantity: d("99999"), Price: d("99999")}
	rErr := checker.CheckOrder(context.Background(), order, d("100"))
	if rErr != nil {
		t.Errorf("store error should fail-open, got %v", rErr)
	}
}

func TestPreTradeChecker_NoLimitsConfigured(t *testing.T) {
	store := &mockStore{limits: map[string]*OrderLimits{}}
	checker := NewPreTradeChecker(store)
	order := &OrderRequest{InstrumentID: "UNKNOWN", Quantity: d("99999"), Price: d("99999")}
	rErr := checker.CheckOrder(context.Background(), order, d("100"))
	if rErr != nil {
		t.Errorf("no limits configured should pass, got %v", rErr)
	}
}

func TestPreTradeChecker_OrderSizeReject(t *testing.T) {
	store := &mockStore{
		limits: map[string]*OrderLimits{
			"WHT": {MaxOrderQty: d("100"), MaxOrderValue: d("50000"), PriceBandPct: 20},
		},
	}
	checker := NewPreTradeChecker(store)
	order := &OrderRequest{InstrumentID: "WHT", Quantity: d("200"), Price: d("100")}
	rErr := checker.CheckOrder(context.Background(), order, d("100"))
	if rErr == nil {
		t.Fatal("expected rejection for oversized order")
	}
	if rErr.Code != "ORDER_QTY_EXCEEDED" {
		t.Errorf("code = %q, want ORDER_QTY_EXCEEDED", rErr.Code)
	}
}

func TestPreTradeChecker_PriceBandReject(t *testing.T) {
	store := &mockStore{
		limits: map[string]*OrderLimits{
			"WHT": {MaxOrderQty: d("1000"), MaxOrderValue: d("1000000"), PriceBandPct: 10},
		},
	}
	checker := NewPreTradeChecker(store)
	// Price 200 vs last 100 = 100% deviation, exceeds 10% band
	order := &OrderRequest{InstrumentID: "WHT", Quantity: d("10"), Price: d("200")}
	rErr := checker.CheckOrder(context.Background(), order, d("100"))
	if rErr == nil {
		t.Fatal("expected rejection for price band violation")
	}
	if rErr.Code != "PRICE_BAND_EXCEEDED" {
		t.Errorf("code = %q, want PRICE_BAND_EXCEEDED", rErr.Code)
	}
}

func TestPreTradeChecker_AllChecksPass(t *testing.T) {
	store := &mockStore{
		limits: map[string]*OrderLimits{
			"WHT": {MaxOrderQty: d("1000"), MaxOrderValue: d("1000000"), PriceBandPct: 10},
		},
	}
	checker := NewPreTradeChecker(store)
	order := &OrderRequest{InstrumentID: "WHT", Quantity: d("10"), Price: d("105")}
	rErr := checker.CheckOrder(context.Background(), order, d("100"))
	if rErr != nil {
		t.Errorf("expected all checks to pass, got %v", rErr)
	}
}

func TestRiskError_Error(t *testing.T) {
	err := &RiskError{Code: "TEST", Message: "test message"}
	if err.Error() != "test message" {
		t.Errorf("Error() = %q, want %q", err.Error(), "test message")
	}
}

func TestDefaultOrderLimits(t *testing.T) {
	defaults := DefaultOrderLimits()
	if !defaults.MaxOrderQty.IsPos() {
		t.Error("default max order qty should be positive")
	}
	if !defaults.MaxOrderValue.IsPos() {
		t.Error("default max order value should be positive")
	}
	if defaults.PriceBandPct != 100.0 {
		t.Errorf("default price band = %v, want 100.0", defaults.PriceBandPct)
	}
}
