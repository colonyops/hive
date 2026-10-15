package core_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/drive-server/internal/config"
	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/cmd/drive-server/internal/store/control"
	"github.com/colonyops/hive/cmd/drive-server/internal/store/userdb"
	"github.com/colonyops/hive/internal/platform/sqlite"
)

const testMaxDocumentSize = 1024

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type env struct {
	dataDir  string
	store    *control.Store
	policies *core.Policies
	accounts *core.Accounts
	tokens   *core.Tokens
	docs     *core.Documents
	clock    *clock
}

func newEnv(t *testing.T, ps ...config.Policy) *env {
	t.Helper()
	dir := t.TempDir()
	store, err := control.Open(t.Context(), dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	pool := userdb.NewPool(zerolog.Nop(), filepath.Join(dir, "users"), 8, 0)
	t.Cleanup(func() { _ = pool.Close() })

	c := &clock{now: time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)}
	policies := core.NewPolicies(ps)
	return &env{
		dataDir:  dir,
		store:    store,
		policies: policies,
		accounts: core.NewAccounts(store.Accounts(), pool.Stores(), policies, core.WithClock(c.Now)),
		tokens:   core.NewTokens(store.Tokens(), store.Accounts(), policies, core.WithClock(c.Now)),
		docs:     core.NewDocuments(pool.Stores(), testMaxDocumentSize, core.WithClock(c.Now)),
		clock:    c,
	}
}

func (e *env) rawDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := sqlite.Open(context.Background(), filepath.Join(e.dataDir, control.FileName), sqlite.Options{MaxOpenConns: 1, MaxIdleConns: 1, BusyTimeout: time.Second})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}
