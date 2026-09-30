package stores

import (
	"database/sql"
	"errors"
	"fmt"
)

// NotFoundError is what a store returns when a query addressed exactly one
// row and found none. It wraps sql.ErrNoRows, so errors.Is(err,
// sql.ErrNoRows) also holds.
type NotFoundError struct {
	Entity string
	Key    string
	err    error
}

func (e NotFoundError) Error() string {
	return fmt.Sprintf("%s %s: not found", e.Entity, e.Key)
}

func (e NotFoundError) Unwrap() error { return e.err }

func IsNotFound(err error) bool {
	var notFound NotFoundError
	return errors.As(err, &notFound)
}

// ErrStale means the row changed after the caller read its revision. A
// revision-guarded UPDATE that matches nothing is a stale read, not a missing
// row, so it gets its own sentinel rather than folding into NotFoundError:
// IsNotFound(err) must stay false for it, or a caller that retries on
// conflict starts silently reporting the item as deleted instead.
var ErrStale = errors.New("stale revision")

// Do not use this for revision-guarded updates; their sql.ErrNoRows means
// ErrStale.
func errTransformQueryOne(entity, key string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return NotFoundError{Entity: entity, Key: key, err: err}
	}
	return err
}

func errTransformQueryMany(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return err
}

func wrap(msg string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", msg, err)
}

func null(v string) sql.NullString { return sql.NullString{String: v, Valid: v != ""} }
