package settings

import (
	"bytes"
	"encoding/json"
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
	cliLogger, closeCLI, err := logutils.NewRoot(logutils.Options{
		Service: logutils.ServiceNameCLI,
		Level:   zerolog.InfoLevel,
		File:    path,
	})
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

func TestNewLoggerKeepsTelemetryArmWhenFileUnavailable(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(blocker, nil, 0o600))

	var telemetry bytes.Buffer
	logger, closeLogger, err := NewLogger(logutils.ServiceNameDesktop, filepath.Join(blocker, "hive.log"), zerolog.InfoLevel, &telemetry)
	require.Error(t, err)
	defer closeLogger()

	logger.Info().Msg("still running")

	var event map[string]string
	require.NoError(t, json.Unmarshal(telemetry.Bytes(), &event))
	assert.Equal(t, logutils.ServiceNameDesktop, event[logutils.ServiceNameKey])
	assert.Equal(t, "still running", event["message"])
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
