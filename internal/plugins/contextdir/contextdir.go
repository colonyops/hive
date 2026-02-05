// Package contextdir provides commands for opening context directories.
package contextdir

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/hay-kot/hive/internal/core/config"
	"github.com/hay-kot/hive/internal/plugins"
)

// Plugin implements the context directory plugin for Hive.
type Plugin struct {
	cfg     config.ContextDirPluginConfig
	dataDir string
}

// New creates a new context directory plugin.
func New(cfg config.ContextDirPluginConfig, dataDir string) *Plugin {
	return &Plugin{
		cfg:     cfg,
		dataDir: dataDir,
	}
}

func (p *Plugin) Name() string { return "contextdir" }

func (p *Plugin) Available() bool {
	// Check if user explicitly disabled
	if p.cfg.Enabled != nil && !*p.cfg.Enabled {
		return false
	}
	// Available on macOS and Linux
	return runtime.GOOS == "darwin" || runtime.GOOS == "linux"
}

func (p *Plugin) Init(_ context.Context) error { return nil }
func (p *Plugin) Close() error                 { return nil }

func (p *Plugin) Commands() map[string]config.UserCommand {
	// Get the context base directory
	contextBase := filepath.Join(p.dataDir, "context")

	var openCmd string
	if runtime.GOOS == "darwin" {
		openCmd = "open"
	} else {
		openCmd = "xdg-open" // Linux
	}

	return map[string]config.UserCommand{
		"ContextOpenSession": {
			Sh:     fmt.Sprintf(`%s "{{ .Path }}/.hive"`, openCmd),
			Help:   "open session context directory",
			Silent: true,
			Exit:   "true",
		},
		"ContextOpenAll": {
			Sh:     fmt.Sprintf(`%s "%s"`, openCmd, contextBase),
			Help:   "open all hive context directories",
			Silent: true,
			Exit:   "true",
		},
	}
}

func (p *Plugin) StatusProvider() plugins.StatusProvider {
	return nil
}
