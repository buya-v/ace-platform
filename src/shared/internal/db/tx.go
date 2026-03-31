package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TxFunc is a function that runs within a database transaction.
// If it returns an error, the transaction is rolled back.
// If it returns nil, the transaction is committed.
type TxFunc func(tx pgx.Tx) error

// RunInTx acquires a connection from the pool, begins a transaction,
// executes fn, and commits on success or rolls back on error.
// It properly handles panics by rolling back before re-panicking.
func RunInTx(ctx context.Context, pool *pgxpool.Pool, fn TxFunc) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("db: begin tx: %w", err)
	}

	defer func() {
		if r := recover(); r != nil {
			_ = tx.Rollback(ctx)
			panic(r)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("db: tx error: %w (rollback also failed: %v)", err, rbErr)
		}
		return fmt.Errorf("db: tx error: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("db: commit tx: %w", err)
	}

	return nil
}

// RunInTxFromPool is a convenience method on Pool that delegates to RunInTx.
func (p *Pool) RunInTx(ctx context.Context, fn TxFunc) error {
	return RunInTx(ctx, p.pool, fn)
}
