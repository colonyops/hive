package agentws

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/colonyops/hive/cmd/desktop/internal/app/configmigrate"
	"github.com/colonyops/hive/cmd/desktop/internal/app/schedule"
)

// TestConfigRoundTrip proves parse -> validate -> serialise for both files
// (spec §13): a Workspace/Library value, marshalled and re-parsed, comes back
// equal.
func TestConfigRoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("Workspace", func(t *testing.T) {
		t.Parallel()

		w := Workspace{
			Version: configmigrate.AgentWorkspaceSet.Current,
			Name:    "Home Assistant",
			Command: "claude",
			MCPs:    []string{"home-assistant"},
			Skills:  []string{"hive-mcp"},
		}
		require.NoError(t, w.Validate())

		data, err := yaml.Marshal(w)
		require.NoError(t, err)

		parsed, err := parseWorkspace(data)
		require.NoError(t, err)
		parsed.Dir = "" // yaml:"-"; parseWorkspace never sets it
		assert.Equal(t, w, parsed)
	})

	t.Run("Library", func(t *testing.T) {
		t.Parallel()

		l := Library{
			Version: configmigrate.MCPLibrarySet.Current,
			Servers: map[string]MCPServer{
				"home-assistant": {
					Title:   "Home Assistant",
					Type:    "http",
					URL:     "http://homeassistant.local:8123/mcp",
					Headers: map[string]string{"Authorization": "Bearer token"},
				},
				"local-tool": {
					Command: "npx",
					Args:    []string{"-y", "@example/mcp"},
					Env:     map[string]string{"TOKEN": "abc"},
				},
			},
		}
		require.NoError(t, l.Validate())

		data, err := yaml.Marshal(l)
		require.NoError(t, err)

		parsed, err := parseLibrary(data)
		require.NoError(t, err)
		assert.Equal(t, l, parsed)
	})
}

func TestStrictDecodeRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	t.Run("Workspace", func(t *testing.T) {
		t.Parallel()
		_, err := parseWorkspace([]byte("version: 5\nname: X\ncommand: claude\nfoo: bar\n"))
		require.Error(t, err)
	})

	t.Run("Library", func(t *testing.T) {
		t.Parallel()
		_, err := parseLibrary([]byte("version: 1\nservers: {}\nfoo: bar\n"))
		require.Error(t, err)
	})
}

func TestLoadWorkspaceSetsDirFromPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, "homeassistant")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	path := filepath.Join(dir, manifestFileName)
	require.NoError(t, os.WriteFile(path, []byte("version: 2\nname: X\nagent: claude\ncommand: claude\n"), 0o600))

	w, err := LoadWorkspace(path)
	require.NoError(t, err)
	assert.Equal(t, "homeassistant", w.Dir)
}

func TestAgentComesFromTheCommandWord(t *testing.T) {
	t.Parallel()

	w, err := parseWorkspace([]byte("version: 5\nname: X\ncommand: /opt/homebrew/bin/Claude --model opus\n"))
	require.NoError(t, err)
	assert.Equal(t, "claude", w.Agent(), "a path and a capital are the same CLI")

	w, err = parseWorkspace([]byte("version: 5\nname: X\ncommand: pi --some-flag\n"))
	require.NoError(t, err)
	assert.Equal(t, "pi", w.Agent())
}

func TestAgentKeyIsRejected(t *testing.T) {
	t.Parallel()

	_, err := parseWorkspace([]byte("version: 5\nname: X\nagent: claude\ncommand: claude\n"))
	require.Error(t, err)
}

func TestLoadWorkspaceMissingFileWrapsNotExist(t *testing.T) {
	t.Parallel()

	_, err := LoadWorkspace(filepath.Join(t.TempDir(), "nope", manifestFileName))
	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestLoadLibraryMissingFileWrapsNotExist(t *testing.T) {
	t.Parallel()

	_, err := LoadLibrary(filepath.Join(t.TempDir(), libraryFileName))
	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

// promptedCommand is the smallest command a schedule can run on: it passes
// the prompt through, which Validate requires once a manifest declares one.
const promptedCommand = "claude" + PromptTail

// TestLoadWorkspaceStampsTheDirOntoEverySchedule: a Spec leaves the workspace
// on its own to reach the scheduler, so the directory it came from has to
// travel with it.
func TestLoadWorkspaceStampsTheDirOntoEverySchedule(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, "product")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	manifest := `version: 5
name: Product
command: ` + promptedCommand + `
schedules:
  - id: weekly-summary
    name: Weekly product summary
    cron: "0 9 * * 5"
    prompt: |
      Summarize product activity since {{ if .LastRun }}{{ date "2006-01-02" .LastRun }}{{ else }}last week{{ end }}.
    on_missed: skip
  - id: daily
    cron: "@daily"
    prompt: Standup.
    disabled: true
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, manifestFileName), []byte(manifest), 0o600))

	w, err := LoadWorkspace(filepath.Join(dir, manifestFileName))
	require.NoError(t, err)
	require.Len(t, w.Schedules, 2)
	for _, spec := range w.Schedules {
		assert.Equal(t, "product", spec.Workspace)
	}
	assert.Equal(t, "Weekly product summary", w.Schedules[0].Name)
	assert.Equal(t, schedule.OnMissedSkip, w.Schedules[0].OnMissed)
	assert.Empty(t, w.Schedules[1].Name)
	assert.True(t, w.Schedules[1].Disabled)
}

func TestLoadWorkspaceRejectsABrokenSchedule(t *testing.T) {
	t.Parallel()

	_, err := parseWorkspace([]byte("version: 5\nname: Product\ncommand: " + promptedCommand + "\nschedules:\n  - id: weekly\n    cron: \"nope\"\n    prompt: go\n"))
	require.Error(t, err)
}

// A command that drops the prompt is fine on its own and a problem the moment
// a schedule relies on it: the manifest, not the launch, is where that says so.
func TestScheduledWorkspaceNeedsACommandThatTakesThePrompt(t *testing.T) {
	t.Parallel()

	_, err := parseWorkspace([]byte("version: 5\nname: Product\ncommand: claude\n"))
	require.NoError(t, err)

	_, err = parseWorkspace([]byte("version: 5\nname: Product\ncommand: claude\nschedules:\n  - id: weekly\n    cron: \"@daily\"\n    prompt: go\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), ".Prompt")
}
