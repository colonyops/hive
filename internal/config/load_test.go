package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The engine reads the same config.yaml as the CLI, so the CLI's own sections
// must load here without error and leave the engine fields intact.
func TestLoadIgnoresCLISectionsAndKeepsEngineFields(t *testing.T) {
	t.Setenv(EnvDefaultAgent, "")
	t.Setenv(EnvContextBaseDir, "")
	t.Setenv(EnvGitPath, "")
	workspace := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`version: 0.2.7
copy_command: pbcopy
keybindings:
  x: {cmd: Nope}
usercommands:
  open: open {{ .Path }}
tui:
  theme: not-a-theme
views:
  sessions:
    group_by: not-a-mode
plugins:
  shell_workers: 3
sources:
  hosts:
    git.example.com: not-a-backend

git_path: /usr/bin/git
auto_delete_corrupted: false
workspaces: [`+workspace+`]
agents:
  default: codex
  codex:
    command: codex
    flags: [--yolo]
rules:
  - pattern: acme/.*
    agent: codex
    max_recycled: 2
tmux:
  poll_interval: 250ms
messaging:
  topic_prefix: bots
database:
  busy_timeout: 100
todos:
  limiter:
    max_pending: 4
`), 0o644))

	dataDir := t.TempDir()
	cfg, err := Load(path, dataDir)
	require.NoError(t, err)

	assert.Equal(t, dataDir, cfg.DataDir)
	assert.Equal(t, "/usr/bin/git", cfg.GitPath)
	assert.False(t, cfg.AutoDeleteCorrupted)
	assert.Equal(t, []string{workspace}, cfg.Workspaces)
	assert.Equal(t, "codex", cfg.Agents.Default)
	assert.Equal(t, AgentProfile{Command: "codex", Flags: []string{"--yolo"}}, cfg.Agents.Profiles["codex"])
	require.Len(t, cfg.Rules, 1)
	assert.Equal(t, "codex", cfg.Rules[0].Agent)
	assert.Equal(t, 2, cfg.GetMaxRecycled("acme/widgets"))
	assert.Equal(t, 250*time.Millisecond, cfg.Tmux.PollInterval)
	assert.Equal(t, "bots", cfg.Messaging.TopicPrefix)
	assert.Equal(t, 100, cfg.Database.BusyTimeout)
	assert.Equal(t, 2, cfg.Database.MaxOpenConns, "unset values still get defaults")
	assert.Equal(t, 4, cfg.Todos.Limiter.MaxPending)
}
