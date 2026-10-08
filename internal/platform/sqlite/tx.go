package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ambientTxKey scopes an ambient transaction to the pool that opened it, so a
// context carrying a transaction on one database never binds queries for
// another.
type ambientTxKey struct{ pool *sql.DB }

// AmbientTx returns the transaction that WithinTx opened on pool and attached
// to ctx, if there is one.
func AmbientTx(ctx context.Context, pool *sql.DB) (*sql.Tx, bool) {
	tx, ok := ctx.Value(ambientTxKey{pool}).(*sql.Tx)
	return tx, ok && tx != nil
}

// WithinTx runs fn inside a transaction on pool. When ctx already carries one
// for pool, fn joins it; otherwise WithinTx begins one and hands fn a context
// that carries it. Only the call that began the transaction commits it or,
// when fn fails, rolls it back.
//
// Joining is required, not an optimization. Open sets _txlock=immediate, so a
// nested BEGIN IMMEDIATE waits out busy_timeout for the write lock its own
// caller holds and then fails with SQLITE_BUSY.
func WithinTx(ctx context.Context, pool *sql.DB, fn func(context.Context, *sql.Tx) error) error {
	if tx, ok := AmbientTx(ctx, pool); ok {
		return fn(ctx, tx)
	}

	tx, err := pool.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	if err := fn(context.WithValue(ctx, ambientTxKey{pool}, tx), tx); err != nil {
		// A cancelled ctx has already rolled the transaction back.
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			return fmt.Errorf("transaction failed: %w (rollback also failed: %w)", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}
