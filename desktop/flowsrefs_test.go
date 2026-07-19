package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/desktop/pipeline/actions"
)

func TestActionsRefs_ResolveAction_ResolvesLoadedActionID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "actions.yml")
	require.NoError(t, os.WriteFile(path, []byte(`version: 1
actions:
  - id: spawn-review
    label: Spawn review
    type: shell
    command_template: "true"
`), 0o644))

	store := actions.NewActionStore(path)
	require.NoError(t, store.Reload())
	adapter := newActionsRefs(store)

	assert.True(t, adapter.ResolveAction("spawn-review"))
	assert.False(t, adapter.ResolveAction("does-not-exist"))
}
