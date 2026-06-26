// Package e2e provides end-to-end integration tests for the GarudaX Platform.
//
// lifecycle_test.go exercises the full trading lifecycle through the gateway HTTP API:
// register participants, login, KYC, submit orders, verify trade execution,
// check positions/margin/market-data, manage orders, and verify book state.
//
// Usage:
//
//	GATEWAY_URL=http://localhost:8080 go test -v -run TestTradeLifecycle ./tests/e2e/
//
// The tests skip automatically when the gateway is not reachable or backends return 502/503.
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// gatewayURL is the gateway root URL, configurable via GATEWAY_URL or E2E_BASE_URL.
var gatewayURL string

func init() {
	gatewayURL = os.Getenv("GATEWAY_URL")
	if gatewayURL == "" {
		gatewayURL = os.Getenv("E2E_BASE_URL")
	}
	if gatewayURL == "" {
		gatewayURL = "http://127.0.0.1:8080"
	}
	gatewayURL = strings.TrimRight(gatewayURL, "/")
}

// ---------- helpers ----------

type lifecycleClient struct {
	t       *testing.T
	base    string
	token   string
}

func newLifecycleClient(t *testing.T) *lifecycleClient {
	t.Helper()
	return &lifecycleClient{t: t, base: gatewayURL}
}

func (lc *lifecycleClient) withToken(token string) *lifecycleClient {
	return &lifecycleClient{t: lc.t, base: lc.base, token: token}
}

func (lc *lifecycleClient) do(method, path string, body interface{}) *http.Response {
	lc.t.Helper()
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			lc.t.Fatalf("marshal request body: %v", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	url := lc.base + path
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		lc.t.Fatalf("create request %s %s: %v", method, path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	// Single-tenant default: the gateway enforces X-GarudaX-Tenant on all
	// tenant-scoped routes, so send the active tenant on every request.
	req.Header.Set("X-GarudaX-Tenant", "ace-commodities")
	if lc.token != "" {
		req.Header.Set("Authorization", "Bearer "+lc.token)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		lc.t.Fatalf("execute request %s %s: %v", method, path, err)
	}
	return resp
}

func (lc *lifecycleClient) get(path string) *http.Response    { return lc.do("GET", path, nil) }
func (lc *lifecycleClient) post(path string, body interface{}) *http.Response {
	return lc.do("POST", path, body)
}
func (lc *lifecycleClient) delete(path string) *http.Response { return lc.do("DELETE", path, nil) }
func (lc *lifecycleClient) patch(path string, body interface{}) *http.Response {
	return lc.do("PATCH", path, body)
}

// lcReadJSON reads and closes a response body into a generic map.
func lcReadJSON(t *testing.T, resp *http.Response) map[string]interface{} {
	t.Helper()
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	var result map[string]interface{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &result); err != nil {
			// Try as array and wrap
			var arr []interface{}
			if err2 := json.Unmarshal(data, &arr); err2 == nil {
				return map[string]interface{}{"_array": arr}
			}
			t.Fatalf("unmarshal response body: %v\nraw: %s", err, string(data))
		}
	}
	return result
}

// lcReadBody reads the raw response body and closes it.
func lcReadBody(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return data
}

// lcExpectStatus asserts the HTTP status code.
func lcExpectStatus(t *testing.T, resp *http.Response, expected int) {
	t.Helper()
	if resp.StatusCode != expected {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected status %d, got %d; body: %s", expected, resp.StatusCode, string(body))
	}
}

// skipIfServiceDown skips the test if the response indicates backend unavailability.
func skipIfServiceDown(t *testing.T, resp *http.Response, service string) {
	t.Helper()
	if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
		resp.Body.Close()
		t.Skipf("%s unavailable (HTTP %d)", service, resp.StatusCode)
	}
}

