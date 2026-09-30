package queries

import (
	"context"
	"database/sql"
	"fmt"
)

// storeTxKey carries an in-progress transaction on the context so composing
// store calls join it without every signature growing a tx parameter.
type storeTxKey struct{}

// WithTransaction begins a transaction and attaches it to the returned
// context, so nested store calls join it instead of opening their own. The
// caller owns Commit and Rollback; WithinTx is the wrapper that owns them for
// you and is what call sites should normally use.
func WithTransaction(ctx context.Context, db *DB) (context.Context, *sql.Tx, error) {
	if tx, ok := txFromContext(ctx); ok {
		return ctx, tx, nil
	}
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return ctx, nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	return context.WithValue(ctx, storeTxKey{}, tx), tx, nil
}

func txFromContext(ctx context.Context) (*sql.Tx, bool) {
	tx, ok := ctx.Value(storeTxKey{}).(*sql.Tx)
	return tx, ok && tx != nil
}
