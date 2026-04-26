// Package server — internal tests for order HTTP handlers.
package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/middleware"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// ============================================================
// Order test helpers
// ============================================================

// orderTickerSeq generates unique tickers for concurrent/repeated test runs.
var orderTickerSeq atomic.Int64

func nextOrderTicker() string {
	n := orderTickerSeq.Add(1)
	return fmt.Sprintf("OT%04d", n)
}

// createInstrForOrders creates an instrument with specified lot_size and tick_size.
func createInstrForOrders(t *testing.T, ts *httptest.Server, lotSize int, tickSize float64) string {
	t.Helper()
	ticker := nextOrderTicker()
	payload := map[string]interface{}{
		"ticker":      ticker,
		"name":        ticker + " Corp",
		"asset_class": "EQUITY",
		"lot_size":    lotSize,
		"tick_size":   tickSize,
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/instruments", payload)
	assertStatus(t, resp, http.StatusCreated)
	var created map[string]interface{}
	decodeBody(t, resp, &created)
	return created["id"].(string)
}

// haltInstr halts an instrument on the test server.
func haltInstr(t *testing.T, ts *httptest.Server, instrID string) {
	t.Helper()
	resp := doJSON(t, ts, http.MethodPut,
		fmt.Sprintf("/api/v1/securities/instruments/%s/status", instrID),
		map[string]interface{}{"status": "HALTED"})
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

// submitBuyOrder submits a BUY order and returns the order ID.
func submitBuyOrder(t *testing.T, ts *httptest.Server, instrID string, orderType string, qty int, price, stopPrice float64) string {
	t.Helper()
	payload := map[string]interface{}{
		"instrument_id":  instrID,
		"participant_id": "P-001",
		"side":           "BUY",
		"order_type":     orderType,
		"quantity":       qty,
	}
	if price > 0 {
		payload["price"] = price
	}
	if stopPrice > 0 {
		payload["stop_price"] = stopPrice
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/orders", payload)
	assertStatus(t, resp, http.StatusCreated)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	order := result["order"].(map[string]interface{})
	return order["id"].(string)
}

// newServerWithFilledOrder creates a server pre-seeded with a FILLED order.
// Returns (orderID, *httptest.Server).
func newServerWithFilledOrder(t *testing.T) (string, *httptest.Server) {
	t.Helper()
	iStore := store.NewInMemoryInstrumentStore()
	oStore := store.NewInMemoryOrderStore()

	inst := &types.Instrument{
		ID:            "pre-inst-filled",
		Ticker:        "PFIL",
		Name:          "Pre-seeded Filled",
		AssetClass:    types.AssetClassEquity,
		TradingStatus: types.TradingStatusActive,
		LotSize:       5,
		TickSize:      0.05,
		CreatedAt:     "2024-01-01T00:00:00Z",
		UpdatedAt:     "2024-01-01T00:00:00Z",
	}
	if err := iStore.Create(inst); err != nil {
		t.Fatalf("pre-seed instrument: %v", err)
	}

	filledOrder := &types.SecurityOrder{
		ID:             "pre-order-filled",
		InstrumentID:   "pre-inst-filled",
		ParticipantID:  "P-001",
		Side:           types.OrderSideBuy,
		OrderType:      types.OrderTypeMarket,
		Quantity:       5,
		Status:         types.OrderStatusFilled,
		TimeInForce:    types.TimeInForceGTC,
		FilledQuantity: 5,
		AvgFillPrice:   10.00,
		CreatedAt:      "2024-01-01T00:00:00Z",
		UpdatedAt:      "2024-01-01T00:00:00Z",
	}
	if err := oStore.Submit(filledOrder); err != nil {
		t.Fatalf("pre-seed order: %v", err)
	}

	ts := newTestServerWithStores(t, iStore, oStore)
	return "pre-order-filled", ts
}

// ============================================================
// TestSubmitOrder
// ============================================================

func TestSubmitOrder_Success(t *testing.T) {
	ts := newTestServer(t)
	instrID := createInstrForOrders(t, ts, 5, 0.05)

	payload := map[string]interface{}{
		"instrument_id":  instrID,
		"participant_id": "P-001",
		"side":           "BUY",
		"order_type":     "LIMIT",
		"quantity":       10, // 10 % 5 == 0 ✓
		"price":          10.00, // 10.00 % 0.05 == 0 ✓
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/orders", payload)
	assertStatus(t, resp, http.StatusCreated)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	order := result["order"].(map[string]interface{})
	if id, ok := order["id"].(string); !ok || id == "" {
		t.Error("expected non-empty id in response")
	}
	if order["status"] != "PENDING" {
		t.Errorf("expected status PENDING, got %v", order["status"])
	}
	if order["time_in_force"] != "GTC" {
		t.Errorf("expected default time_in_force GTC, got %v", order["time_in_force"])
	}
	if order["filled_quantity"] != float64(0) {
		t.Errorf("expected filled_quantity 0, got %v", order["filled_quantity"])
	}
	// Verify trades array is present.
	trades, ok := result["trades"].([]interface{})
	if !ok {
		t.Error("expected 'trades' array in response")
	}
	if len(trades) != 0 {
		t.Errorf("expected 0 trades for unmatched order, got %d", len(trades))
	}
}

func TestSubmitOrder_LotSizeValidation(t *testing.T) {
	ts := newTestServer(t)
	instrID := createInstrForOrders(t, ts, 5, 0.05)

	// quantity=7 is NOT a multiple of lot_size=5.
	payload := map[string]interface{}{
		"instrument_id":  instrID,
		"participant_id": "P-001",
		"side":           "BUY",
		"order_type":     "LIMIT",
		"quantity":       7,
		"price":          10.00,
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/orders", payload)
	assertStatus(t, resp, http.StatusUnprocessableEntity)

	var errResp types.ErrorResponse
	decodeBody(t, resp, &errResp)
	if errResp.Error.Code != "INVALID_LOT_SIZE" {
		t.Errorf("expected INVALID_LOT_SIZE, got %q", errResp.Error.Code)
	}
	if !strings.Contains(errResp.Error.Message, "lot_size") {
		t.Errorf("expected message containing 'lot_size', got %q", errResp.Error.Message)
	}
}

func TestSubmitOrder_TickSizeValidation(t *testing.T) {
	ts := newTestServer(t)
	instrID := createInstrForOrders(t, ts, 5, 0.05)

	// price=10.03 is NOT a multiple of tick_size=0.05.
	payload := map[string]interface{}{
		"instrument_id":  instrID,
		"participant_id": "P-001",
		"side":           "BUY",
		"order_type":     "LIMIT",
		"quantity":       5,
		"price":          10.03,
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/orders", payload)
	assertStatus(t, resp, http.StatusUnprocessableEntity)

	var errResp types.ErrorResponse
	decodeBody(t, resp, &errResp)
	if errResp.Error.Code != "INVALID_TICK_SIZE" {
		t.Errorf("expected INVALID_TICK_SIZE, got %q", errResp.Error.Code)
	}
	if !strings.Contains(errResp.Error.Message, "tick_size") {
		t.Errorf("expected message containing 'tick_size', got %q", errResp.Error.Message)
	}
}

func TestSubmitOrder_InstrumentNotActive(t *testing.T) {
	ts := newTestServer(t)
	instrID := createInstrForOrders(t, ts, 5, 0.05)
	haltInstr(t, ts, instrID)

	payload := map[string]interface{}{
		"instrument_id":  instrID,
		"participant_id": "P-001",
		"side":           "BUY",
		"order_type":     "LIMIT",
		"quantity":       5,
		"price":          10.00,
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/orders", payload)
	assertStatus(t, resp, http.StatusUnprocessableEntity)

	var errResp types.ErrorResponse
	decodeBody(t, resp, &errResp)
	if errResp.Error.Code != "INSTRUMENT_NOT_ACTIVE" {
		t.Errorf("expected INSTRUMENT_NOT_ACTIVE, got %q", errResp.Error.Code)
	}
}

func TestSubmitOrder_InstrumentNotFound(t *testing.T) {
	ts := newTestServer(t)

	payload := map[string]interface{}{
		"instrument_id":  "no-such-instrument",
		"participant_id": "P-001",
		"side":           "BUY",
		"order_type":     "LIMIT",
		"quantity":       5,
		"price":          10.00,
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/orders", payload)
	assertStatus(t, resp, http.StatusUnprocessableEntity)

	var errResp types.ErrorResponse
	decodeBody(t, resp, &errResp)
	if errResp.Error.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %q", errResp.Error.Code)
	}
}

func TestSubmitOrder_MissingInstrumentID(t *testing.T) {
	ts := newTestServer(t)

	payload := map[string]interface{}{
		"participant_id": "P-001",
		"side":           "BUY",
		"order_type":     "LIMIT",
		"quantity":       5,
		"price":          10.00,
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/orders", payload)
	assertStatus(t, resp, http.StatusUnprocessableEntity)

	var errResp types.ErrorResponse
	decodeBody(t, resp, &errResp)
	if errResp.Error.Code != "MISSING_FIELD" {
		t.Errorf("expected MISSING_FIELD, got %q", errResp.Error.Code)
	}
}

func TestSubmitOrder_MarketOrder(t *testing.T) {
	ts := newTestServer(t)
	instrID := createInstrForOrders(t, ts, 5, 0.05)

	// MARKET order — no price required.
	payload := map[string]interface{}{
		"instrument_id":  instrID,
		"participant_id": "P-001",
		"side":           "BUY",
		"order_type":     "MARKET",
		"quantity":       5,
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/orders", payload)
	assertStatus(t, resp, http.StatusCreated)

	var result map[string]interface{}
	decodeBody(t, resp, &result)
	order := result["order"].(map[string]interface{})
	if order["order_type"] != "MARKET" {
		t.Errorf("expected order_type MARKET, got %v", order["order_type"])
	}
}

func TestSubmitOrder_StopLimitMissingStopPrice(t *testing.T) {
	ts := newTestServer(t)
	instrID := createInstrForOrders(t, ts, 5, 0.05)

	// STOP_LIMIT without stop_price (defaults to 0 when JSON-decoded).
	payload := map[string]interface{}{
		"instrument_id":  instrID,
		"participant_id": "P-001",
		"side":           "BUY",
		"order_type":     "STOP_LIMIT",
		"quantity":       5,
		"price":          10.00,
		// stop_price absent → 0 → fails > 0 validation
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/orders", payload)
	assertStatus(t, resp, http.StatusUnprocessableEntity)

	var errResp types.ErrorResponse
	decodeBody(t, resp, &errResp)
	if errResp.Error.Code != "INVALID_FIELD" {
		t.Errorf("expected INVALID_FIELD for missing stop_price, got %q", errResp.Error.Code)
	}
}

func TestSubmitOrder_StopMissingStopPrice(t *testing.T) {
	ts := newTestServer(t)
	instrID := createInstrForOrders(t, ts, 5, 0.05)

	payload := map[string]interface{}{
		"instrument_id":  instrID,
		"participant_id": "P-001",
		"side":           "BUY",
		"order_type":     "STOP",
		"quantity":       5,
		// stop_price absent → 0
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/orders", payload)
	assertStatus(t, resp, http.StatusUnprocessableEntity)
	var errResp types.ErrorResponse
	decodeBody(t, resp, &errResp)
	if errResp.Error.Code != "INVALID_FIELD" {
		t.Errorf("expected INVALID_FIELD, got %q", errResp.Error.Code)
	}
}

func TestSubmitOrder_StopWithStopPrice_Success(t *testing.T) {
	ts := newTestServer(t)
	instrID := createInstrForOrders(t, ts, 5, 0.05)

	payload := map[string]interface{}{
		"instrument_id":  instrID,
		"participant_id": "P-001",
		"side":           "BUY",
		"order_type":     "STOP",
		"quantity":       5,
		"stop_price":     9.50,
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/orders", payload)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
}

func TestSubmitOrder_ShortSellRequiresLocate(t *testing.T) {
	ts := newTestServer(t)
	instrID := createInstrForOrders(t, ts, 5, 0.05)

	// SHORT_SELL without a locate_id should return LOCATE_REQUIRED.
	payload := map[string]interface{}{
		"instrument_id":  instrID,
		"participant_id": "P-001",
		"side":           "SHORT_SELL",
		"order_type":     "LIMIT",
		"quantity":       5,
		"price":          10.00,
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/orders", payload)
	assertStatus(t, resp, http.StatusUnprocessableEntity)

	var errResp types.ErrorResponse
	decodeBody(t, resp, &errResp)
	if errResp.Error.Code != "LOCATE_REQUIRED" {
		t.Errorf("expected LOCATE_REQUIRED, got %q", errResp.Error.Code)
	}
}

func TestSubmitOrder_InvalidSide(t *testing.T) {
	ts := newTestServer(t)
	instrID := createInstrForOrders(t, ts, 5, 0.05)

	payload := map[string]interface{}{
		"instrument_id":  instrID,
		"participant_id": "P-001",
		"side":           "WRONG_SIDE",
		"order_type":     "LIMIT",
		"quantity":       5,
		"price":          10.00,
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/orders", payload)
	assertStatus(t, resp, http.StatusUnprocessableEntity)
	var errResp types.ErrorResponse
	decodeBody(t, resp, &errResp)
	if errResp.Error.Code != "INVALID_FIELD" {
		t.Errorf("expected INVALID_FIELD for invalid side, got %q", errResp.Error.Code)
	}
}

func TestSubmitOrder_InvalidOrderType(t *testing.T) {
	ts := newTestServer(t)
	instrID := createInstrForOrders(t, ts, 5, 0.05)

	payload := map[string]interface{}{
		"instrument_id":  instrID,
		"participant_id": "P-001",
		"side":           "BUY",
		"order_type":     "FLASH_ORDER",
		"quantity":       5,
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/orders", payload)
	assertStatus(t, resp, http.StatusUnprocessableEntity)
	var errResp types.ErrorResponse
	decodeBody(t, resp, &errResp)
	if errResp.Error.Code != "INVALID_FIELD" {
		t.Errorf("expected INVALID_FIELD for invalid order_type, got %q", errResp.Error.Code)
	}
}

func TestSubmitOrder_ZeroQuantity(t *testing.T) {
	ts := newTestServer(t)
	instrID := createInstrForOrders(t, ts, 5, 0.05)

	payload := map[string]interface{}{
		"instrument_id":  instrID,
		"participant_id": "P-001",
		"side":           "BUY",
		"order_type":     "MARKET",
		"quantity":       0,
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/orders", payload)
	assertStatus(t, resp, http.StatusUnprocessableEntity)
	resp.Body.Close()
}

func TestSubmitOrder_SellSuccess(t *testing.T) {
	ts := newTestServer(t)
	instrID := createInstrForOrders(t, ts, 5, 0.05)

	payload := map[string]interface{}{
		"instrument_id":  instrID,
		"participant_id": "P-001",
		"side":           "SELL",
		"order_type":     "LIMIT",
		"quantity":       5,
		"price":          10.00,
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/orders", payload)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
}

// ============================================================
// TestGetOrder
// ============================================================

func TestGetOrder_Success(t *testing.T) {
	ts := newTestServer(t)
	instrID := createInstrForOrders(t, ts, 5, 0.05)
	orderID := submitBuyOrder(t, ts, instrID, "MARKET", 5, 0, 0)

	resp := doJSON(t, ts, http.MethodGet,
		fmt.Sprintf("/api/v1/securities/orders/%s", orderID), nil)
	assertStatus(t, resp, http.StatusOK)

	var got map[string]interface{}
	decodeBody(t, resp, &got)
	if got["id"] != orderID {
		t.Errorf("expected id %q, got %v", orderID, got["id"])
	}
}

func TestGetOrder_NotFound(t *testing.T) {
	ts := newTestServer(t)

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/orders/nonexistent-id", nil)
	assertStatus(t, resp, http.StatusNotFound)

	var errResp types.ErrorResponse
	decodeBody(t, resp, &errResp)
	if errResp.Error.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %q", errResp.Error.Code)
	}
}

// ============================================================
// TestListOrders
// ============================================================

func TestListOrders_Empty(t *testing.T) {
	ts := newTestServer(t)

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/orders", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	data, ok := result["data"]
	if !ok {
		t.Fatal("expected 'data' key in response")
	}
	arr, ok := data.([]interface{})
	if !ok {
		t.Fatalf("expected data to be JSON array, got %T", data)
	}
	if len(arr) != 0 {
		t.Errorf("expected empty array, got %d items", len(arr))
	}
}

func TestListOrders_WithInstrumentIDFilter(t *testing.T) {
	ts := newTestServer(t)

	instrA := createInstrForOrders(t, ts, 5, 0.05)
	instrB := createInstrForOrders(t, ts, 5, 0.05)

	// 2 orders for instrA, 1 for instrB.
	submitBuyOrder(t, ts, instrA, "MARKET", 5, 0, 0)
	submitBuyOrder(t, ts, instrA, "MARKET", 5, 0, 0)
	submitBuyOrder(t, ts, instrB, "MARKET", 5, 0, 0)

	t.Run("filter by instrument_id A", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet,
			fmt.Sprintf("/api/v1/securities/orders?instrument_id=%s", instrA), nil)
		assertStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		data := result["data"].([]interface{})
		if len(data) != 2 {
			t.Errorf("expected 2 orders for instrA, got %d", len(data))
		}
	})

	t.Run("filter by instrument_id B", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet,
			fmt.Sprintf("/api/v1/securities/orders?instrument_id=%s", instrB), nil)
		assertStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		data := result["data"].([]interface{})
		if len(data) != 1 {
			t.Errorf("expected 1 order for instrB, got %d", len(data))
		}
	})

	t.Run("list all", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/orders", nil)
		assertStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		data := result["data"].([]interface{})
		if len(data) != 3 {
			t.Errorf("expected 3 total orders, got %d", len(data))
		}
	})
}

// ============================================================
// TestCancelOrder
// ============================================================

func TestCancelOrder_Success(t *testing.T) {
	ts := newTestServer(t)
	instrID := createInstrForOrders(t, ts, 5, 0.05)
	orderID := submitBuyOrder(t, ts, instrID, "MARKET", 5, 0, 0)

	resp := doJSON(t, ts, http.MethodDelete,
		fmt.Sprintf("/api/v1/securities/orders/%s", orderID), nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["status"] != "CANCELLED" {
		t.Errorf("expected status CANCELLED, got %v", result["status"])
	}
}

func TestCancelOrder_AlreadyCancelled(t *testing.T) {
	ts := newTestServer(t)
	instrID := createInstrForOrders(t, ts, 5, 0.05)
	orderID := submitBuyOrder(t, ts, instrID, "MARKET", 5, 0, 0)

	// First cancel — should succeed.
	resp := doJSON(t, ts, http.MethodDelete,
		fmt.Sprintf("/api/v1/securities/orders/%s", orderID), nil)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// Second cancel — should return 409.
	resp2 := doJSON(t, ts, http.MethodDelete,
		fmt.Sprintf("/api/v1/securities/orders/%s", orderID), nil)
	assertStatus(t, resp2, http.StatusConflict)

	var errResp types.ErrorResponse
	decodeBody(t, resp2, &errResp)
	if errResp.Error.Code != "INVALID_STATE" {
		t.Errorf("expected INVALID_STATE, got %q", errResp.Error.Code)
	}
}

func TestCancelOrder_NotFound(t *testing.T) {
	ts := newTestServer(t)

	resp := doJSON(t, ts, http.MethodDelete, "/api/v1/securities/orders/no-such-order", nil)
	assertStatus(t, resp, http.StatusNotFound)

	var errResp types.ErrorResponse
	decodeBody(t, resp, &errResp)
	if errResp.Error.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %q", errResp.Error.Code)
	}
}

func TestCancelOrder_FilledOrderRejected(t *testing.T) {
	orderID, ts := newServerWithFilledOrder(t)

	resp := doJSON(t, ts, http.MethodDelete,
		fmt.Sprintf("/api/v1/securities/orders/%s", orderID), nil)
	assertStatus(t, resp, http.StatusConflict)

	var errResp types.ErrorResponse
	decodeBody(t, resp, &errResp)
	if errResp.Error.Code != "INVALID_STATE" {
		t.Errorf("expected INVALID_STATE for FILLED order, got %q", errResp.Error.Code)
	}
}

// ============================================================
// Method not allowed for orders
// ============================================================

func TestMethodNotAllowed_Orders(t *testing.T) {
	ts := newTestServer(t)

	resp := doJSON(t, ts, http.MethodDelete, "/api/v1/securities/orders", nil)
	assertStatus(t, resp, http.StatusMethodNotAllowed)

	var errResp types.ErrorResponse
	decodeBody(t, resp, &errResp)
	if errResp.Error.Code != "METHOD_NOT_ALLOWED" {
		t.Errorf("expected METHOD_NOT_ALLOWED, got %q", errResp.Error.Code)
	}
}

func TestMethodNotAllowed_Order(t *testing.T) {
	ts := newTestServer(t)
	instrID := createInstrForOrders(t, ts, 5, 0.05)
	orderID := submitBuyOrder(t, ts, instrID, "MARKET", 5, 0, 0)

	resp := doJSON(t, ts, http.MethodPost,
		fmt.Sprintf("/api/v1/securities/orders/%s", orderID), nil)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	resp.Body.Close()
}

// ============================================================
// Order validation tests — TradingParameterSet enforcement
// ============================================================

// newServerWithParamSet builds a test server with a TradingParameterSet wired
// for a specific instrument. The param set restricts: AllowedOrderTypes, min/max
// order size, and max order value.  Returns (ts, instrumentID).
func newServerWithParamSet(t *testing.T, ps *types.TradingParameterSet) (*httptest.Server, string) {
	t.Helper()

	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	paramStore := store.NewInMemoryTradingParamSetStore()

	// Seed instrument with lot_size=1, tick_size=0.01 for easy test math.
	inst := &types.Instrument{
		ID:            "inst-param-test",
		Ticker:        "PTEST",
		Name:          "Param Test Corp",
		AssetClass:    types.AssetClassEquity,
		TradingStatus: types.TradingStatusActive,
		LotSize:       1,
		TickSize:      0.01,
		CreatedAt:     "2024-01-01T00:00:00Z",
		UpdatedAt:     "2024-01-01T00:00:00Z",
	}
	if err := instrStore.Create(inst); err != nil {
		t.Fatalf("seed instrument: %v", err)
	}

	// Seed param set linked to the instrument.
	ps.InstrumentID = inst.ID
	if err := paramStore.Create(ps); err != nil {
		t.Fatalf("seed param set: %v", err)
	}

	cfg := DefaultConfig()
	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)
	srv := New(
		instrStore, orderStore, tradeStore, positionStore,
		nil,
		store.NewInMemoryCorporateActionStore(),
		store.NewInMemoryEntitlementStore(),
		store.NewInMemoryMarketStore(),
		store.NewInMemorySegmentStore(),
		store.NewInMemoryCircuitBreakerStore(),
		store.NewInMemoryFirmStore(),
		store.NewInMemoryParticipantStore(),
		nil, nil, nil, nil,
		nil, nil, nil, nil,
		nil, nil, nil,
		nil,                // nodeStore
		nil,                // locateStore
		nil,                // rfqStore
		nil,                // giveUpStore
		nil, nil, nil,
		nil, nil, nil, nil,
		nil, nil, nil,
		nil, me, nil, nil,
		nil, nil, nil,
		paramStore,
		cfg,
	)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	ts := httptest.NewServer(tenantMW(mux))
	t.Cleanup(ts.Close)
	return ts, inst.ID
}

// submitOrderRaw submits an order and returns the response (caller must close body).
func submitOrderRaw(t *testing.T, ts *httptest.Server, payload map[string]interface{}) *http.Response {
	t.Helper()
	return doJSON(t, ts, http.MethodPost, "/api/v1/securities/orders", payload)
}

// TestOrderValidation_AllowedOrderType verifies that a LIMIT order succeeds
// when LIMIT is in the AllowedOrderTypes list.
func TestOrderValidation_AllowedOrderType(t *testing.T) {
	ps := &types.TradingParameterSet{
		ID:                "ps-allowed",
		Name:              "LIMIT only",
		AllowedOrderTypes: []string{"LIMIT"},
		MinOrderSize:      1,
		MaxOrderSize:      10000,
	}
	ts, instrID := newServerWithParamSet(t, ps)

	payload := map[string]interface{}{
		"instrument_id":  instrID,
		"participant_id": "P-01",
		"side":           "BUY",
		"order_type":     "LIMIT",
		"quantity":       100,
		"price":          10.00,
	}
	resp := submitOrderRaw(t, ts, payload)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
}

// TestOrderValidation_DisallowedOrderType verifies that a STOP order is rejected
// with 422 ORDER_TYPE_NOT_ALLOWED when STOP is absent from AllowedOrderTypes.
func TestOrderValidation_DisallowedOrderType(t *testing.T) {
	ps := &types.TradingParameterSet{
		ID:                "ps-disallow",
		Name:              "LIMIT only",
		AllowedOrderTypes: []string{"LIMIT"},
		MinOrderSize:      1,
		MaxOrderSize:      10000,
	}
	ts, instrID := newServerWithParamSet(t, ps)

	payload := map[string]interface{}{
		"instrument_id":  instrID,
		"participant_id": "P-01",
		"side":           "BUY",
		"order_type":     "STOP",
		"quantity":       100,
		"stop_price":     10.00,
	}
	resp := submitOrderRaw(t, ts, payload)
	assertStatus(t, resp, http.StatusUnprocessableEntity)

	var errResp types.ErrorResponse
	decodeBody(t, resp, &errResp)
	if errResp.Error.Code != "ORDER_TYPE_NOT_ALLOWED" {
		t.Errorf("expected ORDER_TYPE_NOT_ALLOWED, got %q", errResp.Error.Code)
	}
}

// TestOrderValidation_MinOrderSize verifies that an order below MinOrderSize
// is rejected with 422 ORDER_TOO_SMALL.
func TestOrderValidation_MinOrderSize(t *testing.T) {
	ps := &types.TradingParameterSet{
		ID:           "ps-min",
		Name:         "Min size 50",
		MinOrderSize: 50,
		MaxOrderSize: 10000,
	}
	ts, instrID := newServerWithParamSet(t, ps)

	payload := map[string]interface{}{
		"instrument_id":  instrID,
		"participant_id": "P-01",
		"side":           "BUY",
		"order_type":     "MARKET",
		"quantity":       10, // below min of 50
	}
	resp := submitOrderRaw(t, ts, payload)
	assertStatus(t, resp, http.StatusUnprocessableEntity)

	var errResp types.ErrorResponse
	decodeBody(t, resp, &errResp)
	if errResp.Error.Code != "ORDER_TOO_SMALL" {
		t.Errorf("expected ORDER_TOO_SMALL, got %q", errResp.Error.Code)
	}
}

// TestOrderValidation_MaxOrderSize verifies that an order above MaxOrderSize
// is rejected with 422 ORDER_TOO_LARGE.
func TestOrderValidation_MaxOrderSize(t *testing.T) {
	ps := &types.TradingParameterSet{
		ID:           "ps-max",
		Name:         "Max size 100",
		MinOrderSize: 1,
		MaxOrderSize: 100,
	}
	ts, instrID := newServerWithParamSet(t, ps)

	payload := map[string]interface{}{
		"instrument_id":  instrID,
		"participant_id": "P-01",
		"side":           "BUY",
		"order_type":     "MARKET",
		"quantity":       500, // above max of 100
	}
	resp := submitOrderRaw(t, ts, payload)
	assertStatus(t, resp, http.StatusUnprocessableEntity)

	var errResp types.ErrorResponse
	decodeBody(t, resp, &errResp)
	if errResp.Error.Code != "ORDER_TOO_LARGE" {
		t.Errorf("expected ORDER_TOO_LARGE, got %q", errResp.Error.Code)
	}
}

// TestOrderValidation_MaxOrderValue verifies that price*quantity > MaxOrderValue
// is rejected with 422 ORDER_VALUE_EXCEEDED.
func TestOrderValidation_MaxOrderValue(t *testing.T) {
	ps := &types.TradingParameterSet{
		ID:            "ps-maxval",
		Name:          "Max value 1000",
		MinOrderSize:  1,
		MaxOrderSize:  10000,
		MaxOrderValue: 1000.0, // price*qty must not exceed 1000
	}
	ts, instrID := newServerWithParamSet(t, ps)

	// price=10.00, qty=200 → value=2000 > 1000
	payload := map[string]interface{}{
		"instrument_id":  instrID,
		"participant_id": "P-01",
		"side":           "BUY",
		"order_type":     "LIMIT",
		"quantity":       200,
		"price":          10.00,
	}
	resp := submitOrderRaw(t, ts, payload)
	assertStatus(t, resp, http.StatusUnprocessableEntity)

	var errResp types.ErrorResponse
	decodeBody(t, resp, &errResp)
	if errResp.Error.Code != "ORDER_VALUE_EXCEEDED" {
		t.Errorf("expected ORDER_VALUE_EXCEEDED, got %q", errResp.Error.Code)
	}
}
