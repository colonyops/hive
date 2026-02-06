// Package neovim provides a Neovim plugin for Hive.
package neovim

import (
	"context"
	"os/exec"

	"github.com/hay-kot/hive/internal/core/config"
	"github.com/hay-kot/hive/internal/plugins"
)

// Plugin implements the neovim plugin for Hive.
type Plugin struct {
	cfg config.NeovimPluginConfig
}

// New creates a new neovim plugin.
func New(cfg config.NeovimPluginConfig) *Plugin {
	return &Plugin{cfg: cfg}
}

func (p *Plugin) Name() string { return "neovim" }

func (p *Plugin) Available() bool {
	// Check if user explicitly disabled
	if p.cfg.Enabled != nil && !*p.cfg.Enabled {
		return false
	}
	// Auto-detect: check if nvim is available
	_, err := exec.LookPath("nvim")
	return err == nil
}

func (p *Plugin) Init(_ context.Context) error { return nil }
func (p *Plugin) Close() error                 { return nil }

func (p *Plugin) Commands() map[string]config.UserCommand {
	return map[string]config.UserCommand{
		"NeovimOpen": {
			Sh:     `tmux new-window -t "{{ .Name }}" -c "{{ .Path }}" nvim`,
			Help:   "open neovim in new window in session's tmux session",
			Silent: true,
		},
	}
}

func (p *Plugin) StatusProvider() plugins.StatusProvider {
	return nil // neovim doesn't provide status
}
