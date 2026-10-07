package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
	"github.com/stretchr/testify/require"
)

type hiveConfigEnvFunc func(context.Context, string) string

func (f hiveConfigEnvFunc) Getenv(ctx context.Context, name string) string { return f(ctx, name) }

func isolateSystemPaths(t *testing.T) {
	t.Helper()
	t.Setenv(settings.EnvDataDir, "")
	t.Setenv(settings.EnvConfigDir, "")
	t.Setenv(settings.EnvFlowsDir, "")
	t.Setenv(settings.EnvActionsPath, "")
}

func TestSystemServiceInfo(t *testing.T) {
	isolateSystemPaths(t)
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dataRoot := filepath.Join(dataHome, "hive")

	info := newSystemService(systemOptions{}).Info(t.Context())

	require.Equal(t, dataRoot, info.DataDir.Path)
	require.Equal(t, settings.ConfigDir(), info.ConfigDir.Path)
	require.Equal(t, filepath.Join(dataRoot, "hive.log"), info.LogFile.Path)
	require.Equal(t, filepath.Join(dataRoot, "desktop", "desktop-pipeline.db"), info.Database.Path)
	// The data/config directories are on defaults here, so nothing is overridden.
	require.False(t, info.DataDir.Overridden)
	require.False(t, info.ConfigDir.Overridden)
}

func TestSystemServiceInfoReportsHiveConfig(t *testing.T) {
	isolateSystemPaths(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, nil, 0o644))

	info := newSystemService(systemOptions{
		HiveConfig: staticHiveConfig(HiveConfigLocation{Path: path, EnvironmentOverride: true}),
	}).Info(t.Context())

	require.Equal(t, PathInfo{Path: path, Exists: true, Overridden: true}, info.HiveConfig)
}

