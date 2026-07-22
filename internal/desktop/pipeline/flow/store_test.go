package flow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlowStore_ListGet(t *testing.T) {
	dir := t.TempDir()
	writeFlow(t, dir, "triage.yaml", minimalValidFlowYAML())

	store := NewFlowStore(dir, minimalRefs())

	list := store.List()
	require.Len(t, list, 1)
	assert.Equal(t, "triage", list[0].ID)

	f, ok := store.Get("triage")
	require.True(t, ok)
	assert.Equal(t, "triage", f.ID)

	_, ok = store.Get("does-not-exist")
	assert.False(t, ok)
}

func TestFlowStore_Statuses_IsolatesBrokenFileFromGoodOnes(t *testing.T) {
	dir := t.TempDir()
	writeFlow(t, dir, "good.yaml", minimalValidFlowYAML())
	writeFlow(t, dir, "broken.yaml", `version: 1
nodes:
  - { id: src, type: not-a-real-type }
`)

	store := NewFlowStore(dir, minimalRefs())

	// The broken file must not keep the good one out of List().
	list := store.List()
	require.Len(t, list, 1)
	assert.Equal(t, "good", list[0].ID)

	statuses := store.Statuses()
	require.Len(t, statuses, 2)
	byID := make(map[string]FlowStatus, len(statuses))
	for _, st := range statuses {
		byID[st.ID] = st
	}
	require.Contains(t, byID, "good")
	assert.True(t, byID["good"].Valid)
	require.Contains(t, byID, "broken")
	assert.False(t, byID["broken"].Valid)
	assert.Error(t, byID["broken"].Err)
}

func TestFlowStore_Save_PersistsAndReloads(t *testing.T) {
	dir := t.TempDir()
	store := NewFlowStore(dir, minimalRefs())

	f := Flow{
		ID:        "triage",
		Enabled:   true,
		Resurface: ResurfacePolicyStateChanges,
		Nodes: []Node{
			{ID: "src", Type: "github-source", Config: &GithubSourceConfig{Kind: "search", Query: "is:open"}},
			{ID: "sink", Type: "feed", Config: &FeedConfig{}},
		},
		Wires: []Wire{{From: "src", To: "sink"}},
	}
	require.NoError(t, store.Save(f))

	got, ok := store.Get("triage")
	require.True(t, ok)
	assert.Equal(t, f, got)

	// The file actually landed on disk, at flows/<id>.yaml.
	_, _, err := LoadFlow(filepath.Join(dir, "triage.yaml"), minimalRefs())
	require.NoError(t, err)
}

func TestFlowStore_Save_InvalidFlowRejected_LeavesLastGood(t *testing.T) {
	dir := t.TempDir()
	store := NewFlowStore(dir, minimalRefs())

	good := Flow{
		ID:        "triage",
		Enabled:   true,
		Resurface: ResurfacePolicyStateChanges,
		Nodes: []Node{
			{ID: "src", Type: "github-source", Config: &GithubSourceConfig{Kind: "search", Query: "is:open"}},
			{ID: "sink", Type: "feed", Config: &FeedConfig{}},
		},
		Wires: []Wire{{From: "src", To: "sink"}},
	}
	require.NoError(t, store.Save(good))

	// An invalid edit: the source now has an unknown kind.
	bad := good
	bad.Nodes = append([]Node{}, good.Nodes...)
	bad.Nodes[0] = Node{ID: "src", Type: "github-source", Config: &GithubSourceConfig{Kind: "webhook"}}

	err := store.Save(bad)
	require.Error(t, err)

	got, ok := store.Get("triage")
	require.True(t, ok)
	assert.Equal(t, good, got, "last-good flow must still be served after a rejected save")

	// The on-disk file must also be untouched.
	onDisk, _, err := LoadFlow(filepath.Join(dir, "triage.yaml"), minimalRefs())
	require.NoError(t, err)
	assert.Equal(t, good, onDisk)
}

func TestFlowStore_Save_RejectsInvalidID(t *testing.T) {
	dir := t.TempDir()
	store := NewFlowStore(dir, minimalRefs())
	err := store.Save(Flow{ID: "../escape"})
	require.Error(t, err)
}

func TestFlowStore_Reload_PicksUpExternalChange(t *testing.T) {
	dir := t.TempDir()
	store := NewFlowStore(dir, minimalRefs())
	assert.Empty(t, store.List())

	writeFlow(t, dir, "triage.yaml", minimalValidFlowYAML())
	require.NoError(t, store.Reload())

	list := store.List()
	require.Len(t, list, 1)
	assert.Equal(t, "triage", list[0].ID)
}

