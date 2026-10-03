package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTmuxCaptureRecordingDefaultsDisabled(t *testing.T) {
	cfg := DefaultConfig()
	assert.False(t, cfg.Tmux.CaptureRecording.Enabled)
}

func TestTmuxCaptureRecordingsDir(t *testing.T) {
	cfg := Config{DataDir: t.TempDir()}
	assert.Equal(t, filepath.Join(cfg.DataDir, "recordings", "tmux"), cfg.TmuxCaptureRecordingsDir())
}

func TestLoadTmuxCaptureRecordingEnabled(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("tmux:\n  capture_recording:\n    enabled: true\n"), 0o600))
	t.Setenv("HIVE_DEFAULT_AGENT", "")

	cfg, err := Load(configPath, t.TempDir())
	require.NoError(t, err)
	assert.True(t, cfg.Tmux.CaptureRecording.Enabled)
}
