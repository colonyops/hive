package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/colonyops/hive/internal/desktop/pipeline/flow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrayProfilesIncludesValidAndInvalidFlows(t *testing.T) {
	dir := t.TempDir()
	store := flow.NewFlowStore(dir, nil)
	created, err := store.Create("Triage")
	require.NoError(t, err)
	_, err = store.SetEnabled(created.ID, false)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "broken.yaml"), []byte("name: [\n"), 0o600))
	require.NoError(t, store.Reload())

	assert.Equal(t, []trayProfile{
		{ID: "broken", Label: "broken (invalid)"},
		{ID: created.ID, Label: "Triage", Enabled: false, Valid: true},
	}, trayProfiles(store))
}
