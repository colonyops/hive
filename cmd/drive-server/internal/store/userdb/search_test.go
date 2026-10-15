package userdb_test

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/platform/sqlite"
	"github.com/colonyops/hive/internal/store/migrate"
)

func TestOpenMigratesVersionOneDatabaseAndBackfillsTitlesAndIndex(t *testing.T) {
	p, dir := newPool(t, 4, 0)
	userID := uuid.NewString()
	require.NoError(t, os.MkdirAll(dir, 0o700))

	initSQL, err := os.ReadFile("migrations/0001_init.up.sql")
	require.NoError(t, err)
	conn, err := sqlite.Open(t.Context(), filepath.Join(dir, userID+".db"), sqlite.Options{MaxOpenConns: 1})
	require.NoError(t, err)
	require.NoError(t, migrate.Up(t.Context(), conn, fstest.MapFS{"0001_init.up.sql": {Data: initSQL}}))
	insert := func(path, kind, body string) {
		_, err := conn.Exec(`INSERT INTO documents (id, space, path, kind, sha256, size, body, updated_at) VALUES (?, 'shared', ?, ?, 'x', ?, ?, 1)`,
			uuid.NewString(), path, kind, len(body), []byte(body))
		require.NoError(t, err)
	}
	insert("old/plan.md", "markdown", "# Old Plan\n\nlegacy zebra content")
	insert("page.html", "html", "<title>Legacy Page</title>")
	insert("bare.md", "markdown", "nothing here")
	require.NoError(t, conn.Close())

	s, release, err := p.Open(t.Context(), userID)
	require.NoError(t, err)
	defer release()

	docs, err := s.List(t.Context(), "shared")
	require.NoError(t, err)
	titles := map[string]string{}
	for _, d := range docs {
		titles[d.Path] = d.Title
	}
	assert.Equal(t, map[string]string{"old/plan.md": "Old Plan", "page.html": "Legacy Page", "bare.md": "bare"}, titles)

	hits, err := s.Search(t.Context(), "zebra", "", 10)
	require.NoError(t, err)
	require.Len(t, hits, 1)
	assert.Equal(t, "old/plan.md", hits[0].Document.Path)

	hits, err = s.Search(t.Context(), "legacy page", "", 10)
	require.NoError(t, err)
	require.Len(t, hits, 1, "backfilled titles reach the index")
	assert.Equal(t, "page.html", hits[0].Document.Path)
}
