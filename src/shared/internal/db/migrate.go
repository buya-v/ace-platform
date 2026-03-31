package db

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MigrationResult records the outcome of a single migration file.
type MigrationResult struct {
	Filename string
	Applied  bool
	Error    error
}

// Migrate reads .sql files from the provided filesystem, sorts them
// lexicographically by filename, and executes each one that has not yet
// been applied. It uses a schema_migrations table to track which files
// have already run.
//
// Migration files should be named with a version prefix for ordering,
// e.g., V001__create_users.sql, V002__add_index.sql.
//
// Each migration runs inside its own transaction. If a migration fails,
// subsequent migrations are skipped and the error is returned.
func Migrate(ctx context.Context, pool *pgxpool.Pool, migrations fs.FS) ([]MigrationResult, error) {
	if err := ensureMigrationsTable(ctx, pool); err != nil {
		return nil, fmt.Errorf("db: create migrations table: %w", err)
	}

	files, err := collectMigrationFiles(migrations)
	if err != nil {
		return nil, fmt.Errorf("db: read migration files: %w", err)
	}

	var results []MigrationResult
	for _, filename := range files {
		applied, err := isMigrationApplied(ctx, pool, filename)
		if err != nil {
			return results, fmt.Errorf("db: check migration %s: %w", filename, err)
		}
		if applied {
			results = append(results, MigrationResult{Filename: filename, Applied: false})
			continue
		}

		content, err := fs.ReadFile(migrations, filename)
		if err != nil {
			return results, fmt.Errorf("db: read migration %s: %w", filename, err)
		}

		if err := applyMigration(ctx, pool, filename, string(content)); err != nil {
			results = append(results, MigrationResult{Filename: filename, Applied: false, Error: err})
			return results, fmt.Errorf("db: apply migration %s: %w", filename, err)
		}

		results = append(results, MigrationResult{Filename: filename, Applied: true})
	}

	return results, nil
}

// MigrateFromPool is a convenience method on Pool that delegates to Migrate.
func (p *Pool) Migrate(ctx context.Context, migrations fs.FS) ([]MigrationResult, error) {
	return Migrate(ctx, p.pool, migrations)
}

func ensureMigrationsTable(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename    TEXT PRIMARY KEY,
			applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	return err
}

func collectMigrationFiles(migrations fs.FS) ([]string, error) {
	var files []string
	err := fs.WalkDir(migrations, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".sql") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func isMigrationApplied(ctx context.Context, pool *pgxpool.Pool, filename string) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE filename = $1)",
		filename,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func applyMigration(ctx context.Context, pool *pgxpool.Pool, filename, sql string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, sql); err != nil {
		return fmt.Errorf("exec: %w", err)
	}

	if _, err := tx.Exec(ctx,
		"INSERT INTO schema_migrations (filename) VALUES ($1)",
		filename,
	); err != nil {
		return fmt.Errorf("record migration: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

// CollectMigrationFiles is exported for testing. It reads .sql filenames
// from the provided fs.FS and returns them sorted lexicographically.
func CollectMigrationFiles(migrations fs.FS) ([]string, error) {
	return collectMigrationFiles(migrations)
}

// ApplyMigrationTx applies a single migration SQL string within the
// provided transaction and records it in schema_migrations. Exported
// for testing with mock transactions.
func ApplyMigrationTx(ctx context.Context, tx pgx.Tx, filename, sql string) error {
	if _, err := tx.Exec(ctx, sql); err != nil {
		return fmt.Errorf("exec: %w", err)
	}
	if _, err := tx.Exec(ctx,
		"INSERT INTO schema_migrations (filename) VALUES ($1)",
		filename,
	); err != nil {
		return fmt.Errorf("record migration: %w", err)
	}
	return nil
}
