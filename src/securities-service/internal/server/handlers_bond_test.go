// Package server — tests for fixed-income bond HTTP handlers.
package server

import (
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/middleware"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// newBondTestServer creates a test server wired with a real BondStore.
func newBondTestServer(t *testing.T) (*httptest.Server, *store.InMemoryBondStore) {
	t.Helper()
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)
	bondStore := store.NewInMemoryBondStore()

	cfg := DefaultConfig()
	srv := New(
		instrStore, orderStore, tradeStore, positionStore,
		nil, // settlementStore
		store.NewInMemoryCorporateActionStore(),
		store.NewInMemoryEntitlementStore(),
		store.NewInMemoryMarketStore(),
		store.NewInMemorySegmentStore(),
		store.NewInMemoryCircuitBreakerStore(),
		store.NewInMemoryFirmStore(),
		store.NewInMemoryParticipantStore(),
		nil, // tickTableStore
		nil, // tradeCorrectionStore
		nil, // throttleStore
		nil, // throttleConfigStore
		nil, // announcementStore
		nil, // auditStore
		nil, // pendingChangeStore
		nil, // referencePriceStore
		nil, // surveillanceStore
		nil, // instrumentGroupStore
		nil, // offBookTradeStore
		nil, // nodeStore
		nil, // locateStore
		nil, // rfqStore
		nil, // giveUpStore
		nil, // investigationStore
		nil, // replayStore
		bondStore,
		nil, // strategyStore
		nil, // custodyAccountStore
		nil, // custodyBalanceStore
		nil, // csdTransferStore
		nil, // dayManager
		me,
		nil, // sessionManager
		nil, // settlementEngine
		nil, // producer
		cfg,
	)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	ts := httptest.NewServer(tenantMW(mux))
	t.Cleanup(ts.Close)
	return ts, bondStore
}

// seedBond inserts a bond directly into the store.
func seedBond(t *testing.T, s *store.InMemoryBondStore, bond *types.Bond) {
	t.Helper()
	if err := s.Create(bond); err != nil {
		t.Fatalf("seedBond %s: %v", bond.ID, err)
	}
}

// createBondViaHTTP creates a bond via POST and returns its ID.
func createBondViaHTTP(t *testing.T, ts *httptest.Server, payload map[string]interface{}) string {
	t.Helper()
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/bonds", payload)
	assertStatus(t, resp, http.StatusCreated)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	id, _ := result["id"].(string)
	return id
}

// validBondPayload returns a complete valid bond creation payload.
func validBondPayload(id string) map[string]interface{} {
	return map[string]interface{}{
		"id":                   id,
		"isin":                 "MN1234567890",
		"name":                 "MN Telecom 5Y Bond",
		"issuer":               "MN Telecom",
		"maturity_date":        "2031-04-24",
		"coupon_rate":          0.05,
		"coupon_frequency":     "ANNUAL",
		"par_value":            1000.0,
		"day_count_convention": "ACT/365",
	}
}

// ============================================================
// TestCreateBond
// ============================================================

func TestCreateBond(t *testing.T) {
	ts, _ := newBondTestServer(t)

	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/bonds", validBondPayload("BOND-HTTP-1"))
	assertStatus(t, resp, http.StatusCreated)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	if result["id"] != "BOND-HTTP-1" {
		t.Errorf("id: want BOND-HTTP-1, got %v", result["id"])
	}
	if result["issuer"] != "MN Telecom" {
		t.Errorf("issuer: want MN Telecom, got %v", result["issuer"])
	}
	if result["coupon_rate"] != float64(0.05) {
		t.Errorf("coupon_rate: want 0.05, got %v", result["coupon_rate"])
	}
	if result["trading_status"] != "ACTIVE" {
		t.Errorf("trading_status: want ACTIVE, got %v", result["trading_status"])
	}
	if result["created_at"] == nil || result["created_at"] == "" {
		t.Error("created_at must be set on create")
	}
}

func TestCreateBond_MissingFields(t *testing.T) {
	ts, _ := newBondTestServer(t)

	// Missing isin.
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/bonds",
		map[string]interface{}{"id": "BOND-BAD", "issuer": "X", "par_value": 1000.0})
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()

	// Zero par value.
	resp = doJSON(t, ts, http.MethodPost, "/api/v1/securities/bonds",
		map[string]interface{}{"id": "BOND-ZERO", "isin": "MN000", "issuer": "X", "par_value": 0})
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestCreateBond_Duplicate(t *testing.T) {
	ts, _ := newBondTestServer(t)

	createBondViaHTTP(t, ts, validBondPayload("BOND-DUP"))

	// Second create with same ID should return 409.
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/bonds", validBondPayload("BOND-DUP"))
	assertStatus(t, resp, http.StatusConflict)
	resp.Body.Close()
}

