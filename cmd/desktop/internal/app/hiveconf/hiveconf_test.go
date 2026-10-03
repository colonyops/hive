package hiveconf_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/hiveconf"
)

func write(t *testing.T, dir, contents string) string {
	t.Helper()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
	return path
}

func workspaceDir(t *testing.T, repos int) string {
	t.Helper()
	dir := t.TempDir()
	for i := range repos {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, fmt.Sprintf("repo-%d", i), ".git"), 0o755))
	}
	return dir
}

func TestLoadReportsAMissingFileAsTheOrdinaryFirstRun(t *testing.T) {
	setup := hiveconf.Load(filepath.Join(t.TempDir(), "config.yaml"))

	assert.False(t, setup.Exists)
	assert.False(t, setup.Usable)
	assert.Empty(t, setup.Unreadable, "an absent file is not a failure to report")
	assert.Empty(t, setup.Profiles, "hive's claude fallback is not a choice the user made")
}

func TestLoadReadsWhatTheFileDeclares(t *testing.T) {
	repos := workspaceDir(t, 3)
	path := write(t, t.TempDir(), `
workspaces:
  - `+repos+`
agents:
  default: opencode
  agent_selector: true
  opencode:
    command: opencode
    flags: ["--agent", "free-permissions-runner"]
  claude: {}
`)

	setup := hiveconf.Load(path)

	assert.True(t, setup.Exists)
	assert.True(t, setup.Usable)
	assert.Equal(t, "opencode", setup.DefaultAgent)
	assert.ElementsMatch(t, []hiveconf.Profile{
		{Name: "opencode", Command: "opencode", Flags: []string{"--agent", "free-permissions-runner"}},
		{Name: "claude", Command: "claude"},
	}, setup.Profiles, "a profile with no command is named by its key, and agent_selector is not a profile")
	require.Len(t, setup.Workspaces, 1)
	assert.True(t, setup.Workspaces[0].Exists)
	assert.Equal(t, 3, setup.Workspaces[0].Repos)
}

// A file that declares only one of the two keys leaves the session launcher
// as empty as no file at all, so first run treats it the same way.
func TestLoadReportsAConfigMissingEitherKeyAsUnusable(t *testing.T) {
	repos := workspaceDir(t, 1)
	for name, contents := range map[string]string{
		"profiles but no workspaces": "agents:\n  default: claude\n  claude: {}\n",
		"workspaces but no profiles": "workspaces:\n  - " + repos + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			setup := hiveconf.Load(write(t, t.TempDir(), contents))

			assert.True(t, setup.Exists)
			assert.False(t, setup.Usable)
			assert.True(t, len(setup.Profiles) > 0 || len(setup.Workspaces) > 0, "what it does declare is still reported")
		})
	}
}

func TestLoadReadsHivesDeprecatedRepoDirsSpelling(t *testing.T) {
	repos := workspaceDir(t, 1)
	path := write(t, t.TempDir(), "repo_dirs:\n  - "+repos+"\nagents:\n  default: claude\n  claude: {}\n")

	setup := hiveconf.Load(path)

	assert.True(t, setup.Usable, "an older config is not an unconfigured one")
}

// Callers must distinguish no config from unreadable config because only one
// is safe to overwrite.
func TestLoadReportsAnUnparseableFileWithoutClaimingItIsEmpty(t *testing.T) {
	path := write(t, t.TempDir(), "workspaces: [\n")

	setup := hiveconf.Load(path)

	assert.True(t, setup.Exists)
	assert.False(t, setup.Usable)
	assert.NotEmpty(t, setup.Unreadable)
}

func TestLoadCountsOnlyDirectoriesThatAreRepositories(t *testing.T) {
	dir := workspaceDir(t, 2)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "not-a-repo"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".hidden", ".git"), 0o755))
	path := write(t, t.TempDir(), "workspaces:\n  - "+dir+"\nagents:\n  default: claude\n  claude: {}\n")

	setup := hiveconf.Load(path)

	assert.Equal(t, 2, setup.Workspaces[0].Repos)
}

func TestLoadMarksAConfiguredFolderThatIsNoLongerThere(t *testing.T) {
	path := write(t, t.TempDir(), "workspaces:\n  - /nope/gone\nagents:\n  default: claude\n  claude: {}\n")

	setup := hiveconf.Load(path)

	require.Len(t, setup.Workspaces, 1)
	assert.False(t, setup.Workspaces[0].Exists)
	assert.True(t, setup.Usable, "a moved folder leaves the file valid; it just finds nothing")
}

func TestAgentOptionsMarkWhatThisMachineCanRun(t *testing.T) {
	options := hiveconf.AgentOptions(t.Context(), func(_ context.Context, name string) (string, error) {
		if name == "opencode" {
			return "/usr/local/bin/opencode", nil
		}
		return "", os.ErrNotExist
	})

	names := make([]string, len(options))
	for i, option := range options {
		names[i] = option.Name
	}
	assert.Equal(t, []string{"claude", "opencode", "codex", "pi", "amp", "copilot", "cursor"}, names,
		"the picker follows hive init's order and does not reshuffle between launches")
	for _, option := range options {
		assert.Equal(t, option.Name == "opencode", option.Installed)
	}
}

// Hive reads "<<" inside agents as a profile name, not a merge key, so Load
// must not report the merged-in profiles as declared.
func TestLoadSkipsAYAMLMergeKeyInsideAgents(t *testing.T) {
	path := write(t, t.TempDir(), `shared: &shared
  codex:
    command: codex
    flags: []

agents:
  <<: *shared
  default: claude
  claude:
    command: claude
    flags: []
`)

	setup := hiveconf.Load(path)
	assert.Equal(t, "claude", setup.DefaultAgent)
	assert.Equal(t, []hiveconf.Profile{{Name: "claude", Command: "claude", Flags: []string{}}}, setup.Profiles)
}
