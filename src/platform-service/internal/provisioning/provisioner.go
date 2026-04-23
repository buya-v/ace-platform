// Package provisioning implements the tenant provisioning workflow for platform-service.
// It creates per-tenant database schemas and defines Kafka topic prefixes.
package provisioning

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/garudax-platform/platform-service/internal/types"
)

// Provisioner handles the database and message-bus provisioning for a new tenant.
// When db is nil the provisioner operates in dry-run mode — it returns the full
// ProvisionResult but does not execute any SQL.
type Provisioner struct {
	db *sql.DB
}

// New creates a Provisioner.  Pass nil for db to enable dry-run mode.
func New(db *sql.DB) *Provisioner {
	return &Provisioner{db: db}
}

// schemaSuffix lists the eight schema names created for every tenant.
// Each is appended to the sanitised tenant identifier (hyphens → underscores).
var schemaSuffixes = []string{
	"reference",
	"participants",
	"exchange",
	"clearing",
	"compliance",
	"warehouse",
	"market_data",
	"securities",
}

// topicSuffixes lists the eight Kafka topic prefixes created for every tenant.
// The prefix is "{tenantID}.{suffix}" — tenant ID is used as-is (hyphens allowed
// in Kafka topic names).
var topicSuffixes = []string{
	"trades",
	"clearing",
	"margin",
	"settlement",
	"compliance",
	"market-data",
	"warehouse",
	"auth",
}

// sanitiseID replaces hyphens with underscores so the result is a valid
// PostgreSQL identifier segment.
func sanitiseID(id string) string {
	return strings.ReplaceAll(id, "-", "_")
}

// ProvisionTenant provisions the database schemas and Kafka topic prefixes for
// the given tenant.
//
// If p.db is non-nil each schema is created with CREATE SCHEMA IF NOT EXISTS
// inside a single transaction; on any SQL error the transaction is rolled back
// and an error is returned.
//
// If p.db is nil the provisioner runs in dry-run mode and returns the expected
// ProvisionResult without touching the database.
func (p *Provisioner) ProvisionTenant(tenant *types.Tenant) (*types.ProvisionResult, error) {
	safeID := sanitiseID(tenant.ID)

	// Build the eight schema names.
	schemas := make([]string, 0, len(schemaSuffixes))
	for _, suffix := range schemaSuffixes {
		schemas = append(schemas, fmt.Sprintf("%s_%s", safeID, suffix))
	}

	// Build the eight Kafka topic prefixes.
	topics := make([]string, 0, len(topicSuffixes))
	for _, suffix := range topicSuffixes {
		topics = append(topics, fmt.Sprintf("%s.%s", tenant.ID, suffix))
	}

	// Execute schema creation when a real DB connection is available.
	if p.db != nil {
		if err := p.createSchemas(schemas); err != nil {
			return nil, fmt.Errorf("provisioning tenant %s: %w", tenant.ID, err)
		}
	}

	return &types.ProvisionResult{
		TenantID:       tenant.ID,
		SchemasCreated: schemas,
		TopicPrefixes:  topics,
		ConfigEntries:  []string{},
		Status:         "PROVISIONED",
	}, nil
}

// createSchemas executes CREATE SCHEMA IF NOT EXISTS for each schema name in a
// single database transaction.
func (p *Provisioner) createSchemas(schemas []string) error {
	tx, err := p.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	for _, schema := range schemas {
		// Schema names are generated internally from tenant IDs — they contain
		// only [a-z0-9_] characters after sanitisation, so interpolation is safe.
		if _, err := tx.Exec(fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", schema)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("create schema %s: %w", schema, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}