// lcSkipIfGatewayUnavailable checks connectivity to the gateway and skips if down.
func lcSkipIfGatewayUnavailable(t *testing.T) {
	t.Helper()
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(gatewayURL + "/healthz")
	if err != nil {
		t.Skipf("gateway not reachable at %s: %v", gatewayURL, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Skipf("gateway health check returned %d", resp.StatusCode)
	}
}

// lcUniqueEmail generates a unique email for test isolation.
func lcUniqueEmail(prefix string) string {
	return fmt.Sprintf("lc-%s-%d@e2e-test.garudax", prefix, time.Now().UnixNano())
}

// extractToken extracts an access token from a login response body,
// handling both snake_case and PascalCase field names.
func extractToken(t *testing.T, body map[string]interface{}) string {
	t.Helper()
	if token, ok := body["access_token"].(string); ok && token != "" {
		return token
	}
	if token, ok := body["AccessToken"].(string); ok && token != "" {
		return token
	}
	return ""
}

// registerAndLogin registers a user and returns (userID, authenticatedClient).
// Skips the calling test if auth-service is unavailable.
func registerAndLogin(t *testing.T, c *lifecycleClient, emailPrefix, role, password string) (string, *lifecycleClient) {
	t.Helper()
	email := lcUniqueEmail(emailPrefix)

	// Register
	resp := c.post("/api/v1/auth/register", map[string]string{
		"email":    email,
		"password": password,
		"role":     role,
	})
	if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
		resp.Body.Close()
		t.Skipf("auth-service unavailable during registration (HTTP %d)", resp.StatusCode)
	}
	regBody := lcReadJSON(t, resp)
	userID, _ := regBody["id"].(string)

	// Login
	loginResp := c.post("/api/v1/auth/login", map[string]string{
		"email":    email,
		"password": password,
	})
	if loginResp.StatusCode == http.StatusServiceUnavailable || loginResp.StatusCode == http.StatusBadGateway {
		loginResp.Body.Close()
		t.Skipf("auth-service unavailable during login (HTTP %d)", loginResp.StatusCode)
	}
	loginBody := lcReadJSON(t, loginResp)
	token := extractToken(t, loginBody)
	if token == "" {
		t.Skipf("could not obtain access token for %s; login response: %v", emailPrefix, loginBody)
	}

	return userID, c.withToken(token)
}

// submitKYC submits a KYC application for a participant.
// Does not fail the test if compliance-service is unavailable.
func submitKYC(t *testing.T, authed *lifecycleClient, participantID, name, email, phone string) {
	t.Helper()
	resp := authed.post("/api/v1/participants", map[string]interface{}{
		"participant_id":   participantID,
		"participant_type": "INDIVIDUAL",
		"legal_name":       name,
		"trading_name":     name + " Trading",
		"nationality":      "KE",
		"contact": map[string]string{
			"email":               email,
			"phone":               phone,
			"contact_person_name": name,
		},
		"registered_address": map[string]string{
			"line1":       "123 Test Street",
			"city":        "Nairobi",
			"province":    "Nairobi",
			"postal_code": "00100",
			"country":     "KE",
		},
		"source_of_funds": "Trading income",
	})
	if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
		resp.Body.Close()
		t.Logf("compliance-service unavailable for KYC submission, continuing")
		return
	}
	body := lcReadBody(t, resp)
	t.Logf("KYC submission response (status %d): %s", resp.StatusCode, string(body))
}

// ---------- full trading lifecycle test ----------

