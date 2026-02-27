package hive

import (
	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/doctor"
	"github.com/colonyops/hive/internal/core/eventbus"
	"github.com/colonyops/hive/internal/core/hc"
	"github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/internal/core/messaging"
	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/core/todo"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/hive/plugins"
	"github.com/colonyops/hive/pkg/tmpl"
	"github.com/rs/zerolog"
)

// BuildInfo holds build-time metadata set by the main package.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// App is the central entry point for all hive operations.
// Commands and TUI consume App instead of cherry-picking raw dependencies.
type App struct {
	Sessions *SessionService
	Messages *MessageService
	Context  *ContextService
	Doctor   *DoctorService
	Todos    *TodoService
	HC       *HoneycombService

	Bus      *eventbus.EventBus
	Terminal *terminal.Manager
	Plugins  *plugins.Manager
	Config   *config.Config
	DB       *db.DB
	KV       kv.KV
	Renderer *tmpl.Renderer
	Build    BuildInfo
}

// NewApp constructs an App from explicit dependencies.
func NewApp(
	sessions *SessionService,
	msgStore messaging.Store,
	todoStore todo.Store,
	hcStore hc.Store,
	cfg *config.Config,
	bus *eventbus.EventBus,
	termMgr *terminal.Manager,
	pluginMgr *plugins.Manager,
	database *db.DB,
	kvStore kv.KV,
	renderer *tmpl.Renderer,
	pluginInfos []doctor.PluginInfo,
	logger zerolog.Logger,
) *App {
	msgSvc := NewMessageService(msgStore, cfg, bus)
	return &App{
		Sessions: sessions,
		Messages: msgSvc,
		Context:  NewContextService(cfg, sessions.git),
		Doctor:   NewDoctorService(sessions.sessions, cfg, pluginInfos),
		Todos:    NewTodoService(todoStore, bus, cfg, logger),
		HC:       NewHoneycombService(hcStore, logger),
		Bus:      bus,
		Terminal: termMgr,
		Plugins:  pluginMgr,
		Config:   cfg,
		DB:       database,
		KV:       kvStore,
		Renderer: renderer,
	}
}
