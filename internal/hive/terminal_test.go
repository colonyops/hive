package hive

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/terminal/status"
	tmuxstatus "github.com/colonyops/hive/internal/platform/tmux/status"
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

func TestStatusOptionsFromConfig_NoTerminalSectionMatchesDefaultOptions(t *testing.T) {
	cfg, err := config.Load("", t.TempDir())
	require.NoError(t, err)

	got := StatusOptionsFromConfig(cfg.Terminal.Status, cfg.Tmux.PollInterval)
	assert.Equal(t, status.DefaultOptions(), got)
}

func TestDefaultMissingToleranceMatchesConfigDefault(t *testing.T) {
	// Config-less constructions (tmuxstatus.New, test seams) fall back to
	// tmuxstatus.DefaultMissingTolerance while production reads the config
	// default.
	cfg, err := config.Load("", t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, tmuxstatus.DefaultMissingTolerance, cfg.Terminal.Status.Confirm.Missing.Polls)
}

func TestStatusOptionsFromConfig_YAMLRoundTrip(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	yaml := "terminal:\n" +
		"  status:\n" +
		"    confirm:\n" +
		"      idle:     { polls: 3, min_duration: 5s, stable_content: false }\n" +
		"      approval: { polls: 2 }\n"
	require.NoError(t, os.WriteFile(configPath, []byte(yaml), 0o600))

	cfg, err := config.Load(configPath, t.TempDir())
	require.NoError(t, err)

	got := StatusOptionsFromConfig(cfg.Terminal.Status, cfg.Tmux.PollInterval)

	assert.Equal(t, status.ConfirmPolicy{Polls: 3, MinDuration: 5 * time.Second, StableContent: false}, got.ConfirmIdle)
	assert.Equal(t, status.ConfirmPolicy{Polls: 2}, got.ConfirmApproval)
	assert.Equal(t, cfg.Tmux.PollInterval, got.PollInterval)
}
