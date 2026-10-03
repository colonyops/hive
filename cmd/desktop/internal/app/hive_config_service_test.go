package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/hiveconf"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

func hiveSetupApp(t *testing.T) (core *App, configPath, workspace string) {
	t.Helper()
	root := t.TempDir()
	configPath = filepath.Join(root, "hive.yaml")
	workspace = filepath.Join(root, "code")
	seedRepo(t, workspace, "a-repo")

	t.Setenv(settings.EnvDataDir, filepath.Join(root, "data"))
	t.Setenv(settings.EnvConfigDir, filepath.Join(root, "config"))
	t.Setenv("HIVE_CONFIG", configPath)
	// HIVE_DEFAULT_AGENT wins over agents.default at load, so a developer who
	// exports it would otherwise see these assertions answer to their shell.
	t.Setenv("HIVE_DEFAULT_AGENT", "")
	t.Setenv(settings.EnvMockMode, "feed")

	core, err := New(t.Context(), Config{
		Settings: settings.DefaultSettings(),
		MockMode: settings.MockMode(),
		Logger:   zerolog.Nop(),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = core.Close() })
	return core, configPath, workspace
}

// Hive skips repositories without readable origin remotes, so a bare .git
// directory would not exercise launch-option discovery.
func seedRepo(t *testing.T, parent, name string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	dir := filepath.Join(parent, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for _, args := range [][]string{
		{"init", "--quiet"},
		{"remote", "add", "origin", "https://github.com/example/" + name + ".git"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
}

func TestNewCreatesTheDefaultProfile(t *testing.T) {
	core, _, _ := hiveSetupApp(t)

	statuses := core.Flows.Statuses(t.Context())

	require.Len(t, statuses, 1)
	assert.Equal(t, DefaultProfileName, statuses[0].Flow.Name)
}

func TestSetupReportsFirstRunWhenNoConfigExists(t *testing.T) {
	core, configPath, _ := hiveSetupApp(t)

	setup := core.HiveConfig.Setup(t.Context())

	assert.Equal(t, configPath, setup.Config.Path)
	assert.False(t, setup.Config.Exists)
	assert.False(t, setup.Config.Usable)
	assert.NotEmpty(t, setup.Agents, "the picker has a catalog to offer even with no config")
	assert.Empty(t, setup.Config.Profiles, "hive's claude fallback is not a choice the user made")
}

func TestSaveTakesEffectWithoutARestart(t *testing.T) {
	core, _, workspace := hiveSetupApp(t)

	before, err := core.Sessions.SessionLaunchOptions(t.Context())
	require.NoError(t, err)
	require.Empty(t, before.Repositories, "with no config there is nothing to launch against")

	saved, err := core.HiveConfig.Save(t.Context(), HiveSetupRequest{
		DefaultAgent: "opencode",
		Profiles: []hiveconf.Profile{
			{Name: "opencode", Command: "opencode", Flags: []string{"--agent", "free-permissions-runner"}},
		},
		Workspaces: []string{workspace},
	})
	require.NoError(t, err)
	assert.True(t, saved.Config.Usable)

	after, err := core.Sessions.SessionLaunchOptions(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"opencode"}, after.Agents)
	assert.Equal(t, "opencode", after.DefaultAgent)
	require.Len(t, after.Repositories, 1, "the repository under the saved workspace is offered now, not after a relaunch")
	assert.Equal(t, "a-repo", after.Repositories[0].Name)
}

func TestSaveUpdatesTheWorkspacePresets(t *testing.T) {
	core, _, workspace := hiveSetupApp(t)

	_, err := core.HiveConfig.Save(t.Context(), HiveSetupRequest{
		DefaultAgent: "my-agent",
		Profiles:     []hiveconf.Profile{{Name: "my-agent", Command: "my-agent", Flags: []string{"--yolo"}}},
		Workspaces:   []string{workspace},
	})
	require.NoError(t, err)

	var found bool
	for _, preset := range core.AgentWorkspaces.Presets(t.Context()) {
		if preset.ID == "hive-my-agent" {
			found = true
		}
	}
	assert.True(t, found, "a profile saved just now is offered as a preset")
}

func TestSaveRejectsAnEditThatWouldNotLoadAndLeavesTheRuntimeAlone(t *testing.T) {
	core, configPath, workspace := hiveSetupApp(t)

	_, err := core.HiveConfig.Save(t.Context(), HiveSetupRequest{
		DefaultAgent: "codex",
		Profiles:     []hiveconf.Profile{{Name: "claude"}},
		Workspaces:   []string{workspace},
	})

	require.Error(t, err)
	assert.Equal(t, KindInvalid, KindOf(err), "a default naming no chosen agent is the user's to fix, not a crash")
	_, statErr := os.Stat(configPath)
	assert.True(t, os.IsNotExist(statErr), "a rejected save writes no file at all")
	after, err := core.Sessions.SessionLaunchOptions(t.Context())
	require.NoError(t, err)
	assert.Empty(t, after.Repositories, "and the runtime still serves the config it started with")
}

// config.ApplyEdit validates its owned keys, but only Hive's loader knows the rest of
// the file. Here the process
// environment forces an agent the edit is about to drop — the shape a terminal
// launch with HIVE_DEFAULT_AGENT exported produces — and the write must be
// refused rather than landing a file the next launch dies on.
func TestSaveRefusesWhatHiveWouldNotLoad(t *testing.T) {
	core, configPath, workspace := hiveSetupApp(t)
	_, err := core.HiveConfig.Save(t.Context(), HiveSetupRequest{
		DefaultAgent: "codex",
		Profiles:     []hiveconf.Profile{{Name: "codex"}, {Name: "claude"}},
		Workspaces:   []string{workspace},
	})
	require.NoError(t, err)
	before, err := os.ReadFile(configPath)
	require.NoError(t, err)
	t.Setenv("HIVE_DEFAULT_AGENT", "claude")

	_, err = core.HiveConfig.Save(t.Context(), HiveSetupRequest{
		DefaultAgent: "codex",
		Profiles:     []hiveconf.Profile{{Name: "codex"}},
		Workspaces:   []string{workspace},
	})

	require.Error(t, err)
	assert.Equal(t, KindInvalid, KindOf(err))
	assert.Contains(t, err.Error(), "Hive would not load the result")
	after, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "the refused edit changed nothing on disk")
	require.NoError(t, core.ReloadHiveRuntime(t.Context()), "so the file still loads")
}

// A rule naming a profile is the one dependency between the keys this app
// owns and the ones it does not; dropping the profile would strand the rule.
func TestSaveRefusesToDropAProfileARuleUses(t *testing.T) {
	core, configPath, workspace := hiveSetupApp(t)
	require.NoError(t, os.WriteFile(configPath, []byte("rules:\n  - pattern: acme/*\n    agent: claude\nagents:\n  default: claude\n  claude: {}\n  codex: {}\nworkspaces:\n  - "+workspace+"\n"), 0o644))
	require.NoError(t, core.ReloadHiveRuntime(t.Context()))

	_, err := core.HiveConfig.Save(t.Context(), HiveSetupRequest{
		DefaultAgent: "codex",
		Profiles:     []hiveconf.Profile{{Name: "codex"}},
		Workspaces:   []string{workspace},
	})

	require.Error(t, err)
	assert.Equal(t, KindInvalid, KindOf(err))
	assert.Contains(t, err.Error(), "acme/*")
	options, err := core.Sessions.SessionLaunchOptions(t.Context())
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"claude", "codex"}, options.Agents, "the runtime still serves both profiles")
}

