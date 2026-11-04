package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
	"github.com/colonyops/hive/internal/platform/execenv"
)

func TestHiveCLIServiceKeepsTheChoiceSeparateFromTheLink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	store := settings.NewStore(filepath.Join(t.TempDir(), "settings.yaml"))
	svc, err := newHiveCLIService(store, execenv.NewResolver(execenv.Options{Shell: "/bin/sh", Probe: nil}), "dev", zerolog.Nop())
	require.NoError(t, err)

	status, err := svc.Status(t.Context())
	require.NoError(t, err)
	assert.Nil(t, status.Install, "never asked")

	status, err = svc.SetInstall(t.Context(), true)
	require.NoError(t, err)
	require.NotNil(t, status.Install)
	assert.True(t, *status.Install)
	assert.Equal(t, "dev", string(status.Command.Skip), "a dev build saves the choice and writes no link")
	_, err = os.Lstat(filepath.Join(home, ".local", "bin", "hive"))
	assert.ErrorIs(t, err, os.ErrNotExist)

	persisted, err := store.Persisted()
	require.NoError(t, err)
	require.NotNil(t, persisted.HiveCLI.InstallCommand)
	assert.True(t, *persisted.HiveCLI.InstallCommand)
}
