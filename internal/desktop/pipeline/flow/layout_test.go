package flow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveUI_LoadUI_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "triage.ui.yaml")

	layout := Layout{Nodes: map[string]NodePosition{
		"src":  {X: 10, Y: 20},
		"sink": {X: 240, Y: 20},
	}}

	require.NoError(t, SaveUI(path, layout))
	loaded := LoadUI(path)
	assert.Equal(t, layout, loaded)
}

func TestLoadUI_MissingFile_ReturnsEmptyLayoutNoError(t *testing.T) {
	dir := t.TempDir()
	layout := LoadUI(filepath.Join(dir, "nope.ui.yaml"))
	assert.Equal(t, Layout{Nodes: map[string]NodePosition{}}, layout)
}

func TestLoadUI_BrokenFile_ReturnsEmptyLayoutNoError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "triage.ui.yaml")
	require.NoError(t, os.WriteFile(path, []byte("not: [valid: yaml"), 0o644))

	layout := LoadUI(path)
	assert.Equal(t, Layout{Nodes: map[string]NodePosition{}}, layout)
}

func TestSaveUI_CreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "triage.ui.yaml")
	require.NoError(t, SaveUI(path, Layout{Nodes: map[string]NodePosition{"a": {X: 1, Y: 2}}}))

	loaded := LoadUI(path)
	assert.Equal(t, 1, loaded.Nodes["a"].X)
}
