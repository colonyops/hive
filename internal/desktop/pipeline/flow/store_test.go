package flow

import (
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
		ID:      "triage",
		Enabled: true,
		Nodes: []Node{
			{ID: "src", Type: "github-source", Config: &GithubSourceConfig{Source: "my-source"}},
			{ID: "sink", Type: "feed", Config: &FeedConfig{Feed: "my-feed"}},
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
		ID:      "triage",
		Enabled: true,
		Nodes: []Node{
			{ID: "src", Type: "github-source", Config: &GithubSourceConfig{Source: "my-source"}},
			{ID: "sink", Type: "feed", Config: &FeedConfig{Feed: "my-feed"}},
		},
		Wires: []Wire{{From: "src", To: "sink"}},
	}
	require.NoError(t, store.Save(good))

	// An invalid edit: sink now references an unknown feed.
	bad := good
	bad.Nodes = append([]Node{}, good.Nodes...)
	bad.Nodes[1] = Node{ID: "sink", Type: "feed", Config: &FeedConfig{Feed: "unknown-feed"}}

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

func TestFlowStore_LayoutRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewFlowStore(dir, minimalRefs())

	layout := Layout{Nodes: map[string]NodePosition{"src": {X: 5, Y: 6}}}
	require.NoError(t, store.SaveLayout("triage", layout))
	assert.Equal(t, layout, store.GetLayout("triage"))

	// A flow with no layout file yet gets an empty one, not an error.
	assert.Equal(t, Layout{Nodes: map[string]NodePosition{}}, store.GetLayout("no-layout-yet"))
}
