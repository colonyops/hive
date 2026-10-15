// Package control is control.db: accounts, API tokens, sessions, share links,
// and the mail outbox. It implements the core store ports.
package control

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/cmd/drive-server/internal/store/control/db"
	"github.com/colonyops/hive/internal/platform/sqlite"
	"github.com/colonyops/hive/internal/store/migrate"
)

//go:embed migrations/*.up.sql
var migrationsFS embed.FS

// FileName is control.db's name inside the data directory.
const FileName = "control.db"

type Store struct {
	conn *sql.DB
	q    *db.Queries
}

// Open opens or creates <dataDir>/control.db and applies pending migrations.
// The server and the admin commands both open it; WAL and busy_timeout make
// them wait on each other instead of failing.
func Open(ctx context.Context, dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	// One connection: a query on s.q inside a transaction would wait forever
	// for the connection the transaction holds, so transactions use the
	// tx-bound Queries they are handed.
	conn, err := sqlite.Open(ctx, filepath.Join(dataDir, FileName), sqlite.Options{
		MaxOpenConns: 1,
		MaxIdleConns: 1,
		BusyTimeout:  5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("open control.db: %w", err)
	}
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err == nil {
		err = migrate.Up(ctx, conn, sub)
	}
	if err != nil {
		return nil, errors.Join(fmt.Errorf("migrate control.db: %w", err), conn.Close())
	}
	return &Store{conn: conn, q: db.New(conn)}, nil
}

func (s *Store) Close() error { return s.conn.Close() }

func (s *Store) Accounts() core.AccountStore { return accounts{s} }
func (s *Store) Tokens() core.TokenStore     { return tokens{s} }
func (s *Store) Shares() core.ShareStore     { return shares{s} }
func (s *Store) Codes() core.CodeStore       { return codes{s} }
func (s *Store) Sessions() core.SessionStore { return sessions{s} }
func (s *Store) Outbox() core.OutboxStore    { return outbox{s} }

func wrap(op string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, sql.ErrNoRows):
		return core.ErrNotFound
	default:
		return fmt.Errorf("control: %s: %w", op, err)
	}
}

func toNanos(t time.Time) int64 { return t.UnixNano() }

func fromNanos(n int64) time.Time { return time.Unix(0, n).UTC() }

func toNullNanos(t *time.Time) sql.NullInt64 {
	if t == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: t.UnixNano(), Valid: true}
}

func fromNullNanos(n sql.NullInt64) *time.Time {
	if !n.Valid {
		return nil
	}
	t := fromNanos(n.Int64)
	return &t
}
