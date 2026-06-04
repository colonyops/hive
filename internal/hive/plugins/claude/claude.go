// Package claude provides Claude Code integration for Hive.
package claude

import (
	"context"
	"os/exec"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/hive/plugins"
)

// Plugin implements Claude Code command integration.
type Plugin struct {
	cfg config.ClaudePluginConfig
}

// New creates a new Claude plugin.
func New(cfg config.ClaudePluginConfig) *Plugin {
	return &Plugin{cfg: cfg}
}

func (p *Plugin) Name() string {
	return "claude"
}

func (p *Plugin) Available() bool {
	if p.cfg.Enabled != nil && !*p.cfg.Enabled {
		return false
	}
	_, err := exec.LookPath("claude")
	return err == nil
}

func (p *Plugin) Init(_ context.Context) error {
	return nil
}

func (p *Plugin) Close() error {
	return nil
}

func (p *Plugin) Commands() map[string]config.UserCommand {
	return map[string]config.UserCommand{
		"ClaudeFork": {
			Sh: `
# Fork current Claude session in new tmux window and focus it
cd "{{ .Path }}" && \
window_name="{{ .Name }}-fork" && \
tmux new-window -n "$window_name" -c "{{ .Path }}" \
  "exec claude --fork-session" && \
tmux select-window -t "$window_name"
`,
			Help:   "fork Claude session in new window",
			Silent: true,
			Scope:  []string{"sessions"},
		},
	}
}

func (p *Plugin) StatusProvider() plugins.StatusProvider {
	return nil
}
