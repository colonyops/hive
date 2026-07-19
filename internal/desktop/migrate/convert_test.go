package migrate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/desktop/pipeline/flow"
)

// sampleProfiles mirrors the shape of a real profiles.yaml: shared sources, a
// profile whose feeds reference them, one with a filters block.
const sampleProfiles = `sources:
  - id: my-prs
    kind: search
    query: "is:open is:pr author:@me archived:false"
  - id: inbox
    kind: notifications
profiles:
  - id: triage
    name: Frontend Triage
    feeds:
      - id: my-open-prs
        name: My open PRs
        sources: [my-prs]
      - id: notifications-inbox
        name: Notifications inbox
        sources: [inbox]
        filters:
          exclude_authors: ["dependabot[bot]"]
          reasons: [mention, review_requested]
`

func writeProfiles(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "profiles.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestConvert_DryRun_ProducesValidFlowAndWritesNothing(t *testing.T) {
	dir := t.TempDir()
	profilesPath := writeProfiles(t, dir, sampleProfiles)
	flowsDir := filepath.Join(dir, "flows")

	report, err := Convert(profilesPath, flowsDir, Options{Write: false})
	require.NoError(t, err)
	require.True(t, report.DryRun)
	require.Len(t, report.Flows, 1)

	fr := report.Flows[0]
	assert.Equal(t, "triage", fr.ProfileID)
	assert.False(t, fr.Written)
	assert.False(t, fr.Skipped, "reason: %s", fr.Reason)
	assert.Contains(t, fr.YAML, "type: github-source")
	assert.Contains(t, fr.YAML, "type: github-filter")
	assert.Contains(t, fr.YAML, "type: feed")

	// Dry-run must not touch disk.
	_, err = os.Stat(flowsDir)
	assert.True(t, os.IsNotExist(err), "dry-run must not create the flows dir")
}

func TestConvert_Write_ProducesLoadableFlowWithWiredGraph(t *testing.T) {
	dir := t.TempDir()
	profilesPath := writeProfiles(t, dir, sampleProfiles)
	flowsDir := filepath.Join(dir, "flows")

	report, err := Convert(profilesPath, flowsDir, Options{Write: true})
	require.NoError(t, err)
	require.Len(t, report.Flows, 1)
	require.True(t, report.Flows[0].Written, "reason: %s", report.Flows[0].Reason)
	assert.Equal(t, profilesPath+".bak", report.BackupPath)

	// The written flow loads clean (no action refs → an empty resolver).
	flowPath := filepath.Join(flowsDir, "triage.yaml")
	f, warnings, err := flow.LoadFlow(flowPath, flow.MapRefs{})
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Equal(t, "triage", f.ID)
	assert.Equal(t, "Frontend Triage", f.Name)

	byID := make(map[string]flow.Node, len(f.Nodes))
	for _, n := range f.Nodes {
		byID[n.ID] = n
	}
	// Two sources, one filter (only notifications-inbox had filters), two feeds.
	require.Contains(t, byID, "my-prs")
	require.Contains(t, byID, "inbox")
	require.Contains(t, byID, "notifications-inbox-filter")
	require.Contains(t, byID, "my-open-prs")
	require.Contains(t, byID, "notifications-inbox")

	src := byID["my-prs"].Config.(*flow.GithubSourceConfig)
	assert.Equal(t, "search", src.Kind)
	assert.Equal(t, "is:open is:pr author:@me archived:false", src.Query)

	filt := byID["notifications-inbox-filter"].Config.(*flow.GithubFilterConfig)
	assert.Equal(t, []string{"dependabot[bot]"}, filt.ExcludeAuthors)
	assert.ElementsMatch(t, []string{"mention", "review_requested"}, filt.Reasons)

	// The un-filtered feed wires straight from its source; the filtered feed
	// goes source -> filter(pass) -> feed.
	assert.Contains(t, f.Wires, flow.Wire{From: "my-prs", To: "my-open-prs"})
	assert.Contains(t, f.Wires, flow.Wire{From: "inbox", To: "notifications-inbox-filter"})
	assert.Contains(t, f.Wires, flow.Wire{From: "notifications-inbox-filter", Out: 0, To: "notifications-inbox"})

	// A sibling layout file was written too.
	_, err = os.Stat(filepath.Join(flowsDir, "triage.ui.yaml"))
	require.NoError(t, err)
}

func TestConvert_Write_DoesNotOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	profilesPath := writeProfiles(t, dir, sampleProfiles)
	flowsDir := filepath.Join(dir, "flows")
	require.NoError(t, os.MkdirAll(flowsDir, 0o700))
	existing := filepath.Join(flowsDir, "triage.yaml")
	require.NoError(t, os.WriteFile(existing, []byte("version: 1\nnodes: []\n"), 0o644))

	report, err := Convert(profilesPath, flowsDir, Options{Write: true})
	require.NoError(t, err)
	require.Len(t, report.Flows, 1)
	assert.True(t, report.Flows[0].Skipped)
	assert.Contains(t, report.Flows[0].Reason, "already exists")

	// Force overwrites.
	report, err = Convert(profilesPath, flowsDir, Options{Write: true, Force: true})
	require.NoError(t, err)
	assert.True(t, report.Flows[0].Written)
}

func TestConvert_UnknownSourceRef_IsWarningNotFailure(t *testing.T) {
	dir := t.TempDir()
	profilesPath := writeProfiles(t, dir, `profiles:
  - id: p
    name: P
    feeds:
      - id: f
        name: F
        sources: [ghost]
`)
	report, err := Convert(profilesPath, filepath.Join(dir, "flows"), Options{Write: false})
	require.NoError(t, err)
	require.Len(t, report.Flows, 1)
	assert.NotEmpty(t, report.Flows[0].Warnings)
	assert.Contains(t, report.Flows[0].Warnings[0], "unknown source")
}

func TestSlugify(t *testing.T) {
	assert.Equal(t, "frontend-triage", slugify("Frontend Triage"))
	assert.Equal(t, "my-prs", slugify("my-prs"))
	assert.Equal(t, "flow", slugify("!!!"))
	assert.Equal(t, "a-b", slugify("  a  b  "))
}
