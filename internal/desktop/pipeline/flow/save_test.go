package flow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveFlow_NewFile_LoadSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "triage.yaml")

	f := Flow{
		ID:      "triage",
		Name:    "Frontend Triage",
		Enabled: true,
		Nodes: []Node{
			{ID: "in-prs", Type: "github-source", Config: &GithubSourceConfig{Kind: "search", Query: "is:open is:pr"}},
			{ID: "drop-bots", Type: "github-filter", Config: &GithubFilterConfig{ExcludeAuthors: []string{"*[bot]"}, Repos: []string{"colonyops/*"}}},
			{ID: "tag", Type: "function", Name: "Tag reviewed", Config: &FunctionConfig{OnMessage: "return msg;", OutputsN: 2, Timeout: Duration(5e9)}},
			{ID: "team-feed", Type: "feed", Config: &FeedConfig{}},
			{ID: "spawn-review", Type: "action", Disabled: true, Config: &ActionConfig{Action: "review-pr"}},
		},
		Wires: []Wire{
			{From: "in-prs", To: "drop-bots"},
			{From: "drop-bots", To: "tag"},
			{From: "tag", Out: 0, To: "team-feed"},
			{From: "tag", Out: 1, To: "spawn-review"},
		},
	}

	require.NoError(t, SaveFlow(path, f))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "# Hive Desktop flow", "new file gets the header comment")

	loaded, warnings, err := LoadFlow(path, workedExampleRefs())
	require.NoError(t, err)
	// spawn-review is disabled, which validateFlow reports as a soft warning.
	assert.Equal(t, []string{`node "spawn-review" is disabled`}, warnings)
	assert.Equal(t, f, loaded)

	// Save again (this time editing an existing file) and load once more —
	// still structurally identical.
	require.NoError(t, SaveFlow(path, loaded))
	reloaded, _, err := LoadFlow(path, workedExampleRefs())
	require.NoError(t, err)
	assert.Equal(t, f, reloaded)
}

func TestSaveFlow_WorkedExample_LoadSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := writeFlow(t, dir, "triage.yaml", workedExampleYAML)

	original, _, err := LoadFlow(path, workedExampleRefs())
	require.NoError(t, err)

	require.NoError(t, SaveFlow(path, original))

	roundTripped, _, err := LoadFlow(path, workedExampleRefs())
	require.NoError(t, err)
	assert.Equal(t, original, roundTripped)
}

func TestSaveFlow_EditPreservesHeaderAndUnrelatedKeys(t *testing.T) {
	dir := t.TempDir()
	original := `# Hand-written header — must survive an app edit.
# Second header line.
version: 1
name: Frontend Triage
nodes:
  - { id: src, type: github-source, kind: search, query: "is:open" }
  - { id: sink, type: feed }
wires:
  - { from: src, to: sink }
`
	path := writeFlow(t, dir, "triage.yaml", original)

	f, _, err := LoadFlow(path, minimalRefs())
	require.NoError(t, err)
	f.Name = "Renamed Triage"

	require.NoError(t, SaveFlow(path, f))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "# Hand-written header — must survive an app edit.")
	assert.Contains(t, string(data), "# Second header line.")

	reloaded, _, err := LoadFlow(path, minimalRefs())
	require.NoError(t, err)
	assert.Equal(t, "Renamed Triage", reloaded.Name)
	require.Len(t, reloaded.Nodes, 2)
}

func TestSaveFlow_PathMustMatchFlowID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "triage.yaml")
	f := Flow{ID: "other-id", Nodes: []Node{}}
	err := SaveFlow(path, f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match")
}

func TestSaveFlow_EmptyFileTreatedAsNew(t *testing.T) {
	dir := t.TempDir()
	path := writeFlow(t, dir, "triage.yaml", "")

	f := Flow{
		ID:      "triage",
		Enabled: true,
		Nodes: []Node{
			{ID: "src", Type: "github-source", Config: &GithubSourceConfig{Kind: "search", Query: "is:open"}},
			{ID: "sink", Type: "feed", Config: &FeedConfig{}},
		},
		Wires: []Wire{{From: "src", To: "sink"}},
	}
	require.NoError(t, SaveFlow(path, f))

	loaded, _, err := LoadFlow(path, minimalRefs())
	require.NoError(t, err)
	assert.Equal(t, f, loaded)
}

func TestSaveFlow_DisabledFlowRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "triage.yaml")

	f := Flow{
		ID:      "triage",
		Enabled: false,
		Nodes: []Node{
			{ID: "src", Type: "github-source", Config: &GithubSourceConfig{Kind: "search", Query: "is:open"}},
			{ID: "sink", Type: "feed", Config: &FeedConfig{}},
		},
		Wires: []Wire{{From: "src", To: "sink"}},
	}
	require.NoError(t, SaveFlow(path, f))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "enabled: false")

	loaded, _, err := LoadFlow(path, minimalRefs())
	require.NoError(t, err)
	assert.False(t, loaded.Enabled)
	assert.Equal(t, f, loaded)
}
