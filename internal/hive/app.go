package hive

import (
	"github.com/hay-kot/hive/internal/core/config"
	"github.com/hay-kot/hive/internal/core/messaging"
	"github.com/hay-kot/hive/internal/core/terminal"
	"github.com/hay-kot/hive/internal/data/db"
	"github.com/hay-kot/hive/internal/hive/plugins"
	"github.com/hay-kot/hive/pkg/tmpl"
)

// App is the central entry point for all hive operations.
// Commands and TUI consume App instead of cherry-picking raw dependencies.
type App struct {
	Sessions *SessionService
	Messages *MessageService
	Context  *ContextService
	Doctor   *DoctorService

	Terminal *terminal.Manager
	Plugins  *plugins.Manager
	Config   *config.Config
	DB       *db.DB
	Renderer *tmpl.Renderer
}

// NewApp constructs an App from explicit dependencies.
func NewApp(
	sessions *SessionService,
	msgStore messaging.Store,
	cfg *config.Config,
	termMgr *terminal.Manager,
	pluginMgr *plugins.Manager,
	database *db.DB,
	renderer *tmpl.Renderer,
) *App {
	return &App{
		Sessions: sessions,
		Messages: NewMessageService(msgStore, cfg),
		Context:  NewContextService(cfg, sessions.git),
		Doctor:   NewDoctorService(sessions.sessions, cfg),
		Terminal: termMgr,
		Plugins:  pluginMgr,
		Config:   cfg,
		DB:       database,
		Renderer: renderer,
	}
}
