// Package server — tests for the SSE (Server-Sent Events) endpoint.
package server

import (
	"bufio"
	"net/http"
	"strings"
	"testing"
)

// ============================================================
// TestSSEEndpoint_Returns200 — Content-Type text/event-stream
// ============================================================

// TestSSEEndpoint_Returns200 verifies that the SSE endpoint:
//   - Responds with HTTP 200
//   - Sets Content-Type to text/event-stream
//   - Sends at least one SSE event before the client closes the connection
func TestSSEEndpoint_Returns200(t *testing.T) {
	ts := newTestServer(t)

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/securities/events", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("X-GarudaX-Tenant", testTenant)

	// Use a client that does NOT follow redirects and closes immediately
	// after reading the first chunk of data.
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("expected Content-Type text/event-stream, got %q", ct)
	}

	// Read the initial "connected" event from the stream.
	scanner := bufio.NewScanner(resp.Body)
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
		// Stop after reading the first double-newline (end of first event).
		if line == "" && len(lines) >= 2 {
			break
		}
	}

	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines from SSE stream, got %d", len(lines))
	}

	// The first event should be "event: connected".
	found := false
	for _, l := range lines {
		if strings.HasPrefix(l, "event:") || strings.HasPrefix(l, "data:") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected SSE event or data line, got lines: %v", lines)
	}
}

// TestSSEEndpoint_MethodNotAllowed verifies POST to the SSE endpoint returns 405.
func TestSSEEndpoint_MethodNotAllowed(t *testing.T) {
	ts := newTestServer(t)

	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/events", nil)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestSSEEndpoint_CacheControlNoCache verifies the SSE response carries
// the Cache-Control: no-cache directive required for EventSource clients.
func TestSSEEndpoint_CacheControlNoCache(t *testing.T) {
	ts := newTestServer(t)

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/securities/events", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("X-GarudaX-Tenant", testTenant)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	cc := resp.Header.Get("Cache-Control")
	if !strings.Contains(cc, "no-cache") {
		t.Errorf("expected Cache-Control: no-cache, got %q", cc)
	}
}
