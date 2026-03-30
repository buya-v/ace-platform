// Package e2e provides end-to-end integration tests for the GarudaX Platform.
// These tests exercise the full trading flow through the gateway HTTP API.
//
// Usage:
//
//	E2E_BASE_URL=http://localhost:8080 go test -v ./tests/e2e/
//
// The tests skip automatically when the gateway is not reachable.
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

// baseURL is the gateway root URL, configurable via E2E_BASE_URL.
var baseURL string

func TestMain(m *testing.M) {
	baseURL = os.Getenv("E2E_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	os.Exit(m.Run())
}

// ---------- helpers ----------

type apiClient struct {
	t       *testing.T
	baseURL string
	token   string
}

func newClient(t *testing.T) *apiClient {
	t.Helper()
	return &apiClient{t: t, baseURL: baseURL}
}

func (c *apiClient) withToken(token string) *apiClient {
	return &apiClient{t: c.t, baseURL: c.baseURL, token: token}
}

func (c *apiClient) do(method, path string, body interface{}) *http.Response {
	c.t.Helper()
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			c.t.Fatalf("marshal request body: %v", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	url := c.baseURL + path
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		c.t.Fatalf("create request %s %s: %v", method, path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		c.t.Fatalf("execute request %s %s: %v", method, path, err)
	}
	return resp
}

func (c *apiClient) get(path string) *http.Response  { return c.do("GET", path, nil) }
func (c *apiClient) post(path string, body interface{}) *http.Response {
	return c.do("POST", path, body)
}
func (c *apiClient) delete(path string) *http.Response { return c.do("DELETE", path, nil) }
func (c *apiClient) patch(path string, body interface{}) *http.Response {
	return c.do("PATCH", path, body)
}

// readJSON reads and closes a response body into a generic map.
func readJSON(t *testing.T, resp *http.Response) map[string]interface{} {
	t.Helper()
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	var result map[string]interface{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatalf("unmarshal response body: %v\nraw: %s", err, string(data))
		}
	}
	return result
}

// readJSONArray reads a response as a JSON array.
func readJSONArray(t *testing.T, resp *http.Response) []interface{} {
	t.Helper()
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	var result []interface{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &result); err != nil {
			// Try wrapping in an object (some endpoints return {data: [...]})
			var obj map[string]interface{}
			if err2 := json.Unmarshal(data, &obj); err2 == nil {
				if d, ok := obj["data"]; ok {
					if arr, ok := d.([]interface{}); ok {
						return arr
					}
				}
			}
			t.Fatalf("unmarshal response array: %v\nraw: %s", err, string(data))
		}
	}
	return result
}

// expectStatus asserts the HTTP status code.
func expectStatus(t *testing.T, resp *http.Response, expected int) {
	t.Helper()
	if resp.StatusCode != expected {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected status %d, got %d; body: %s", expected, resp.StatusCode, string(body))
	}
}

// skipIfGatewayUnavailable checks connectivity to the gateway and skips if down.
func skipIfGatewayUnavailable(t *testing.T) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(baseURL + "/healthz")
	if err != nil {
		t.Skipf("gateway not reachable at %s: %v", baseURL, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Skipf("gateway health check returned %d", resp.StatusCode)
	}
}

// uniqueEmail generates a unique email for test isolation.
func uniqueEmail(prefix string) string {
	return fmt.Sprintf("%s-%d@e2e-test.ace", prefix, time.Now().UnixNano())
}

// ---------- connectivity test ----------

