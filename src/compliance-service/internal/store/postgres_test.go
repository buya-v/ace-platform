package store

import (
	"database/sql"
	"testing"
	"time"
)

func TestNullableString(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"hello", true},
		{"", false},
		{"some value", true},
	}
	for _, tt := range tests {
		ns := nullableString(tt.input)
		if ns.Valid != tt.valid {
			t.Errorf("nullableString(%q): expected valid=%v, got %v", tt.input, tt.valid, ns.Valid)
		}
		if tt.valid && ns.String != tt.input {
			t.Errorf("nullableString(%q): expected %q, got %q", tt.input, tt.input, ns.String)
		}
	}
}

func TestNullableTime(t *testing.T) {
	zero := time.Time{}
	now := time.Now()

	nt := nullableTime(zero)
	if nt.Valid {
		t.Error("expected zero time to produce invalid NullTime")
	}

	nt = nullableTime(now)
	if !nt.Valid {
		t.Error("expected non-zero time to produce valid NullTime")
	}
	if !nt.Time.Equal(now) {
		t.Error("expected time value to match")
	}
}

func TestOpenPostgres_InvalidDSN(t *testing.T) {
	// sql.Open with pgx driver doesn't fail on invalid DSN at open time
	// (it fails on first query). But we can test that Open itself doesn't panic.
	// We skip this if the pgx driver is not registered.
	_, err := sql.Open("pgx", "postgres://invalid:invalid@localhost:99999/nonexistent")
	if err != nil {
		// Driver not registered — that's OK for unit tests without the pgx import
		t.Skipf("pgx driver not registered: %v", err)
	}
}

func TestNewPostgresOnboardingStore(t *testing.T) {
	// Verify constructor doesn't panic with nil db (it just stores the reference)
	store := NewPostgresOnboardingStore(nil)
	if store == nil {
		t.Error("expected non-nil store")
	}
	if store.db != nil {
		t.Error("expected nil db in store")
	}
}

func TestNewPostgresScreeningStore(t *testing.T) {
	store := NewPostgresScreeningStore(nil)
	if store == nil {
		t.Error("expected non-nil store")
	}
	if store.db != nil {
		t.Error("expected nil db in store")
	}
}
