// Package tmux provides a tmux plugin for Hive with default session management commands.
package tmux

import (
	"context"
	"os/exec"

	"github.com/hay-kot/hive/internal/core/config"
	"github.com/hay-kot/hive/internal/hive/plugins"
)

// Plugin implements the tmux plugin for Hive.
type Plugin struct {
	cfg config.TmuxPluginConfig
}

// New creates a new tmux plugin.
func New(cfg config.TmuxPluginConfig) *Plugin {
	return &Plugin{cfg: cfg}
}

func (p *Plugin) Name() string { return "tmux" }

func (p *Plugin) Available() bool {
	if p.cfg.Enabled != nil && !*p.cfg.Enabled {
		return false
	}
	_, err := exec.LookPath("tmux")
	return err == nil
}

func (p *Plugin) Init(_ context.Context) error { return nil }
func (p *Plugin) Close() error                 { return nil }

func (p *Plugin) Commands() map[string]config.UserCommand {
	return map[string]config.UserCommand{
		"TmuxOpen": {
			Sh:     `{{ hiveTmux }} {{ .Name | shq }} {{ .Path | shq }} '' {{ .TmuxWindow | shq }}`,
			Help:   "open tmux session",
			Exit:   "$HIVE_POPUP",
			Silent: true,
		},
		"TmuxStart": {
			Sh:     `{{ hiveTmux }} -b {{ .Name | shq }} {{ .Path | shq }}`,
			Help:   "start tmux session (background)",
			Silent: true,
		},
		"TmuxKill": {
			Sh:      `tmux kill-session -t {{ .Name | shq }} 2>/dev/null || true`,
			Help:    "kill tmux session",
			Confirm: "Kill tmux session?",
			Silent:  true,
		},
		"TmuxPopUp": {
			Sh:     `tmux display-popup -E -w 100% -h 100% -t {{ .Name | shq }} -- tmux attach-session -t {{ .Name | shq }}`,
			Help:   "popup tmux session",
			Silent: true,
		},
		"AgentSend": {
			Sh:     `{{ agentSend }} {{ .Name | shq }}:{{ agentWindow }}`,
			Help:   "send enter to agent",
			Silent: true,
		},
		"AgentSendClear": {
			Sh:     `{{ agentSend }} {{ .Name | shq }}:{{ agentWindow }} /clear`,
			Help:   "send /clear to agent",
			Silent: true,
		},
	}
}

func (p *Plugin) StatusProvider() plugins.StatusProvider {
	return nil
}
