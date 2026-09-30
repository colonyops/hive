package mcpsrv_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/agentws"
)

// seedWorkspace writes one agent workspace's manifest under the config dir
// the harness points the app at, and reports the manifest's path through
// path so a test can read the file back.
func seedWorkspace(dir, manifest string, path *string) func(t *testing.T, configDir string) {
	return func(t *testing.T, configDir string) {
		t.Helper()
		workspace := filepath.Join(configDir, "workspaces", dir)
		require.NoError(t, os.MkdirAll(workspace, 0o755))
		file := filepath.Join(workspace, "agent-workspace.yaml")
		require.NoError(t, os.WriteFile(file, []byte(manifest), 0o644))
		if path != nil {
			*path = file
		}
	}
}

type scheduleRow struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Cron      string `json:"cron"`
	OnMissed  string `json:"onMissed"`
	Disabled  bool   `json:"disabled"`
	NextRunAt string `json:"nextRunAt"`
}

// The schedule tools edit one entry of a workspace's manifest in place. The
// rest of the file, its comments included, survives; a bad edit is refused
// with a kind the agent can branch on.
func TestScheduleToolsEditOneWorkspaceEntryInPlace(t *testing.T) {
	var demoManifest string
	_, session := testSession(t,
		seedWorkspace("demo", "# keep me\nversion: 5\nname: Demo\ncommand: claude"+agentws.PromptTail+"\n", &demoManifest),
		seedWorkspace("broken", "version: 5\nname: Broken\ncommand: claude"+agentws.PromptTail+"\nschedules:\n  - id: weekly\n    cron: not a cron\n    prompt: go\n", nil),
	)

	var workspaces struct {
		Workspaces []struct {
			Dir       string   `json:"dir"`
			Problem   string   `json:"problem"`
			Schedules []string `json:"schedules"`
		} `json:"workspaces"`
	}
	call(t, session, "list_workspaces", map[string]any{}, &workspaces)
	byDir := map[string]string{}
	for _, w := range workspaces.Workspaces {
		byDir[w.Dir] = w.Problem
	}
	require.Contains(t, byDir, "demo")
	require.Contains(t, byDir, "broken")
	assert.Empty(t, byDir["demo"])
	assert.NotEmpty(t, byDir["broken"], "a workspace whose manifest does not parse still lists, with its problem")

	var put scheduleRow
	call(t, session, "put_schedule", map[string]any{
		"workspace": "demo", "id": "weekly", "name": "Weekly summary", "cron": "0 9 * * 5", "prompt": "Summarize {{ .Workspace.Name }}.",
	}, &put)
	assert.Equal(t, "weekly", put.ID)
	assert.Equal(t, "run", put.OnMissed, "the manifest omits the default; the view names it")
	_, err := time.Parse(time.RFC3339, put.NextRunAt)
	require.NoError(t, err, "times are RFC 3339: %q", put.NextRunAt)

	raw, err := os.ReadFile(demoManifest)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "# keep me", "the write edits the file in place")
	assert.Contains(t, string(raw), "id: weekly")

	var preview struct {
		Next           []string `json:"next"`
		Prompt         string   `json:"prompt"`
		FirstRunPrompt string   `json:"firstRunPrompt"`
	}
	call(t, session, "preview_schedule", map[string]any{
		"workspace": "demo", "cron": "0 9 * * 5",
		"prompt": "Summarize {{ .Workspace.Name }}{{ if .LastRun }} since {{ date \"2006-01-02\" .LastRun }}{{ end }}.",
	}, &preview)
	assert.Len(t, preview.Next, 5)
	assert.Contains(t, preview.Prompt, "Summarize Demo since ")
	assert.Equal(t, "Summarize Demo.", preview.FirstRunPrompt, "the first run, with no previous one, is previewed too")

	var listed struct {
		Workspace string        `json:"workspace"`
		Schedules []scheduleRow `json:"schedules"`
	}
	// HIVE_AGENT_WORKSPACE holds the workspace's absolute path, and the
	// argument descriptions send an agent to it, so the path is accepted too.
	call(t, session, "list_schedules", map[string]any{"workspace": filepath.Dir(demoManifest)}, &listed)
	assert.Equal(t, "demo", listed.Workspace)
	require.Len(t, listed.Schedules, 1)
	assert.Equal(t, "weekly", listed.Schedules[0].ID)

	assert.Contains(t, callErr(t, session, "schedule_runs", map[string]any{"workspace": "demo", "id": "never-existed"}), "not_found",
		"an id that is neither declared nor has run is not an empty history")
	assert.Contains(t, callErr(t, session, "put_schedule", map[string]any{"workspace": "demo", "id": "bad", "cron": "not a cron", "prompt": "go"}), "invalid")

	var removed struct {
		Removed bool `json:"removed"`
	}
	call(t, session, "remove_schedule", map[string]any{"workspace": "demo", "id": "weekly"}, &removed)
	assert.True(t, removed.Removed)
	call(t, session, "list_schedules", map[string]any{"workspace": "demo"}, &listed)
	assert.Empty(t, listed.Schedules)
}
