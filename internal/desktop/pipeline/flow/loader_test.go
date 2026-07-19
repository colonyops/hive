package flow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// workedExampleRefs resolves every reference used by workedExampleYAML.
func workedExampleRefs() MapRefs {
	return MapRefs{
		Sources: map[string]string{"team-prs": "github-search"},
		Feeds:   map[string]bool{"team-review": true},
		Actions: map[string]bool{"review-pr": true},
	}
}

// workedExampleYAML is the design doc's worked example: a github-source ->
// github-filter -> function(outputs:2) -> {feed, action} flow.
const workedExampleYAML = `version: 1
name: Frontend Triage
nodes:
  - { id: in-prs, type: github-source, source: team-prs }
  - { id: drop-bots, type: github-filter, exclude_authors: ["*[bot]"], repos: ["colonyops/*"] }
  - id: tag
    type: function
    outputs: 2
    on_message: |
      if (msg.payload.state === "closed") return null;
      msg.payload.tag = "review"; return [msg, null];
  - { id: team-feed, type: feed, feed: team-review }
  - { id: spawn-review, type: action, action: review-pr }
wires:
  - { from: in-prs, to: drop-bots }
  - { from: drop-bots, to: tag }
  - { from: tag, out: 0, to: team-feed }
  - { from: tag, out: 0, to: spawn-review }
`

func writeFlow(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestLoadFlow_WorkedExample(t *testing.T) {
	dir := t.TempDir()
	path := writeFlow(t, dir, "triage.yaml", workedExampleYAML)

	f, warnings, err := LoadFlow(path, workedExampleRefs())
	require.NoError(t, err)
	assert.Empty(t, warnings)

	assert.Equal(t, "triage", f.ID)
	assert.Equal(t, "Frontend Triage", f.Name)
	assert.True(t, f.Enabled)
	require.Len(t, f.Nodes, 5)
	require.Len(t, f.Wires, 4)

	var tag *Node
	for i := range f.Nodes {
		if f.Nodes[i].ID == "tag" {
			tag = &f.Nodes[i]
		}
	}
	require.NotNil(t, tag)
	fc, ok := tag.Config.(*FunctionConfig)
	require.True(t, ok)
	assert.Equal(t, 2, fc.Outputs())
}

func TestLoadFlow_IDIsFilenameStem(t *testing.T) {
	dir := t.TempDir()
	pathYaml := writeFlow(t, dir, "my-flow.yaml", minimalValidFlowYAML())
	f, _, err := LoadFlow(pathYaml, minimalRefs())
	require.NoError(t, err)
	assert.Equal(t, "my-flow", f.ID)

	pathYml := writeFlow(t, dir, "other.yml", minimalValidFlowYAML())
	f2, _, err := LoadFlow(pathYml, minimalRefs())
	require.NoError(t, err)
	assert.Equal(t, "other", f2.ID)
}

// minimalValidFlowYAML is a single source -> feed flow, used by tests that
// don't care about the worked example's specifics.
func minimalValidFlowYAML() string {
	return `version: 1
nodes:
  - { id: src, type: github-source, source: my-source }
  - { id: sink, type: feed, feed: my-feed }
wires:
  - { from: src, to: sink }
`
}

func minimalRefs() MapRefs {
	return MapRefs{
		Sources: map[string]string{"my-source": "github-search"},
		Feeds:   map[string]bool{"my-feed": true},
	}
}

func TestLoadFlow_MissingFile(t *testing.T) {
	_, _, err := LoadFlow(filepath.Join(t.TempDir(), "nope.yaml"), minimalRefs())
	require.Error(t, err)
}

func TestLoadFlows_IsolatesBrokenFileFromGoodOnes(t *testing.T) {
	dir := t.TempDir()
	writeFlow(t, dir, "good-a.yaml", minimalValidFlowYAML())
	writeFlow(t, dir, "good-b.yml", minimalValidFlowYAML())
	writeFlow(t, dir, "broken.yaml", `version: 1
nodes:
  - { id: src, type: not-a-real-type }
`)
	writeFlow(t, dir, "ignored.txt", "not a flow")

	flows, perFileErrors, warnings := LoadFlows(dir, minimalRefs())

	require.Len(t, flows, 2)
	ids := []string{flows[0].ID, flows[1].ID}
	assert.Contains(t, ids, "good-a")
	assert.Contains(t, ids, "good-b")

	require.Contains(t, perFileErrors, "broken.yaml")
	assert.Contains(t, perFileErrors["broken.yaml"].Error(), "unknown type")

	assert.NotContains(t, perFileErrors, "good-a.yaml")
	assert.NotContains(t, perFileErrors, "good-b.yml")
	assert.NotContains(t, perFileErrors, "ignored.txt")
	_ = warnings
}

func TestLoadFlows_MissingDir(t *testing.T) {
	flows, perFileErrors, _ := LoadFlows(filepath.Join(t.TempDir(), "does-not-exist"), minimalRefs())
	assert.Empty(t, flows)
	assert.NotEmpty(t, perFileErrors)
}
