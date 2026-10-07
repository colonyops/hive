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

func TestNewRootCLIAndDesktopShareOneFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "hive.log")
	var telemetry bytes.Buffer

	desktop, closeDesktop, err := NewRoot(Options{
		Service: ServiceNameDesktop,
		Level:   zerolog.InfoLevel,
		File:    path,
		JSON:    []io.Writer{&telemetry},
	})
	require.NoError(t, err)
	cli, closeCLI, err := NewRoot(Options{Service: ServiceNameCLI, Level: zerolog.InfoLevel, File: path})
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
	assert.Contains(t, lines[1], "service_name=hive-cli")
	for _, line := range lines {
		assert.Equal(t, 1, strings.Count(line, ComponentKey+"="))
	}

	var event map[string]string
	require.NoError(t, json.Unmarshal(telemetry.Bytes(), &event))
	assert.Equal(t, ServiceNameDesktop, event[ServiceNameKey])
	assert.Equal(t, "desktop started", event["message"])
}