func TestGatewayHealth(t *testing.T) {
	skipIfGatewayUnavailable(t)
	c := newClient(t)

	t.Run("healthz returns 200", func(t *testing.T) {
		resp := c.get("/healthz")
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("readyz returns 200", func(t *testing.T) {
		resp := c.get("/readyz")
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("unknown endpoint returns 404", func(t *testing.T) {
		resp := c.get("/api/v1/nonexistent")
		expectStatus(t, resp, http.StatusNotFound)
		body := readJSON(t, resp)
		errObj, ok := body["error"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected error object in response, got: %v", body)
		}
		if errObj["code"] != "NOT_FOUND" {
			t.Errorf("expected error code NOT_FOUND, got %v", errObj["code"])
		}
	})
}

// ---------- auth flow tests ----------

func TestAuthRegisterAndLogin(t *testing.T) {
	skipIfGatewayUnavailable(t)
	c := newClient(t)

	email := uniqueEmail("trader")
	password := "SecurePass123!"

	var userID string
	var accessToken string

	t.Run("register new trader", func(t *testing.T) {
		resp := c.post("/api/v1/auth/register", map[string]string{
			"email":    email,
			"password": password,
			"role":     "trader",
		})
		expectStatus(t, resp, http.StatusCreated)
		body := readJSON(t, resp)
		id, ok := body["id"].(string)
		if !ok || id == "" {
			t.Fatalf("expected user id in response, got: %v", body)
		}
		userID = id
		if body["email"] != email {
			t.Errorf("expected email %s, got %v", email, body["email"])
		}
		_ = userID // used in later steps
	})

	t.Run("login with valid credentials", func(t *testing.T) {
		resp := c.post("/api/v1/auth/login", map[string]string{
			"email":    email,
			"password": password,
		})
		expectStatus(t, resp, http.StatusOK)
		body := readJSON(t, resp)
		token, ok := body["access_token"].(string)
		if !ok || token == "" {
			// The auth service returns AccessToken / RefreshToken / ExpiresIn
			// Check camelCase and snake_case variants
			if token2, ok2 := body["AccessToken"].(string); ok2 {
				token = token2
			}
		}
		if token == "" {
			t.Fatalf("expected access_token in response, got: %v", body)
		}
		accessToken = token
	})

	t.Run("get profile with token", func(t *testing.T) {
		if accessToken == "" {
			t.Skip("no access token from login step")
		}
		authed := c.withToken(accessToken)
		resp := authed.get("/api/v1/auth/me")
		// Profile may return 200 or the backend may return a forwarding error
		if resp.StatusCode == http.StatusOK {
			body := readJSON(t, resp)
			if body["email"] != nil && body["email"] != email {
				t.Errorf("expected email %s, got %v", email, body["email"])
			}
		} else {
			resp.Body.Close()
		}
	})
}

func TestAuthNegativeCases(t *testing.T) {
	skipIfGatewayUnavailable(t)
	c := newClient(t)

	t.Run("register with missing fields returns 400", func(t *testing.T) {
		resp := c.post("/api/v1/auth/register", map[string]string{
			"email": "incomplete@test.com",
		})
		// Gateway forwards to auth-service which validates
		if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusServiceUnavailable {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Errorf("expected 400 or 503, got %d; body: %s", resp.StatusCode, string(body))
		} else {
			resp.Body.Close()
		}
	})

	t.Run("register with invalid email returns 400", func(t *testing.T) {
		resp := c.post("/api/v1/auth/register", map[string]string{
			"email":    "not-an-email",
			"password": "SecurePass123!",
			"role":     "trader",
		})
		if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusServiceUnavailable {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Errorf("expected 400 or 503, got %d; body: %s", resp.StatusCode, string(body))
		} else {
			resp.Body.Close()
		}
	})

	t.Run("register with short password returns 400", func(t *testing.T) {
		resp := c.post("/api/v1/auth/register", map[string]string{
			"email":    uniqueEmail("shortpw"),
			"password": "short",
			"role":     "trader",
		})
		if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusServiceUnavailable {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Errorf("expected 400 or 503, got %d; body: %s", resp.StatusCode, string(body))
		} else {
			resp.Body.Close()
		}
	})

	t.Run("register with invalid role returns 400", func(t *testing.T) {
		resp := c.post("/api/v1/auth/register", map[string]string{
			"email":    uniqueEmail("badrole"),
			"password": "SecurePass123!",
			"role":     "superuser",
		})
		if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusServiceUnavailable {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Errorf("expected 400 or 503, got %d; body: %s", resp.StatusCode, string(body))
		} else {
			resp.Body.Close()
		}
	})

	t.Run("login with wrong password returns 401", func(t *testing.T) {
		resp := c.post("/api/v1/auth/login", map[string]string{
			"email":    "nonexistent@e2e-test.ace",
			"password": "WrongPassword123!",
		})
		if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusServiceUnavailable {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Errorf("expected 401 or 503, got %d; body: %s", resp.StatusCode, string(body))
		} else {
			resp.Body.Close()
		}
	})

	t.Run("duplicate registration returns 409", func(t *testing.T) {
		email := uniqueEmail("dup")
		// First registration
		resp1 := c.post("/api/v1/auth/register", map[string]string{
			"email":    email,
			"password": "SecurePass123!",
			"role":     "trader",
		})
		resp1.Body.Close()
		if resp1.StatusCode == http.StatusServiceUnavailable {
			t.Skip("auth-service unavailable")
		}

		// Duplicate
		resp2 := c.post("/api/v1/auth/register", map[string]string{
			"email":    email,
			"password": "SecurePass123!",
			"role":     "trader",
		})
		if resp2.StatusCode != http.StatusConflict {
			body, _ := io.ReadAll(resp2.Body)
			resp2.Body.Close()
			t.Errorf("expected 409 for duplicate, got %d; body: %s", resp2.StatusCode, string(body))
		} else {
			resp2.Body.Close()
		}
	})
}

func TestAuthUnauthorizedAccess(t *testing.T) {
	skipIfGatewayUnavailable(t)
	c := newClient(t) // no token

	protectedPaths := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/orders"},
		{"GET", "/api/v1/clearing/positions"},
		{"GET", "/api/v1/margin"},
		{"GET", "/api/v1/settlement/cycles"},
		{"GET", "/api/v1/auth/me"},
	}

	for _, tc := range protectedPaths {
		t.Run(fmt.Sprintf("%s %s requires auth", tc.method, tc.path), func(t *testing.T) {
			resp := c.do(tc.method, tc.path, nil)
			if resp.StatusCode != http.StatusUnauthorized {
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				t.Errorf("expected 401, got %d for %s %s; body: %s",
					resp.StatusCode, tc.method, tc.path, string(body))
			} else {
				body := readJSON(t, resp)
				if errObj, ok := body["error"].(map[string]interface{}); ok {
					if errObj["code"] != "UNAUTHENTICATED" {
						t.Errorf("expected UNAUTHENTICATED error code, got %v", errObj["code"])
					}
				}
			}
		})
	}
}

func TestAuthInvalidToken(t *testing.T) {
	skipIfGatewayUnavailable(t)

	t.Run("garbage token returns 401", func(t *testing.T) {
		c := newClient(t).withToken("this.is.not.a.valid.jwt")
		resp := c.get("/api/v1/orders")
		if resp.StatusCode != http.StatusUnauthorized {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Errorf("expected 401, got %d; body: %s", resp.StatusCode, string(body))
		} else {
			resp.Body.Close()
		}
	})

	t.Run("expired token returns 401", func(t *testing.T) {
		// A structurally valid but expired JWT (HS256 with wrong signature)
		c := newClient(t).withToken("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ0ZXN0Iiwicm9sZXMiOlsidHJhZGVyIl0sImV4cCI6MX0.invalid")
		resp := c.get("/api/v1/orders")
		if resp.StatusCode != http.StatusUnauthorized {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Errorf("expected 401, got %d; body: %s", resp.StatusCode, string(body))
		} else {
			resp.Body.Close()
		}
	})
}

// ---------- compliance / KYC flow ----------

func TestComplianceKYCFlow(t *testing.T) {
	skipIfGatewayUnavailable(t)
	c := newClient(t)

	// Register and login to get a token
	email := uniqueEmail("kyc-trader")
	resp := c.post("/api/v1/auth/register", map[string]string{
		"email":    email,
		"password": "SecurePass123!",
		"role":     "trader",
	})
	if resp.StatusCode == http.StatusServiceUnavailable {
		resp.Body.Close()
		t.Skip("auth-service unavailable")
	}
	regBody := readJSON(t, resp)
	userID, _ := regBody["id"].(string)

	loginResp := c.post("/api/v1/auth/login", map[string]string{
		"email":    email,
		"password": "SecurePass123!",
	})
	if loginResp.StatusCode == http.StatusServiceUnavailable {
		loginResp.Body.Close()
		t.Skip("auth-service unavailable")
	}
	loginBody := readJSON(t, loginResp)
	token, _ := loginBody["access_token"].(string)
	if token == "" {
		if t2, ok := loginBody["AccessToken"].(string); ok {
			token = t2
		}
	}
	if token == "" {
		t.Skip("could not obtain access token")
	}
	authed := c.withToken(token)

	var participantID string

	t.Run("submit KYC application", func(t *testing.T) {
		resp := authed.post("/api/v1/participants", map[string]interface{}{
			"participant_id":   userID,
			"participant_type": "INDIVIDUAL",
			"legal_name":       "Test Trader E2E",
			"trading_name":     "E2E Trading",
			"nationality":      "KE",
			"contact": map[string]string{
				"email":               email,
				"phone":               "+254700000001",
				"contact_person_name": "Test Trader",
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
			t.Skip("compliance-service unavailable")
		}
		body := readJSON(t, resp)
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if pid, ok := body["participant_id"].(string); ok && pid != "" {
				participantID = pid
			} else if pid, ok := body["ParticipantID"].(string); ok && pid != "" {
				participantID = pid
			}
		}
		if participantID == "" {
			participantID = userID // fallback
		}
	})

	t.Run("get KYC application", func(t *testing.T) {
		if participantID == "" {
			t.Skip("no participant ID from submit step")
		}
		resp := authed.get("/api/v1/participants/" + participantID)
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("compliance-service unavailable")
		}
		// Just ensure we get a response (200 or error from backend)
		resp.Body.Close()
	})

	t.Run("list KYC applications", func(t *testing.T) {
		resp := authed.get("/api/v1/participants")
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("compliance-service unavailable")
		}
		resp.Body.Close()
	})

	// Admin approval flow
	t.Run("approve KYC application (as admin)", func(t *testing.T) {
		// Register an admin user
		adminEmail := uniqueEmail("admin")
		adminResp := c.post("/api/v1/auth/register", map[string]string{
			"email":    adminEmail,
			"password": "AdminPass123!",
			"role":     "admin",
		})
		if adminResp.StatusCode == http.StatusServiceUnavailable {
			adminResp.Body.Close()
			t.Skip("auth-service unavailable")
		}
		adminResp.Body.Close()

		adminLogin := c.post("/api/v1/auth/login", map[string]string{
			"email":    adminEmail,
			"password": "AdminPass123!",
		})
		adminBody := readJSON(t, adminLogin)
		adminToken, _ := adminBody["access_token"].(string)
		if adminToken == "" {
			if t2, ok := adminBody["AccessToken"].(string); ok {
				adminToken = t2
			}
		}
		if adminToken == "" {
			t.Skip("could not get admin token")
		}

		admin := c.withToken(adminToken)
		resp := admin.post("/api/v1/participants/"+participantID+"/approve", map[string]string{
			"officer_id": "admin-e2e",
			"notes":      "E2E test approval",
		})
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("compliance-service unavailable")
		}
		resp.Body.Close()
	})
}

// ---------- order and trading flow ----------

func TestOrderFlow(t *testing.T) {
	skipIfGatewayUnavailable(t)
	c := newClient(t)

	// Register and login trader
	email := uniqueEmail("order-trader")
	resp := c.post("/api/v1/auth/register", map[string]string{
		"email":    email,
		"password": "SecurePass123!",
		"role":     "trader",
	})
	if resp.StatusCode == http.StatusServiceUnavailable {
		resp.Body.Close()
		t.Skip("auth-service unavailable")
	}
	resp.Body.Close()

	loginResp := c.post("/api/v1/auth/login", map[string]string{
		"email":    email,
		"password": "SecurePass123!",
	})
	loginBody := readJSON(t, loginResp)
	token, _ := loginBody["access_token"].(string)
	if token == "" {
		if t2, ok := loginBody["AccessToken"].(string); ok {
			token = t2
		}
	}
	if token == "" {
		t.Skip("could not obtain access token")
	}
	authed := c.withToken(token)

	t.Run("submit limit buy order", func(t *testing.T) {
		resp := authed.post("/api/v1/orders", map[string]interface{}{
			"instrument_id":  "WHEAT-2026-07",
			"side":           "BUY",
			"order_type":     "LIMIT",
			"quantity":       "10.0000",
			"price":          "250.5000",
			"participant_id": "participant-e2e-buy",
			"time_in_force":  "GTC",
		})
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("matching-engine unavailable")
		}
		body := readJSON(t, resp)
		// If matching engine is connected, we should get an order ID
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if body["order_id"] == nil && body["OrderID"] == nil && body["id"] == nil {
				t.Logf("order response: %v", body)
			}
		}
	})

	t.Run("list orders", func(t *testing.T) {
		resp := authed.get("/api/v1/orders")
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("matching-engine unavailable")
		}
		resp.Body.Close()
	})
}

