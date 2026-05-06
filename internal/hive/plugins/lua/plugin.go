package lua

import (
	"context"
	"fmt"
	"os"

	"github.com/rs/zerolog/log"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/hive/plugins"
)

// Plugin adapts a Lua entry file to Hive's plugin interface.
type Plugin struct {
	cfg      config.LuaPluginConfig
	runtime  *Runtime
	modules  []HostModule
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
	p.shutdown()

	// Build into a fresh CommandsModule so a partial init (failure during
	// entry-file load or while calling the entrypoint) cannot leave stale
	// commands reachable from MergedCommands.
	cmdModule := &CommandsModule{}

	tickerModule := &TickerModule{PluginName: p.Name()}

	modules := []HostModule{
		&LogModule{PluginName: p.Name()},
		&PluginInfoModule{
			Name:       p.Name(),
			Entry:      p.cfg.ResolvedEntry(),
			ModuleRoot: p.cfg.ModuleRoot(),
		},
		cmdModule,
		tickerModule,
	}

	runtime, err := NewRuntime(p.cfg.ModuleRoot(), modules...)
	if err != nil {
		return err
	}
	// Runtime is wired in after construction because NewRuntime is the function that produces it. Register stores no Runtime calls, so this is safe.
	tickerModule.Runtime = runtime

	// Stash modules and runtime now so a failure during LoadEntrypoint or
	// CallEntrypoint can be cleaned up via shutdown(), which closes every
	// HostModuleCloser before tearing down the runtime.
	p.runtime = runtime
	p.modules = modules

	entrypoint, err := runtime.LoadEntrypoint(p.cfg.ResolvedEntry())
	if err != nil {
		p.shutdown()
		return err
	}

	if err := runtime.CallEntrypoint(entrypoint); err != nil {
		p.shutdown()
		return fmt.Errorf("initialize lua plugin %q: %w", p.cfg.ResolvedEntry(), err)
	}

	p.commands = cmdModule.Commands()
	return nil
}

func (p *Plugin) Close() error {
	p.shutdown()
	return nil
}

// shutdown tears down the runtime and every module that owns background
// resources, in reverse-registration order. Module Close errors are logged
// but do not short-circuit the loop — every module gets a chance to release
// its resources before the runtime goes away. Safe to call on a partially
// initialised Plugin and idempotent across repeated calls.
func (p *Plugin) shutdown() {
	for i := len(p.modules) - 1; i >= 0; i-- {
		closer, ok := p.modules[i].(HostModuleCloser)
		if !ok {
			continue
		}
		if err := closer.Close(); err != nil {
			log.Warn().
				Str("plugin", p.Name()).
				Err(err).
				Msg("host module close failed")
		}
	}
	p.modules = nil

	if p.runtime != nil {
		p.runtime.Close()
		p.runtime = nil
	}
	p.commands = nil
}

func (p *Plugin) Commands() map[string]config.UserCommand {
	return p.commands
}

func (p *Plugin) StatusProvider() plugins.StatusProvider {
	return nil
}
