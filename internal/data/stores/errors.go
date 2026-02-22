package stores

import (
	"database/sql"
	"errors"
)

// IsNotFoundError returns true if the error is a "not found" error.
func IsNotFoundError(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