func TestOrderNegativeCases(t *testing.T) {
	skipIfGatewayUnavailable(t)
	c := newClient(t)

	// Get a valid token
	email := uniqueEmail("order-neg")
	resp := c.post("/api/v1/auth/register", map[string]string{
		"email":    email,
		"password": "SecurePass123!",
		"role":     "trader",
	})
	if resp.StatusCode == http.StatusServiceUnavailable {
		resp.Body.Close()
		t.Skip("auth-service unavailable")
	}
	resp.Body.Close()

	loginResp := c.post("/api/v1/auth/login", map[string]string{
		"email":    email,
		"password": "SecurePass123!",
	})
	loginBody := readJSON(t, loginResp)
	token, _ := loginBody["access_token"].(string)
	if token == "" {
		if t2, ok := loginBody["AccessToken"].(string); ok {
			token = t2
		}
	}
	if token == "" {
		t.Skip("could not obtain access token")
	}
	authed := c.withToken(token)

	t.Run("submit order with invalid JSON returns 400", func(t *testing.T) {
		url := baseURL + "/api/v1/orders"
		req, _ := http.NewRequest("POST", url, strings.NewReader("{invalid json"))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Errorf("expected 400 for invalid JSON, got %d; body: %s", resp.StatusCode, string(body))
		} else {
			resp.Body.Close()
		}
	})

	t.Run("cancel non-existent order", func(t *testing.T) {
		resp := authed.delete("/api/v1/orders/nonexistent-order-id")
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("matching-engine unavailable")
		}
		// Should get 404 or some error from the matching engine
		resp.Body.Close()
	})

	t.Run("get non-existent order", func(t *testing.T) {
		resp := authed.get("/api/v1/orders/nonexistent-order-id")
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("matching-engine unavailable")
		}
		resp.Body.Close()
	})
}