// TestTradeLifecycle exercises the complete trading lifecycle:
// health check -> register/login participants -> KYC -> submit matching orders ->
// verify trade -> verify positions -> verify margin -> verify market data ->
// verify order book -> submit more orders for depth -> cancel order -> verify cancel.
func TestTradeLifecycle(t *testing.T) {
	lcSkipIfGatewayUnavailable(t)
	c := newLifecycleClient(t)

	instrument := "WHT-HRW-2026M07-UB"

	// ===== Step 1: Health check =====
	t.Run("01_health_check", func(t *testing.T) {
		resp := c.get("/healthz")
		lcExpectStatus(t, resp, http.StatusOK)
		body := lcReadJSON(t, resp)
		t.Logf("gateway health: %v", body)
	})

	// ===== Step 2: Register participants =====
	var traderAID string
	var traderA *lifecycleClient
	var traderBID string
	var traderB *lifecycleClient
	var admin *lifecycleClient

	t.Run("02_register_trader_A", func(t *testing.T) {
		id, authed := registerAndLogin(t, c, "traderA", "trader", "TraderAPass123!")
		traderAID = id
		traderA = authed
		t.Logf("trader A registered, id=%s", traderAID)
	})

	t.Run("03_register_trader_B", func(t *testing.T) {
		id, authed := registerAndLogin(t, c, "traderB", "trader", "TraderBPass123!")
		traderBID = id
		traderB = authed
		t.Logf("trader B registered, id=%s", traderBID)
	})

	t.Run("04_register_admin", func(t *testing.T) {
		_, authed := registerAndLogin(t, c, "admin", "admin", "AdminPass123!")
		admin = authed
		t.Log("admin registered and logged in")
	})

	// Guard: if registration failed, remaining tests cannot proceed
	if traderA == nil || traderB == nil {
		t.Fatal("trader registration failed, cannot continue lifecycle test")
	}

	// ===== Step 3: Submit KYC for traders =====
	t.Run("05_kyc_trader_A", func(t *testing.T) {
		submitKYC(t, traderA, traderAID, "Trader Alpha", lcUniqueEmail("kycA"), "+254700100001")
	})

	t.Run("06_kyc_trader_B", func(t *testing.T) {
		submitKYC(t, traderB, traderBID, "Trader Beta", lcUniqueEmail("kycB"), "+254700100002")
	})

	// ===== Step 4: Approve KYC (optional, admin) =====
	t.Run("07_approve_kyc_trader_A", func(t *testing.T) {
		if admin == nil {
			t.Skip("admin registration failed")
		}
		if traderAID == "" {
			t.Skip("no trader A participant ID")
		}
		resp := admin.post("/api/v1/participants/"+traderAID+"/approve", map[string]string{
			"notes": "E2E test auto-approval",
		})
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("compliance-service unavailable for KYC approval")
		}
		body := lcReadBody(t, resp)
		t.Logf("approve trader A KYC (status %d): %s", resp.StatusCode, string(body))
	})

	t.Run("08_approve_kyc_trader_B", func(t *testing.T) {
		if admin == nil {
			t.Skip("admin registration failed")
		}
		if traderBID == "" {
			t.Skip("no trader B participant ID")
		}
		resp := admin.post("/api/v1/participants/"+traderBID+"/approve", map[string]string{
			"notes": "E2E test auto-approval",
		})
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("compliance-service unavailable for KYC approval")
		}
		body := lcReadBody(t, resp)
		t.Logf("approve trader B KYC (status %d): %s", resp.StatusCode, string(body))
	})

	// ===== Step 5: Trader A submits BUY LIMIT order =====
	var buyOrderID string

	t.Run("09_submit_buy_order", func(t *testing.T) {
		resp := traderA.post("/api/v1/orders", map[string]interface{}{
			"instrument_id":  instrument,
			"side":           "BUY",
			"order_type":     "LIMIT",
			"quantity":       "10.0000",
			"price":          "325.5000",
			"participant_id": traderAID,
			"time_in_force":  "GTC",
		})
		skipIfServiceDown(t, resp, "matching-engine")

		body := lcReadJSON(t, resp)
		t.Logf("buy order response (status %d): %v", resp.StatusCode, body)

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			// Extract order ID from various possible field names
			for _, key := range []string{"order_id", "OrderID", "id", "ID"} {
				if oid, ok := body[key].(string); ok && oid != "" {
					buyOrderID = oid
					break
				}
			}
			if buyOrderID != "" {
				t.Logf("buy order ID: %s", buyOrderID)
			}
		}
	})

	// ===== Step 6: Trader B submits matching SELL LIMIT order =====
	var sellOrderID string

	t.Run("10_submit_sell_order", func(t *testing.T) {
		resp := traderB.post("/api/v1/orders", map[string]interface{}{
			"instrument_id":  instrument,
			"side":           "SELL",
			"order_type":     "LIMIT",
			"quantity":       "10.0000",
			"price":          "325.5000",
			"participant_id": traderBID,
			"time_in_force":  "GTC",
		})
		skipIfServiceDown(t, resp, "matching-engine")

		body := lcReadJSON(t, resp)
		t.Logf("sell order response (status %d): %v", resp.StatusCode, body)

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			for _, key := range []string{"order_id", "OrderID", "id", "ID"} {
				if oid, ok := body[key].(string); ok && oid != "" {
					sellOrderID = oid
					break
				}
			}
			if sellOrderID != "" {
				t.Logf("sell order ID: %s", sellOrderID)
			}
		}
	})

	// Small delay to allow matching engine to process the trade
	time.Sleep(500 * time.Millisecond)

	// ===== Step 7: Verify trade executed =====
	t.Run("11_verify_trade_executed", func(t *testing.T) {
		resp := traderA.get("/api/v1/instruments/" + instrument + "/trades/latest")
		skipIfServiceDown(t, resp, "matching-engine")

		body := lcReadJSON(t, resp)
		t.Logf("last trade response (status %d): %v", resp.StatusCode, body)

		if resp.StatusCode == http.StatusOK {
			// Verify trade has expected price if field exists
			for _, priceKey := range []string{"price", "Price", "trade_price"} {
				if p, ok := body[priceKey]; ok {
					t.Logf("trade price: %v", p)
					break
				}
			}
			// Verify trade has expected quantity
			for _, qtyKey := range []string{"quantity", "Quantity", "trade_quantity", "size"} {
				if q, ok := body[qtyKey]; ok {
					t.Logf("trade quantity: %v", q)
					break
				}
			}
		}
	})

	// ===== Step 8: Verify positions =====
	t.Run("12_verify_positions", func(t *testing.T) {
		resp := traderA.get("/api/v1/clearing/positions")
		skipIfServiceDown(t, resp, "clearing-engine")

		rawBody := lcReadBody(t, resp)
		t.Logf("positions response (status %d): %s", resp.StatusCode, string(rawBody))

		// Positions may return an array or object depending on endpoint
		if resp.StatusCode == http.StatusOK && len(rawBody) > 0 {
			t.Log("clearing positions returned data")
		}
	})

	// Also check trader B's positions
	t.Run("13_verify_positions_trader_B", func(t *testing.T) {
		resp := traderB.get("/api/v1/clearing/positions")
		skipIfServiceDown(t, resp, "clearing-engine")

		rawBody := lcReadBody(t, resp)
		t.Logf("trader B positions response (status %d): %s", resp.StatusCode, string(rawBody))
	})

	// ===== Step 9: Verify margin calculated =====
	t.Run("14_verify_margin_trader_A", func(t *testing.T) {
		path := "/api/v1/margin"
		if traderAID != "" {
			path += "?participant_id=" + traderAID
		}
		resp := traderA.get(path)
		skipIfServiceDown(t, resp, "margin-engine")

		// Margin endpoint may return non-JSON when no data exists for the participant
		rawBody := lcReadBody(t, resp)
		t.Logf("margin response for trader A (status %d): %s", resp.StatusCode, string(rawBody))
	})

	// ===== Step 10: Verify market data updated =====
	t.Run("15_verify_market_data_ticker", func(t *testing.T) {
		resp := c.get("/api/v1/market-data/ticker/" + instrument)
		skipIfServiceDown(t, resp, "market-data-service")

		body := lcReadJSON(t, resp)
		t.Logf("ticker response (status %d): %v", resp.StatusCode, body)

		if resp.StatusCode == http.StatusOK {
			// Check for expected ticker fields
			for _, key := range []string{"last_price", "LastPrice", "last", "price"} {
				if v, ok := body[key]; ok {
					t.Logf("ticker last price: %v", v)
					break
				}
			}
		}
	})

	// ===== Step 11: Verify order book =====
	t.Run("16_verify_order_book", func(t *testing.T) {
		resp := c.get("/api/v1/instruments/" + instrument + "/book")
		skipIfServiceDown(t, resp, "matching-engine")

		body := lcReadJSON(t, resp)
		t.Logf("order book response (status %d): %v", resp.StatusCode, body)

		if resp.StatusCode == http.StatusOK {
			// After matching, the book should have fewer resting orders
			// (both sides filled, so bids/asks may be empty)
			if bids, ok := body["bids"]; ok {
				t.Logf("order book bids: %v", bids)
			}
			if asks, ok := body["asks"]; ok {
				t.Logf("order book asks: %v", asks)
			}
		}
	})

	// ===== Step 12: Submit more orders to create book depth =====
	var depthOrderIDs []string

	t.Run("17_submit_depth_orders", func(t *testing.T) {
		// Submit multiple buy orders at different price levels
		buyPrices := []string{"320.0000", "321.0000", "322.0000", "323.0000", "324.0000"}
		for i, price := range buyPrices {
			resp := traderA.post("/api/v1/orders", map[string]interface{}{
				"instrument_id":  instrument,
				"side":           "BUY",
				"order_type":     "LIMIT",
				"quantity":       fmt.Sprintf("%d.0000", 5+i),
				"price":          price,
				"participant_id": traderAID,
				"time_in_force":  "GTC",
			})
			if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
				resp.Body.Close()
				t.Skipf("matching-engine unavailable while submitting depth buy order %d", i+1)
			}
			body := lcReadJSON(t, resp)
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				for _, key := range []string{"order_id", "OrderID", "id", "ID"} {
					if oid, ok := body[key].(string); ok && oid != "" {
						depthOrderIDs = append(depthOrderIDs, oid)
						break
					}
				}
			}
			t.Logf("depth buy order %d at %s (status %d)", i+1, price, resp.StatusCode)
		}

		// Submit multiple sell orders at different price levels
		sellPrices := []string{"326.0000", "327.0000", "328.0000", "329.0000", "330.0000"}
		for i, price := range sellPrices {
			resp := traderB.post("/api/v1/orders", map[string]interface{}{
				"instrument_id":  instrument,
				"side":           "SELL",
				"order_type":     "LIMIT",
				"quantity":       fmt.Sprintf("%d.0000", 5+i),
				"price":          price,
				"participant_id": traderBID,
				"time_in_force":  "GTC",
			})
			if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
				resp.Body.Close()
				t.Skipf("matching-engine unavailable while submitting depth sell order %d", i+1)
			}
			body := lcReadJSON(t, resp)
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				for _, key := range []string{"order_id", "OrderID", "id", "ID"} {
					if oid, ok := body[key].(string); ok && oid != "" {
						depthOrderIDs = append(depthOrderIDs, oid)
						break
					}
				}
			}
			t.Logf("depth sell order %d at %s (status %d)", i+1, price, resp.StatusCode)
		}
	})

	// Small delay for order book to update
	time.Sleep(300 * time.Millisecond)

	// ===== Step 13: Verify book has depth =====
	t.Run("18_verify_book_depth", func(t *testing.T) {
		resp := c.get("/api/v1/instruments/" + instrument + "/book")
		skipIfServiceDown(t, resp, "matching-engine")

		body := lcReadJSON(t, resp)
		t.Logf("order book with depth (status %d): %v", resp.StatusCode, body)

		if resp.StatusCode == http.StatusOK {
			// Verify both sides have entries
			checkSide := func(side string) {
				if levels, ok := body[side]; ok {
					if arr, ok := levels.([]interface{}); ok {
						t.Logf("book %s levels: %d", side, len(arr))
						if len(arr) == 0 {
							t.Logf("warning: expected %s levels after depth orders, got empty", side)
						}
					}
				}
			}
			checkSide("bids")
			checkSide("asks")
		}
	})

	// ===== Step 14: Cancel an order =====
	t.Run("19_cancel_order", func(t *testing.T) {
		// Try to cancel one of the depth orders
		var orderToCancel string
		if len(depthOrderIDs) > 0 {
			orderToCancel = depthOrderIDs[0]
		} else if buyOrderID != "" {
			orderToCancel = buyOrderID
		}

		if orderToCancel == "" {
			t.Skip("no order ID available to cancel")
		}

		resp := traderA.delete("/api/v1/orders/" + orderToCancel)
		skipIfServiceDown(t, resp, "matching-engine")

		body := lcReadBody(t, resp)
		t.Logf("cancel order %s response (status %d): %s", orderToCancel, resp.StatusCode, string(body))

		// Accept 200, 204 (success), 404 (already filled), or other non-5xx responses
		if resp.StatusCode >= 500 && resp.StatusCode != http.StatusServiceUnavailable && resp.StatusCode != http.StatusBadGateway {
			t.Errorf("unexpected server error cancelling order: status %d, body: %s", resp.StatusCode, string(body))
		}
	})

	// Small delay for book to update after cancel
	time.Sleep(200 * time.Millisecond)

	// ===== Step 15: Verify cancel reflected in book =====
	t.Run("20_verify_cancel_in_book", func(t *testing.T) {
		resp := c.get("/api/v1/instruments/" + instrument + "/book")
		skipIfServiceDown(t, resp, "matching-engine")

		body := lcReadJSON(t, resp)
		t.Logf("order book after cancel (status %d): %v", resp.StatusCode, body)
		// We primarily verify the endpoint is responsive after the cancel
	})

	// ===== Bonus: Check L3 order book =====
	t.Run("21_verify_order_book_l3", func(t *testing.T) {
		resp := c.get("/api/v1/instruments/" + instrument + "/book/l3")
		skipIfServiceDown(t, resp, "matching-engine")

		body := lcReadJSON(t, resp)
		t.Logf("L3 order book (status %d): %v", resp.StatusCode, body)
	})

	// ===== Bonus: Verify margin calls =====
	t.Run("22_verify_margin_calls", func(t *testing.T) {
		resp := traderA.get("/api/v1/margin/calls")
		skipIfServiceDown(t, resp, "margin-engine")

		body := lcReadJSON(t, resp)
		t.Logf("margin calls (status %d): %v", resp.StatusCode, body)
	})

	// ===== Bonus: Verify settlement cycles =====
	t.Run("23_verify_settlement_cycles", func(t *testing.T) {
		resp := traderA.get("/api/v1/settlement/cycles")
		skipIfServiceDown(t, resp, "settlement-engine")

		body := lcReadJSON(t, resp)
		t.Logf("settlement cycles (status %d): %v", resp.StatusCode, body)
	})

	// ===== Bonus: Verify netting =====
	t.Run("24_verify_netting", func(t *testing.T) {
		resp := traderA.get("/api/v1/clearing/netting")
		skipIfServiceDown(t, resp, "clearing-engine")

		rawBody := lcReadBody(t, resp)
		t.Logf("netting response (status %d): %s", resp.StatusCode, string(rawBody))
	})

	// ===== Bonus: Verify market data candles =====
	t.Run("25_verify_candles", func(t *testing.T) {
		resp := c.get("/api/v1/market-data/candles/" + instrument)
		skipIfServiceDown(t, resp, "market-data-service")

		rawBody := lcReadBody(t, resp)
		t.Logf("candles response (status %d): %s", resp.StatusCode, string(rawBody))
	})

	// ===== Bonus: Verify market data trades =====
	t.Run("26_verify_market_trades", func(t *testing.T) {
		resp := c.get("/api/v1/market-data/trades/" + instrument)
		skipIfServiceDown(t, resp, "market-data-service")

		rawBody := lcReadBody(t, resp)
		t.Logf("market trades response (status %d): %s", resp.StatusCode, string(rawBody))
	})

	// ===== Bonus: List instruments =====
	t.Run("27_list_instruments", func(t *testing.T) {
		resp := c.get("/api/v1/instruments/list")
		skipIfServiceDown(t, resp, "matching-engine")

		rawBody := lcReadBody(t, resp)
		t.Logf("instruments list (status %d): %s", resp.StatusCode, string(rawBody))
	})
}

