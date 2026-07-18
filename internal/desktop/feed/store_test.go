package feed

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newStoreAt returns a store rooted in dir: the profiles config and the
// state files live side by side, which is all tests need.
func newStoreAt(dir string) *Store {
	return NewStore(filepath.Join(dir, "profiles.yaml"), dir)
}

func TestStoreCreateProfilePersists(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := newStoreAt(dir)

	def, err := store.CreateProfile("  Frontend Triage  ")
	require.NoError(t, err)
	assert.Equal(t, "frontend-triage", def.ID)
	assert.Equal(t, "Frontend Triage", def.Name)
	assert.Len(t, def.Feeds, 3)

	// A fresh store instance reads the persisted file.
	reloaded, err := newStoreAt(dir).Profiles()
	require.NoError(t, err)
	require.Len(t, reloaded, 1)
	assert.Equal(t, def.ID, reloaded[0].ID)
	assert.Equal(t, "notifications-inbox", reloaded[0].Feeds[1].ID)

	assert.FileExists(t, filepath.Join(dir, "profiles.yaml"))
}

func TestStoreCreateProfileUniqueIDs(t *testing.T) {
	t.Parallel()

	store := newStoreAt(t.TempDir())

	first, err := store.CreateProfile("Work")
	require.NoError(t, err)
	second, err := store.CreateProfile("Work")
	require.NoError(t, err)

	assert.Equal(t, "work", first.ID)
	assert.Equal(t, "work-2", second.ID)
}

func TestStoreCreateProfileRejectsEmptyName(t *testing.T) {
	t.Parallel()

	_, err := newStoreAt(t.TempDir()).CreateProfile("   ")
	require.Error(t, err)
}

func TestStoreCreateProfilePreservesComments(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`# my dotfiles-managed feeds
profiles:
  # the day job
  - id: work
    name: Work
    feeds:
      - id: prs
        name: My PRs
        kind: search
        query: "is:open is:pr author:@me"
`), 0o600))

	store := newStoreAt(dir)
	_, err := store.CreateProfile("Side Projects")
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "# my dotfiles-managed feeds")
	assert.Contains(t, string(data), "# the day job")
	assert.Contains(t, string(data), "side-projects")

	// The appended document round-trips through a full load.
	profiles, err := newStoreAt(dir).Profiles()
	require.NoError(t, err)
	require.Len(t, profiles, 2)
	assert.Equal(t, "work", profiles[0].ID)
	assert.Equal(t, "side-projects", profiles[1].ID)
	assert.Len(t, profiles[1].Feeds, 3)
}

func TestStoreCreateProfileRefusesBrokenConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "profiles.yaml"), []byte("profiles: ["), 0o600))

	_, err := newStoreAt(dir).CreateProfile("Work")
	require.ErrorContains(t, err, "profiles.yaml")
}

func TestStoreEmptyDirHasNoProfiles(t *testing.T) {
	t.Parallel()

	profiles, err := newStoreAt(t.TempDir()).Profiles()
	require.NoError(t, err)
	assert.Empty(t, profiles)
}

func TestStoreReloadKeepsLastGoodOnError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := newStoreAt(dir)
	def, err := store.CreateProfile("Work")
	require.NoError(t, err)

	// Break the file behind the store's back, as a live edit would.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "profiles.yaml"), []byte("profiles: ["), 0o600))
	require.Error(t, store.Reload())

	profiles, err := store.Profiles()
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	assert.Equal(t, def.ID, profiles[0].ID)

	info := store.ConfigInfo()
	assert.False(t, info.Valid)
	assert.NotEmpty(t, info.Error)
}

func TestStoreReloadPicksUpExternalEdit(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := newStoreAt(dir)
	_, err := store.CreateProfile("Work")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "profiles.yaml"), []byte(`profiles:
  - id: oss
    name: Open Source
    feeds:
      - id: inbox
        name: Inbox
        kind: notifications
        repos: ["colonyops/*"]
`), 0o600))
	require.NoError(t, store.Reload())

	profiles, err := store.Profiles()
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	assert.Equal(t, "oss", profiles[0].ID)
	assert.Equal(t, []string{"colonyops/*"}, profiles[0].Feeds[0].Repos)
}

func TestStoreConfigInfoMissingFile(t *testing.T) {
	t.Parallel()

	info := newStoreAt(t.TempDir()).ConfigInfo()
	assert.False(t, info.Exists)
	assert.True(t, info.Valid)
	assert.Contains(t, info.YAML, "profiles:")
}

func TestStoreMarkReadRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := newStoreAt(dir)
	readAt := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)

	_, ok := store.ReadAt("o/r#1")
	assert.False(t, ok)

	require.NoError(t, store.MarkRead("o/r#1", readAt))

	got, ok := store.ReadAt("o/r#1")
	require.True(t, ok)
	assert.True(t, got.Equal(readAt))

	// Persisted across instances.
	got, ok = newStoreAt(dir).ReadAt("o/r#1")
	require.True(t, ok)
	assert.True(t, got.Equal(readAt))
}

func TestProfileLetter(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "F", ProfileLetter("frontend triage"))
	assert.Equal(t, "9", ProfileLetter("9lives"))
	assert.Equal(t, "H", ProfileLetter("  hive"))
	assert.Equal(t, "?", ProfileLetter("---"))
}
