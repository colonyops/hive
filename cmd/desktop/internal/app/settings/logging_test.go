package settings

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/pkg/logutils"
)

func TestCLIAndDesktopLoggersAppendToSameFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hive.log")
	desktopLogger, closeDesktop, err := NewLogger(logutils.ServiceNameDesktop, path, zerolog.InfoLevel)
	require.NoError(t, err)
	cliLogger, closeCLI, err := logutils.New(logutils.ServiceNameCLI, "info", path)
	require.NoError(t, err)

	desktopLogger.Info().Msg("desktop started")
	cliLogger.Info().Msg("cli started")
	closeDesktop()
	closeCLI()

	lines, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(lines), "service_name=hive-desktop")
	assert.Contains(t, string(lines), "service_name=hive-cli")
	assert.Contains(t, string(lines), "desktop started")
	assert.Contains(t, string(lines), "cli started")
}

func TestResolveLogLevel(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		t.Setenv(EnvLogLevel, "")
		level, err := ResolveLogLevel()
		require.NoError(t, err)
		require.Equal(t, zerolog.InfoLevel, level)
	})
	t.Run("valid", func(t *testing.T) {
		t.Setenv(EnvLogLevel, "debug")
		level, err := ResolveLogLevel()
		require.NoError(t, err)
		require.Equal(t, zerolog.DebugLevel, level)
	})
	t.Run("invalid", func(t *testing.T) {
		t.Setenv(EnvLogLevel, "verbose")
		_, err := ResolveLogLevel()
		require.ErrorContains(t, err, EnvLogLevel)
	})
}