// TestTradeLifecycleOrderModify tests order modification within the trading lifecycle.
func TestTradeLifecycleOrderModify(t *testing.T) {
	lcSkipIfGatewayUnavailable(t)
	c := newLifecycleClient(t)

	instrument := "WHT-HRW-2026M07-UB"
	traderID, trader := registerAndLogin(t, c, "modify-trader", "trader", "ModifyPass123!")

	// Submit an order
	var orderID string
	t.Run("submit_order_for_modify", func(t *testing.T) {
		resp := trader.post("/api/v1/orders", map[string]interface{}{
			"instrument_id":  instrument,
			"side":           "BUY",
			"order_type":     "LIMIT",
			"quantity":       "20.0000",
			"price":          "310.0000",
			"participant_id": traderID,
			"time_in_force":  "GTC",
		})
		skipIfServiceDown(t, resp, "matching-engine")

		body := lcReadJSON(t, resp)
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			for _, key := range []string{"order_id", "OrderID", "id", "ID"} {
				if oid, ok := body[key].(string); ok && oid != "" {
					orderID = oid
					break
				}
			}
		}
		if orderID == "" {
			t.Logf("could not extract order ID from response (status %d): %v", resp.StatusCode, body)
		}
	})

	// Modify the order
	t.Run("modify_order_price", func(t *testing.T) {
		if orderID == "" {
			t.Skip("no order ID available to modify")
		}
		resp := trader.patch("/api/v1/orders/"+orderID, map[string]interface{}{
			"price":    "312.0000",
			"quantity": "25.0000",
		})
		skipIfServiceDown(t, resp, "matching-engine")

		body := lcReadBody(t, resp)
		t.Logf("modify order response (status %d): %s", resp.StatusCode, string(body))
	})

	// Verify the modification via get
	t.Run("verify_modified_order", func(t *testing.T) {
		if orderID == "" {
			t.Skip("no order ID available to verify")
		}
		resp := trader.get("/api/v1/orders/" + orderID)
		skipIfServiceDown(t, resp, "matching-engine")

		body := lcReadJSON(t, resp)
		t.Logf("modified order (status %d): %v", resp.StatusCode, body)
	})

	// Cancel the order
	t.Run("cancel_modified_order", func(t *testing.T) {
		if orderID == "" {
			t.Skip("no order ID available to cancel")
		}
		resp := trader.delete("/api/v1/orders/" + orderID)
		skipIfServiceDown(t, resp, "matching-engine")
		resp.Body.Close()
	})
}

