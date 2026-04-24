// Package server — tests for service-desk HTTP handlers (P3b).
package server

import (
	"net/http"
	"testing"
)

// ============================================================
// TestServiceDeskSubmitOrder
// ============================================================

// TestServiceDeskSubmitOrder verifies that a valid operator-submitted order
// returns HTTP 201 and the created order in the response body.
func TestServiceDeskSubmitOrder(t *testing.T) {
	ts := newTestServer(t)

	instrID := createInstr(t, ts, map[string]interface{}{
		"ticker":      "SD01",
		"name":        "ServiceDesk Instrument One",
		"asset_class": "EQUITY",
		"lot_size":    100,
		"tick_size":   0.01,
	})

	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/service-desk/orders", map[string]interface{}{
		"participant_id": "PART-001",
		"instrument_id":  instrID,
		"side":           "BUY",
		"order_type":     "LIMIT",
		"quantity":       500,
		"price":          12.75,
		"time_in_force":  "GTC",
	})
	assertStatus(t, resp, http.StatusCreated)

	var order map[string]interface{}
	decodeBody(t, resp, &order)

	if id, ok := order["id"].(string); !ok || id == "" {
		t.Error("expected non-empty id in created order")
	}
	if order["participant_id"] != "PART-001" {
		t.Errorf("expected participant_id PART-001, got %v", order["participant_id"])
	}
	if order["instrument_id"] != instrID {
		t.Errorf("expected instrument_id %q, got %v", instrID, order["instrument_id"])
	}
	if order["status"] != "PENDING" {
		t.Errorf("expected status PENDING, got %v", order["status"])
	}
	if order["side"] != "BUY" {
		t.Errorf("expected side BUY, got %v", order["side"])
	}
}

// TestServiceDeskSubmitOrder_MissingParticipant verifies 400 when participant_id is absent.
func TestServiceDeskSubmitOrder_MissingParticipant(t *testing.T) {
	ts := newTestServer(t)

	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/service-desk/orders", map[string]interface{}{
		"instrument_id": "some-instr",
		"side":          "BUY",
		"order_type":    "LIMIT",
		"quantity":      100,
		"price":         10.00,
	})
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

// TestServiceDeskSubmitOrder_MissingInstrument verifies 400 when instrument_id is absent.
func TestServiceDeskSubmitOrder_MissingInstrument(t *testing.T) {
	ts := newTestServer(t)

	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/service-desk/orders", map[string]interface{}{
		"participant_id": "PART-001",
		"side":           "BUY",
		"order_type":     "LIMIT",
		"quantity":       100,
		"price":          10.00,
	})
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

// ============================================================
// TestServiceDeskCancelOrder
// ============================================================

// TestServiceDeskCancelOrder submits an order via service-desk, then cancels
// it and verifies HTTP 200 with the cancelled order in the response body.
func TestServiceDeskCancelOrder(t *testing.T) {
	ts := newTestServer(t)

	instrID := createInstr(t, ts, map[string]interface{}{
		"ticker":      "SD02",
		"name":        "ServiceDesk Instrument Two",
		"asset_class": "EQUITY",
		"lot_size":    100,
		"tick_size":   0.01,
	})

	// Submit the order.
	submitResp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/service-desk/orders", map[string]interface{}{
		"participant_id": "PART-002",
		"instrument_id":  instrID,
		"side":           "SELL",
		"order_type":     "LIMIT",
		"quantity":       200,
		"price":          15.00,
		"time_in_force":  "DAY",
	})
	assertStatus(t, submitResp, http.StatusCreated)

	var submitted map[string]interface{}
	decodeBody(t, submitResp, &submitted)
	orderID := submitted["id"].(string)

	// Cancel the order.
	cancelResp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/service-desk/cancel-order", map[string]interface{}{
		"order_id": orderID,
		"reason":   "operator correction",
	})
	assertStatus(t, cancelResp, http.StatusOK)

	var cancelled map[string]interface{}
	decodeBody(t, cancelResp, &cancelled)

	if cancelled["id"] != orderID {
		t.Errorf("expected id %q, got %v", orderID, cancelled["id"])
	}
	if cancelled["status"] != "CANCELLED" {
		t.Errorf("expected status CANCELLED, got %v", cancelled["status"])
	}
}

// TestServiceDeskCancelOrder_MissingReason verifies 400 when reason is absent.
func TestServiceDeskCancelOrder_MissingReason(t *testing.T) {
	ts := newTestServer(t)

	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/service-desk/cancel-order", map[string]interface{}{
		"order_id": "some-order-id",
	})
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

// TestServiceDeskCancelOrder_NonExistent verifies a non-existent order returns 400.
func TestServiceDeskCancelOrder_NonExistent(t *testing.T) {
	ts := newTestServer(t)

	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/service-desk/cancel-order", map[string]interface{}{
		"order_id": "no-such-order",
		"reason":   "test",
	})
	// Cancel of a non-existent order → store.ErrNotFound → CANCEL_FAILED → 400.
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}