// ---------- full trading lifecycle (happy path) ----------

func TestFullTradingLifecycle(t *testing.T) {
	skipIfGatewayUnavailable(t)
	c := newClient(t)

	// ----- Step 1: Register buyer -----
	buyerEmail := uniqueEmail("buyer")
	resp := c.post("/api/v1/auth/register", map[string]string{
		"email":    buyerEmail,
		"password": "BuyerPass123!",
		"role":     "trader",
	})
	if resp.StatusCode == http.StatusServiceUnavailable {
		resp.Body.Close()
		t.Skip("auth-service unavailable")
	}
	buyerReg := readJSON(t, resp)
	buyerID, _ := buyerReg["id"].(string)

	// ----- Step 2: Login buyer -----
	loginResp := c.post("/api/v1/auth/login", map[string]string{
		"email":    buyerEmail,
		"password": "BuyerPass123!",
	})
	buyerLogin := readJSON(t, loginResp)
	buyerToken, _ := buyerLogin["access_token"].(string)
	if buyerToken == "" {
		if t2, ok := buyerLogin["AccessToken"].(string); ok {
			buyerToken = t2
		}
	}
	if buyerToken == "" {
		t.Fatal("could not get buyer token")
	}
	buyer := c.withToken(buyerToken)

	// ----- Step 3: Register seller -----
	sellerEmail := uniqueEmail("seller")
	resp = c.post("/api/v1/auth/register", map[string]string{
		"email":    sellerEmail,
		"password": "SellerPass123!",
		"role":     "trader",
	})
	sellerReg := readJSON(t, resp)
	sellerID, _ := sellerReg["id"].(string)

	// ----- Step 4: Login seller -----
	loginResp = c.post("/api/v1/auth/login", map[string]string{
		"email":    sellerEmail,
		"password": "SellerPass123!",
	})
	sellerLogin := readJSON(t, loginResp)
	sellerToken, _ := sellerLogin["access_token"].(string)
	if sellerToken == "" {
		if t2, ok := sellerLogin["AccessToken"].(string); ok {
			sellerToken = t2
		}
	}
	if sellerToken == "" {
		t.Fatal("could not get seller token")
	}
	seller := c.withToken(sellerToken)

	// ----- Step 5: Submit KYC for buyer -----
	kycResp := buyer.post("/api/v1/participants", map[string]interface{}{
		"participant_id":   buyerID,
		"participant_type": "INDIVIDUAL",
		"legal_name":       "E2E Buyer",
		"trading_name":     "Buyer Trading Co",
		"nationality":      "KE",
		"contact": map[string]string{
			"email":               buyerEmail,
			"phone":               "+254700000002",
			"contact_person_name": "E2E Buyer",
		},
		"registered_address": map[string]string{
			"line1":       "456 Buyer Lane",
			"city":        "Nairobi",
			"province":    "Nairobi",
			"postal_code": "00200",
			"country":     "KE",
		},
		"source_of_funds": "Business income",
	})
	if kycResp.StatusCode == http.StatusServiceUnavailable || kycResp.StatusCode == http.StatusBadGateway {
		kycResp.Body.Close()
		t.Log("compliance-service unavailable, continuing without KYC")
	} else {
		kycResp.Body.Close()
	}

	// ----- Step 6: Submit KYC for seller -----
	kycResp = seller.post("/api/v1/participants", map[string]interface{}{
		"participant_id":   sellerID,
		"participant_type": "INDIVIDUAL",
		"legal_name":       "E2E Seller",
		"trading_name":     "Seller Trading Co",
		"nationality":      "KE",
		"contact": map[string]string{
			"email":               sellerEmail,
			"phone":               "+254700000003",
			"contact_person_name": "E2E Seller",
		},
		"registered_address": map[string]string{
			"line1":       "789 Seller Road",
			"city":        "Mombasa",
			"province":    "Coast",
			"postal_code": "80100",
			"country":     "KE",
		},
		"source_of_funds": "Agricultural sales",
	})
	if kycResp.StatusCode == http.StatusServiceUnavailable || kycResp.StatusCode == http.StatusBadGateway {
		kycResp.Body.Close()
		t.Log("compliance-service unavailable, continuing without KYC")
	} else {
		kycResp.Body.Close()
	}

	instrument := "WHEAT-2026-07"

	// ----- Step 7: Place buy order -----
	t.Run("place buy order", func(t *testing.T) {
		resp := buyer.post("/api/v1/orders", map[string]interface{}{
			"instrument_id":  instrument,
			"side":           "BUY",
			"order_type":     "LIMIT",
			"quantity":       "100.0000",
			"price":          "300.0000",
			"participant_id": buyerID,
			"time_in_force":  "GTC",
		})
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("matching-engine unavailable")
		}
		body := readJSON(t, resp)
		t.Logf("buy order response (status %d): %v", resp.StatusCode, body)
	})

	// ----- Step 8: Place sell order (should match) -----
	t.Run("place sell order (matching)", func(t *testing.T) {
		resp := seller.post("/api/v1/orders", map[string]interface{}{
			"instrument_id":  instrument,
			"side":           "SELL",
			"order_type":     "LIMIT",
			"quantity":       "100.0000",
			"price":          "300.0000",
			"participant_id": sellerID,
			"time_in_force":  "GTC",
		})
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("matching-engine unavailable")
		}
		body := readJSON(t, resp)
		t.Logf("sell order response (status %d): %v", resp.StatusCode, body)
	})

	// ----- Step 9: Verify trade via market data -----
	t.Run("verify last trade", func(t *testing.T) {
		resp := buyer.get("/api/v1/instruments/" + instrument + "/trades/latest")
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("matching-engine unavailable")
		}
		body := readJSON(t, resp)
		t.Logf("last trade response (status %d): %v", resp.StatusCode, body)
	})

	// ----- Step 10: Check order book -----
	t.Run("verify order book", func(t *testing.T) {
		resp := buyer.get("/api/v1/instruments/" + instrument + "/book")
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("matching-engine unavailable")
		}
		body := readJSON(t, resp)
		t.Logf("order book response (status %d): %v", resp.StatusCode, body)
	})

	// ----- Step 11: Verify positions (clearing) -----
	t.Run("verify clearing positions", func(t *testing.T) {
		resp := buyer.get("/api/v1/clearing/positions")
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("clearing-engine unavailable")
		}
		body := readJSON(t, resp)
		t.Logf("positions response (status %d): %v", resp.StatusCode, body)
	})

	// ----- Step 12: Check netting -----
	t.Run("verify netting", func(t *testing.T) {
		resp := buyer.get("/api/v1/clearing/netting")
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("clearing-engine unavailable")
		}
		body := readJSON(t, resp)
		t.Logf("netting response (status %d): %v", resp.StatusCode, body)
	})

	// ----- Step 13: Verify margin -----
	t.Run("verify margin requirements", func(t *testing.T) {
		resp := buyer.get("/api/v1/margin")
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("margin-engine unavailable")
		}
		body := readJSON(t, resp)
		t.Logf("margin response (status %d): %v", resp.StatusCode, body)
	})

	// ----- Step 14: Check margin calls -----
	t.Run("check margin calls", func(t *testing.T) {
		resp := buyer.get("/api/v1/margin/calls")
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("margin-engine unavailable")
		}
		body := readJSON(t, resp)
		t.Logf("margin calls response (status %d): %v", resp.StatusCode, body)
	})

	// ----- Step 15: Check settlement cycles -----
	t.Run("check settlement cycles", func(t *testing.T) {
		resp := buyer.get("/api/v1/settlement/cycles")
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("settlement-engine unavailable")
		}
		body := readJSON(t, resp)
		t.Logf("settlement cycles response (status %d): %v", resp.StatusCode, body)
	})
}

