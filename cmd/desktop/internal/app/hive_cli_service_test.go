package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/hivecli"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

type hiveCLIFixture struct {
	svc       *HiveCLIService
	home      string
	exe       string
	link      string
	shellPath string
}

func newHiveCLIFixture(t *testing.T, version string) *hiveCLIFixture {
	t.Helper()
	root := t.TempDir()
	f := &hiveCLIFixture{
		home: filepath.Join(root, "home"),
		exe:  filepath.Join(root, "Applications", "Hive.app", "Contents", "MacOS", "hive-desktop"),
	}
	f.link = hivecli.LinkPath(f.home)
	f.shellPath = filepath.Dir(f.link)
	require.NoError(t, os.MkdirAll(filepath.Dir(f.exe), 0o755))
	require.NoError(t, os.WriteFile(f.exe, []byte("#!/bin/sh\n"), 0o755))
	f.svc = newHiveCLIService(hiveCLIOptions{
		Store:      settings.NewStore(filepath.Join(root, "settings.yaml")),
		Home:       func() (string, error) { return f.home, nil },
		Executable: func() (string, error) { return f.exe, nil },
		Version:    version,
		ShellPath:  func(context.Context) string { return f.shellPath },
		CommandVersion: func(_ context.Context, path string) (string, error) {
			return "hive version " + version + " (abc1234) now\n", nil
		},
		Logger: zerolog.Nop(),
	})
	return f
}

func (f *hiveCLIFixture) resolvedExe(t *testing.T) string {
	t.Helper()
	exe, err := filepath.EvalSymlinks(f.exe)
	require.NoError(t, err)
	return exe
}

func TestHiveCLIUnaskedInstallIsLeftAlone(t *testing.T) {
	f := newHiveCLIFixture(t, "0.61.0")

	require.NoError(t, f.svc.Sync(t.Context()))

	status, err := f.svc.Status(t.Context())
	require.NoError(t, err)
	assert.False(t, status.Asked)
	assert.False(t, status.Link.Exists, "an install that was never asked gets no command")
}

func TestHiveCLISetInstallLinksAndUnlinks(t *testing.T) {
	f := newHiveCLIFixture(t, "0.61.0")

	status, err := f.svc.SetInstall(t.Context(), true)
	require.NoError(t, err)
	assert.True(t, status.Asked)
	assert.True(t, status.Enabled)
	assert.True(t, status.Link.AppOwned)
	assert.Equal(t, f.resolvedExe(t), status.Link.Target)
	assert.True(t, status.LinkDirOnPath)
	assert.Equal(t, f.link, status.Resolved)
	assert.False(t, status.Shadowed)
	assert.Equal(t, "0.61.0", status.CommandVersion)
	assert.False(t, status.VersionsDiffer)

	status, err = f.svc.SetInstall(t.Context(), false)
	require.NoError(t, err)
	assert.True(t, status.Asked)
	assert.False(t, status.Enabled)
	assert.False(t, status.Link.Exists)
}

func TestHiveCLIReportsAForeignCommandWithoutTouchingIt(t *testing.T) {
	f := newHiveCLIFixture(t, "0.61.0")
	require.NoError(t, os.MkdirAll(filepath.Dir(f.link), 0o755))
	require.NoError(t, os.WriteFile(f.link, []byte("brew"), 0o755))

	status, err := f.svc.SetInstall(t.Context(), true)
	require.NoError(t, err)
	assert.True(t, status.Conflict)
	body, err := os.ReadFile(f.link)
	require.NoError(t, err)
	assert.Equal(t, "brew", string(body))

	status, err = f.svc.SetInstall(t.Context(), false)
	require.NoError(t, err)
	assert.True(t, status.Link.Exists, "turning it off removes only what the app put there")
}

func TestHiveCLIReportsAShadowingCommand(t *testing.T) {
	f := newHiveCLIFixture(t, "0.61.0")
	brew := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(brew, "hive"), []byte("brew"), 0o755))
	f.shellPath = brew + string(os.PathListSeparator) + filepath.Dir(f.link)
	f.svc.opts.CommandVersion = func(context.Context, string) (string, error) {
		return "hive version v0.58.0 (abc1234) now\n", nil
	}

	status, err := f.svc.SetInstall(t.Context(), true)
	require.NoError(t, err)
	assert.True(t, status.Shadowed)
	assert.Equal(t, filepath.Join(brew, "hive"), status.Resolved)
	assert.Equal(t, "v0.58.0", status.CommandVersion)
	assert.True(t, status.VersionsDiffer)
}

func TestHiveCLIDevBuildNeverTouchesTheLink(t *testing.T) {
	f := newHiveCLIFixture(t, "dev")
	installed := filepath.Join(t.TempDir(), "Hive.app", "Contents", "MacOS", "hive-desktop")
	require.NoError(t, os.MkdirAll(filepath.Dir(f.link), 0o755))
	require.NoError(t, os.Symlink(installed, f.link))

	status, err := f.svc.SetInstall(t.Context(), false)
	require.NoError(t, err)
	assert.NotEmpty(t, status.Unsupported)
	assert.Equal(t, installed, status.Link.Target, "a dev build's settings must not remove the installed app's link")
}

func TestHiveCLIReportsADirectoryMissingFromPath(t *testing.T) {
	f := newHiveCLIFixture(t, "0.61.0")
	f.shellPath = "/usr/bin"

	status, err := f.svc.SetInstall(t.Context(), true)
	require.NoError(t, err)
	assert.False(t, status.LinkDirOnPath)
	assert.True(t, status.Shadowed, "the shell cannot reach the app's command")
}
