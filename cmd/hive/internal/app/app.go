// Package app is the hive CLI's composition root: the engine services plus the
// pieces only the CLI has (plugins, the command set, sources, its config).
package app

import (
	"context"

	"github.com/colonyops/hive/cmd/hive/internal/config"
	"github.com/colonyops/hive/cmd/hive/internal/plugins"
	"github.com/colonyops/hive/cmd/hive/internal/sources"
	"github.com/colonyops/hive/internal/core/doctor"
	"github.com/colonyops/hive/internal/core/eventbus"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/domain/hc"
	"github.com/colonyops/hive/internal/domain/kv"
	"github.com/colonyops/hive/internal/domain/messaging"
	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/domain/terminal"
	"github.com/colonyops/hive/internal/domain/todo"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/pkg/tmpl"
	"github.com/rs/zerolog"
)

// BuildInfo holds build-time metadata set by the main package.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// Multiplexer exposes the shared operations used by application commands.
type Multiplexer interface {
	hive.SessionMultiplexer
	terminal.PaneSource
	ResolveTarget(ctx context.Context, raw string) (multiplexer.Pane, error)
	SendLiteral(ctx context.Context, target multiplexer.Target, text string) error
	SendKey(ctx context.Context, target multiplexer.Target, key multiplexer.NamedKey) error
	Paste(ctx context.Context, target multiplexer.Target, text []byte, opts multiplexer.PasteOptions) error
}

// App is the central entry point for all hive operations.
// Commands and TUI consume App instead of cherry-picking raw dependencies.
type App struct {
	Sessions  *hive.SessionService
	Messages  *hive.MessageService
	Context   *hive.ContextService
	Doctor    *hive.DoctorService
	Todos     *hive.TodoService
	Honeycomb *hive.HoneycombService
	Status    *hive.StatusService

	Bus         *eventbus.EventBus
	Terminal    *terminal.Manager
	Multiplexer Multiplexer
	Plugins     *plugins.Manager
	CommandSet  *plugins.CommandSet
	Config      *config.Config
	DB          *db.DB
	KV          kv.KV
	Renderer    *tmpl.Renderer
	Build       BuildInfo
	Sources     *sources.Registry
}

// NewApp constructs an App from explicit dependencies. The engine services
// get the engine half of cfg; doctor validates all of it.
func NewApp(
	sessions *hive.SessionService,
	sessionStore session.Store,
	msgStore messaging.Store,
	todoStore todo.Store,
	hcStore hc.Store,
	cfg *config.Config,
	bus *eventbus.EventBus,
	termMgr *terminal.Manager,
	multiplexer Multiplexer,
	pluginMgr *plugins.Manager,
	commandSet *plugins.CommandSet,
	database *db.DB,
	kvStore kv.KV,
	renderer *tmpl.Renderer,
	pluginInfos []doctor.PluginInfo,
	logger zerolog.Logger,
) *App {
	engineCfg := &cfg.Config
	return &App{
		Sessions:    sessions,
		Messages:    hive.NewMessageService(msgStore, engineCfg, bus),
		Context:     hive.NewContextService(engineCfg, sessions.Git()),
		Doctor:      hive.NewDoctorService(sessionStore, engineCfg, cfg, pluginInfos),
		Todos:       hive.NewTodoService(todoStore, bus, engineCfg, logger.With().Str("component", "todos").Logger()),
		Honeycomb:   hive.NewHoneycombService(hcStore, logger.With().Str("component", "honeycomb").Logger()),
		Status:      hive.NewStatusService(termMgr, cfg.Git.StatusWorkers),
		Bus:         bus,
		Terminal:    termMgr,
		Multiplexer: multiplexer,
		Plugins:     pluginMgr,
		CommandSet:  commandSet,
		Config:      cfg,
		DB:          database,
		KV:          kvStore,
		Renderer:    renderer,
	}
}