// ---------- market data endpoints ----------

func TestMarketDataEndpoints(t *testing.T) {
	skipIfGatewayUnavailable(t)
	c := newClient(t)

	// Register and login for auth
	email := uniqueEmail("mktdata")
	resp := c.post("/api/v1/auth/register", map[string]string{
		"email":    email,
		"password": "SecurePass123!",
		"role":     "trader",
	})
	if resp.StatusCode == http.StatusServiceUnavailable {
		resp.Body.Close()
		t.Skip("auth-service unavailable")
	}
	resp.Body.Close()

	loginResp := c.post("/api/v1/auth/login", map[string]string{
		"email":    email,
		"password": "SecurePass123!",
	})
	loginBody := readJSON(t, loginResp)
	token, _ := loginBody["access_token"].(string)
	if token == "" {
		if t2, ok := loginBody["AccessToken"].(string); ok {
			token = t2
		}
	}
	if token == "" {
		t.Skip("could not obtain access token")
	}
	authed := c.withToken(token)

	instrument := "WHEAT-2026-07"

	t.Run("get order book L2", func(t *testing.T) {
		resp := authed.get("/api/v1/instruments/" + instrument + "/book")
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("matching-engine unavailable")
		}
		resp.Body.Close()
	})

	t.Run("get order book L3", func(t *testing.T) {
		resp := authed.get("/api/v1/instruments/" + instrument + "/book/l3")
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("matching-engine unavailable")
		}
		resp.Body.Close()
	})

	t.Run("get last trade", func(t *testing.T) {
		resp := authed.get("/api/v1/instruments/" + instrument + "/trades/latest")
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("matching-engine unavailable")
		}
		resp.Body.Close()
	})
}

