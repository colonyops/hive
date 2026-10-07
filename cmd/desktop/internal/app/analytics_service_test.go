package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
	"github.com/colonyops/hive/internal/config"
	domain "github.com/colonyops/hive/internal/domain/usageanalytics"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	usage "github.com/colonyops/hive/internal/hive/usageanalytics"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func newAnalyticsApp(t *testing.T, enabled bool) (*App, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv(config.EnvDefaultAgent, "")
	t.Setenv(config.EnvAnalyticsEnabled, "")
	t.Setenv(config.EnvConfig, filepath.Join(root, "hive.yaml"))
	t.Setenv(config.EnvDataDir, filepath.Join(root, "hive"))
	t.Setenv(settings.EnvHiveDataDir, filepath.Join(root, "hive"))
	t.Setenv(settings.EnvDataDir, filepath.Join(root, "desktop"))
	t.Setenv(settings.EnvConfigDir, filepath.Join(root, "config"))
	require.NoError(t, config.SetAnalyticsEnabled(filepath.Join(root, "hive.yaml"), enabled, nil))
	core, err := New(t.Context(), Config{Settings: settings.DefaultSettings(), MockMode: "feed", Logger: zerolog.Nop()})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, core.Close()) })
	return core, root
}

func TestDesktopAnalyticsCompositionAndSharedHistory(t *testing.T) {
	core, root := newAnalyticsApp(t, true)
	require.Same(t, core.usageAnalytics, core.Terminals.analytics)
	remote := filepath.Join(root, "repository.git")
	require.NoError(t, exec.CommandContext(t.Context(), "git", "init", "--bare", remote).Run())
	_, err := core.hive.Sessions().CreateSession(t.Context(), sessionsvc.CreateOptions{Name: "desktop", Remote: remote, SkipSpawn: true})
	require.NoError(t, err)
	require.NoError(t, core.ReloadHiveRuntime(t.Context()))
	_, err = core.hive.Sessions().CreateSession(t.Context(), sessionsvc.CreateOptions{Name: "after reload", Remote: remote, SkipSpawn: true})
	require.NoError(t, err)
	_, err = core.hive.Sessions().CreateSession(t.Context(), sessionsvc.CreateOptions{Name: "failure", Remote: filepath.Join(root, "missing.git"), SkipSpawn: true})
	require.Error(t, err)
	core.usageAnalytics.Close()
	cli := usage.New(t.Context(), zerolog.Nop(), usage.Options{Enabled: true, DataDir: core.hiveDataDir, Surface: "cli"})
	command, _ := domain.LookupCommand("ls")
	cli.Record(t.Context(), domain.CommandCompleted(command, true, 0))
	cli.Close()
	summary, err := core.Analytics.Summary(t.Context())
	require.NoError(t, err)
	require.EqualValues(t, 2, summary.Counts.HiveSessions)
	require.EqualValues(t, 1, summary.Counts.CLICommands)
}

func TestAnalyticsSettingsRestartDisabledReadAndClear(t *testing.T) {
	core, _ := newAnalyticsApp(t, false)
	cli := usage.New(t.Context(), zerolog.Nop(), usage.Options{Enabled: true, DataDir: core.hiveDataDir, Surface: "cli"})
	cli.Record(t.Context(), domain.TerminalStarted(true))
	cli.Close()
	summary, err := core.Analytics.Summary(t.Context())
	require.NoError(t, err)
	require.False(t, summary.Enabled)
	require.False(t, summary.Active)
	require.EqualValues(t, 1, summary.Counts.TerminalStarts)
	require.NoError(t, core.Analytics.SetEnabled(t.Context(), true))
	summary, err = core.Analytics.Summary(t.Context())
	require.NoError(t, err)
	require.True(t, summary.Enabled)
	require.True(t, summary.RestartNeeded)
	require.False(t, summary.EffectiveEnabled)
	core.Analytics.environmentDisabled = true
	summary, err = core.Analytics.Summary(t.Context())
	require.NoError(t, err)
	require.True(t, summary.EnvironmentOverride)
	require.False(t, summary.RestartNeeded)
	require.NoError(t, core.Analytics.Clear(t.Context()))
	summary, err = core.Analytics.Summary(t.Context())
	require.NoError(t, err)
	require.Zero(t, summary.Counts.TerminalStarts)
}

func TestClearRejectsQueuedEventsFromThisAndOtherProcesses(t *testing.T) {
	core, _ := newAnalyticsApp(t, true)
	cli := usage.New(t.Context(), zerolog.Nop(), usage.Options{Enabled: true, DataDir: core.hiveDataDir, Surface: "cli"})
	core.usageAnalytics.Record(t.Context(), domain.SessionCreated("full", false))
	cli.Record(t.Context(), domain.SessionCreated("full", false))
	require.NoError(t, core.Analytics.Clear(t.Context()))
	core.usageAnalytics.Close()
	cli.Close()
	summary, err := core.Analytics.Summary(t.Context())
	require.NoError(t, err)
	require.Zero(t, summary.Counts.HiveSessions)
}

func TestMissingHistoryClearCreatesNothing(t *testing.T) {
	core, _ := newAnalyticsApp(t, false)
	require.NoError(t, core.Analytics.Clear(t.Context()))
	_, err := os.Stat(filepath.Join(core.hiveDataDir, "usage-analytics.db"))
	require.True(t, os.IsNotExist(err))
}

type usageCapture struct{ events []domain.Event }

func (c *usageCapture) Record(_ context.Context, e domain.Event) { c.events = append(c.events, e) }
