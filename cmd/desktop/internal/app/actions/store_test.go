package actions

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ids(list []Action) []string {
	out := make([]string, 0, len(list))
	for _, a := range list {
		out = append(out, a.ID)
	}
	return out
}

func TestActionStore_ListGet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "actions.yml")
	require.NoError(t, os.WriteFile(path, []byte(multiActionYAML), 0o644))

	store := NewActionStore(path)

	list := store.List()
	require.Len(t, list, 3)
	assert.Equal(t, []string{"spawn-review", "run-lint", "notify"}, ids(list), "List keeps actions.yml order")

	a, ok := store.Get("run-lint")
	require.True(t, ok)
	assert.Equal(t, "shell", a.Type)

	_, ok = store.Get("nonexistent")
	assert.False(t, ok)
}

func TestActionStore_MissingFile_IsEmptyNotError(t *testing.T) {
	store := NewActionStore(filepath.Join(t.TempDir(), "nope.yml"))
	assert.Empty(t, store.List())
	assert.NoError(t, store.Err())
}

func TestActionStore_Reload_RetainsLastGoodOnFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "actions.yml")
	require.NoError(t, os.WriteFile(path, []byte(multiActionYAML), 0o644))

	store := NewActionStore(path)
	require.Len(t, store.List(), 3)

	// Break the file: now a bad edit.
	require.NoError(t, os.WriteFile(path, []byte(`version: 1
actions:
  - id: x
    type: not-a-type
`), 0o644))

	err := store.Reload()
	require.Error(t, err)
	assert.Equal(t, err, store.Err())

	// Last-good actions are still served.
	assert.Len(t, store.List(), 3)
}

func TestActionStore_Reload_RetainsLastGoodOnVersionReject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "actions.yml")
	require.NoError(t, os.WriteFile(path, []byte(multiActionYAML), 0o644))

	store := NewActionStore(path)
	require.Len(t, store.List(), 3)

	// A file written by a newer build: forward-only migration cannot downgrade it.
	require.NoError(t, os.WriteFile(path, []byte("version: 2\nactions: []\n"), 0o644))

	err := store.Reload()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version")

	// Last-good actions are still served.
	assert.Len(t, store.List(), 3)
}

func TestActionStore_Reload_PicksUpValidEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "actions.yml")
	require.NoError(t, os.WriteFile(path, []byte(`version: 1
actions: []
`), 0o644))

	store := NewActionStore(path)
	assert.Empty(t, store.List())

	require.NoError(t, os.WriteFile(path, []byte(multiActionYAML), 0o644))
	require.NoError(t, store.Reload())
	assert.Len(t, store.List(), 3)
}