// ============================================================
// TestListBonds
// ============================================================

func TestListBonds(t *testing.T) {
	ts, _ := newBondTestServer(t)

	// Empty initially.
	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/bonds", nil)
	assertStatus(t, resp, http.StatusOK)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["total"] != float64(0) {
		t.Errorf("initial total: want 0, got %v", result["total"])
	}

	// Create two bonds.
	createBondViaHTTP(t, ts, validBondPayload("BOND-L1"))
	p2 := validBondPayload("BOND-L2")
	p2["isin"] = "MN0987654321"
	p2["issuer"] = "MN Energy"
	p2["coupon_rate"] = 0.07
	p2["day_count_convention"] = "30/360"
	createBondViaHTTP(t, ts, p2)

	resp = doJSON(t, ts, http.MethodGet, "/api/v1/securities/bonds", nil)
	assertStatus(t, resp, http.StatusOK)
	decodeBody(t, resp, &result)

	if result["total"] != float64(2) {
		t.Errorf("total after 2 creates: want 2, got %v", result["total"])
	}
	data, _ := result["data"].([]interface{})
	if len(data) != 2 {
		t.Errorf("data length: want 2, got %d", len(data))
	}
}

// ============================================================
// Accrued interest tests
//
// Formula: accrued = coupon_rate * par_value * (days / basis)
//
// Test case: 90 days, coupon_rate=0.05, par_value=1000
//   ACT/360: 90 * 0.05 * 1000 / 360 = 12.50
//   ACT/365: 90 * 0.05 * 1000 / 365 = 12.33  (rounded to 2dp)
//   30/360:  30-day months → 3 months * 30 = 90 days → 90/360 → 12.50
// ============================================================

// approxEqual returns true if a and b differ by less than epsilon.
func approxEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

// TestAccruedInterest_ACT360 verifies: 90 * 0.05 * 1000 / 360 = 12.50
func TestAccruedInterest_ACT360(t *testing.T) {
	couponRate := 0.05
	parValue := 1000.0
	lastCoupon := time.Date(2026, 1, 24, 0, 0, 0, 0, time.UTC)
	settlement := time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC) // exactly 90 days later

	got := calcAccruedInterest(couponRate, parValue, types.DayCountACT360, lastCoupon, settlement)
	want := 12.50

	if !approxEqual(got, want, 0.005) {
		t.Errorf("ACT/360 accrued interest: want %.4f, got %.4f", want, got)
	}
	if roundTo2DP(got) != 12.50 {
		t.Errorf("ACT/360 rounded to 2dp: want 12.50, got %.2f", roundTo2DP(got))
	}
}

// TestAccruedInterest_ACT365 verifies: 90 * 0.05 * 1000 / 365 = 12.33 (rounded)
func TestAccruedInterest_ACT365(t *testing.T) {
	couponRate := 0.05
	parValue := 1000.0
	lastCoupon := time.Date(2026, 1, 24, 0, 0, 0, 0, time.UTC)
	settlement := time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC) // 90 days

	got := calcAccruedInterest(couponRate, parValue, types.DayCountACT365, lastCoupon, settlement)
	// 0.05 * 1000 * 90/365 = 12.328767...
	wantRaw := 0.05 * 1000 * 90.0 / 365.0

	if !approxEqual(got, wantRaw, 0.0001) {
		t.Errorf("ACT/365 accrued interest: want %.6f, got %.6f", wantRaw, got)
	}
	if roundTo2DP(got) != 12.33 {
		t.Errorf("ACT/365 rounded to 2dp: want 12.33, got %.2f", roundTo2DP(got))
	}
}

// TestAccruedInterest_30_360 verifies 30/360 convention.
// Using 3 full 30-day months: 30/360 standardised days = 3*30 = 90 days → 90/360 = 0.25
// accrued = 0.05 * 1000 * 0.25 = 12.50
func TestAccruedInterest_30_360(t *testing.T) {
	couponRate := 0.05
	parValue := 1000.0
	// Use dates that give exactly 90 standardised 30/360 days.
	// Jan 1 to Apr 1 = 3 months * 30 = 90 days in 30/360.
	lastCoupon := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	settlement := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	got := calcAccruedInterest(couponRate, parValue, types.DayCount30360, lastCoupon, settlement)
	want := 12.50 // 0.05 * 1000 * 90/360

	if !approxEqual(got, want, 0.005) {
		t.Errorf("30/360 accrued interest: want %.4f, got %.4f", want, got)
	}
	if roundTo2DP(got) != 12.50 {
		t.Errorf("30/360 rounded to 2dp: want 12.50, got %.2f", roundTo2DP(got))
	}
}

