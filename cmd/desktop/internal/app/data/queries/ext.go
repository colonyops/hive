package queries

import (
	"context"
	"database/sql"

	"github.com/colonyops/hive/internal/platform/sqlite"
)

// Ctx returns a DB bound to the ambient transaction if the context carries
// one for this database, and the receiver otherwise. A store method that
// participates in cross-domain work begins with `db := db.Ctx(ctx)`; one that
// does not is unaffected, which is what makes adoption incremental.
func (db *DB) Ctx(ctx context.Context) *DB {
	tx, ok := sqlite.AmbientTx(ctx, db.conn)
	if !ok {
		return db
	}
	return db.boundTo(tx)
}

// boundTo copies the whole receiver rather than listing fields, so a field
// added to DB cannot be silently dropped from the transaction-bound form.
func (db *DB) boundTo(tx *sql.Tx) *DB {
	bound := *db
	bound.tx = tx
	bound.Queries = db.WithTx(tx)
	return &bound
}

// WithinTx runs fn inside a transaction, joining an ambient one on this
// database rather than opening a second (see sqlite.WithinTx). Only the
// outermost caller commits or rolls back; an inner fn that fails returns its
// error up to that caller, which rolls the whole unit back.
//
// context.WithoutCancel preserves the transaction value, so a goroutine
// detached from a ctx inside WithinTx would run store calls on a transaction
// it did not open. Do not start one here.
func (db *DB) WithinTx(ctx context.Context, fn func(context.Context, *DB) error) error {
	return sqlite.WithinTx(ctx, db.conn, func(ctx context.Context, tx *sql.Tx) error {
		return fn(ctx, db.boundTo(tx))
	})
}
