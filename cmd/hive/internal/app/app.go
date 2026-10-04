// Package app is the hive CLI's composition root: the engine services plus the
// pieces only the CLI has (plugins, the command set, sources, its config).
package app

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/hive/internal/config"
	"github.com/colonyops/hive/cmd/hive/internal/plugins"
	"github.com/colonyops/hive/cmd/hive/internal/sources"
	"github.com/colonyops/hive/internal/domain/kv"
	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/internal/domain/terminal"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/doctor"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/store"
)

// BuildInfo holds build-time metadata set by the main package.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// Multiplexer exposes the shared operations used by application commands.
type Multiplexer interface {
	sessionsvc.Multiplexer
	terminal.PaneSource
	ResolveTarget(ctx context.Context, raw string) (multiplexer.Pane, error)
	SendLiteral(ctx context.Context, target multiplexer.Target, text string) error
	SendKey(ctx context.Context, target multiplexer.Target, key multiplexer.NamedKey) error
	Paste(ctx context.Context, target multiplexer.Target, text []byte, opts multiplexer.PasteOptions) error
}

// App is the central entry point for all hive operations. It embeds the
// engine, whose accessors (Sessions, Messages, HC, ...) commands and the TUI
// call per use, and adds what only the CLI has.
type App struct {
	*hive.Engine

	// Logger carries no cmp label. A command or component adds its own.
	Logger zerolog.Logger

	// Config is the CLI config. It shadows Engine.Config, and its embedded
	// engine half is the config the engine was built from.
	Config      *config.Config
	Doctor      *doctor.Service
	Multiplexer Multiplexer
	Plugins     *plugins.Manager
	CommandSet  *plugins.CommandSet
	KV          kv.KV
	Build       BuildInfo
	Sources     *sources.Registry
}

// NewApp wraps an engine built from &cfg.Config with the CLI pieces. Doctor
// validates the whole CLI config, so `hive doctor` keeps reporting errors in
// sections only the CLI reads.
func NewApp(
	logger zerolog.Logger,
	engine *hive.Engine,
	cfg *config.Config,
	multiplexer Multiplexer,
	pluginMgr *plugins.Manager,
	commandSet *plugins.CommandSet,
	kvStore kv.KV,
	pluginInfos []doctor.PluginInfo,
) *App {
	return &App{
		Engine:      engine,
		Logger:      logger,
		Config:      cfg,
		Doctor:      doctor.NewService(store.NewSessionStore(engine.DB()), &cfg.Config, cfg, pluginInfos),
		Multiplexer: multiplexer,
		Plugins:     pluginMgr,
		CommandSet:  commandSet,
		KV:          kvStore,
	}
}