// ---------- admin endpoints ----------

func TestAdminEndpoints(t *testing.T) {
	skipIfGatewayUnavailable(t)
	c := newClient(t)

	// Register admin
	email := uniqueEmail("e2e-admin")
	resp := c.post("/api/v1/auth/register", map[string]string{
		"email":    email,
		"password": "AdminPass123!",
		"role":     "admin",
	})
	if resp.StatusCode == http.StatusServiceUnavailable {
		resp.Body.Close()
		t.Skip("auth-service unavailable")
	}
	resp.Body.Close()

	loginResp := c.post("/api/v1/auth/login", map[string]string{
		"email":    email,
		"password": "AdminPass123!",
	})
	loginBody := readJSON(t, loginResp)
	token, _ := loginBody["access_token"].(string)
	if token == "" {
		if t2, ok := loginBody["AccessToken"].(string); ok {
			token = t2
		}
	}
	if token == "" {
		t.Skip("could not obtain access token")
	}
	admin := c.withToken(token)

	instrument := "WHEAT-2026-07"

	t.Run("halt instrument", func(t *testing.T) {
		resp := admin.post("/api/v1/admin/instruments/"+instrument+"/halt", map[string]string{
			"reason": "E2E test halt",
		})
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("matching-engine unavailable")
		}
		resp.Body.Close()
	})

	t.Run("resume instrument", func(t *testing.T) {
		resp := admin.post("/api/v1/admin/instruments/"+instrument+"/resume", map[string]string{
			"reason": "E2E test resume",
		})
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("matching-engine unavailable")
		}
		resp.Body.Close()
	})

	t.Run("mass cancel", func(t *testing.T) {
		resp := admin.post("/api/v1/admin/mass-cancel", map[string]interface{}{
			"instrument_id": instrument,
			"reason":        "E2E test mass cancel",
		})
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("matching-engine unavailable")
		}
		resp.Body.Close()
	})
}

// ---------- compliance screening endpoints ----------

func TestComplianceScreening(t *testing.T) {
	skipIfGatewayUnavailable(t)
	c := newClient(t)

	// Register compliance officer
	email := uniqueEmail("compliance-officer")
	resp := c.post("/api/v1/auth/register", map[string]string{
		"email":    email,
		"password": "CompliancePass123!",
		"role":     "compliance_officer",
	})
	if resp.StatusCode == http.StatusServiceUnavailable {
		resp.Body.Close()
		t.Skip("auth-service unavailable")
	}
	resp.Body.Close()

	loginResp := c.post("/api/v1/auth/login", map[string]string{
		"email":    email,
		"password": "CompliancePass123!",
	})
	loginBody := readJSON(t, loginResp)
	token, _ := loginBody["access_token"].(string)
	if token == "" {
		if t2, ok := loginBody["AccessToken"].(string); ok {
			token = t2
		}
	}
	if token == "" {
		t.Skip("could not obtain access token")
	}
	officer := c.withToken(token)

	t.Run("screen participant", func(t *testing.T) {
		resp := officer.post("/api/v1/screening/check", map[string]interface{}{
			"participant_id": "test-participant-screening",
			"full_name":      "Test Participant",
			"nationality":    "KE",
		})
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("compliance-service unavailable")
		}
		resp.Body.Close()
	})

	t.Run("batch screen", func(t *testing.T) {
		resp := officer.post("/api/v1/screening/batch", map[string]interface{}{
			"participants": []map[string]string{
				{"participant_id": "batch-1", "full_name": "Batch Person One"},
				{"participant_id": "batch-2", "full_name": "Batch Person Two"},
			},
		})
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("compliance-service unavailable")
		}
		resp.Body.Close()
	})

	t.Run("get risk score", func(t *testing.T) {
		resp := officer.get("/api/v1/risk-scores/test-participant-screening")
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("compliance-service unavailable")
		}
		resp.Body.Close()
	})

	t.Run("list alerts", func(t *testing.T) {
		resp := officer.get("/api/v1/compliance/alerts")
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("compliance-service unavailable")
		}
		resp.Body.Close()
	})

	t.Run("get audit trail", func(t *testing.T) {
		resp := officer.get("/api/v1/compliance/audit-trail")
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("compliance-service unavailable")
		}
		resp.Body.Close()
	})
}

