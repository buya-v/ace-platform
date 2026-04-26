// Package store_test — compile-time interface compliance tests for all PG store types,
// plus constructor and NewPool smoke tests.
package store_test

import (
	"database/sql"
	"testing"

	"github.com/garudax-platform/securities-service/internal/db"
	"github.com/garudax-platform/securities-service/internal/store"
)

// ── Compile-time interface compliance ─────────────────────────────────────────
// Each test assigns a *PgXxxStore to the corresponding interface variable.
// The compiler rejects the assignment if any method is missing or has the wrong
// signature. No database connection is needed.

func TestPgInstrumentStore_ImplementsInterface(t *testing.T) {
	var _ store.InstrumentStore = &store.PgInstrumentStore{}
}

func TestPgOrderStore_ImplementsInterface(t *testing.T) {
	var _ store.OrderStore = &store.PgOrderStore{}
}

func TestPgTradeStore_ImplementsInterface(t *testing.T) {
	var _ store.TradeStore = &store.PgTradeStore{}
}

func TestPgPositionStore_ImplementsInterface(t *testing.T) {
	var _ store.PositionStore = &store.PgPositionStore{}
}

func TestPgMarketStore_ImplementsInterface(t *testing.T) {
	var _ store.MarketStore = &store.PgMarketStore{}
}

func TestPgSegmentStore_ImplementsInterface(t *testing.T) {
	var _ store.SegmentStore = &store.PgSegmentStore{}
}

func TestPgFirmStore_ImplementsInterface(t *testing.T) {
	var _ store.FirmStore = &store.PgFirmStore{}
}

func TestPgParticipantStore_ImplementsInterface(t *testing.T) {
	var _ store.ParticipantStore = &store.PgParticipantStore{}
}

func TestPgSettlementStore_ImplementsInterface(t *testing.T) {
	var _ store.SettlementStore = &store.PgSettlementStore{}
}

func TestPgAuditStore_ImplementsInterface(t *testing.T) {
	var _ store.AuditStore = &store.PgAuditStore{}
}

// ── NewPool — invalid URL returns error ───────────────────────────────────────

func TestNewPool_InvalidURL(t *testing.T) {
	_, err := db.NewPool("invalid://url")
	if err == nil {
		t.Fatal("expected error from NewPool with invalid URL, got nil")
	}
}

// ── Constructor smoke tests ───────────────────────────────────────────────────
// Verify that each New* function accepts a *sql.DB and returns the expected type.
// A nil *sql.DB is used because the constructors only store the pointer; they do
// not call any DB methods during construction.

func TestPgStoreConstructors(t *testing.T) {
	var nilDB *sql.DB // intentionally nil — constructors must not dereference it

	t.Run("NewPgInstrumentStore", func(t *testing.T) {
		s := store.NewPgInstrumentStore(nilDB)
		if s == nil {
			t.Fatal("NewPgInstrumentStore returned nil")
		}
		// Confirm it satisfies the interface at runtime.
		var _ store.InstrumentStore = s
	})

	t.Run("NewPgOrderStore", func(t *testing.T) {
		s := store.NewPgOrderStore(nilDB)
		if s == nil {
			t.Fatal("NewPgOrderStore returned nil")
		}
		var _ store.OrderStore = s
	})

	t.Run("NewPgTradeStore", func(t *testing.T) {
		s := store.NewPgTradeStore(nilDB)
		if s == nil {
			t.Fatal("NewPgTradeStore returned nil")
		}
		var _ store.TradeStore = s
	})

	t.Run("NewPgPositionStore", func(t *testing.T) {
		s := store.NewPgPositionStore(nilDB)
		if s == nil {
			t.Fatal("NewPgPositionStore returned nil")
		}
		var _ store.PositionStore = s
	})

	t.Run("NewPgMarketStore", func(t *testing.T) {
		s := store.NewPgMarketStore(nilDB)
		if s == nil {
			t.Fatal("NewPgMarketStore returned nil")
		}
		var _ store.MarketStore = s
	})

	t.Run("NewPgSegmentStore", func(t *testing.T) {
		s := store.NewPgSegmentStore(nilDB)
		if s == nil {
			t.Fatal("NewPgSegmentStore returned nil")
		}
		var _ store.SegmentStore = s
	})

	t.Run("NewPgFirmStore", func(t *testing.T) {
		s := store.NewPgFirmStore(nilDB)
		if s == nil {
			t.Fatal("NewPgFirmStore returned nil")
		}
		var _ store.FirmStore = s
	})

	t.Run("NewPgParticipantStore", func(t *testing.T) {
		s := store.NewPgParticipantStore(nilDB)
		if s == nil {
			t.Fatal("NewPgParticipantStore returned nil")
		}
		var _ store.ParticipantStore = s
	})

	t.Run("NewPgSettlementStore", func(t *testing.T) {
		s := store.NewPgSettlementStore(nilDB)
		if s == nil {
			t.Fatal("NewPgSettlementStore returned nil")
		}
		var _ store.SettlementStore = s
	})

	t.Run("NewPgAuditStore", func(t *testing.T) {
		s := store.NewPgAuditStore(nilDB)
		if s == nil {
			t.Fatal("NewPgAuditStore returned nil")
		}
		var _ store.AuditStore = s
	})
}
