// Package server — internal tests for order HTTP handlers.
package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

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
	return result["id"].(string)
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

	if id, ok := result["id"].(string); !ok || id == "" {
		t.Error("expected non-empty id in response")
	}
	if result["status"] != "PENDING" {
		t.Errorf("expected status PENDING, got %v", result["status"])
	}
	if result["time_in_force"] != "GTC" {
		t.Errorf("expected default time_in_force GTC, got %v", result["time_in_force"])
	}
	if result["filled_quantity"] != float64(0) {
		t.Errorf("expected filled_quantity 0, got %v", result["filled_quantity"])
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
	if result["order_type"] != "MARKET" {
		t.Errorf("expected order_type MARKET, got %v", result["order_type"])
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

func TestSubmitOrder_ShortSellDisabled(t *testing.T) {
	ts := newTestServer(t)
	instrID := createInstrForOrders(t, ts, 5, 0.05)

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
	if errResp.Error.Code != "SHORT_SELL_DISABLED" {
		t.Errorf("expected SHORT_SELL_DISABLED, got %q", errResp.Error.Code)
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
