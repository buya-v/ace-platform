// Package server — tests for participant HTTP handlers including lock/unlock.
package server

import (
	"fmt"
	"net/http"
	"testing"
)

// ============================================================
// TestLockParticipant
// ============================================================

func TestLockParticipant(t *testing.T) {
	ts := newTestServer(t)

	// Create a firm and participant first.
	firmPayload := map[string]interface{}{
		"id":   "firm-lock-test",
		"name": "Lock Test Firm",
	}
	firmResp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/firms", firmPayload)
	assertStatus(t, firmResp, http.StatusCreated)
	firmResp.Body.Close()

	pPayload := map[string]interface{}{
		"id":          "part-lock-test",
		"firm_id":     "firm-lock-test",
		"name":        "Lock Test Trader",
		"permissions": []string{"ORDER_CREATE"},
	}
	createResp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/participants", pPayload)
	assertStatus(t, createResp, http.StatusCreated)
	createResp.Body.Close()

	t.Run("lock participant sets PARTICIPANT_LOCKED status", func(t *testing.T) {
		payload := map[string]interface{}{
			"reason": "compliance investigation",
		}
		resp := doJSON(t, ts, http.MethodPost,
			"/api/v1/securities/participants/part-lock-test/lock", payload)
		assertStatus(t, resp, http.StatusOK)

		var body map[string]interface{}
		decodeBody(t, resp, &body)

		if body["status"] != "PARTICIPANT_LOCKED" {
			t.Errorf("status: want PARTICIPANT_LOCKED, got %v", body["status"])
		}
		if body["lock_reason"] != "compliance investigation" {
			t.Errorf("lock_reason: want 'compliance investigation', got %v", body["lock_reason"])
		}
		if body["locked_at"] == nil || body["locked_at"] == "" {
			t.Error("locked_at should be set after lock")
		}
	})

	t.Run("lock non-existent participant returns 404", func(t *testing.T) {
		payload := map[string]interface{}{
			"reason": "test",
		}
		resp := doJSON(t, ts, http.MethodPost,
			"/api/v1/securities/participants/no-such-participant/lock", payload)
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})

	t.Run("lock without reason is accepted", func(t *testing.T) {
		// Create another participant.
		p2 := map[string]interface{}{
			"id":          "part-lock-noreason",
			"firm_id":     "firm-lock-test",
			"name":        "No Reason Trader",
			"permissions": []string{},
		}
		r := doJSON(t, ts, http.MethodPost, "/api/v1/securities/participants", p2)
		assertStatus(t, r, http.StatusCreated)
		r.Body.Close()

		resp := doJSON(t, ts, http.MethodPost,
			"/api/v1/securities/participants/part-lock-noreason/lock",
			map[string]interface{}{})
		assertStatus(t, resp, http.StatusOK)

		var body map[string]interface{}
		decodeBody(t, resp, &body)
		if body["status"] != "PARTICIPANT_LOCKED" {
			t.Errorf("status: want PARTICIPANT_LOCKED, got %v", body["status"])
		}
	})

	t.Run("GET method on lock returns 405", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet,
			"/api/v1/securities/participants/part-lock-test/lock", nil)
		assertStatus(t, resp, http.StatusMethodNotAllowed)
		resp.Body.Close()
	})
}

// ============================================================
// TestUnlockParticipant
// ============================================================

func TestUnlockParticipant(t *testing.T) {
	ts := newTestServer(t)

	// Create firm and participant.
	firmPayload := map[string]interface{}{
		"id":   "firm-unlock-test",
		"name": "Unlock Test Firm",
	}
	firmResp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/firms", firmPayload)
	assertStatus(t, firmResp, http.StatusCreated)
	firmResp.Body.Close()

	pPayload := map[string]interface{}{
		"id":          "part-unlock-test",
		"firm_id":     "firm-unlock-test",
		"name":        "Unlock Test Trader",
		"permissions": []string{"ORDER_CREATE"},
	}
	createResp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/participants", pPayload)
	assertStatus(t, createResp, http.StatusCreated)
	createResp.Body.Close()

	// Lock the participant first.
	lockPayload := map[string]interface{}{
		"reason": "test lock",
	}
	lockResp := doJSON(t, ts, http.MethodPost,
		"/api/v1/securities/participants/part-unlock-test/lock", lockPayload)
	assertStatus(t, lockResp, http.StatusOK)
	lockResp.Body.Close()

	t.Run("unlock sets PARTICIPANT_ACTIVE and clears lock fields", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodPost,
			"/api/v1/securities/participants/part-unlock-test/unlock", map[string]interface{}{})
		assertStatus(t, resp, http.StatusOK)

		var body map[string]interface{}
		decodeBody(t, resp, &body)

		if body["status"] != "PARTICIPANT_ACTIVE" {
			t.Errorf("status: want PARTICIPANT_ACTIVE, got %v", body["status"])
		}
		// lock_reason and locked_at should be cleared.
		if reason, ok := body["lock_reason"]; ok && reason != "" && reason != nil {
			t.Errorf("lock_reason should be cleared after unlock, got %v", reason)
		}
		if lockedAt, ok := body["locked_at"]; ok && lockedAt != "" && lockedAt != nil {
			t.Errorf("locked_at should be cleared after unlock, got %v", lockedAt)
		}
	})

	t.Run("unlock non-existent participant returns 404", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodPost,
			"/api/v1/securities/participants/no-such-participant/unlock",
			map[string]interface{}{})
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})

	t.Run("GET method on unlock returns 405", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet,
			"/api/v1/securities/participants/part-unlock-test/unlock", nil)
		assertStatus(t, resp, http.StatusMethodNotAllowed)
		resp.Body.Close()
	})
}

// ============================================================
// TestLockUnlock_RoundTrip
// ============================================================

func TestLockUnlock_RoundTrip(t *testing.T) {
	ts := newTestServer(t)

	// Setup.
	doJSON(t, ts, http.MethodPost, "/api/v1/securities/firms",
		map[string]interface{}{"id": "firm-rt", "name": "Round Trip Firm"}).Body.Close()

	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("part-rt-%d", i)
		r := doJSON(t, ts, http.MethodPost, "/api/v1/securities/participants",
			map[string]interface{}{
				"id":          id,
				"firm_id":     "firm-rt",
				"name":        fmt.Sprintf("Trader %d", i),
				"permissions": []string{},
			})
		assertStatus(t, r, http.StatusCreated)
		r.Body.Close()
	}

	t.Run("lock then unlock restores ACTIVE status", func(t *testing.T) {
		// Lock.
		lr := doJSON(t, ts, http.MethodPost,
			"/api/v1/securities/participants/part-rt-0/lock",
			map[string]interface{}{"reason": "round-trip test"})
		assertStatus(t, lr, http.StatusOK)
		var locked map[string]interface{}
		decodeBody(t, lr, &locked)
		if locked["status"] != "PARTICIPANT_LOCKED" {
			t.Fatalf("after lock: want PARTICIPANT_LOCKED, got %v", locked["status"])
		}

		// Unlock.
		ur := doJSON(t, ts, http.MethodPost,
			"/api/v1/securities/participants/part-rt-0/unlock",
			map[string]interface{}{})
		assertStatus(t, ur, http.StatusOK)
		var unlocked map[string]interface{}
		decodeBody(t, ur, &unlocked)
		if unlocked["status"] != "PARTICIPANT_ACTIVE" {
			t.Fatalf("after unlock: want PARTICIPANT_ACTIVE, got %v", unlocked["status"])
		}
	})
}
