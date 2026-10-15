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

func TestOpenBackfillsSearchTextOfVersionTwoDatabase(t *testing.T) {
	p, dir := newPool(t, 4, 0)
	userID := uuid.NewString()
	require.NoError(t, os.MkdirAll(dir, 0o700))

	upTo2 := fstest.MapFS{}
	for _, name := range []string{"0001_init.up.sql", "0002_search.up.sql"} {
		data, err := os.ReadFile(filepath.Join("migrations", name))
		require.NoError(t, err)
		upTo2[name] = &fstest.MapFile{Data: data}
	}
	conn, err := sqlite.Open(t.Context(), filepath.Join(dir, userID+".db"), sqlite.Options{MaxOpenConns: 1})
	require.NoError(t, err)
	require.NoError(t, migrate.Up(t.Context(), conn, upTo2))
	insert := func(path, kind, body string) {
		_, err := conn.Exec(`INSERT INTO documents (id, space, path, kind, sha256, size, body, updated_at, title) VALUES (?, 'shared', ?, ?, 'x', ?, ?, 1, ?)`,
			uuid.NewString(), path, kind, len(body), []byte(body), path)
		require.NoError(t, err)
	}
	insert("a.md", "markdown", "plain giraffe text")
	insert("b.html", "html", `<div class="x">hello</div><script>var tapir;</script>`)
	insert("c.html", "html", `<div></div>`)
	require.NoError(t, conn.Close())

	s, release, err := p.Open(t.Context(), userID)
	require.NoError(t, err)
	defer release()

	paths := func(q string) []string {
		hits, err := s.Search(t.Context(), q, "", 10)
		require.NoError(t, err)
		var out []string
		for _, h := range hits {
			out = append(out, h.Document.Path)
		}
		return out
	}
	assert.Equal(t, []string{"a.md"}, paths("giraffe"))
	assert.Equal(t, []string{"b.html"}, paths("hello"))
	assert.Empty(t, paths("div"))
	assert.Empty(t, paths("class"))
	assert.Empty(t, paths("tapir"))
}
