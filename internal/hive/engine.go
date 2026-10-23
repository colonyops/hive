// Package hive is the hive engine both programs run on. Each application
// service lives in its own subpackage; Engine composes them over the drivers a
// program hands it and rebuilds the config-derived ones on Reload.
package hive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"

	"github.com/colonyops/hive/internal/hive/doctor"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/kv"
	"github.com/colonyops/hive/internal/domain/notify"
	"github.com/colonyops/hive/internal/domain/review"
	"github.com/colonyops/hive/internal/domain/terminal"
	drivesvc "github.com/colonyops/hive/internal/hive/drive"
	"github.com/colonyops/hive/internal/hive/driveremote"
	"github.com/colonyops/hive/internal/hive/events"
	"github.com/colonyops/hive/internal/hive/gitstatus"
	hcsvc "github.com/colonyops/hive/internal/hive/hc"
	msgsvc "github.com/colonyops/hive/internal/hive/messaging"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/hive/session/scripts"
	statussvc "github.com/colonyops/hive/internal/hive/status"
	todosvc "github.com/colonyops/hive/internal/hive/todo"
	"github.com/colonyops/hive/internal/platform/credentials"
	"github.com/colonyops/hive/internal/platform/git"
	"github.com/colonyops/hive/internal/store"
	"github.com/colonyops/hive/internal/store/db"
	"github.com/colonyops/hive/pkg/executil"
	"github.com/colonyops/hive/pkg/tmpl"
)

// Ports are the drivers and process-lived handles a program gives the engine.
// None of them is rebuilt on Reload.
type Ports struct {
	// DB is hive.db and Bus the event bus. StartRuntime opens, starts, and
	// closes both; a program never sets them.
	DB       *db.DB
	Bus      *events.EventBus
	Executor executil.Executor
	Mux      sessionsvc.Multiplexer
	// PaneSource feeds terminal status. Nil disables status: Status and
	// Terminal return nil.
	PaneSource terminal.PaneSource
	// DataDir holds the bundled scripts the renderer points spawn commands at.
	DataDir string
	// Styler, Stdout and Stderr shape hook and file-copy output. Nil means
	// plain text to io.Discard.
	Styler sessionsvc.OutputStyler
	Stdout io.Writer
	Stderr io.Writer
	Logger zerolog.Logger
	// Credentials holds the drive server token. Nil disables DriveRemote.
	Credentials credentials.Store
}

// services is everything one config decides. It is never mutated after
// build, so a caller holding one sees a consistent set.
type services struct {
	cfg       *config.Config
	renderer  *tmpl.Renderer
	git       git.Git
	sessions  *sessionsvc.Service
	terminal  *terminal.Manager
	status    *statussvc.Service
	messages  *msgsvc.Service
	todos     *todosvc.Service
	gitStatus *gitstatus.Service
}

// Engine is the hive engine. Accessors return the services built from the
// current config. Fetch a service per call (e.Sessions().X) and do not hold
// it across a Reload, or it keeps serving the old config.
//
// The drive service is the exception: it holds the snapshot of the context
// root, so it is built once and follows a Reload through Reconfigure.
type Engine struct {
	ports         Ports
	hc            *hcsvc.Service
	kv            *store.KVStore
	notifications notify.Store
	reviews       review.Store
	drive         *drivesvc.Service
	driveRemote   *driveremote.Service
	reloadMu      sync.Mutex
	current       atomic.Pointer[services]
}

