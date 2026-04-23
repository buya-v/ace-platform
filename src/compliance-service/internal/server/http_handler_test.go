package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garudax-platform/compliance-service/internal/onboarding"
	"github.com/garudax-platform/compliance-service/internal/screening"
)

// newTestHTTPServer creates a Server with in-memory stores and returns an httptest.Server
// backed by the compliance service route handlers.
func newTestHTTPServer(t *testing.T) *httptest.Server {
	t.Helper()
	onboardStore := onboarding.NewInMemoryStore()
	onboardSvc := onboarding.NewService(onboardStore)
	screenStore := screening.NewInMemoryStore()
	screeningSvc := screening.NewService(screenStore, nil, onboardStore)
	srv := NewServer(onboardSvc, screeningSvc, DefaultConfig())
	mux := http.NewServeMux()
	srv.mountRoutes(mux)
	return httptest.NewServer(mux)
}

// mustMarshal marshals v to JSON and panics on error (test helper only).
func mustMarshal(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// TestSubmitApplicationReturnsObject verifies that POST /application returns a
// JSON object (map with "application_id" key), not a JSON array.
// This is the contract test for T102: fix compliance KYC endpoint response format.
func TestSubmitApplicationReturnsObject(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()

	body := mustMarshal(map[string]interface{}{
		"participant_id":   "part-test-001",
		"participant_type": "INDIVIDUAL",
		"legal_name":       "Test User E2E",
		"nationality":      "KE",
		"source_of_funds":  "Trading income",
		"contact": map[string]string{
			"email": "test@example.com",
			"phone": "+254700100001",
		},
		"registered_address": map[string]string{
			"line1":       "123 Test Street",
			"city":        "Nairobi",
			"postal_code": "00100",
			"country":     "KE",
		},
	})

	resp, err := http.Post(ts.URL+"/application", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /application failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201 Created, got %d: %s", resp.StatusCode, string(data))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	// Must be a JSON object (not an array).
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("POST /application: expected JSON object, got parse error: %v\nbody: %s", err, string(data))
	}

	// Verify required snake_case fields are present (json tags applied).
	for _, field := range []string{"application_id", "participant_id", "status"} {
		if _, ok := result[field]; !ok {
			t.Errorf("POST /application: expected field %q in response, got keys: %v", field, responseKeys(result))
		}
	}

	if result["application_id"] == "" || result["application_id"] == nil {
		t.Errorf("POST /application: application_id must not be empty")
	}
	if result["participant_id"] != "part-test-001" {
		t.Errorf("POST /application: expected participant_id=part-test-001, got %v", result["participant_id"])
	}
	if result["status"] == "" || result["status"] == nil {
		t.Errorf("POST /application: status must not be empty")
	}
}

// TestSubmitApplicationValidation verifies that validation errors return 400 with plain text.
func TestSubmitApplicationValidation(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()

	cases := []struct {
		name string
		body map[string]interface{}
		want int
	}{
		{
			name: "missing participant_id",
			body: map[string]interface{}{
				"participant_type": "INDIVIDUAL",
				"legal_name":       "Test User",
				"nationality":      "KE",
			},
			want: http.StatusBadRequest,
		},
		{
			name: "invalid nationality (3 chars)",
			body: map[string]interface{}{
				"participant_id":   "p1",
				"participant_type": "INDIVIDUAL",
				"legal_name":       "Test User",
				"nationality":      "KEN",
			},
			want: http.StatusBadRequest,
		},
		{
			name: "invalid participant_type",
			body: map[string]interface{}{
				"participant_id":   "p1",
				"participant_type": "UNKNOWN",
				"legal_name":       "Test User",
				"nationality":      "KE",
			},
			want: http.StatusBadRequest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Post(ts.URL+"/application", "application/json", bytes.NewReader(mustMarshal(tc.body)))
			if err != nil {
				t.Fatalf("POST /application: %v", err)
			}
			resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Errorf("expected status %d, got %d", tc.want, resp.StatusCode)
			}
		})
	}
}

// TestListApplicationsReturnsWrappedObject verifies that GET /applications returns
// {"applications":[...],"total":N}, not a bare array.
func TestListApplicationsReturnsWrappedObject(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()

	// First create one application
	body := mustMarshal(map[string]interface{}{
		"participant_id":   "part-list-001",
		"participant_type": "INDIVIDUAL",
		"legal_name":       "List Test User",
		"nationality":      "KE",
	})
	createResp, err := http.Post(ts.URL+"/application", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create application: %v", err)
	}
	createResp.Body.Close()

	// Now list
	resp, err := http.Get(ts.URL + "/applications")
	if err != nil {
		t.Fatalf("GET /applications: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	data, _ := io.ReadAll(resp.Body)

	// Must be a JSON object with "applications" and "total" keys (NOT a bare array).
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("GET /applications: expected JSON object, got: %v\nbody: %s", err, string(data))
	}

	if _, ok := result["applications"]; !ok {
		t.Errorf("GET /applications: expected 'applications' key, got: %v", responseKeys(result))
	}
	if _, ok := result["total"]; !ok {
		t.Errorf("GET /applications: expected 'total' key, got: %v", responseKeys(result))
	}
}

// TestListApplicationsEmptyReturnsEmptyArray verifies that GET /applications with
// no data returns {"applications":[],"total":0} rather than {"applications":null,...}.
func TestListApplicationsEmptyReturnsEmptyArray(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/applications")
	if err != nil {
		t.Fatalf("GET /applications: %v", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal: %v, body: %s", err, string(data))
	}

	apps, ok := result["applications"]
	if !ok {
		t.Fatalf("missing 'applications' key")
	}
	// apps should be a JSON array (not null) — Go slice encodes as [] not null
	if apps == nil {
		t.Errorf("GET /applications: 'applications' must be [] not null when empty")
	}
}

// TestSubmitApplicationResponseFields verifies the snake_case JSON fields
// from the KYCApplication type tags.
func TestSubmitApplicationResponseFields(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()

	body := mustMarshal(map[string]interface{}{
		"participant_id":   "part-fields-001",
		"participant_type": "CORPORATE",
		"legal_name":       "Fields Test Corp",
		"nationality":      "ZA",
		"source_of_funds":  "Business revenue",
	})

	resp, err := http.Post(ts.URL+"/application", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /application: %v", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal: %v, body: %s", err, string(data))
	}

	// Verify snake_case field names (not PascalCase).
	// These are set by the json tags on KYCApplication.
	snakeCaseFields := []string{
		"application_id",
		"participant_id",
		"participant_type",
		"status",
		"legal_name",
		"nationality",
		"created_at",
		"updated_at",
	}
	for _, field := range snakeCaseFields {
		if _, ok := result[field]; !ok {
			t.Errorf("expected snake_case field %q in response; got keys: %v", field, responseKeys(result))
		}
	}

	// PascalCase should NOT appear (old behaviour without json tags).
	pascalCaseFields := []string{
		"ApplicationID",
		"ParticipantID",
		"ParticipantType",
		"LegalName",
	}
	for _, field := range pascalCaseFields {
		if _, ok := result[field]; ok {
			t.Errorf("PascalCase field %q must not appear in response (add json tags)", field)
		}
	}
}

// responseKeys returns a slice of keys from a map for error messages.
func responseKeys(m map[string]interface{}) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
