package hive

import (
	"os"
	"testing"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTmuxIntegrationCaptureRecordingDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DataDir = t.TempDir()

	assert.NotNil(t, newTmuxIntegration(&cfg, nil))
	_, err := os.Stat(cfg.TmuxCaptureRecordingsDir())
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestNewTmuxIntegrationCaptureRecordingEnabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DataDir = t.TempDir()
	cfg.Tmux.CaptureRecording.Enabled = true

	assert.NotNil(t, newTmuxIntegration(&cfg, nil))
	info, err := os.Stat(cfg.TmuxCaptureRecordingsDir())
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
}