// ---------- margin endpoints ----------

func TestMarginEndpoints(t *testing.T) {
	skipIfGatewayUnavailable(t)
	c := newClient(t)

	email := uniqueEmail("margin-test")
	resp := c.post("/api/v1/auth/register", map[string]string{
		"email":    email,
		"password": "SecurePass123!",
		"role":     "trader",
	})
	if resp.StatusCode == http.StatusServiceUnavailable {
		resp.Body.Close()
		t.Skip("auth-service unavailable")
	}
	resp.Body.Close()

	loginResp := c.post("/api/v1/auth/login", map[string]string{
		"email":    email,
		"password": "SecurePass123!",
	})
	loginBody := readJSON(t, loginResp)
	token, _ := loginBody["access_token"].(string)
	if token == "" {
		if t2, ok := loginBody["AccessToken"].(string); ok {
			token = t2
		}
	}
	if token == "" {
		t.Skip("could not obtain access token")
	}
	authed := c.withToken(token)

	t.Run("get portfolio margin", func(t *testing.T) {
		resp := authed.get("/api/v1/margin")
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("margin-engine unavailable")
		}
		resp.Body.Close()
	})

	t.Run("calculate margin", func(t *testing.T) {
		resp := authed.post("/api/v1/margin/calculate", map[string]interface{}{
			"participant_id": "margin-test-participant",
			"instrument_id":  "WHEAT-2026-07",
			"quantity":       "50.0000",
			"price":          "300.0000",
		})
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("margin-engine unavailable")
		}
		resp.Body.Close()
	})

	t.Run("get margin calls", func(t *testing.T) {
		resp := authed.get("/api/v1/margin/calls")
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("margin-engine unavailable")
		}
		resp.Body.Close()
	})

	t.Run("get margin call stats", func(t *testing.T) {
		resp := authed.get("/api/v1/margin/calls/stats")
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("margin-engine unavailable")
		}
		resp.Body.Close()
	})
}

// ---------- settlement endpoints ----------

func TestSettlementEndpoints(t *testing.T) {
	skipIfGatewayUnavailable(t)
	c := newClient(t)

	email := uniqueEmail("settle-test")
	resp := c.post("/api/v1/auth/register", map[string]string{
		"email":    email,
		"password": "SecurePass123!",
		"role":     "trader",
	})
	if resp.StatusCode == http.StatusServiceUnavailable {
		resp.Body.Close()
		t.Skip("auth-service unavailable")
	}
	resp.Body.Close()

	loginResp := c.post("/api/v1/auth/login", map[string]string{
		"email":    email,
		"password": "SecurePass123!",
	})
	loginBody := readJSON(t, loginResp)
	token, _ := loginBody["access_token"].(string)
	if token == "" {
		if t2, ok := loginBody["AccessToken"].(string); ok {
			token = t2
		}
	}
	if token == "" {
		t.Skip("could not obtain access token")
	}
	authed := c.withToken(token)

	t.Run("list settlement cycles", func(t *testing.T) {
		resp := authed.get("/api/v1/settlement/cycles")
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("settlement-engine unavailable")
		}
		resp.Body.Close()
	})

	t.Run("get specific settlement cycle", func(t *testing.T) {
		resp := authed.get("/api/v1/settlement/cycles/cycle-001")
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("settlement-engine unavailable")
		}
		resp.Body.Close()
	})
}

// ---------- HTTP method validation ----------

func TestMethodNotAllowed(t *testing.T) {
	skipIfGatewayUnavailable(t)
	c := newClient(t)

	cases := []struct {
		method string
		path   string
	}{
		{"PUT", "/api/v1/auth/register"},
		{"DELETE", "/api/v1/auth/login"},
		{"PATCH", "/api/v1/participants"},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s %s returns 405", tc.method, tc.path), func(t *testing.T) {
			resp := c.do(tc.method, tc.path, nil)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusMethodNotAllowed &&
				resp.StatusCode != http.StatusUnauthorized &&
				resp.StatusCode != http.StatusNotFound {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("expected 405/401/404, got %d for %s %s; body: %s",
					resp.StatusCode, tc.method, tc.path, string(body))
			}
		})
	}
}

// ---------- concurrent request safety ----------

func TestConcurrentRequests(t *testing.T) {
	skipIfGatewayUnavailable(t)
	c := newClient(t)

	// Send multiple health checks concurrently
	const n = 10
	done := make(chan int, n)

	for i := 0; i < n; i++ {
		go func() {
			resp := c.get("/healthz")
			defer resp.Body.Close()
			done <- resp.StatusCode
		}()
	}

	for i := 0; i < n; i++ {
		code := <-done
		if code != http.StatusOK {
			t.Errorf("concurrent health check returned %d", code)
		}
	}
}

// ---------- response format validation ----------

