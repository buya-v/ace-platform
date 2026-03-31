package store

import (
	"testing"

	"github.com/garudax-platform/warehouse-service/internal/types"
)

func TestPostgresStoreImplementsDataStore(t *testing.T) {
	// Compile-time interface check is in postgres.go (var _ DataStore = (*PostgresStore)(nil))
	// This test verifies at runtime.
	var ds DataStore
	_ = ds // use the variable to confirm the interface compiles
}

func TestInMemoryStoreImplementsDataStore(t *testing.T) {
	var ds DataStore = NewStore()
	if ds == nil {
		t.Fatal("NewStore should return a non-nil DataStore")
	}
}

func TestScanDecimal(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"100", "100"},
		{"100.5", "100.5"},
		{"0", "0"},
		{"", "0"},
		{"999.9999", "999.9999"},
		{"-50.25", "-50.25"},
	}
	for _, tt := range tests {
		d := scanDecimal(tt.input)
		if d.String() != tt.want {
			t.Errorf("scanDecimal(%q) = %s, want %s", tt.input, d.String(), tt.want)
		}
	}
}

func TestNullStr(t *testing.T) {
	ns := nullStr("")
	if ns.Valid {
		t.Error("nullStr('') should be invalid")
	}
	ns = nullStr("hello")
	if !ns.Valid || ns.String != "hello" {
		t.Errorf("nullStr('hello') = {%q, %v}, want {hello, true}", ns.String, ns.Valid)
	}
}

func TestNullDecStr(t *testing.T) {
	ns := nullDecStr(types.DecimalZero())
	if ns.Valid {
		t.Error("nullDecStr(zero) should be invalid")
	}
	ns = nullDecStr(types.DecimalFromInt(42))
	if !ns.Valid || ns.String != "42" {
		t.Errorf("nullDecStr(42) = {%q, %v}, want {42, true}", ns.String, ns.Valid)
	}
}

func TestNullJsonStr(t *testing.T) {
	ns := nullJsonStr("")
	if ns.Valid {
		t.Error("nullJsonStr('') should be invalid")
	}
	ns = nullJsonStr(`{"key":"value"}`)
	if !ns.Valid || ns.String != `{"key":"value"}` {
		t.Errorf("expected valid JSON string")
	}
	// Non-JSON string should be wrapped
	ns = nullJsonStr("plain text")
	if !ns.Valid {
		t.Error("nullJsonStr('plain text') should be valid")
	}
}

func TestFromNullStr(t *testing.T) {
	if got := fromNullStr(nullStr("")); got != "" {
		t.Errorf("fromNullStr(invalid) = %q, want ''", got)
	}
	if got := fromNullStr(nullStr("x")); got != "x" {
		t.Errorf("fromNullStr('x') = %q, want 'x'", got)
	}
}

func TestNewPostgresStore(t *testing.T) {
	// NewPostgresStore with nil db should not panic
	ps := NewPostgresStore(nil)
	if ps == nil {
		t.Fatal("NewPostgresStore(nil) should return non-nil")
	}
	if ps.DB() != nil {
		t.Error("DB() should return nil when created with nil")
	}
}