func TestFlowStore_Create_SeedsStarterFlowWithUniqueID(t *testing.T) {
	dir := t.TempDir()
	store := NewFlowStore(dir, minimalRefs())

	f, err := store.Create("Frontend Triage")
	require.NoError(t, err)
	assert.Equal(t, "frontend-triage", f.ID)
	assert.Equal(t, "Frontend Triage", f.Name)
	assert.True(t, f.Enabled)
	assert.NotEmpty(t, f.Nodes)
	assert.NotEmpty(t, f.Wires)

	// It loads back clean (starter flow references no actions).
	loaded, warnings, err := LoadFlow(filepath.Join(dir, "frontend-triage.yaml"), minimalRefs())
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Equal(t, "Frontend Triage", loaded.Name)

	// A second create of the same name gets a unique id, not a clobber.
	f2, err := store.Create("Frontend Triage")
	require.NoError(t, err)
	assert.Equal(t, "frontend-triage-2", f2.ID)

	// A layout was seeded too.
	layout := store.GetLayout("frontend-triage")
	assert.NotEmpty(t, layout.Nodes)
}

func TestFlowStore_Rename_UpdatesOnlyDisplayName(t *testing.T) {
	dir := t.TempDir()
	store := NewFlowStore(dir, minimalRefs())
	created, err := store.Create("Frontend Triage")
	require.NoError(t, err)

	renamed, err := store.Rename(created.ID, "  Team Triage  ")
	require.NoError(t, err)
	assert.Equal(t, created.ID, renamed.ID)
	assert.Equal(t, "Team Triage", renamed.Name)
	assert.Equal(t, created.Nodes, renamed.Nodes)
	assert.Equal(t, created.Wires, renamed.Wires)

	loaded, ok := store.Get(created.ID)
	require.True(t, ok)
	assert.Equal(t, "Team Triage", loaded.Name)

	_, err = store.Rename(created.ID, "   ")
	require.ErrorContains(t, err, "name cannot be empty")
	_, err = store.Rename("missing", "Name")
	require.ErrorContains(t, err, "not found")
}

func TestFlowStore_SetEnabled_PreservesFlowAndPersists(t *testing.T) {
	dir := t.TempDir()
	store := NewFlowStore(dir, minimalRefs())
	created, err := store.Create("Triage")
	require.NoError(t, err)

	disabled, err := store.SetEnabled(created.ID, false)
	require.NoError(t, err)
	assert.False(t, disabled.Enabled)
	assert.Equal(t, created.Name, disabled.Name)
	assert.Equal(t, created.Nodes, disabled.Nodes)
	assert.Equal(t, created.Wires, disabled.Wires)

	loaded, _, err := LoadFlow(filepath.Join(dir, created.ID+".yaml"), minimalRefs())
	require.NoError(t, err)
	assert.False(t, loaded.Enabled)

	// A disk edit that has not reached the debounced watcher must survive the
	// toggle; SetEnabled reads the current file rather than the cached graph.
	loaded.Name = "Externally edited"
	require.NoError(t, SaveFlow(filepath.Join(dir, created.ID+".yaml"), loaded))

	enabled, err := store.SetEnabled(created.ID, true)
	require.NoError(t, err)
	assert.True(t, enabled.Enabled)
	assert.Equal(t, "Externally edited", enabled.Name)

	_, err = store.SetEnabled("missing", false)
	require.ErrorContains(t, err, "not found")
}

func TestFlowStore_Delete_RemovesFlowAndLayout(t *testing.T) {
	dir := t.TempDir()
	store := NewFlowStore(dir, minimalRefs())
	f, err := store.Create("Triage")
	require.NoError(t, err)

	require.NoError(t, store.Delete(f.ID))

	_, ok := store.Get(f.ID)
	assert.False(t, ok)
	_, err = os.Stat(filepath.Join(dir, f.ID+".yaml"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(dir, f.ID+".ui.yaml"))
	assert.True(t, os.IsNotExist(err))

	// Deleting a non-existent flow is not an error (end state is the same).
	require.NoError(t, store.Delete("never-existed"))
}

func TestFlowStore_LayoutRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewFlowStore(dir, minimalRefs())

	layout := Layout{Nodes: map[string]NodePosition{"src": {X: 5, Y: 6}}}
	require.NoError(t, store.SaveLayout("triage", layout))
	assert.Equal(t, layout, store.GetLayout("triage"))

	// A flow with no layout file yet gets an empty one, not an error.
	assert.Equal(t, Layout{Nodes: map[string]NodePosition{}}, store.GetLayout("no-layout-yet"))
}