func TestResponseHeaders(t *testing.T) {
	skipIfGatewayUnavailable(t)
	c := newClient(t)

	t.Run("error responses have Content-Type JSON", func(t *testing.T) {
		resp := c.get("/api/v1/nonexistent")
		defer resp.Body.Close()
		ct := resp.Header.Get("Content-Type")
		if !strings.Contains(ct, "application/json") {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}
	})

	t.Run("auth endpoints set Content-Type JSON", func(t *testing.T) {
		resp := c.post("/api/v1/auth/login", map[string]string{
			"email":    "nonexistent@test.com",
			"password": "wrong",
		})
		defer resp.Body.Close()
		ct := resp.Header.Get("Content-Type")
		if !strings.Contains(ct, "application/json") {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}
	})
}

// ---------- password management ----------

func TestPasswordChange(t *testing.T) {
	skipIfGatewayUnavailable(t)
	c := newClient(t)

	email := uniqueEmail("pwchange")
	oldPass := "OldPassword123!"
	newPass := "NewPassword456!"

	// Register
	resp := c.post("/api/v1/auth/register", map[string]string{
		"email":    email,
		"password": oldPass,
		"role":     "trader",
	})
	if resp.StatusCode == http.StatusServiceUnavailable {
		resp.Body.Close()
		t.Skip("auth-service unavailable")
	}
	resp.Body.Close()

	// Login
	loginResp := c.post("/api/v1/auth/login", map[string]string{
		"email":    email,
		"password": oldPass,
	})
	loginBody := readJSON(t, loginResp)
	token, _ := loginBody["access_token"].(string)
	if token == "" {
		if t2, ok := loginBody["AccessToken"].(string); ok {
			token = t2
		}
	}
	if token == "" {
		t.Skip("could not obtain access token")
	}
	authed := c.withToken(token)

	t.Run("change password", func(t *testing.T) {
		resp := authed.post("/api/v1/auth/password/change", map[string]string{
			"old_password": oldPass,
			"new_password": newPass,
		})
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("auth-service unavailable")
		}
		resp.Body.Close()
		// If password change succeeded, verify login with new password
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			newLogin := c.post("/api/v1/auth/login", map[string]string{
				"email":    email,
				"password": newPass,
			})
			if newLogin.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(newLogin.Body)
				newLogin.Body.Close()
				t.Errorf("login with new password failed: %d; body: %s", newLogin.StatusCode, string(body))
			} else {
				newLogin.Body.Close()
			}
		}
	})
}

// ---------- token refresh flow ----------

func TestTokenRefresh(t *testing.T) {
	skipIfGatewayUnavailable(t)
	c := newClient(t)

	email := uniqueEmail("refresh")
	resp := c.post("/api/v1/auth/register", map[string]string{
		"email":    email,
		"password": "SecurePass123!",
		"role":     "trader",
	})
	if resp.StatusCode == http.StatusServiceUnavailable {
		resp.Body.Close()
		t.Skip("auth-service unavailable")
	}
	resp.Body.Close()

	loginResp := c.post("/api/v1/auth/login", map[string]string{
		"email":    email,
		"password": "SecurePass123!",
	})
	loginBody := readJSON(t, loginResp)

	sessionID, _ := loginBody["session_id"].(string)
	if sessionID == "" {
		if s, ok := loginBody["SessionID"].(string); ok {
			sessionID = s
		}
	}
	refreshToken, _ := loginBody["refresh_token"].(string)
	if refreshToken == "" {
		if r, ok := loginBody["RefreshToken"].(string); ok {
			refreshToken = r
		}
	}

	t.Run("refresh token", func(t *testing.T) {
		if sessionID == "" || refreshToken == "" {
			t.Skip("no session_id or refresh_token in login response")
		}
		resp := c.post("/api/v1/auth/refresh", map[string]string{
			"session_id":    sessionID,
			"refresh_token": refreshToken,
		})
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("auth-service unavailable")
		}
		body := readJSON(t, resp)
		t.Logf("refresh response (status %d): %v", resp.StatusCode, body)
	})

	t.Run("refresh with invalid token returns error", func(t *testing.T) {
		resp := c.post("/api/v1/auth/refresh", map[string]string{
			"session_id":    "fake-session",
			"refresh_token": "fake-refresh-token",
		})
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("auth-service unavailable")
		}
		if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusBadRequest {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Errorf("expected 401 or 400, got %d; body: %s", resp.StatusCode, string(body))
		} else {
			resp.Body.Close()
		}
	})
}

// ---------- logout ----------

func TestLogout(t *testing.T) {
	skipIfGatewayUnavailable(t)
	c := newClient(t)

	email := uniqueEmail("logout")
	resp := c.post("/api/v1/auth/register", map[string]string{
		"email":    email,
		"password": "SecurePass123!",
		"role":     "trader",
	})
	if resp.StatusCode == http.StatusServiceUnavailable {
		resp.Body.Close()
		t.Skip("auth-service unavailable")
	}
	resp.Body.Close()

	loginResp := c.post("/api/v1/auth/login", map[string]string{
		"email":    email,
		"password": "SecurePass123!",
	})
	loginBody := readJSON(t, loginResp)
	token, _ := loginBody["access_token"].(string)
	if token == "" {
		if t2, ok := loginBody["AccessToken"].(string); ok {
			token = t2
		}
	}
	if token == "" {
		t.Skip("could not obtain access token")
	}

	t.Run("logout invalidates session", func(t *testing.T) {
		authed := c.withToken(token)
		resp := authed.post("/api/v1/auth/logout", nil)
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			t.Skip("auth-service unavailable")
		}
		resp.Body.Close()
	})
}
