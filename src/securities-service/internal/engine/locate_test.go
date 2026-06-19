package engine

import (
	"errors"
	"testing"
	"time"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// fixedClock returns a clock function pinned to t.
func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func TestLocateEngine_Request_Success(t *testing.T) {
	ls := store.NewInMemoryLocateStore()
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	e := NewLocateEngine(ls).WithClock(fixedClock(now))

	req := &types.LocateRequest{InstrumentID: 1, BorrowerFirmID: 10, Quantity: 100}
	if err := e.Request(req); err != nil {
		t.Fatalf("Request: unexpected error: %v", err)
	}
	if req.ID == 0 {
		t.Errorf("expected assigned ID, got 0")
	}
	if req.Status != LocateStatusPending {
		t.Errorf("expected PENDING status, got %q", req.Status)
	}
	// Default expiry should be now+24h.
	wantExp := now.Add(defaultLocateTTL).Format(time.RFC3339)
	if req.ExpiresAt != wantExp {
		t.Errorf("expected default expiry %q, got %q", wantExp, req.ExpiresAt)
	}
}

func TestLocateEngine_Request_HonoursExplicitExpiry(t *testing.T) {
	ls := store.NewInMemoryLocateStore()
	e := NewLocateEngine(ls)
	exp := "2030-01-01T00:00:00Z"
	req := &types.LocateRequest{InstrumentID: 1, BorrowerFirmID: 10, Quantity: 5, ExpiresAt: exp}
	if err := e.Request(req); err != nil {
		t.Fatalf("Request: %v", err)
	}
	if req.ExpiresAt != exp {
		t.Errorf("explicit expiry overwritten: got %q", req.ExpiresAt)
	}
}

func TestLocateEngine_Request_Validation(t *testing.T) {
	e := NewLocateEngine(store.NewInMemoryLocateStore())
	cases := []struct {
		name string
		req  *types.LocateRequest
		code string
	}{
		{"nil body", nil, CodeMissingField},
		{"missing instrument", &types.LocateRequest{BorrowerFirmID: 10, Quantity: 1}, CodeMissingField},
		{"missing borrower", &types.LocateRequest{InstrumentID: 1, Quantity: 1}, CodeMissingField},
		{"zero quantity", &types.LocateRequest{InstrumentID: 1, BorrowerFirmID: 10, Quantity: 0}, CodeInvalidField},
		{"negative quantity", &types.LocateRequest{InstrumentID: 1, BorrowerFirmID: 10, Quantity: -5}, CodeInvalidField},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := e.Request(tc.req)
			var ssErr *ShortSellError
			if !errors.As(err, &ssErr) {
				t.Fatalf("expected *ShortSellError, got %v", err)
			}
			if ssErr.Code != tc.code {
				t.Errorf("expected code %q, got %q", tc.code, ssErr.Code)
			}
		})
	}
}

func TestLocateEngine_Request_NilStore(t *testing.T) {
	e := NewLocateEngine(nil)
	err := e.Request(&types.LocateRequest{InstrumentID: 1, BorrowerFirmID: 10, Quantity: 1})
	if err == nil {
		t.Fatal("expected error with nil store")
	}
}

