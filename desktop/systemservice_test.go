package main

import (
	"path/filepath"
	"testing"

	"github.com/colonyops/hive/internal/desktop"
	"github.com/stretchr/testify/require"
)

func TestSystemServiceInfo(t *testing.T) {
	dataRoot := t.TempDir()
	t.Setenv("HIVE_DATA_DIR", dataRoot)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	info := NewSystemService().Info()

	require.Equal(t, dataRoot, info.DataDir.Path)
	require.Equal(t, desktop.ConfigDir(), info.ConfigDir.Path)
	require.Equal(t, filepath.Join(dataRoot, "desktop", "desktop.log"), info.LogFile.Path)
	require.Equal(t, filepath.Join(dataRoot, "desktop", "desktop-pipeline.db"), info.Database.Path)
	// The data/config directories are on defaults here, so nothing is overridden.
	require.False(t, info.DataDir.Overridden)
	require.False(t, info.ConfigDir.Overridden)
}

func TestSystemServiceInfoReflectsOverride(t *testing.T) {
	t.Setenv("HIVE_DATA_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	require.NoError(t, desktop.SaveBootstrap(desktop.Bootstrap{DataDir: "/somewhere/data"}))

	info := NewSystemService().Info()
	require.True(t, info.DataDir.Overridden)
	require.False(t, info.ConfigDir.Overridden)
}

func TestSystemServiceSetDataDirPersists(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	target := t.TempDir()

	require.NoError(t, NewSystemService().SetDataDir(target))

	b, err := desktop.LoadBootstrap()
	require.NoError(t, err)
	require.Equal(t, filepath.Clean(target), b.DataDir)
}

func TestSystemServiceSetDataDirRejectsRelative(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	require.Error(t, NewSystemService().SetDataDir("relative/path"))
	require.Error(t, NewSystemService().SetDataDir(""))
}

func TestSystemServiceClearConfigDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	require.NoError(t, desktop.SaveBootstrap(desktop.Bootstrap{DataDir: "/d", ConfigDir: "/c"}))

	require.NoError(t, NewSystemService().ClearConfigDir())

	b, err := desktop.LoadBootstrap()
	require.NoError(t, err)
	require.Equal(t, "/d", b.DataDir)
	require.Empty(t, b.ConfigDir)
}

func TestSystemServiceCheckAllowed(t *testing.T) {
	t.Setenv("HIVE_DATA_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s := NewSystemService()

	require.NoError(t, s.checkAllowed(desktop.DataDir()))
	require.NoError(t, s.checkAllowed(desktop.LogFile()))
	require.Error(t, s.checkAllowed("/etc/passwd"))
}
