package credentials

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

// newTestKeychainStore points a store at an in-memory keyring and a temp
// index. keyring.MockInit swaps a package-level provider, so nothing in this
// file may call t.Parallel().
func newTestKeychainStore(t *testing.T) *KeychainStore {
	t.Helper()
	keyring.MockInit()
	return NewKeychainStore(filepath.Join(t.TempDir(), "state", "credentials.json"))
}

// The index records which accounts exist; the keychain holds what they are.
// A value reaching the index would put a token in a plain file inside the
// data dir — the exact failure the "config holds refs, never tokens" rule
// exists to prevent, one directory over.
func TestIndexHoldsRefsAndNeverValues(t *testing.T) {
	store := newTestKeychainStore(t)
	const token = "ghp_realtokenvalue"
	require.NoError(t, store.Set(Ref{Provider: "github", Account: "octocat"}, token))

	raw, err := os.ReadFile(store.indexPath)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), token)
	assert.Contains(t, string(raw), "github/octocat")
}

// The keychain is the truth and the index is a cache. A credential revoked
// out from under the app — deleted in Keychain Access, or a machine restored
// from a backup that carried the index but not the keychain — must read as
// absent. Reporting it as present is a "Connected" badge over nothing, and
// the user's only clue would be an empty feed.
func TestKeychainServicesAreIsolated(t *testing.T) {
	keyring.MockInit()
	ref := Ref{Provider: "github", Account: "octocat"}
	first := NewKeychainStoreWithService(filepath.Join(t.TempDir(), "first.json"), "sh.hive.desktop.test.first")
	second := NewKeychainStoreWithService(filepath.Join(t.TempDir(), "second.json"), "sh.hive.desktop.test.second")
	require.NoError(t, first.Set(ref, "token-value"))

	_, err := second.Get(ref)
	require.ErrorIs(t, err, ErrNotFound)
	value, err := first.Get(ref)
	require.NoError(t, err)
	assert.Equal(t, "token-value", value)
}

func TestGetPrunesTheIndexWhenTheKeychainEntryIsGone(t *testing.T) {
	store := newTestKeychainStore(t)
	ref := Ref{Provider: "github", Account: "octocat"}
	require.NoError(t, store.Set(ref, "token-value"))

	// Out-of-band removal: the keychain loses the value, the index does not
	// hear about it.
	require.NoError(t, keyring.Delete(keyringService, ref.String()))

	_, err := store.Get(ref)
	require.ErrorIs(t, err, ErrNotFound)

	refs, err := store.List()
	require.NoError(t, err)
	assert.Empty(t, refs, "Get should have pruned the stale ref")
}

// A partly-corrupt index must not lock the user out of the credentials that
// are still fine, because the index is a cache and the recovery from
// "everything is gone" is re-authenticating every provider.
func TestListSkipsMalformedIndexEntries(t *testing.T) {
	store := newTestKeychainStore(t)
	good := Ref{Provider: "github", Account: "octocat"}
	require.NoError(t, store.Set(good, "token-value"))

	require.NoError(t, os.WriteFile(store.indexPath,
		[]byte(`{"refs":["github/octocat","","not-a-ref","/no-provider"]}`), 0o600))

	refs, err := store.List()
	require.NoError(t, err)
	assert.Equal(t, []Ref{good}, refs)
}

// First run: no index file yet. An error here would surface as a startup
// failure on a machine that has simply never stored a credential.
func TestListOnAMissingIndexIsEmpty(t *testing.T) {
	store := newTestKeychainStore(t)
	refs, err := store.List()
	require.NoError(t, err)
	assert.Empty(t, refs)
}

// The index is rewritten whole on every change, so a torn write loses every
// ref at once. Writing through a temp file and renaming is what keeps a
// crash mid-write from doing that; this pins that no partial file is left
// behind in its place.
func TestIndexWriteLeavesNoTemporaryFiles(t *testing.T) {
	store := newTestKeychainStore(t)
	require.NoError(t, store.Set(Ref{Provider: "github", Account: "octocat"}, "token-value"))
	require.NoError(t, store.Set(Ref{Provider: "grafana", Account: "prod"}, "token-value"))
	require.NoError(t, store.Delete(Ref{Provider: "github", Account: "octocat"}))

	entries, err := os.ReadDir(filepath.Dir(store.indexPath))
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	assert.Equal(t, []string{filepath.Base(store.indexPath)}, names)
}

// Reads and writes are serialised inside the store. Without that, two
// concurrent Sets read the same index, and the second write drops the first
// ref — a credential that stored successfully but cannot be enumerated.
func TestConcurrentSetsAllReachTheIndex(t *testing.T) {
	store := newTestKeychainStore(t)

	const count = 12
	var wg sync.WaitGroup
	for i := range count {
		wg.Go(func() {
			ref := Ref{Provider: "github", Account: fmt.Sprintf("account-%02d", i)}
			assert.NoError(t, store.Set(ref, fmt.Sprintf("token-%02d", i)))
		})
	}
	wg.Wait()

	refs, err := store.List()
	require.NoError(t, err)
	assert.Len(t, refs, count)
}

// The set of accounts a user has configured is not world-readable
// information, and the directory is created by this package on first write.
func TestIndexIsWrittenPrivately(t *testing.T) {
	store := newTestKeychainStore(t)
	require.NoError(t, store.Set(Ref{Provider: "github", Account: "octocat"}, "token-value"))

	info, err := os.Stat(store.indexPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	dir, err := os.Stat(filepath.Dir(store.indexPath))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dir.Mode().Perm())
}

// The on-disk shape is a wrapper object, not a bare array, so a later field
// does not have to change the file's top-level type. Pinning it here means a
// change to the encoding is a deliberate edit rather than a silent one.
func TestIndexOnDiskShape(t *testing.T) {
	store := newTestKeychainStore(t)
	require.NoError(t, store.Set(Ref{Provider: "grafana", Account: "prod"}, "token-value"))
	require.NoError(t, store.Set(Ref{Provider: "github", Account: "octocat"}, "token-value"))

	raw, err := os.ReadFile(store.indexPath)
	require.NoError(t, err)

	var idx index
	require.NoError(t, json.Unmarshal(raw, &idx))
	assert.Equal(t, []string{"github/octocat", "grafana/prod"}, idx.Refs)
}
