package flow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleSidebar() SidebarLayout {
	return SidebarLayout{Items: []SidebarItem{
		{Feed: "my-open-prs"},
		{Folder: &SidebarFolder{ID: "work", Name: "Work", Feeds: []string{"assigned", "notifications"}}},
		{Feed: "misc"},
	}}
}

func TestSaveSidebar_LoadSidebar_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "triage.sidebar.yaml")

	layout := sampleSidebar()
	require.NoError(t, SaveSidebar(path, layout))
	assert.Equal(t, layout, LoadSidebar(path))
}

func TestLoadSidebar_MissingFile_ReturnsEmptyNoError(t *testing.T) {
	dir := t.TempDir()
	layout := LoadSidebar(filepath.Join(dir, "nope.sidebar.yaml"))
	assert.Equal(t, SidebarLayout{Items: []SidebarItem{}}, layout)
}

func TestLoadSidebar_BrokenFile_ReturnsEmptyNoError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "triage.sidebar.yaml")
	require.NoError(t, os.WriteFile(path, []byte("items: [folder: {"), 0o644))

	assert.Equal(t, SidebarLayout{Items: []SidebarItem{}}, LoadSidebar(path))
}

func TestSaveSidebar_CreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "triage.sidebar.yaml")
	require.NoError(t, SaveSidebar(path, sampleSidebar()))

	loaded := LoadSidebar(path)
	require.Len(t, loaded.Items, 3)
	assert.Equal(t, "my-open-prs", loaded.Items[0].Feed)
}

func TestFlowStore_SidebarRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewFlowStore(dir, minimalRefs())

	layout := sampleSidebar()
	require.NoError(t, store.SaveSidebar("triage", layout))
	assert.Equal(t, layout, store.GetSidebar("triage"))

	// A flow with no sidebar file yet gets an empty layout, not an error.
	assert.Equal(t, SidebarLayout{Items: []SidebarItem{}}, store.GetSidebar("no-sidebar-yet"))
}

func TestFlowStore_Delete_RemovesSidebarFile(t *testing.T) {
	dir := t.TempDir()
	store := NewFlowStore(dir, minimalRefs())
	f, err := store.Create("Triage")
	require.NoError(t, err)
	require.NoError(t, store.SaveSidebar(f.ID, sampleSidebar()))

	require.NoError(t, store.Delete(f.ID))

	_, err = os.Stat(filepath.Join(dir, f.ID+".sidebar.yaml"))
	assert.True(t, os.IsNotExist(err), "sidebar layout file must be removed with the flow")
}

func TestLoadFlows_SkipsSidebarFile(t *testing.T) {
	dir := t.TempDir()
	writeFlow(t, dir, "triage.yaml", minimalValidFlowYAML())
	// A sibling .sidebar.yaml must be ignored by the flow loader — not parsed
	// as a flow (which would surface a spurious broken-file error).
	require.NoError(t, os.WriteFile(filepath.Join(dir, "triage.sidebar.yaml"),
		[]byte("items:\n  - feed: my-open-prs\n"), 0o644))

	flows, perFileErrors, _ := LoadFlows(dir, minimalRefs())
	require.Len(t, flows, 1)
	assert.Equal(t, "triage", flows[0].ID)
	assert.Empty(t, perFileErrors)
}