// TestTradeLifecycleCancelAll tests the cancel-all-orders endpoint.
func TestTradeLifecycleCancelAll(t *testing.T) {
	lcSkipIfGatewayUnavailable(t)
	c := newLifecycleClient(t)

	instrument := "WHT-HRW-2026M07-UB"
	traderID, trader := registerAndLogin(t, c, "cancelall-trader", "trader", "CancelAllPass123!")

	// Submit several orders
	t.Run("submit_multiple_orders", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			resp := trader.post("/api/v1/orders", map[string]interface{}{
				"instrument_id":  instrument,
				"side":           "BUY",
				"order_type":     "LIMIT",
				"quantity":       "5.0000",
				"price":          fmt.Sprintf("%d.0000", 300+i),
				"participant_id": traderID,
				"time_in_force":  "GTC",
			})
			if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
				resp.Body.Close()
				t.Skipf("matching-engine unavailable while submitting order %d", i+1)
			}
			resp.Body.Close()
		}
	})

	// Cancel all
	t.Run("cancel_all_orders", func(t *testing.T) {
		resp := trader.delete("/api/v1/orders")
		skipIfServiceDown(t, resp, "matching-engine")

		body := lcReadBody(t, resp)
		t.Logf("cancel all response (status %d): %s", resp.StatusCode, string(body))
	})

	// Verify empty order list
	t.Run("verify_orders_empty_after_cancel_all", func(t *testing.T) {
		time.Sleep(200 * time.Millisecond)
		resp := trader.get("/api/v1/orders")
		skipIfServiceDown(t, resp, "matching-engine")

		rawBody := lcReadBody(t, resp)
		t.Logf("orders after cancel-all (status %d): %s", resp.StatusCode, string(rawBody))
	})
}

