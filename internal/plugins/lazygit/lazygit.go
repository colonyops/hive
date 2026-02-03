// Package lazygit provides a lazygit plugin for Hive.
package lazygit

import (
	"context"
	"os/exec"

	"github.com/hay-kot/hive/internal/core/config"
	"github.com/hay-kot/hive/internal/plugins"
)

// Plugin implements the lazygit plugin for Hive.
type Plugin struct {
	cfg config.LazyGitPluginConfig
}

// New creates a new lazygit plugin.
func New(cfg config.LazyGitPluginConfig) *Plugin {
	return &Plugin{cfg: cfg}
}

func (p *Plugin) Name() string { return "lazygit" }

func (p *Plugin) Available() bool {
	// Check if user explicitly disabled
	if p.cfg.Enabled != nil && !*p.cfg.Enabled {
		return false
	}
	// Auto-detect: check if lazygit is available
	_, err := exec.LookPath("lazygit")
	return err == nil
}

func (p *Plugin) Init(_ context.Context) error { return nil }
func (p *Plugin) Close() error                 { return nil }

func (p *Plugin) Commands() map[string]config.UserCommand {
	return map[string]config.UserCommand{
		"LazyGitOpen": {
			Sh:     `tmux popup -E -w 100% -h 100% -- sh -c 'cd "{{ .Path }}" && lazygit'`,
			Help:   "open lazygit in session directory",
			Silent: true,
		},
		"LazyGitCommits": {
			Sh:     `tmux popup -E -w 100% -h 100% -- sh -c 'cd "{{ .Path }}" && lazygit log'`,
			Help:   "open lazygit commit log",
			Silent: true,
		},
	}
}

func (p *Plugin) StatusProvider() plugins.StatusProvider {
	return nil // lazygit doesn't provide status
}
