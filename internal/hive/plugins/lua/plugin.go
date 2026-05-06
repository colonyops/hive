package lua

import (
	"context"
	"fmt"
	"os"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/hive/plugins"
)

// Plugin adapts a Lua entry file to Hive's plugin interface.
type Plugin struct {
	cfg      config.LuaPluginConfig
	runtime  *Runtime
	commands map[string]config.UserCommand
}

// New creates a Lua-backed Hive plugin.
func New(cfg config.LuaPluginConfig) *Plugin {
	return &Plugin{cfg: cfg}
}

func (p *Plugin) Name() string { return "lua" }

func (p *Plugin) Available() bool {
	info, err := os.Stat(p.cfg.ResolvedEntry())
	return err == nil && !info.IsDir()
}

func (p *Plugin) Init(_ context.Context) error {
	if p.runtime != nil {
		p.runtime.Close()
		p.runtime = nil
	}
	p.commands = nil

	// Build into a fresh CommandsModule so a partial init (failure during
	// entry-file load or while calling the entrypoint) cannot leave stale
	// commands reachable from MergedCommands.
	cmdModule := &CommandsModule{}
	modules := []HostModule{
		&LogModule{PluginName: p.Name()},
		&PluginInfoModule{
			Name:       p.Name(),
			Entry:      p.cfg.ResolvedEntry(),
			ModuleRoot: p.cfg.ModuleRoot(),
		},
		cmdModule,
	}

	runtime, err := NewRuntime(p.cfg.ModuleRoot(), modules...)
	if err != nil {
		return err
	}

	entrypoint, err := runtime.LoadEntrypoint(p.cfg.ResolvedEntry())
	if err != nil {
		runtime.Close()
		return err
	}

	if err := runtime.CallEntrypoint(entrypoint); err != nil {
		runtime.Close()
		return fmt.Errorf("initialize lua plugin %q: %w", p.cfg.ResolvedEntry(), err)
	}

	p.runtime = runtime
	p.commands = cmdModule.Commands()
	return nil
}

func (p *Plugin) Close() error {
	if p.runtime != nil {
		p.runtime.Close()
		p.runtime = nil
	}
	p.commands = nil
	return nil
}

func (p *Plugin) Commands() map[string]config.UserCommand {
	return p.commands
}

func (p *Plugin) StatusProvider() plugins.StatusProvider {
	return nil
}
