// Package hive is the hive engine both programs run on. Each application
// service lives in its own subpackage; Engine composes them over the drivers a
// program hands it and rebuilds the config-derived ones on Reload.
package hive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/terminal"
	"github.com/colonyops/hive/internal/hive/events"
	"github.com/colonyops/hive/internal/hive/gitstatus"
	hcsvc "github.com/colonyops/hive/internal/hive/hc"
	msgsvc "github.com/colonyops/hive/internal/hive/messaging"
	"github.com/colonyops/hive/internal/hive/repocontext"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/hive/session/scripts"
	statussvc "github.com/colonyops/hive/internal/hive/status"
	todosvc "github.com/colonyops/hive/internal/hive/todo"
	"github.com/colonyops/hive/internal/platform/git"
	"github.com/colonyops/hive/internal/store"
	"github.com/colonyops/hive/internal/store/db"
	"github.com/colonyops/hive/pkg/executil"
	"github.com/colonyops/hive/pkg/tmpl"
)

// Ports are the drivers and process-lived handles a program gives the engine.
// None of them is rebuilt on Reload.
type Ports struct {
	// DB is hive.db, opened with OpenDB. The program closes it.
	DB *db.DB
	// Bus is the event bus. The program starts and stops it.
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
	context   *repocontext.Service
	todos     *todosvc.Service
	gitStatus *gitstatus.Service
}

// Engine is the hive engine. Accessors return the services built from the
// current config. Fetch a service per call (e.Sessions().X) and do not hold
// it across a Reload, or it keeps serving the old config.
type Engine struct {
	ports    Ports
	hc       *hcsvc.Service
	reloadMu sync.Mutex
	current  atomic.Pointer[services]
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

// New builds the engine from cfg over the given ports.
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
		ports: p,
		hc:    hcsvc.NewService(store.NewHCStore(p.DB), p.Logger.With().Str("component", "honeycomb").Logger()),
	}
	built, err := e.build(cfg)
	if err != nil {
		return nil, err
	}
	e.current.Store(built)
	return e, nil
}

// Reload swaps in services built from cfg and publishes config.reloaded. A
// config that fails validation changes nothing: the running services keep
// serving the config they were built from.
//
// The database, the bus and the honeycomb service are kept, so a changed
// database section or data dir still needs a restart.
func (e *Engine) Reload(cfg *config.Config) error {
	e.reloadMu.Lock()
	defer e.reloadMu.Unlock()

	built, err := e.build(cfg)
	if err != nil {
		return err
	}
	e.current.Store(built)
	e.ports.Bus.PublishConfigReloaded(events.ConfigReloadedPayload{Config: cfg})
	return nil
}

// build holds no handle that needs closing: the services spawn per call, and
// the capture recorder writes one file per capture. So a replaced set is
// dropped, not closed.
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
			store.NewSessionStore(p.DB), gitExec, cfg, p.Bus, p.Executor, renderer,
			p.Styler, p.Logger, p.Stdout, p.Stderr, p.Mux,
		),
		messages:  msgsvc.NewService(store.NewMessageStore(p.DB, cfg.Messaging.MaxMessages), cfg, p.Bus),
		context:   repocontext.NewService(cfg, gitExec),
		todos:     todosvc.NewService(store.NewTodoStore(p.DB), p.Bus, cfg, p.Logger.With().Str("component", "todos").Logger()),
		gitStatus: gitstatus.NewService(gitExec, cfg.Git.StatusWorkers),
	}
	if p.PaneSource != nil {
		set.terminal = NewTerminalManager(cfg, p.PaneSource)
		set.status = statussvc.NewService(set.terminal, cfg.Git.StatusWorkers)
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

func (e *Engine) Messages() *msgsvc.Service { return e.load().messages }

func (e *Engine) Context() *repocontext.Service { return e.load().context }

func (e *Engine) Todos() *todosvc.Service { return e.load().todos }

func (e *Engine) GitStatus() *gitstatus.Service { return e.load().gitStatus }

// Bus returns the event bus the engine publishes on.
func (e *Engine) Bus() *events.EventBus { return e.ports.Bus }

// DB returns hive.db.
func (e *Engine) DB() *db.DB { return e.ports.DB }