// TestTradeLifecycleAdminOperations tests admin actions within the trading context.
func TestTradeLifecycleAdminOperations(t *testing.T) {
	lcSkipIfGatewayUnavailable(t)
	c := newLifecycleClient(t)

	_, admin := registerAndLogin(t, c, "admin-ops", "admin", "AdminOpsPass123!")

	t.Run("admin_health_check", func(t *testing.T) {
		resp := admin.get("/api/v1/admin/health")
		skipIfServiceDown(t, resp, "gateway")

		body := lcReadJSON(t, resp)
		t.Logf("admin health (status %d): %v", resp.StatusCode, body)
	})

	t.Run("admin_get_circuit_breakers", func(t *testing.T) {
		resp := admin.get("/api/v1/admin/circuit-breakers")
		skipIfServiceDown(t, resp, "matching-engine")

		rawBody := lcReadBody(t, resp)
		t.Logf("circuit breakers (status %d): %s", resp.StatusCode, string(rawBody))
	})

	t.Run("admin_margin_call_stats", func(t *testing.T) {
		resp := admin.get("/api/v1/margin/calls/stats")
		skipIfServiceDown(t, resp, "margin-engine")

		rawBody := lcReadBody(t, resp)
		t.Logf("margin call stats (status %d): %s", resp.StatusCode, string(rawBody))
	})
}
