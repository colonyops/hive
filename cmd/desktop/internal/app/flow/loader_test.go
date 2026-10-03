package flow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmigrate "github.com/colonyops/hive/internal/config/migrate"
)

type testRefs struct {
	actions map[string]bool
}

func (r testRefs) ResolveAction(id string) bool {
	return r.actions[id]
}

// workedExampleRefs resolves every reference used by workedExampleYAML — now
// only the action node's actions.yml id.
func workedExampleRefs() testRefs {
	return testRefs{
		actions: map[string]bool{"review-pr": true},
	}
}

func writeFlow(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// TestLoadFlow_WorkedExample doubles as the guarantee behind the flows
// authoring prompt: WorkedExampleYAML is the example that prompt hands an
// agent, so it has to parse and validate cleanly here.
func TestLoadFlow_WorkedExample(t *testing.T) {
	dir := t.TempDir()
	path := writeFlow(t, dir, "triage.yaml", WorkedExampleYAML)

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
  - { id: src, type: sources.github, credential: github/octocat, kind: search, query: "is:open" }
  - { id: sink, type: feed }
wires:
  - { from: src, to: sink }
`
}

// minimalRefs — the minimal flow references no actions, so an empty resolver
// suffices.
func minimalRefs() testRefs {
	return testRefs{}
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

func TestLoadFlows_SkipsUIYAMLSiblings(t *testing.T) {
	dir := t.TempDir()
	writeFlow(t, dir, "triage.yaml", minimalValidFlowYAML())
	// A sibling layout file, in the same directory, must never be treated
	// as a flow definition — its content isn't the flow schema at all.
	writeFlow(t, dir, "triage.ui.yaml", "nodes:\n  src: { x: 10, y: 20 }\n")
	writeFlow(t, dir, "triage.ui.yml", "nodes: {}\n")

	flows, perFileErrors, _ := LoadFlows(dir, minimalRefs())

	require.Len(t, flows, 1)
	assert.Equal(t, "triage", flows[0].ID)
	assert.Empty(t, perFileErrors)
}

func TestLoadFlows_MissingDir(t *testing.T) {
	flows, perFileErrors, _ := LoadFlows(filepath.Join(t.TempDir(), "does-not-exist"), minimalRefs())
	assert.Empty(t, flows)
	assert.NotEmpty(t, perFileErrors)
}

// TestLoadFlow_InjectedSetMigratesThroughRealLoader proves migrate-before-
// strict-decode composes through the real loader: a version:1 fixture is
// driven through LoadFlow -> configmigrate.FlowSet.Apply -> parseFlow with a
// temporarily raised Current, and must come out as a valid current flow.
func TestLoadFlow_InjectedSetMigratesThroughRealLoader(t *testing.T) {
	original := configmigrate.FlowSet
	configmigrate.FlowSet = configmigrate.Set{
		Name:     "flow",
		Baseline: 1,
		Current:  2,
		Migrations: []configmigrate.Migration{
			{To: 2, Migrate: func(doc map[string]any) error { return nil }},
		},
	}
	t.Cleanup(func() { configmigrate.FlowSet = original })

	dir := t.TempDir()
	path := writeFlow(t, dir, "triage.yaml", minimalValidFlowYAML())

	f, _, err := LoadFlow(path, minimalRefs())
	require.NoError(t, err)
	assert.Equal(t, "triage", f.ID)
	require.Len(t, f.Nodes, 2)
}

func TestLoadFlow_RejectsVersionNewerThanCurrent(t *testing.T) {
	dir := t.TempDir()
	path := writeFlow(t, dir, "triage.yaml", `version: 2
nodes:
  - { id: sink, type: feed }
`)
	_, _, err := LoadFlow(path, minimalRefs())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version")
}

func TestLoadFlows_RejectNewerVersionIsIsolatedNotFatal(t *testing.T) {
	dir := t.TempDir()
	writeFlow(t, dir, "good.yaml", minimalValidFlowYAML())
	writeFlow(t, dir, "newer.yaml", `version: 2
nodes:
  - { id: sink, type: feed }
`)

	flows, perFileErrors, _ := LoadFlows(dir, minimalRefs())
	require.Len(t, flows, 1)
	assert.Equal(t, "good", flows[0].ID)
	assert.Contains(t, perFileErrors, "newer.yaml")
}

func TestLoadFlow_CorruptFileErrors(t *testing.T) {
	dir := t.TempDir()
	path := writeFlow(t, dir, "broken.yaml", "not: [unterminated")
	_, _, err := LoadFlow(path, minimalRefs())
	require.Error(t, err)
}

func TestLoadFlows_CorruptFileIsIsolatedNotFatal(t *testing.T) {
	dir := t.TempDir()
	writeFlow(t, dir, "good.yaml", minimalValidFlowYAML())
	writeFlow(t, dir, "broken.yaml", "not: [unterminated")

	flows, perFileErrors, _ := LoadFlows(dir, minimalRefs())
	require.Len(t, flows, 1)
	assert.Contains(t, perFileErrors, "broken.yaml")
}

// TestLoadFlow_CurrentFileIsNotRewritten proves the read path is pure Apply:
// loading an already-current flow must never write to disk.
func TestLoadFlow_CurrentFileIsNotRewritten(t *testing.T) {
	dir := t.TempDir()
	path := writeFlow(t, dir, "triage.yaml", minimalValidFlowYAML())
	before, err := os.ReadFile(path)
	require.NoError(t, err)

	_, _, err = LoadFlow(path, minimalRefs())
	require.NoError(t, err)

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, after, "load path is pure Apply and must never write")
}

// A source connector's config reaches the loader through SourceConfig's strict
// decode, and the exec connector is the first one whose fields are not all
// strings — a duration and a map. This is the whole path a hand-authored flow
// takes: parse, validate, and save back to the same shape.
func TestLoadFlow_ExecSourceRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := writeFlow(t, dir, "oncall.yaml", `version: 1
name: On-call
enabled: true
nodes:
  - id: src
    type: sources.exec
    command: gcx irm oncall alert-groups list -o json
    timeout: 30s
    interval: 1h
    cwd: ~/src
    env:
      GCX_PROFILE: prod
  - id: inbox
    type: feed
    name: On-call
wires:
  - from: src
    to: inbox
`)

	f, warnings, err := LoadFlow(path, testRefs{})
	require.NoError(t, err)
	assert.Empty(t, warnings)
	require.Len(t, f.Nodes, 2)

	require.NoError(t, SaveFlow(path, f))
	saved, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(saved), "timeout: 30s", "a duration must not round-trip as a nanosecond count")
	assert.Contains(t, string(saved), "GCX_PROFILE: prod")
}

// An unknown key in a connector's config is a typo the author must see, not a
// field silently dropped on the next save.
func TestLoadFlow_ExecSourceRejectsAnUnknownField(t *testing.T) {
	dir := t.TempDir()
	path := writeFlow(t, dir, "oncall.yaml", `version: 1
name: On-call
enabled: true
nodes:
  - id: src
    type: sources.exec
    command: echo '[]'
    timeout: 30s
    shell: fish
`)

	_, _, err := LoadFlow(path, testRefs{})
	require.ErrorContains(t, err, "shell")
}
