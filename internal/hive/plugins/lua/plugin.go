package lua

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/internal/hive/plugins"
)

// pluginName identifies this plugin to logs, the hive.plugin Lua bindings,
// and the kv.Scoped namespace shared between production and tests.
const pluginName = "lua"

// Plugin adapts a Lua entry file to Hive's plugin interface.
type Plugin struct {
	cfg     config.LuaPluginConfig
	kvStore kv.KV
	pool    *plugins.WorkerPool
	cmdSet  *plugins.CommandSet
	logger  zerolog.Logger
	runtime *Runtime
	modules []HostModule
}

// New creates a Lua-backed Hive plugin. The shared worker pool throttles
// hive.sh.* shell execution; pass nil only in tests that don't exercise the
// shell module. cmdSet is the shared command registry the plugin pushes
// hive.commands(...) registrations into; tests that don't exercise commands
// may pass plugins.NewCommandSet(nil, nil).
func New(cfg config.LuaPluginConfig, kvStore kv.KV, pool *plugins.WorkerPool, cmdSet *plugins.CommandSet, logger zerolog.Logger) *Plugin {
	return &Plugin{cfg: cfg, kvStore: kvStore, pool: pool, cmdSet: cmdSet, logger: logger}
}

// Name returns the plugin identifier used in logs and the kv namespace.
func (p *Plugin) Name() string { return pluginName }

// Available reports whether the configured Lua entry file exists. Used by
// the Hive plugin manager to skip plugins that aren't installed.
func (p *Plugin) Available() bool {
	info, err := os.Stat(p.cfg.ResolvedEntry())
	return err == nil && !info.IsDir()
}

// Init builds the Lua runtime, loads the entrypoint, and runs it once
// to register commands and other plugin state. Re-initialisation is
// supported: any prior runtime is shut down first so commands from a
// previous Init can't leak into the shared CommandSet's plugin slot.
func (p *Plugin) Init(_ context.Context) error {
	p.shutdown()

	cmdModule := &CommandsModule{
		PluginName: p.Name(),
		Set:        p.cmdSet,
	}

	tickerModule := &TickerModule{
		Logger: p.logger.With().Str("module", "ticker").Logger(),
	}

	shModule := &ShModule{
		Pool:           p.pool,
		DefaultTimeout: cmp.Or(p.cfg.ShellTimeout, 30*time.Second),
		Logger:         p.logger.With().Str("module", "sh").Logger(),
	}

	httpModule := &HTTPModule{
		DefaultTimeout:  cmp.Or(p.cfg.HTTPTimeout, 30*time.Second),
		DefaultMaxBytes: cmp.Or(p.cfg.HTTPMaxBytes, int64(10*1024*1024)),
		Logger:          p.logger.With().Str("module", "http").Logger(),
	}

	modules := []HostModule{
		&LogModule{PluginName: p.Name(), Logger: p.logger},
		&PluginInfoModule{
			Name:       p.Name(),
			Entry:      p.cfg.ResolvedEntry(),
			ModuleRoot: p.cfg.ModuleRoot(),
		},
		cmdModule,
		tickerModule,
		&JSONModule{},
		&KVModule{
			Store:  kv.Scoped[string](p.kvStore, p.Name()),
			Logger: p.logger.With().Str("module", "kv").Logger(),
		},
		shModule,
		httpModule,
	}

	runtime, err := NewRuntime(p.logger, p.cfg.ModuleRoot(), p.cfg.DispatcherQueueSize, modules...)
	if err != nil {
		return err
	}
	// Wired post-construction; Register makes no Runtime calls.
	tickerModule.Runtime = runtime
	shModule.Runtime = runtime
	httpModule.Runtime = runtime

	// Stash now so an entrypoint failure below cleans up via shutdown().
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

	return nil
}

// Close releases the Lua runtime and any resources its host modules hold.
// Safe to call multiple times; safe on a partially-initialised plugin.
func (p *Plugin) Close() error {
	p.shutdown()
	return nil
}

// shutdown stops the runtime and host modules before clearing the plugin's
// slot in the shared CommandSet. Order matters: closing the runtime drains
// any queued hive.commands(...) work, and closing host modules stops tickers
// that could still submit more. Clearing the slot last guarantees no writer
// can repopulate it after the clear. Errors are logged but do not
// short-circuit. Safe on a partial init; idempotent.
func (p *Plugin) shutdown() {
	for i := len(p.modules) - 1; i >= 0; i-- {
		closer, ok := p.modules[i].(HostModuleCloser)
		if !ok {
			continue
		}
		if err := closer.Close(); err != nil {
			p.logger.Warn().
				Err(err).
				Msg("host module close failed")
		}
	}
	p.modules = nil

	if p.runtime != nil {
		p.runtime.Close()
		p.runtime = nil
	}

	if p.cmdSet != nil {
		p.cmdSet.SetPlugin(p.Name(), nil)
	}
}

// Commands returns the user commands registered by the plugin's
// entrypoint. Returns nil if Init has not run or if it failed.
func (p *Plugin) Commands() map[string]config.UserCommand {
	if p.cmdSet == nil {
		return nil
	}
	return p.cmdSet.Plugin(p.Name())
}

// StatusProvider returns nil because Lua plugins don't expose a status
// provider yet. Reserved for a future hook.
func (p *Plugin) StatusProvider() plugins.StatusProvider {
	return nil
}
