package userdb_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/cmd/drive-server/internal/store/userdb"
	"github.com/colonyops/hive/internal/domain/drive"
)

func newPool(t *testing.T, maxOpen int, idle time.Duration) (*userdb.Pool, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "users")
	p := userdb.NewPool(zerolog.Nop(), dir, maxOpen, idle)
	t.Cleanup(func() { _ = p.Close() })
	return p, dir
}

func put(t *testing.T, s core.DocumentStore, path string) {
	t.Helper()
	_, err := s.Put(t.Context(), core.NewDocument{
		ID: uuid.NewString(), Space: "shared", Path: path, Kind: drive.KindMarkdown, SHA256: "x", Body: []byte("body"), UpdatedAt: time.Now(),
	}, nil)
	require.NoError(t, err)
}

func TestOpenRejectsNonCanonicalIDs(t *testing.T) {
	p, dir := newPool(t, 4, 0)
	upper := uuid.NewString()
	for _, id := range []string{"../x", "not-a-uuid", "", "a/b", `..\x`, "{" + upper + "}", "URN:UUID:" + upper, strings.ToUpper(upper)} {
		_, _, err := p.Open(t.Context(), id)
		require.Error(t, err, id)
	}
	_, err := os.Stat(dir)
	require.ErrorIs(t, err, os.ErrNotExist, "a rejected id must not create anything")

	_, release, err := p.Open(t.Context(), uuid.NewString())
	require.NoError(t, err)
	release()
}

func TestEvictionNeverClosesALeasedHandle(t *testing.T) {
	p, _ := newPool(t, 1, 0)
	a, b, c := uuid.NewString(), uuid.NewString(), uuid.NewString()

	held, release, err := p.Open(t.Context(), a)
	require.NoError(t, err)
	defer release()

	for _, id := range []string{b, c} {
		_, rel, err := p.Open(t.Context(), id)
		require.NoError(t, err)
		rel()
	}

	put(t, held, "kept.md")
	_, _, err = held.Get(t.Context(), "shared", "kept.md")
	require.NoError(t, err)
}

func TestEvictionClosesIdleHandlesAndReopensData(t *testing.T) {
	p, _ := newPool(t, 1, 0)
	a, b := uuid.NewString(), uuid.NewString()

	first, release, err := p.Open(t.Context(), a)
	require.NoError(t, err)
	put(t, first, "a.md")
	release()

	_, rel, err := p.Open(t.Context(), b)
	require.NoError(t, err)
	rel()

	_, err = first.Usage(t.Context())
	require.Error(t, err, "the evicted handle is closed")

	again, release, err := p.Open(t.Context(), a)
	require.NoError(t, err)
	defer release()
	_, _, err = again.Get(t.Context(), "shared", "a.md")
	require.NoError(t, err)
}

func TestIdleCloseThenReopen(t *testing.T) {
	p, _ := newPool(t, 4, 20*time.Millisecond)
	id := uuid.NewString()

	first, release, err := p.Open(t.Context(), id)
	require.NoError(t, err)
	put(t, first, "a.md")
	release()

	require.Eventually(t, func() bool {
		_, err := first.Usage(t.Context())
		return err != nil
	}, 2*time.Second, 5*time.Millisecond, "idle handle should close")

	again, release, err := p.Open(t.Context(), id)
	require.NoError(t, err)
	defer release()
	_, _, err = again.Get(t.Context(), "shared", "a.md")
	require.NoError(t, err)
}

func TestIdleCloseSkipsLeasedHandle(t *testing.T) {
	p, _ := newPool(t, 4, 20*time.Millisecond)
	held, release, err := p.Open(t.Context(), uuid.NewString())
	require.NoError(t, err)
	defer release()

	time.Sleep(150 * time.Millisecond)
	_, err = held.Usage(t.Context())
	require.NoError(t, err)
}

func TestRemoveDeletesAllFiles(t *testing.T) {
	p, dir := newPool(t, 4, 0)
	id := uuid.NewString()

	udb, release, err := p.Open(t.Context(), id)
	require.NoError(t, err)
	put(t, udb, "a.md")
	release()
	_, err = os.Stat(filepath.Join(dir, id+".db-wal"))
	require.NoError(t, err, "WAL file should exist while the handle is open")

	require.NoError(t, p.Remove(t.Context(), id))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestRemoveWaitsForLeases(t *testing.T) {
	p, dir := newPool(t, 4, 0)
	id := uuid.NewString()
	udb, release, err := p.Open(t.Context(), id)
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { done <- p.Remove(t.Context(), id) }()

	select {
	case <-done:
		t.Fatal("Remove returned while a lease was live")
	case <-time.After(50 * time.Millisecond):
	}
	put(t, udb, "a.md")
	release()
	require.NoError(t, <-done)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestRemoveOfUnknownUserSucceeds(t *testing.T) {
	p, _ := newPool(t, 4, 0)
	require.NoError(t, p.Remove(t.Context(), uuid.NewString()))
}

func TestCloseClosesHandlesAndRejectsOpen(t *testing.T) {
	p, _ := newPool(t, 4, 20*time.Millisecond)
	udb, release, err := p.Open(t.Context(), uuid.NewString())
	require.NoError(t, err)
	release()

	require.NoError(t, p.Close())
	_, err = udb.Usage(t.Context())
	require.Error(t, err)
	_, _, err = p.Open(t.Context(), uuid.NewString())
	require.Error(t, err)
	require.NoError(t, p.Close(), "Close is idempotent")
}
