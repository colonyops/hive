package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadTerminalConfirmDefaults(t *testing.T) {
	cfg, err := Load("", t.TempDir())
	require.NoError(t, err)

	confirm := cfg.Terminal.Status.Confirm
	assert.Equal(t, 2, confirm.Idle.Polls)
	assert.Equal(t, 2*time.Second, confirm.Idle.MinDuration)
	require.NotNil(t, confirm.Idle.StableContent)
	assert.True(t, *confirm.Idle.StableContent)

	assert.Equal(t, 2, confirm.Missing.Polls)
	assert.Zero(t, confirm.Missing.MinDuration)
	assert.Nil(t, confirm.Missing.StableContent)

	assert.Equal(t, 1, confirm.Approval.Polls)
	assert.Zero(t, confirm.Approval.MinDuration)
	assert.Nil(t, confirm.Approval.StableContent)
}

func TestLoadTerminalConfirmYAML(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	yaml := "terminal:\n" +
		"  status:\n" +
		"    confirm:\n" +
		"      idle:     { polls: 3, min_duration: 4s, stable_content: false }\n" +
		"      missing:  { polls: 5 }\n" +
		"      approval: { polls: 2 }\n"
	require.NoError(t, os.WriteFile(configPath, []byte(yaml), 0o600))

	cfg, err := Load(configPath, t.TempDir())
	require.NoError(t, err)

	confirm := cfg.Terminal.Status.Confirm
	assert.Equal(t, 3, confirm.Idle.Polls)
	assert.Equal(t, 4*time.Second, confirm.Idle.MinDuration)
	require.NotNil(t, confirm.Idle.StableContent)
	assert.False(t, *confirm.Idle.StableContent)

	assert.Equal(t, 5, confirm.Missing.Polls)
	assert.Equal(t, 2, confirm.Approval.Polls)
}

func TestValidate_TerminalConfirmPollsZeroAllowed(t *testing.T) {
	// 0 is applyDefaults' "unset" sentinel (same pattern as MaxRecycled): a
	// config built without going through Load (so defaults never ran, as
	// validConfig deliberately does for every other field too) must still
	// pass Validate.
	cfg := validConfig(t)
	cfg.Terminal.Status.Confirm.Idle.Polls = 0

	assert.NoError(t, cfg.Validate())
}

func TestValidate_TerminalConfirmNegativePollsRejected(t *testing.T) {
	cfg := validConfig(t)
	cfg.Terminal.Status.Confirm.Idle.Polls = -1

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "terminal.status.confirm.idle.polls")
}

func TestValidate_TerminalConfirmNegativeMinDurationRejected(t *testing.T) {
	cfg := validConfig(t)
	cfg.Terminal.Status.Confirm.Approval.MinDuration = -1 * time.Second

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "terminal.status.confirm.approval.min_duration")
}

// TestLoadTerminalConfirmNegativeMinDurationRejected proves the negative
// value survives applyDefaults (its zero-check doesn't match a negative
// number) and is still caught by Validate at the end of the real Load
// pipeline, not just when constructing a Config by hand.
func TestLoadTerminalConfirmNegativeMinDurationRejected(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("terminal:\n  status:\n    confirm:\n      idle: { min_duration: -1s }\n"), 0o600))

	_, err := Load(configPath, t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "min_duration")
}

func TestWarnings_TerminalConfirmSubPollInterval(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	yaml := "tmux:\n  poll_interval: 1.5s\n" +
		"terminal:\n  status:\n    confirm:\n      idle: { min_duration: 1s }\n"
	require.NoError(t, os.WriteFile(configPath, []byte(yaml), 0o600))

	cfg, err := Load(configPath, t.TempDir())
	require.NoError(t, err)

	warnings := cfg.Warnings()
	var found bool
	for _, w := range warnings {
		if w.Item == "terminal.status.confirm.idle.min_duration" {
			found = true
		}
	}
	assert.True(t, found, "a min_duration shorter than tmux.poll_interval should warn")
}

func TestWarnings_TerminalConfirmDefaultsDoNotWarn(t *testing.T) {
	cfg, err := Load("", t.TempDir())
	require.NoError(t, err)

	for _, w := range cfg.Warnings() {
		assert.NotEqual(t, "Terminal", w.Category, "shipped defaults must not trigger the sub-interval warning")
	}
}
