// Package provisioning_test exercises the Provisioner in both dry-run and live modes.
package provisioning_test

import (
	"database/sql"
	"database/sql/driver"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/garudax-platform/platform-service/internal/provisioning"
	"github.com/garudax-platform/platform-service/internal/types"
)

// --- Minimal in-memory SQL driver for testing createSchemas ---
// Implements database/sql/driver interfaces without external dependencies.

var (
	fakeDriverOnce sync.Once
)

func init() {
	fakeDriverOnce.Do(func() {
		sql.Register("fakedb", &fakeDriver{})
	})
}

type fakeDriver struct{}

func (d *fakeDriver) Open(_ string) (driver.Conn, error) { return &fakeConn{}, nil }

type fakeConn struct{ closed bool }

func (c *fakeConn) Prepare(query string) (driver.Stmt, error) { return &fakeStmt{}, nil }
func (c *fakeConn) Close() error                              { c.closed = true; return nil }
func (c *fakeConn) Begin() (driver.Tx, error)                 { return &fakeTx{}, nil }

type fakeTx struct{ rolledBack bool }

func (t *fakeTx) Commit() error   { return nil }
func (t *fakeTx) Rollback() error { t.rolledBack = true; return nil }

type fakeStmt struct{}

func (s *fakeStmt) Close() error                                    { return nil }
func (s *fakeStmt) NumInput() int                                   { return -1 }
func (s *fakeStmt) Exec(_ []driver.Value) (driver.Result, error)    { return fakeResult{}, nil }
func (s *fakeStmt) Query(_ []driver.Value) (driver.Rows, error)     { return &fakeRows{}, nil }

type fakeResult struct{}

func (fakeResult) LastInsertId() (int64, error) { return 0, nil }
func (fakeResult) RowsAffected() (int64, error) { return 0, nil }

type fakeRows struct{ done bool }

func (r *fakeRows) Columns() []string            { return nil }
func (r *fakeRows) Close() error                 { return nil }
func (r *fakeRows) Next(_ []driver.Value) error  { return io.EOF }

