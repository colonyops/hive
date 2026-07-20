package actions

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActionStore_ListGet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "actions.yml")
	require.NoError(t, os.WriteFile(path, []byte(multiActionYAML), 0o644))

	store := NewActionStore(path)

	list := store.List()
	require.Len(t, list, 3)
	assert.Equal(t, "notify", list[0].ID, "List is sorted by id")

	a, ok := store.Get("run-lint")
	require.True(t, ok)
	assert.Equal(t, "shell", a.Type)

	_, ok = store.Get("nonexistent")
	assert.False(t, ok)
}

func TestActionStore_ViewsForFiltersAndHidesExecutionConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "actions.yml")
	require.NoError(t, os.WriteFile(path, []byte(multiActionYAML), 0o644))

	store := NewActionStore(path)
	assert.Equal(t, []View{
		{ID: "notify", Label: "Notify", Type: "publish-event", AutoApply: false},
		{ID: "run-lint", Label: "Run lint", Type: "shell", AutoApply: false},
		{ID: "spawn-review", Label: "Spawn review agent", Type: "launch-session", AutoApply: true},
	}, store.ViewsFor("PR"))
	assert.Equal(t, []View{
		{ID: "notify", Label: "Notify", Type: "publish-event", AutoApply: false},
		{ID: "run-lint", Label: "Run lint", Type: "shell", AutoApply: false},
	}, store.ViewsFor("issue"))
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
