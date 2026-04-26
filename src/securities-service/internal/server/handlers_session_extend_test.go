// Package server — tests for session extend/shorten HTTP handlers.
package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// doJSONNoTenant sends a JSON request without the X-GarudaX-Tenant header.
// Used to test tenant-required rejection paths.
func doJSONNoTenant(t *testing.T, ts *httptest.Server, method, path string, body interface{}) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("json encode: %v", err)
		}
	}
	req, err := http.NewRequest(method, ts.URL+path, &buf)
	if err != nil {
		t.Fatalf("http.NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// Deliberately omit X-GarudaX-Tenant.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http.Do: %v", err)
	}
	return resp
}

// ============================================================
// TestExtendSession
// ============================================================

func TestExtendSession(t *testing.T) {
	ts := newTestServer(t)

	t.Run("extend returns 200 with new_end_time set", func(t *testing.T) {
		before := time.Now().UTC()

		payload := map[string]interface{}{
			"duration_minutes": 30,
			"reason":           "volatility event",
		}
		resp := doJSON(t, ts, http.MethodPost,
			"/api/v1/securities/sessions/INST-001/extend", payload)
		assertStatus(t, resp, http.StatusOK)

		var body map[string]interface{}
		decodeBody(t, resp, &body)

		if body["action"] != "EXTEND" {
			t.Errorf("action: want EXTEND, got %v", body["action"])
		}
		if body["instrument_id"] != "INST-001" {
			t.Errorf("instrument_id: want INST-001, got %v", body["instrument_id"])
		}
		if body["duration_minutes"].(float64) != 30 {
			t.Errorf("duration_minutes: want 30, got %v", body["duration_minutes"])
		}
		if body["reason"] != "volatility event" {
			t.Errorf("reason: want 'volatility event', got %v", body["reason"])
		}

		// new_end_time must be set and approximately now + 30 minutes.
		rawEnd, ok := body["new_end_time"].(string)
		if !ok || rawEnd == "" {
			t.Fatalf("new_end_time missing or empty in response")
		}
		endTime, err := time.Parse(time.RFC3339, rawEnd)
		if err != nil {
			t.Fatalf("new_end_time parse error: %v", err)
		}
		// Allow ±5s tolerance around expected end.
		expectedEnd := before.Add(30 * time.Minute)
		diff := endTime.Sub(expectedEnd)
		if diff < -5*time.Second || diff > 5*time.Second {
			t.Errorf("new_end_time %v not within 5s of expected %v", endTime, expectedEnd)
		}
	})

	t.Run("extend with zero duration returns 400", func(t *testing.T) {
		payload := map[string]interface{}{
			"duration_minutes": 0,
		}
		resp := doJSON(t, ts, http.MethodPost,
			"/api/v1/securities/sessions/INST-001/extend", payload)
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})

	t.Run("extend with negative duration returns 400", func(t *testing.T) {
		payload := map[string]interface{}{
			"duration_minutes": -10,
		}
		resp := doJSON(t, ts, http.MethodPost,
			"/api/v1/securities/sessions/INST-001/extend", payload)
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})

	t.Run("extend without tenant header returns 401", func(t *testing.T) {
		resp := doJSONNoTenant(t, ts, http.MethodPost,
			"/api/v1/securities/sessions/INST-001/extend",
			map[string]interface{}{"duration_minutes": 15})
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 without tenant, got %d", resp.StatusCode)
		}
	})

	t.Run("GET method on extend returns 405", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet,
			"/api/v1/securities/sessions/INST-001/extend", nil)
		assertStatus(t, resp, http.StatusMethodNotAllowed)
		resp.Body.Close()
	})
}

// ============================================================
// TestShortenSession
// ============================================================

func TestShortenSession(t *testing.T) {
	ts := newTestServer(t)

	t.Run("shorten returns 200 with new_end_time set", func(t *testing.T) {
		before := time.Now().UTC()

		payload := map[string]interface{}{
			"duration_minutes": 10,
			"reason":           "early close",
		}
		resp := doJSON(t, ts, http.MethodPost,
			"/api/v1/securities/sessions/INST-002/shorten", payload)
		assertStatus(t, resp, http.StatusOK)

		var body map[string]interface{}
		decodeBody(t, resp, &body)

		if body["action"] != "SHORTEN" {
			t.Errorf("action: want SHORTEN, got %v", body["action"])
		}
		if body["instrument_id"] != "INST-002" {
			t.Errorf("instrument_id: want INST-002, got %v", body["instrument_id"])
		}
		if body["duration_minutes"].(float64) != 10 {
			t.Errorf("duration_minutes: want 10, got %v", body["duration_minutes"])
		}

		rawEnd, ok := body["new_end_time"].(string)
		if !ok || rawEnd == "" {
			t.Fatalf("new_end_time missing or empty in response")
		}
		endTime, err := time.Parse(time.RFC3339, rawEnd)
		if err != nil {
			t.Fatalf("new_end_time parse error: %v", err)
		}
		// new_end_time ≈ now + 10 minutes (±5s).
		expectedEnd := before.Add(10 * time.Minute)
		diff := endTime.Sub(expectedEnd)
		if diff < -5*time.Second || diff > 5*time.Second {
			t.Errorf("new_end_time %v not within 5s of expected %v", endTime, expectedEnd)
		}
	})

	t.Run("shorten without reason is accepted", func(t *testing.T) {
		payload := map[string]interface{}{
			"duration_minutes": 5,
		}
		resp := doJSON(t, ts, http.MethodPost,
			"/api/v1/securities/sessions/INST-003/shorten", payload)
		assertStatus(t, resp, http.StatusOK)

		var body map[string]interface{}
		decodeBody(t, resp, &body)
		if body["action"] != "SHORTEN" {
			t.Errorf("action: want SHORTEN, got %v", body["action"])
		}
	})

	t.Run("shorten with zero duration returns 400", func(t *testing.T) {
		payload := map[string]interface{}{
			"duration_minutes": 0,
		}
		resp := doJSON(t, ts, http.MethodPost,
			"/api/v1/securities/sessions/INST-002/shorten", payload)
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})

	t.Run("GET method on shorten returns 405", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet,
			"/api/v1/securities/sessions/INST-001/shorten", nil)
		assertStatus(t, resp, http.StatusMethodNotAllowed)
		resp.Body.Close()
	})
}