// A reload that fails after the file landed is the one outcome Save cannot
// undo, so it is reported as what it is: saved, not applied.
func TestSaveReportsAWriteWhoseReloadFailed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	workspace := t.TempDir()
	service := newHiveConfigService(hiveConfigOptions{
		Location: staticHiveConfig(HiveConfigLocation{Path: path}),
		Reload:   func(context.Context) error { return errors.New("boom") },
	})

	_, err := service.Save(t.Context(), HiveSetupRequest{
		DefaultAgent: "claude",
		Profiles:     []hiveconf.Profile{{Name: "claude"}},
		Workspaces:   []string{workspace},
	})

	require.Error(t, err)
	assert.Equal(t, KindInternal, KindOf(err))
	assert.Contains(t, err.Error(), "restart Hive")
	assert.True(t, hiveconf.Load(path).Usable, "the write itself landed")
}

func TestSaveRewritesOnlyTheKeysItOwns(t *testing.T) {
	core, configPath, workspace := hiveSetupApp(t)
	require.NoError(t, os.WriteFile(configPath, []byte("copy_command: pbcopy\n# keep me\ntmux:\n  poll_interval: 900ms\n"), 0o644))
	require.NoError(t, core.ReloadHiveRuntime(t.Context()))

	_, err := core.HiveConfig.Save(t.Context(), HiveSetupRequest{
		DefaultAgent: "claude",
		Profiles:     []hiveconf.Profile{{Name: "claude", Command: "claude"}},
		Workspaces:   []string{workspace},
	})
	require.NoError(t, err)

	raw, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "# keep me")
	assert.Contains(t, string(raw), "poll_interval: 900ms")
	assert.Contains(t, string(raw), "pbcopy")
}