func TestSystemServiceInfoReflectsOverride(t *testing.T) {
	isolateSystemPaths(t)
	t.Setenv(settings.EnvDataDir, t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	require.NoError(t, settings.SaveBootstrap(settings.Bootstrap{DataDir: "/somewhere/data"}))

	info := newSystemService(systemOptions{}).Info(t.Context())
	require.True(t, info.DataDir.Overridden)
	require.False(t, info.ConfigDir.Overridden)
}

func TestSystemServiceSetDataDirPersists(t *testing.T) {
	isolateSystemPaths(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	target := t.TempDir()

	require.NoError(t, newSystemService(systemOptions{}).SetDataDir(t.Context(), target))

	b, err := settings.LoadBootstrap()
	require.NoError(t, err)
	require.Equal(t, filepath.Clean(target), b.DataDir)
}

func TestSystemServiceSetDataDirRejectsRelative(t *testing.T) {
	isolateSystemPaths(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	require.Error(t, newSystemService(systemOptions{}).SetDataDir(t.Context(), "relative/path"))
	require.Error(t, newSystemService(systemOptions{}).SetDataDir(t.Context(), ""))
}

func TestSystemServiceClearConfigDir(t *testing.T) {
	isolateSystemPaths(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	require.NoError(t, settings.SaveBootstrap(settings.Bootstrap{DataDir: "/d", ConfigDir: "/c"}))

	require.NoError(t, newSystemService(systemOptions{}).ClearConfigDir(t.Context()))

	b, err := settings.LoadBootstrap()
	require.NoError(t, err)
	require.Equal(t, "/d", b.DataDir)
	require.Empty(t, b.ConfigDir)
}

func TestSystemServiceCheckAllowed(t *testing.T) {
	isolateSystemPaths(t)
	t.Setenv(settings.EnvDataDir, t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	hiveConfig := filepath.Join(t.TempDir(), "config.yaml")
	s := newSystemService(systemOptions{HiveConfig: staticHiveConfig(HiveConfigLocation{Path: hiveConfig})})

	require.NoError(t, s.checkAllowed(settings.DataDir()))
	require.NoError(t, s.checkAllowed(settings.LogFile()))
	// The report dialog shows a saved bundle through OpenPath rather than
	// through a reveal of its own.
	require.NoError(t, s.checkAllowed(settings.ReportsDir()))
	require.NoError(t, s.checkAllowed(hiveConfig))
	require.Error(t, s.checkAllowed("/etc/passwd"))
}

// staticHiveConfig pins the location for a test. Production reads it through a
// function because a reload re-resolves it; a test that never reloads does not
// care which value it gets, only that it is the same one.
func staticHiveConfig(location HiveConfigLocation) func() HiveConfigLocation {
	return func() HiveConfigLocation { return location }
}

func TestResolveHiveConfigLocation(t *testing.T) {
	t.Run("environment override", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "custom.yaml")
		location := resolveHiveConfigLocation(t.Context(), hiveConfigEnvFunc(func(context.Context, string) string { return path }))

		require.Equal(t, HiveConfigLocation{Path: path, EnvironmentOverride: true}, location)
	})

	for _, name := range []string{"config.yaml", "config.yml", "hive.yaml", "hive.yml"} {
		t.Run("discovered "+name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", dir)
			path := filepath.Join(dir, "hive", name)
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, nil, 0o644))

			location := resolveHiveConfigLocation(t.Context(), hiveConfigEnvFunc(func(context.Context, string) string { return "" }))

			require.Equal(t, HiveConfigLocation{Path: path}, location)
		})
	}

	t.Run("discovery order", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)
		for _, name := range []string{"hive.yml", "config.yaml"} {
			path := filepath.Join(dir, "hive", name)
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, nil, 0o644))
		}

		location := resolveHiveConfigLocation(t.Context(), hiveConfigEnvFunc(func(context.Context, string) string { return "" }))

		require.Equal(t, filepath.Join(dir, "hive", "config.yaml"), location.Path)
	})

	t.Run("login shell XDG config home", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "")
		dir := t.TempDir()
		path := filepath.Join(dir, "hive", "config.yml")
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, nil, 0o644))

		location := resolveHiveConfigLocation(t.Context(), hiveConfigEnvFunc(func(_ context.Context, name string) string {
			if name == "XDG_CONFIG_HOME" {
				return dir
			}
			return ""
		}))

		require.Equal(t, HiveConfigLocation{Path: path}, location)
	})

	t.Run("default creation path", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)

		location := resolveHiveConfigLocation(t.Context(), hiveConfigEnvFunc(func(context.Context, string) string { return "" }))

		require.Equal(t, HiveConfigLocation{Path: filepath.Join(dir, "hive", "config.yaml")}, location)
	})
}

func TestResolveHiveDefaultAgent(t *testing.T) {
	t.Run("configured profile", func(t *testing.T) {
		got := resolveHiveDefaultAgent(t.Context(), hiveConfigEnvFunc(func(_ context.Context, name string) string {
			if name == "HIVE_DEFAULT_AGENT" {
				return " pi "
			}
			return ""
		}), "claude", []string{"claude", "pi"})
		require.Equal(t, "pi", got)
	})

	for _, tc := range []struct {
		name      string
		preferred string
	}{
		{name: "unset"},
		{name: "unknown", preferred: "missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveHiveDefaultAgent(t.Context(), hiveConfigEnvFunc(func(context.Context, string) string {
				return tc.preferred
			}), "claude", []string{"claude", "pi"})
			require.Equal(t, "claude", got)
		})
	}
}

func TestSystemServiceOpenHiveConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.yaml")
	var opened string
	s := newSystemService(systemOptions{
		HiveConfig: staticHiveConfig(HiveConfigLocation{Path: path}),
		OpenPath: func(path string) error {
			opened = path
			return nil
		},
	})

	require.NoError(t, s.OpenHiveConfig(t.Context()))
	require.Equal(t, path, opened)
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(contents), "# Hive configuration")

	require.NoError(t, os.WriteFile(path, []byte("agents:\n  default: pi\n"), 0o644))
	require.NoError(t, s.OpenHiveConfig(t.Context()))
	contents, err = os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "agents:\n  default: pi\n", string(contents))
}
