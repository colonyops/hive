package logutils

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type markHook struct{}

func (markHook) Run(e *zerolog.Event, _ zerolog.Level, _ string) { e.Str("hooked", "yes") }

func cliOptions(path string) Options {
	return Options{Service: ServiceNameCLI, Level: zerolog.InfoLevel, File: path}
}

func desktopOptions(path string, console, telemetry *bytes.Buffer) Options {
	return Options{
		Service: ServiceNameDesktop,
		Level:   zerolog.InfoLevel,
		File:    path,
		Console: console,
		JSON:    []io.Writer{telemetry},
		Hooks:   []zerolog.Hook{markHook{}},
	}
}

func TestNewRootCLIAndDesktopShareOneFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "hive.log")
	var console, telemetry bytes.Buffer

	desktop, closeDesktop, err := NewRoot(desktopOptions(path, &console, &telemetry))
	require.NoError(t, err)
	cli, closeCLI, err := NewRoot(cliOptions(path))
	require.NoError(t, err)

	desktopApp := Component(desktop, "app")
	desktopApp.Info().Msg("desktop started")
	cliRoot := Component(cli, "cli")
	cliRoot.Info().Msg("cli started")
	closeDesktop()
	closeDesktop()
	closeCLI()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 2)
	assert.Contains(t, lines[0], "service_name=hive-desktop")
	assert.Contains(t, lines[0], "desktop started")
	assert.Contains(t, lines[0], "hooked=yes")
	assert.Equal(t, 1, strings.Count(lines[0], ComponentKey+"="))
	assert.Contains(t, lines[1], "service_name=hive-cli")
	assert.Contains(t, lines[1], "cli started")
	assert.NotContains(t, lines[1], "hooked")
	assert.Equal(t, 1, strings.Count(lines[1], ComponentKey+"="))
	assert.Contains(t, console.String(), "desktop started")

	var event map[string]string
	require.NoError(t, json.Unmarshal(telemetry.Bytes(), &event))
	assert.Equal(t, ServiceNameDesktop, event[ServiceNameKey])
	assert.Equal(t, "app", event[ComponentKey])
	assert.Equal(t, "yes", event["hooked"])
	assert.Equal(t, "desktop started", event["message"])

	fileInfo, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), fileInfo.Mode().Perm()&^umask(t))
	dirInfo, err := os.Stat(filepath.Dir(path))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), dirInfo.Mode().Perm()&^umask(t))
}

func TestNewRootReturnsFallbackLoggerWhenFileUnavailable(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(blocker, nil, 0o600))
	var console bytes.Buffer

	logger, cleanup, err := NewRoot(Options{
		Service: ServiceNameDesktop,
		Level:   zerolog.InfoLevel,
		File:    filepath.Join(blocker, "hive.log"),
		Console: &console,
	})
	require.ErrorContains(t, err, "create log dir")
	cleanup()

	logger.Info().Msg("fallback")
	assert.Contains(t, console.String(), "fallback")
}

func TestNewRootAppliesLevel(t *testing.T) {
	var out bytes.Buffer
	logger, cleanup, err := NewRoot(Options{Service: ServiceNameCLI, Level: zerolog.WarnLevel, JSON: []io.Writer{&out}})
	require.NoError(t, err)
	defer cleanup()

	logger.Info().Msg("dropped")
	assert.Empty(t, out.String())
	logger.Warn().Msg("kept")
	assert.Contains(t, out.String(), "kept")
}

func TestNewRootWithoutSinksDiscards(t *testing.T) {
	logger, cleanup, err := NewRoot(Options{Service: ServiceNameCLI, Level: zerolog.InfoLevel})
	require.NoError(t, err)
	defer cleanup()
	logger.Info().Msg("nowhere")
}

func umask(t *testing.T) os.FileMode {
	t.Helper()
	probe := filepath.Join(t.TempDir(), "probe")
	require.NoError(t, os.WriteFile(probe, nil, 0o777))
	info, err := os.Stat(probe)
	require.NoError(t, err)
	return 0o777 &^ info.Mode().Perm()
}