func TestLocateEngine_ApproveAndConsume(t *testing.T) {
	ls := store.NewInMemoryLocateStore()
	e := NewLocateEngine(ls)

	req := &types.LocateRequest{InstrumentID: 7, BorrowerFirmID: 3, Quantity: 100}
	if err := e.Request(req); err != nil {
		t.Fatalf("Request: %v", err)
	}
	id := "1"

	// Cannot consume while PENDING.
	if err := e.Consume(id, 7, 3, 50); err == nil {
		t.Fatal("expected error consuming a PENDING locate")
	}

	if err := e.Approve(id, "LENDER-1"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if err := e.Consume(id, 7, 3, 50); err != nil {
		t.Fatalf("Consume after approve: %v", err)
	}
	// Second consume must fail — already USED.
	if err := e.Consume(id, 7, 3, 50); err == nil {
		t.Fatal("expected error consuming an already-USED locate")
	}
}

func TestLocateEngine_Approve_NilStore(t *testing.T) {
	if err := NewLocateEngine(nil).Approve("1", "L"); err == nil {
		t.Fatal("expected error with nil store")
	}
}

func TestLocateEngine_Consume_NotFound(t *testing.T) {
	e := NewLocateEngine(store.NewInMemoryLocateStore())
	err := e.Consume("999", 1, 1, 1)
	if err != ErrLocateNotFound {
		t.Fatalf("expected ErrLocateNotFound, got %v", err)
	}
}

func TestLocateEngine_Consume_NilStore(t *testing.T) {
	if err := NewLocateEngine(nil).Consume("1", 1, 1, 1); err == nil {
		t.Fatal("expected error with nil store")
	}
}

func TestLocateEngine_Validate_Rules(t *testing.T) {
	e := NewLocateEngine(nil) // Validate is pure; store not needed.

	approved := func() *types.LocateRequest {
		return &types.LocateRequest{
			ID: 1, InstrumentID: 5, BorrowerFirmID: 9, Quantity: 100,
			Status: LocateStatusApproved, ExpiresAt: "2030-01-01T00:00:00Z",
		}
	}

	t.Run("nil locate", func(t *testing.T) {
		if err := e.Validate(nil, 5, 9, 1); err != ErrLocateNotFound {
			t.Errorf("expected ErrLocateNotFound, got %v", err)
		}
	})
	t.Run("not approved", func(t *testing.T) {
		loc := approved()
		loc.Status = LocateStatusPending
		err := e.Validate(loc, 5, 9, 1)
		var ssErr *ShortSellError
		if !errors.As(err, &ssErr) || ssErr.Code != CodeInvalidLocate {
			t.Errorf("expected INVALID_LOCATE, got %v", err)
		}
	})
	t.Run("instrument mismatch", func(t *testing.T) {
		if err := e.Validate(approved(), 6, 9, 1); err != ErrLocateInstrumentMismatch {
			t.Errorf("expected instrument mismatch, got %v", err)
		}
	})
	t.Run("borrower mismatch", func(t *testing.T) {
		if err := e.Validate(approved(), 5, 8, 1); err != ErrLocateBorrowerMismatch {
			t.Errorf("expected borrower mismatch, got %v", err)
		}
	})
	t.Run("insufficient quantity", func(t *testing.T) {
		if err := e.Validate(approved(), 5, 9, 101); err != ErrLocateInsufficientQty {
			t.Errorf("expected insufficient qty, got %v", err)
		}
	})
	t.Run("zero context skips cross-checks", func(t *testing.T) {
		if err := e.Validate(approved(), 0, 0, 0); err != nil {
			t.Errorf("expected nil with zero context, got %v", err)
		}
	})
	t.Run("valid", func(t *testing.T) {
		if err := e.Validate(approved(), 5, 9, 100); err != nil {
			t.Errorf("expected valid, got %v", err)
		}
	})
}

func TestLocateEngine_Expiry(t *testing.T) {
	past := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	loc := &types.LocateRequest{
		ID: 1, InstrumentID: 5, BorrowerFirmID: 9, Quantity: 100,
		Status: LocateStatusApproved,
		// Expired one hour before "now".
		ExpiresAt: past.Add(-1 * time.Hour).Format(time.RFC3339),
	}
	e := NewLocateEngine(nil).WithClock(fixedClock(past))

	if !e.IsExpired(loc) {
		t.Fatal("expected locate to be expired")
	}
	if err := e.Validate(loc, 5, 9, 1); err != ErrLocateExpired {
		t.Fatalf("expected ErrLocateExpired, got %v", err)
	}

	// Future expiry is not expired.
	loc.ExpiresAt = past.Add(time.Hour).Format(time.RFC3339)
	if e.IsExpired(loc) {
		t.Error("future expiry should not be expired")
	}
}

func TestLocateEngine_IsExpired_EdgeCases(t *testing.T) {
	e := NewLocateEngine(nil)
	if e.IsExpired(nil) {
		t.Error("nil locate must not be expired")
	}
	if e.IsExpired(&types.LocateRequest{}) {
		t.Error("empty expiry must not be expired")
	}
	if e.IsExpired(&types.LocateRequest{ExpiresAt: "not-a-date"}) {
		t.Error("unparseable expiry must not be treated as expired")
	}
}

func TestLocateEngine_WithClock_NilIgnored(t *testing.T) {
	e := NewLocateEngine(nil)
	orig := e.now
	e.WithClock(nil)
	if e.now == nil {
		t.Fatal("WithClock(nil) must not clear the clock")
	}
	_ = orig
}
