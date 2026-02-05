// Package yazi provides a Yazi file manager plugin for Hive.
package yazi

import (
	"context"
	"os/exec"

	"github.com/hay-kot/hive/internal/core/config"
	"github.com/hay-kot/hive/internal/plugins"
)

// Plugin implements the yazi plugin for Hive.
type Plugin struct {
	cfg config.YaziPluginConfig
}

// New creates a new yazi plugin.
func New(cfg config.YaziPluginConfig) *Plugin {
	return &Plugin{cfg: cfg}
}

func (p *Plugin) Name() string { return "yazi" }

func (p *Plugin) Available() bool {
	// Check if user explicitly disabled
	if p.cfg.Enabled != nil && !*p.cfg.Enabled {
		return false
	}
	// Auto-detect: check if yazi is available
	_, err := exec.LookPath("yazi")
	return err == nil
}

func (p *Plugin) Init(_ context.Context) error { return nil }
func (p *Plugin) Close() error                 { return nil }

func (p *Plugin) Commands() map[string]config.UserCommand {
	return map[string]config.UserCommand{
		"YaziOpen": {
			Sh:     `tmux popup -E -w 95% -h 95% -d "{{ .Path }}" yazi`,
			Help:   "open yazi file manager in session directory",
			Silent: true,
		},
	}
}

func (p *Plugin) StatusProvider() plugins.StatusProvider {
	return nil // yazi doesn't provide status
}