// OpenDB opens hive.db in dataDir and imports the JSON stores that predate
// it.
func OpenDB(ctx context.Context, dataDir string, opts config.DatabaseConfig) (*db.DB, error) {
	database, err := db.Open(dataDir, db.OpenOptions{
		MaxOpenConns: opts.MaxOpenConns,
		MaxIdleConns: opts.MaxIdleConns,
		BusyTimeout:  opts.BusyTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := store.MigrateFromJSON(ctx, database, dataDir); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("migrate from JSON: %w", err)
	}
	return database, nil
}

func New(cfg *config.Config, p Ports) (*Engine, error) {
	switch {
	case p.DB == nil:
		return nil, errors.New("hive engine: DB is required")
	case p.Bus == nil:
		return nil, errors.New("hive engine: Bus is required")
	case p.Executor == nil:
		return nil, errors.New("hive engine: Executor is required")
	case p.Mux == nil:
		return nil, errors.New("hive engine: Mux is required")
	}
	if p.Styler == nil {
		p.Styler = sessionsvc.PlainStyler{}
	}
	if p.Stdout == nil {
		p.Stdout = io.Discard
	}
	if p.Stderr == nil {
		p.Stderr = io.Discard
	}

	e := &Engine{
		ports:         p,
		hc:            hcsvc.NewService(p.Logger, store.NewHCStore(p.DB)),
		kv:            store.NewKVStore(p.DB),
		notifications: store.NewNotifyStore(p.DB),
		reviews:       store.NewReviewStore(p.DB),
	}
	built, err := e.build(cfg)
	if err != nil {
		return nil, err
	}
	e.current.Store(built)
	e.drive = drivesvc.NewService(p.Logger, cfg, built.git, p.Bus)
	if p.Credentials != nil {
		e.driveRemote = driveremote.New(p.Logger, e.drive, store.NewDriveUploadStore(p.DB), p.Credentials, os.Getenv)
	}
	return e, nil
}

// Reload swaps in services built from cfg and publishes config.reloaded. A
// config that fails validation changes nothing: the running services keep
// serving the config they were built from.
//
// The database, the bus, and the services that read no config (honeycomb,
// KV, notifications, reviews) are kept, so a changed database section or data
// dir still needs a restart. The drive service is kept too and handed the
// new config and git executor; it re-roots itself when the context dir
// moved.
func (e *Engine) Reload(cfg *config.Config) error {
	e.reloadMu.Lock()
	defer e.reloadMu.Unlock()

	built, err := e.build(cfg)
	if err != nil {
		return err
	}
	e.current.Store(built)
	e.drive.Reconfigure(cfg, built.git)
	e.ports.Bus.PublishConfigReloaded(events.ConfigReloadedPayload{Config: cfg})
	return nil
}

// build holds no handle that needs closing: the services spawn per call. So a
// replaced set is dropped, not closed.
func (e *Engine) build(cfg *config.Config) (*services, error) {
	if cfg == nil {
		return nil, errors.New("hive engine: config is required")
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("hive engine: invalid config: %w", err)
	}

	p := e.ports
	profile := cfg.Agents.DefaultProfile()
	renderer := tmpl.New(tmpl.Config{
		ScriptPaths:  scripts.ScriptPaths(p.DataDir),
		AgentCommand: profile.CommandOrDefault(cfg.Agents.Default),
		AgentWindow:  cfg.Agents.Default,
		AgentFlags:   profile.ShellFlags(),
	})
	gitExec := git.NewExecutor(cfg.GitPath, p.Executor)
	set := &services{
		cfg:      cfg,
		renderer: renderer,
		git:      gitExec,
		sessions: sessionsvc.NewService(
			p.Logger, store.NewSessionStore(p.DB), gitExec, cfg, p.Bus, p.Executor, renderer,
			p.Styler, p.Stdout, p.Stderr, p.Mux,
		),
		messages:  msgsvc.NewService(store.NewMessageStore(p.DB, cfg.Messaging.MaxMessages), cfg, p.Bus),
		todos:     todosvc.NewService(p.Logger, store.NewTodoStore(p.Logger, p.DB), p.Bus, cfg),
		gitStatus: gitstatus.NewService(p.Logger, gitExec, cfg.Git.StatusWorkers),
	}
	if p.PaneSource != nil {
		set.terminal = NewTerminalManager(p.Logger, cfg, p.PaneSource)
		set.status = statussvc.NewService(p.Logger, set.terminal, cfg.Git.StatusWorkers)
	}
	return set, nil
}

func (e *Engine) load() *services { return e.current.Load() }

// Config returns the config the current services were built from.
func (e *Engine) Config() *config.Config { return e.load().cfg }

// Renderer returns the template renderer for the current default agent.
func (e *Engine) Renderer() *tmpl.Renderer { return e.load().renderer }

// Git returns the git executor for the current GitPath.
func (e *Engine) Git() git.Git { return e.load().git }

func (e *Engine) Sessions() *sessionsvc.Service { return e.load().sessions }

// Terminal returns the terminal status manager, or nil without a PaneSource.
func (e *Engine) Terminal() *terminal.Manager { return e.load().terminal }

// Status returns the terminal status service, or nil without a PaneSource.
func (e *Engine) Status() *statussvc.Service { return e.load().status }

// HC returns the honeycomb service. It reads no config, so Reload keeps it.
func (e *Engine) HC() *hcsvc.Service { return e.hc }

// KV returns the persistent key-value store. Reload keeps it. The runtime
// runs SweepKV.
func (e *Engine) KV() kv.KV { return e.kv }

// Notifications returns the notification history. Reload keeps it.
func (e *Engine) Notifications() notify.Store { return e.notifications }

// Reviews returns the document review sessions and comments. Reload keeps it.
func (e *Engine) Reviews() review.Store { return e.reviews }

func (e *Engine) Messages() *msgsvc.Service { return e.load().messages }

// Drive returns the Hive Drive service. Reload keeps it and re-roots it.
func (e *Engine) Drive() *drivesvc.Service { return e.drive }

// DriveRemote returns the drive server client service, or nil when the
// program gave the engine no credential store.
func (e *Engine) DriveRemote() *driveremote.Service { return e.driveRemote }

func (e *Engine) Todos() *todosvc.Service { return e.load().todos }

func (e *Engine) GitStatus() *gitstatus.Service { return e.load().gitStatus }

// Doctor returns the health checks for the current config. validator checks
// the program's whole config: a program whose config has sections the engine
// does not read passes its own, so those sections are checked too. plugins is
// what the program wants reported; the engine has none.
func (e *Engine) Doctor(validator doctor.ConfigValidator, plugins []doctor.PluginInfo) *doctor.Service {
	return doctor.NewService(store.NewSessionStore(e.ports.DB), e.load().cfg, validator, plugins)
}

func (e *Engine) Bus() *events.EventBus { return e.ports.Bus }
