package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	modernc "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

const (
	contentionHandles    = 2
	contentionGoroutines = 8
	contentionIterations = 200
)

// The busy timeout is generous on purpose: the test asserts that no
// transaction fails at once with SQLITE_BUSY, not how long a waiter queues
// under the race detector.
var contentionOptions = Options{MaxOpenConns: 2, MaxIdleConns: 2, BusyTimeout: 30 * time.Second}

func TestOpen_ReadThenWriteContention(t *testing.T) {
	t.Run("immediate", func(t *testing.T) {
		handles := openHandles(t, func(ctx context.Context, path string) (*sql.DB, error) {
			return Open(ctx, path, contentionOptions)
		})

		errs := runContention(t.Context(), handles)

		require.Empty(t, errs)
		var n int
		require.NoError(t, handles[0].QueryRowContext(t.Context(), `SELECT n FROM counter WHERE id = 1`).Scan(&n))
		assert.Equal(t, contentionHandles*contentionGoroutines*contentionIterations, n)
	})

	t.Run("deferred", func(t *testing.T) {
		handles := openHandles(t, func(ctx context.Context, path string) (*sql.DB, error) {
			return open(ctx, dataSourceName(path, "deferred", contentionOptions.BusyTimeout), contentionOptions)
		})

		errs := runContention(t.Context(), handles)

		busy := 0
		for _, err := range errs {
			if isBusy(err) {
				busy++
			}
		}
		t.Logf("deferred locking: %d of %d transactions failed with SQLITE_BUSY (%d errors total)",
			busy, contentionHandles*contentionGoroutines*contentionIterations, len(errs))
	})
}

func openHandles(t *testing.T, openDB func(context.Context, string) (*sql.DB, error)) []*sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "contention.db")
	handles := make([]*sql.DB, contentionHandles)
	for i := range handles {
		db, err := openDB(t.Context(), path)
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })
		handles[i] = db
	}
	createCounter(t, handles[0])
	return handles
}

func createCounter(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.ExecContext(t.Context(), `CREATE TABLE counter (id INTEGER PRIMARY KEY, n INTEGER NOT NULL);
		INSERT INTO counter (id, n) VALUES (1, 0)`)
	require.NoError(t, err)
}

func runContention(ctx context.Context, handles []*sql.DB) []error {
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	for _, db := range handles {
		for range contentionGoroutines {
			wg.Go(func() {
				for range contentionIterations {
					if err := incrementCounter(ctx, db); err != nil {
						mu.Lock()
						errs = append(errs, err)
						mu.Unlock()
					}
				}
			})
		}
	}
	wg.Wait()
	return errs
}

func incrementCounter(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT n FROM counter WHERE id = 1`).Scan(&n); err != nil {
		return errors.Join(err, tx.Rollback())
	}
	if _, err := tx.ExecContext(ctx, `UPDATE counter SET n = ? WHERE id = 1`, n+1); err != nil {
		return errors.Join(err, tx.Rollback())
	}
	return tx.Commit()
}

func isBusy(err error) bool {
	var sqliteErr *modernc.Error
	return errors.As(err, &sqliteErr) && sqliteErr.Code()&0xff == sqlite3.SQLITE_BUSY
}