func TestSetupReportsTheEnvironmentOverridingTheChosenAgent(t *testing.T) {
	core, _, workspace := hiveSetupApp(t)
	_, err := core.HiveConfig.Save(t.Context(), HiveSetupRequest{
		DefaultAgent: "codex",
		Profiles:     []hiveconf.Profile{{Name: "codex"}, {Name: "claude"}},
		Workspaces:   []string{workspace},
	})
	require.NoError(t, err)
	t.Setenv("HIVE_DEFAULT_AGENT", "claude")

	setup := core.HiveConfig.Setup(t.Context())

	assert.Equal(t, "codex", setup.Config.DefaultAgent, "the file still says what the user chose")
	assert.Equal(t, "claude", setup.DefaultAgentOverride, "and the screen can say what is actually winning")
}

// Hive ignores HIVE_DEFAULT_AGENT when it names no configured profile, so
// reporting one would warn about an override that is not happening.
func TestSetupIgnoresAnEnvironmentAgentThatIsNotConfigured(t *testing.T) {
	core, _, workspace := hiveSetupApp(t)
	_, err := core.HiveConfig.Save(t.Context(), HiveSetupRequest{
		DefaultAgent: "codex",
		Profiles:     []hiveconf.Profile{{Name: "codex"}},
		Workspaces:   []string{workspace},
	})
	require.NoError(t, err)
	t.Setenv("HIVE_DEFAULT_AGENT", "not-a-profile")

	setup := core.HiveConfig.Setup(t.Context())

	assert.Empty(t, setup.DefaultAgentOverride)
	assert.Equal(t, "codex", setup.Config.DefaultAgent)
}

func TestInspectWorkspaceAnswersBeforeTheSave(t *testing.T) {
	core, _, workspace := hiveSetupApp(t)

	found, err := core.HiveConfig.InspectWorkspace(t.Context(), workspace)
	require.NoError(t, err)
	assert.True(t, found.Exists)
	assert.Equal(t, 1, found.Repos)

	_, err = core.HiveConfig.InspectWorkspace(t.Context(), filepath.Join(workspace, "a-repo"))
	require.Error(t, err, "a repository is not the folder that holds repositories")
	assert.Equal(t, KindInvalid, KindOf(err))
}

func TestReloadFailureLeavesTheRunningServicesServing(t *testing.T) {
	core, configPath, workspace := hiveSetupApp(t)
	_, err := core.HiveConfig.Save(t.Context(), HiveSetupRequest{
		DefaultAgent: "claude",
		Profiles:     []hiveconf.Profile{{Name: "claude"}},
		Workspaces:   []string{workspace},
	})
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(configPath, []byte("workspaces: [\n"), 0o644))
	require.Error(t, core.ReloadHiveRuntime(t.Context()))

	after, err := core.Sessions.SessionLaunchOptions(t.Context())
	require.NoError(t, err)
	assert.Len(t, after.Repositories, 1, "the last good config is still serving")
}
