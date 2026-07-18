package feed

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreCreateProfilePersists(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewStore(dir)

	def, err := store.CreateProfile("  Frontend Triage  ")
	require.NoError(t, err)
	assert.NotEmpty(t, def.ID)
	assert.Equal(t, "Frontend Triage", def.Name)
	assert.Len(t, def.Feeds, 3)

	// A fresh store instance reads the persisted file.
	reloaded, err := NewStore(dir).Profiles()
	require.NoError(t, err)
	require.Len(t, reloaded, 1)
	assert.Equal(t, def.ID, reloaded[0].ID)
	assert.Equal(t, "notifications-inbox", reloaded[0].Feeds[1].ID)

	assert.FileExists(t, filepath.Join(dir, "profiles.json"))
}

func TestStoreCreateProfileRejectsEmptyName(t *testing.T) {
	t.Parallel()

	_, err := NewStore(t.TempDir()).CreateProfile("   ")
	require.Error(t, err)
}

func TestStoreEmptyDirHasNoProfiles(t *testing.T) {
	t.Parallel()

	profiles, err := NewStore(t.TempDir()).Profiles()
	require.NoError(t, err)
	assert.Empty(t, profiles)
}

func TestStoreMarkReadRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewStore(dir)
	readAt := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)

	_, ok := store.ReadAt("o/r#1")
	assert.False(t, ok)

	require.NoError(t, store.MarkRead("o/r#1", readAt))

	got, ok := store.ReadAt("o/r#1")
	require.True(t, ok)
	assert.True(t, got.Equal(readAt))

	// Persisted across instances.
	got, ok = NewStore(dir).ReadAt("o/r#1")
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