// TestAccruedInterest_ViaHTTP tests the full HTTP endpoint for accrued interest.
func TestAccruedInterest_ViaHTTP(t *testing.T) {
	ts, bStore := newBondTestServer(t)

	seedBond(t, bStore, &types.Bond{
		ID:                 "BOND-AI-1",
		ISIN:               "MN1111111111",
		Name:               "Test Bond",
		Issuer:             "Test Issuer",
		MaturityDate:       "2031-04-24",
		CouponRate:         0.05,
		CouponFrequency:    "ANNUAL",
		ParValue:           1000.0,
		DayCountConvention: types.DayCountACT360,
		TradingStatus:      types.TradingStatusActive,
		CreatedAt:          "2026-04-24T00:00:00Z",
		UpdatedAt:          "2026-04-24T00:00:00Z",
	})

	// 90 days, ACT/360 → 12.50
	resp := doJSON(t, ts, http.MethodGet,
		"/api/v1/securities/bonds/BOND-AI-1/accrued-interest?last_coupon_date=2026-01-24&settlement_date=2026-04-24",
		nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	if result["bond_id"] != "BOND-AI-1" {
		t.Errorf("bond_id: want BOND-AI-1, got %v", result["bond_id"])
	}
	accruedRaw, ok := result["accrued_interest"].(float64)
	if !ok {
		t.Fatalf("accrued_interest not a float: %v", result["accrued_interest"])
	}
	if accruedRaw != 12.50 {
		t.Errorf("accrued_interest: want 12.50, got %.4f", accruedRaw)
	}
}

func TestAccruedInterest_MissingParams(t *testing.T) {
	ts, bStore := newBondTestServer(t)

	seedBond(t, bStore, &types.Bond{
		ID: "BOND-AI-2", ISIN: "MN2222222222", Issuer: "X", ParValue: 1000,
		DayCountConvention: types.DayCountACT365, TradingStatus: types.TradingStatusActive,
		CreatedAt: "2026-04-24T00:00:00Z", UpdatedAt: "2026-04-24T00:00:00Z",
	})

	// Missing last_coupon_date.
	resp := doJSON(t, ts, http.MethodGet,
		"/api/v1/securities/bonds/BOND-AI-2/accrued-interest?settlement_date=2026-04-24", nil)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()

	// Missing settlement_date.
	resp = doJSON(t, ts, http.MethodGet,
		"/api/v1/securities/bonds/BOND-AI-2/accrued-interest?last_coupon_date=2026-01-24", nil)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestAccruedInterest_BondNotFound(t *testing.T) {
	ts, _ := newBondTestServer(t)

	resp := doJSON(t, ts, http.MethodGet,
		"/api/v1/securities/bonds/NO-BOND/accrued-interest?last_coupon_date=2026-01-24&settlement_date=2026-04-24",
		nil)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// ============================================================
// TestBondEndpoints_NotConfigured (503)
// ============================================================

func TestBondEndpoints_NotConfigured(t *testing.T) {
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)
	cfg := DefaultConfig()
	srv := New(
		instrStore, orderStore, tradeStore, positionStore,
		nil, store.NewInMemoryCorporateActionStore(), store.NewInMemoryEntitlementStore(),
		store.NewInMemoryMarketStore(), store.NewInMemorySegmentStore(),
		store.NewInMemoryCircuitBreakerStore(), store.NewInMemoryFirmStore(),
		store.NewInMemoryParticipantStore(),
		nil, nil, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil,
		nil,  // investigationStore
		nil,  // replayStore
		nil,  // bondStore = nil
		nil, nil, nil, nil, // strategyStore, custodyAccountStore, custodyBalanceStore, csdTransferStore
		nil, me, nil, nil, nil, cfg,
	)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)
	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	httpTS := httptest.NewServer(tenantMW(mux))
	t.Cleanup(httpTS.Close)

	paths := []string{
		"/api/v1/securities/bonds",
		"/api/v1/securities/bonds/some-id",
		"/api/v1/securities/bonds/some-id/accrued-interest",
	}
	methods := []string{http.MethodGet, http.MethodGet, http.MethodGet}

	for i, path := range paths {
		resp := doJSON(t, httpTS, methods[i], path, nil)
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("path %s: expected 503, got %d", path, resp.StatusCode)
		}
		resp.Body.Close()
	}
}
