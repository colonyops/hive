package control_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/cmd/drive-server/internal/store/control"
	"github.com/colonyops/hive/internal/platform/sqlite"
)

func openStore(t *testing.T) *control.Store {
	t.Helper()
	s, err := control.Open(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func account(id, email string) core.Account {
	return core.Account{ID: id, Email: email, CreatedAt: time.Unix(1_700_000_000, 5).UTC()}
}

func TestAccountCreateAndGet(t *testing.T) {
	accounts := openStore(t).Accounts()
	want := account("a1", "dev@example.com")
	require.NoError(t, accounts.Insert(t.Context(), want))

	byEmail, err := accounts.GetByEmail(t.Context(), "dev@example.com")
	require.NoError(t, err)
	assert.Equal(t, want, byEmail)

	byID, err := accounts.GetByID(t.Context(), "a1")
	require.NoError(t, err)
	assert.Equal(t, want, byID)

	_, err = accounts.GetByEmail(t.Context(), "nobody@example.com")
	require.ErrorIs(t, err, core.ErrNotFound)
	_, err = accounts.GetByID(t.Context(), "nope")
	require.ErrorIs(t, err, core.ErrNotFound)
}

func TestAccountInsertKeepsExistingEmail(t *testing.T) {
	accounts := openStore(t).Accounts()
	require.NoError(t, accounts.Insert(t.Context(), account("first", "dev@example.com")))
	require.NoError(t, accounts.Insert(t.Context(), account("second", "dev@example.com")))

	got, err := accounts.GetByEmail(t.Context(), "dev@example.com")
	require.NoError(t, err)
	assert.Equal(t, "first", got.ID)
}

func TestAccountList(t *testing.T) {
	accounts := openStore(t).Accounts()
	require.NoError(t, accounts.Insert(t.Context(), account("a1", "a@example.com")))
	require.NoError(t, accounts.Insert(t.Context(), account("a2", "b@example.com")))

	got, err := accounts.List(t.Context())
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "a@example.com", got[0].Email)
	assert.Equal(t, "b@example.com", got[1].Email)
}

func TestAccountDeleteCascades(t *testing.T) {
	s := openStore(t)
	require.NoError(t, s.Accounts().Insert(t.Context(), account("a1", "a@example.com")))
	require.NoError(t, s.Accounts().Insert(t.Context(), account("a2", "b@example.com")))
	tok := core.Token{ID: "t1", UserID: "a1", Name: "cli", Scope: core.ScopePush, CreatedAt: time.Unix(1_700_000_000, 0).UTC()}
	require.NoError(t, s.Tokens().Insert(t.Context(), tok, "hash-a"))
	other := core.Token{ID: "t2", UserID: "a2", Name: "cli", Scope: core.ScopeRead, CreatedAt: tok.CreatedAt}
	require.NoError(t, s.Tokens().Insert(t.Context(), other, "hash-b"))

	require.NoError(t, s.Accounts().DeleteByEmail(t.Context(), "a@example.com"))

	_, err := s.Accounts().GetByID(t.Context(), "a1")
	require.ErrorIs(t, err, core.ErrNotFound)
	_, err = s.Tokens().GetBySecretHash(t.Context(), "hash-a")
	require.ErrorIs(t, err, core.ErrNotFound)
	got, err := s.Tokens().GetBySecretHash(t.Context(), "hash-b")
	require.NoError(t, err)
	assert.Equal(t, other, got)

	require.ErrorIs(t, s.Accounts().DeleteByEmail(t.Context(), "a@example.com"), core.ErrNotFound)
}

func TestTokenRoundTrip(t *testing.T) {
	s := openStore(t)
	require.NoError(t, s.Accounts().Insert(t.Context(), account("a1", "a@example.com")))
	expires := time.Unix(1_800_000_000, 7).UTC()
	want := core.Token{ID: "t1", UserID: "a1", Name: "laptop", Scope: core.ScopeRead, ExpiresAt: &expires, CreatedAt: time.Unix(1_700_000_000, 0).UTC()}
	require.NoError(t, s.Tokens().Insert(t.Context(), want, "hash"))

	got, err := s.Tokens().GetBySecretHash(t.Context(), "hash")
	require.NoError(t, err)
	assert.Equal(t, want, got)

	used := time.Unix(1_700_000_100, 0).UTC()
	require.NoError(t, s.Tokens().Touch(t.Context(), "t1", used))
	list, err := s.Tokens().ListByUser(t.Context(), "a1")
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.NotNil(t, list[0].LastUsedAt)
	assert.True(t, used.Equal(*list[0].LastUsedAt))
}

func TestOpenIsRepeatable(t *testing.T) {
	dir := t.TempDir()
	first, err := control.Open(t.Context(), dir)
	require.NoError(t, err)
	require.NoError(t, first.Accounts().Insert(t.Context(), account("a1", "a@example.com")))
	require.NoError(t, first.Close())

	second, err := control.Open(t.Context(), dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Close() })
	_, err = second.Accounts().GetByID(t.Context(), "a1")
	require.NoError(t, err)
}

func TestMigrationAddsTheExpiryPresetToExistingLinks(t *testing.T) {
	dir := t.TempDir()
	init0001, err := os.ReadFile("migrations/0001_init.up.sql")
	require.NoError(t, err)
	conn, err := sqlite.Open(context.Background(), filepath.Join(dir, control.FileName), sqlite.Options{MaxOpenConns: 1})
	require.NoError(t, err)
	for _, stmt := range []string{
		string(init0001),
		`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at INTEGER NOT NULL)`,
		`INSERT INTO schema_migrations VALUES (1, 'init', 0)`,
		`INSERT INTO accounts (id, email, created_at) VALUES ('u1', 'dev@example.com', 0)`,
		`INSERT INTO share_links (id, slug, user_id, document_id, expires_at, created_at) VALUES ('l1', 's1', 'u1', 'd1', 5000, 0)`,
	} {
		_, err := conn.Exec(stmt)
		require.NoError(t, err, stmt)
	}
	require.NoError(t, conn.Close())

	s, err := control.Open(t.Context(), dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	link, err := s.Shares().GetBySlug(t.Context(), "s1")
	require.NoError(t, err)
	assert.Empty(t, link.ExpiresIn)
	require.NotNil(t, link.ExpiresAt)
	assert.Equal(t, int64(5000), link.ExpiresAt.UnixNano())

	require.NoError(t, s.Shares().UpdateExpiry(t.Context(), "l1", nil, ""))
	require.NoError(t, s.Shares().UpdateExpiry(t.Context(), "l1", link.ExpiresAt, "7d"))
	link, err = s.Shares().GetBySlug(t.Context(), "s1")
	require.NoError(t, err)
	assert.Equal(t, "7d", link.ExpiresIn)
}