// openFakeDB opens a connection to the in-memory fake driver.
func openFakeDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("fakedb", "test")
	if err != nil {
		t.Fatalf("sql.Open(fakedb): %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// newTenant is a helper that builds a minimal Tenant for testing.
func newTenant(id, name string) *types.Tenant {
	return &types.Tenant{
		ID:     id,
		Name:   name,
		Status: types.TenantStatusOnboarding,
	}
}

// TestProvisionTenant_DryRun verifies that provisioning with a nil DB (dry-run) returns
// 8 schemas, 8 topic prefixes, and status PROVISIONED.
func TestProvisionTenant_DryRun(t *testing.T) {
	p := provisioning.New(nil) // nil db = dry-run
	tenant := newTenant("test-exchange", "Test Exchange")

	result, err := p.ProvisionTenant(tenant)
	if err != nil {
		t.Fatalf("ProvisionTenant() unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("ProvisionTenant() returned nil result")
	}

	if result.TenantID != "test-exchange" {
		t.Errorf("TenantID = %q, want test-exchange", result.TenantID)
	}
	if result.Status != "PROVISIONED" {
		t.Errorf("Status = %q, want PROVISIONED", result.Status)
	}
	if len(result.SchemasCreated) != 8 {
		t.Errorf("SchemasCreated count = %d, want 8", len(result.SchemasCreated))
	}
	if len(result.TopicPrefixes) != 8 {
		t.Errorf("TopicPrefixes count = %d, want 8", len(result.TopicPrefixes))
	}
	if result.ConfigEntries == nil {
		t.Error("ConfigEntries must not be nil")
	}
}

// TestProvisionTenant_SchemaNames verifies that hyphens in the tenant ID are
// converted to underscores in schema names and the expected suffixes are present.
func TestProvisionTenant_SchemaNames(t *testing.T) {
	p := provisioning.New(nil)
	tenant := newTenant("mse-equities", "Mongolian Stock Exchange")

	result, err := p.ProvisionTenant(tenant)
	if err != nil {
		t.Fatalf("ProvisionTenant() error: %v", err)
	}

	wantSchemas := []string{
		"mse_equities_reference",
		"mse_equities_participants",
		"mse_equities_exchange",
		"mse_equities_clearing",
		"mse_equities_compliance",
		"mse_equities_warehouse",
		"mse_equities_market_data",
		"mse_equities_securities",
	}

	if len(result.SchemasCreated) != len(wantSchemas) {
		t.Fatalf("SchemasCreated count = %d, want %d", len(result.SchemasCreated), len(wantSchemas))
	}

	schemaSet := map[string]bool{}
	for _, s := range result.SchemasCreated {
		schemaSet[s] = true
	}
	for _, want := range wantSchemas {
		if !schemaSet[want] {
			t.Errorf("schema %q missing from SchemasCreated", want)
		}
	}

	// Verify no hyphen survives in any schema name.
	for _, s := range result.SchemasCreated {
		if strings.Contains(s, "-") {
			t.Errorf("schema %q contains a hyphen — expected underscore-only identifiers", s)
		}
	}
}

// TestProvisionTenant_TopicPrefixes verifies that Kafka topic prefixes use the
// raw tenant ID (hyphens preserved) and the expected suffixes are present.
func TestProvisionTenant_TopicPrefixes(t *testing.T) {
	p := provisioning.New(nil)
	tenant := newTenant("test-exchange", "Test Exchange")

	result, err := p.ProvisionTenant(tenant)
	if err != nil {
		t.Fatalf("ProvisionTenant() error: %v", err)
	}

	wantTopics := []string{
		"test-exchange.trades",
		"test-exchange.clearing",
		"test-exchange.margin",
		"test-exchange.settlement",
		"test-exchange.compliance",
		"test-exchange.market-data",
		"test-exchange.warehouse",
		"test-exchange.auth",
	}

	if len(result.TopicPrefixes) != len(wantTopics) {
		t.Fatalf("TopicPrefixes count = %d, want %d", len(result.TopicPrefixes), len(wantTopics))
	}

	topicSet := map[string]bool{}
	for _, tp := range result.TopicPrefixes {
		topicSet[tp] = true
	}
	for _, want := range wantTopics {
		if !topicSet[want] {
			t.Errorf("topic %q missing from TopicPrefixes", want)
		}
	}

	// Verify each topic starts with the raw tenant ID (hyphens preserved).
	for _, tp := range result.TopicPrefixes {
		if !strings.HasPrefix(tp, "test-exchange.") {
			t.Errorf("topic %q does not start with test-exchange.", tp)
		}
	}
}

// TestProvisionTenant_NoHyphenConversionInTopics verifies that schema conversion
// (hyphen→underscore) does NOT apply to topic prefixes.
func TestProvisionTenant_NoHyphenConversionInTopics(t *testing.T) {
	p := provisioning.New(nil)
	tenant := newTenant("ace-commodities", "ACE Commodity Exchange")

	result, err := p.ProvisionTenant(tenant)
	if err != nil {
		t.Fatalf("ProvisionTenant() error: %v", err)
	}

	for _, tp := range result.TopicPrefixes {
		if !strings.HasPrefix(tp, "ace-commodities.") {
			t.Errorf("topic %q should start with ace-commodities. (raw ID with hyphens)", tp)
		}
		// Also make sure schema uses underscore.
	}
	for _, s := range result.SchemasCreated {
		if !strings.HasPrefix(s, "ace_commodities_") {
			t.Errorf("schema %q should start with ace_commodities_ (sanitised ID)", s)
		}
	}
}

// TestProvisionTenant_StatusIsProvisioned verifies the status field for any tenant.
func TestProvisionTenant_StatusIsProvisioned(t *testing.T) {
	p := provisioning.New(nil)
	for _, id := range []string{"ace-commodities", "mse-equities", "my-new-venue"} {
		tenant := newTenant(id, "Some Exchange")
		result, err := p.ProvisionTenant(tenant)
		if err != nil {
			t.Errorf("ProvisionTenant(%q) unexpected error: %v", id, err)
			continue
		}
		if result.Status != "PROVISIONED" {
			t.Errorf("ProvisionTenant(%q).Status = %q, want PROVISIONED", id, result.Status)
		}
	}
}

// TestProvisionTenant_TenantIDPreserved verifies the TenantID in the result
// matches the input tenant's ID exactly.
func TestProvisionTenant_TenantIDPreserved(t *testing.T) {
	p := provisioning.New(nil)
	tenant := newTenant("mse-equities", "Mongolian Stock Exchange")

	result, err := p.ProvisionTenant(tenant)
	if err != nil {
		t.Fatalf("ProvisionTenant() error: %v", err)
	}
	if result.TenantID != tenant.ID {
		t.Errorf("TenantID = %q, want %q", result.TenantID, tenant.ID)
	}
}

// TestProvisionTenant_LiveDB verifies the provisioner's createSchemas path
// is exercised when a non-nil *sql.DB is provided (using the fake in-memory driver).
func TestProvisionTenant_LiveDB(t *testing.T) {
	db := openFakeDB(t)
	p := provisioning.New(db)
	tenant := newTenant("live-test", "Live Test Exchange")

	result, err := p.ProvisionTenant(tenant)
	if err != nil {
		t.Fatalf("ProvisionTenant(live db) unexpected error: %v", err)
	}
	if result.Status != "PROVISIONED" {
		t.Errorf("Status = %q, want PROVISIONED", result.Status)
	}
	if len(result.SchemasCreated) != 8 {
		t.Errorf("SchemasCreated count = %d, want 8", len(result.SchemasCreated))
	}
	// Verify schema sanitisation also works in live mode.
	for _, s := range result.SchemasCreated {
		if !strings.HasPrefix(s, "live_test_") {
			t.Errorf("schema %q should start with live_test_", s)
		}
	}
}
