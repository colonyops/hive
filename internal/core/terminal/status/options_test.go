package status_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/terminal/status"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOptionsFromConfig_NoTerminalSectionMatchesDefaultOptions(t *testing.T) {
	cfg, err := config.Load("", t.TempDir())
	require.NoError(t, err)

	got := status.OptionsFromConfig(cfg.Terminal.Status, cfg.Tmux.PollInterval)
	assert.Equal(t, status.DefaultOptions(), got)
}

func TestOptionsFromConfig_YAMLRoundTrip(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	yaml := "terminal:\n" +
		"  status:\n" +
		"    confirm:\n" +
		"      idle:     { polls: 3, min_duration: 5s, stable_content: false }\n" +
		"      approval: { polls: 2 }\n"
	require.NoError(t, os.WriteFile(configPath, []byte(yaml), 0o600))

	cfg, err := config.Load(configPath, t.TempDir())
	require.NoError(t, err)

	got := status.OptionsFromConfig(cfg.Terminal.Status, cfg.Tmux.PollInterval)

	assert.Equal(t, status.ConfirmPolicy{Polls: 3, MinDuration: 5 * time.Second, StableContent: false}, got.ConfirmIdle)
	assert.Equal(t, status.ConfirmPolicy{Polls: 2}, got.ConfirmApproval)
	assert.Equal(t, cfg.Tmux.PollInterval, got.PollInterval)
}
