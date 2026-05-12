package hive

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/doctor"
	"github.com/colonyops/hive/internal/core/eventbus"
	"github.com/colonyops/hive/internal/core/git"
	"github.com/colonyops/hive/internal/core/hc"
	"github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/internal/core/messaging"
	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/core/todo"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/data/stores"
	"github.com/colonyops/hive/internal/hive/plugins"
	"github.com/colonyops/hive/internal/hive/plugins/claude"
	"github.com/colonyops/hive/internal/hive/plugins/contextdir"
	"github.com/colonyops/hive/internal/hive/plugins/github"
	"github.com/colonyops/hive/internal/hive/plugins/lazygit"
	"github.com/colonyops/hive/internal/hive/plugins/neovim"
	plugintmux "github.com/colonyops/hive/internal/hive/plugins/tmux"
	"github.com/colonyops/hive/internal/hive/scripts"
	"github.com/colonyops/hive/internal/hive/sweep"
	"github.com/colonyops/hive/pkg/executil"
	"github.com/colonyops/hive/pkg/tmpl"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// BuildInfo holds build-time metadata set by the main package.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// BootstrapOptions are the inputs to FullBootstrap / MinimalBootstrap that
// originate from CLI flags. main.go constructs this from *commands.Flags
// after the global Before resolves defaults, then assigns it to App.Opts
// before any per-command Before hook runs.
type BootstrapOptions struct {
	DataDir    string
	ConfigPath string
	LogFile    string // effective log file path (post-default)
	LogLevel   string // already used by logger init; included for completeness
	Version    string // build version, passed to scripts.EnsureExtracted
}

// App is the central entry point for all hive operations.
// Commands and TUI consume App instead of cherry-picking raw dependencies.
type App struct {
	Sessions  *SessionService
	Messages  *MessageService
	Context   *ContextService
	Doctor    *DoctorService
	Todos     *TodoService
	Honeycomb *HoneycombService

	Bus        *eventbus.EventBus
	Terminal   *terminal.Manager
	Plugins    *plugins.Manager
	CommandSet *plugins.CommandSet
	Config     *config.Config
	DB         *db.DB
	KV         kv.KV
	Renderer   *tmpl.Renderer
	Build      BuildInfo

	// Opts holds the resolved bootstrap inputs. main.go's global Before
	// sets these before wiring commands; FullBootstrap / MinimalBootstrap
	// read from them.
	Opts BootstrapOptions

	// LogCloser, if set, is invoked by Shutdown to close the log file.
	LogCloser func()

	// Lifecycle state, owned by FullBootstrap / MinimalBootstrap and
	// consumed by Shutdown.
	sweepCancel context.CancelFunc
	busCancel   context.CancelFunc
	bgWg        sync.WaitGroup
	fullBooted  bool // FullBootstrap is idempotent
	minBooted   bool // MinimalBootstrap is idempotent
}

// NewApp constructs an App from explicit dependencies.
//
// Deprecated for new call sites: prefer FullBootstrap, which builds the
// services in place. NewApp is retained for tests that wire dependencies
// directly.
func NewApp(
	sessions *SessionService,
	msgStore messaging.Store,
	todoStore todo.Store,
	hcStore hc.Store,
	cfg *config.Config,
	bus *eventbus.EventBus,
	termMgr *terminal.Manager,
	pluginMgr *plugins.Manager,
	commandSet *plugins.CommandSet,
	database *db.DB,
	kvStore kv.KV,
	renderer *tmpl.Renderer,
	pluginInfos []doctor.PluginInfo,
	logger zerolog.Logger,
) *App {
	return &App{
		Sessions:   sessions,
		Messages:   NewMessageService(msgStore, cfg, bus),
		Context:    NewContextService(cfg, sessions.git),
		Doctor:     NewDoctorService(sessions.sessions, cfg, pluginInfos),
		Todos:      NewTodoService(todoStore, bus, cfg, logger.With().Str("component", "todos").Logger()),
		Honeycomb:  NewHoneycombService(hcStore, logger.With().Str("component", "honeycomb").Logger()),
		Bus:        bus,
		Terminal:   termMgr,
		Plugins:    pluginMgr,
		CommandSet: commandSet,
		Config:     cfg,
		DB:         database,
		KV:         kvStore,
		Renderer:   renderer,
	}
}

// FullBootstrap performs the heavy initialization required by user-facing
// commands: config load, scripts extract, style application, DB open,
// migrations, stores, KV sweep goroutine, event bus, services, plugin
// discovery, plugin init. Idempotent — safe to call from per-command Before
// hooks (subsequent invocations are no-ops).
//
// PRECONDITION: a.Opts must be set by main.go's global Before.
//
// Unit tests that build *App directly without calling FullBootstrap are
// detected via Opts.DataDir being empty and skipped; in production, the
// global Before always populates Opts with a default DataDir.
func (a *App) FullBootstrap(ctx context.Context) error {
	if a.fullBooted {
		return nil
	}
	if a.Opts.DataDir == "" {
		// Test/uninitialized mode — caller constructed App directly. Do
		// not attempt heavy init.
		return nil
	}

	// Extract bundled scripts (non-fatal on failure)
	if err := scripts.EnsureExtracted(a.Opts.DataDir, a.Opts.Version); err != nil {
		log.Warn().Err(err).Msg("failed to extract bundled scripts")
	}

	cfg, err := config.Load(a.Opts.ConfigPath, a.Opts.DataDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Template renderer
	agentProfile := cfg.Agents.DefaultProfile()
	renderer := tmpl.New(tmpl.Config{
		ScriptPaths:  scripts.ScriptPaths(a.Opts.DataDir),
		AgentCommand: agentProfile.CommandOrDefault(cfg.Agents.Default),
		AgentWindow:  cfg.Agents.Default,
		AgentFlags:   agentProfile.ShellFlags(),
	})

	// Apply configured theme (validation ensures name is valid)
	palette, _ := styles.GetPalette(cfg.TUI.Theme)
	styles.SetTheme(palette)

	// Open database connection
	dbOpts := db.OpenOptions{
		MaxOpenConns: cfg.Database.MaxOpenConns,
		MaxIdleConns: cfg.Database.MaxIdleConns,
		BusyTimeout:  cfg.Database.BusyTimeout,
	}
	database, err := db.Open(cfg.DataDir, dbOpts)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	if err := stores.MigrateFromJSON(ctx, database, cfg.DataDir); err != nil {
		if closeErr := database.Close(); closeErr != nil {
			return fmt.Errorf("migrate from JSON: %w (close also failed: %w)", err, closeErr)
		}
		return fmt.Errorf("migrate from JSON: %w", err)
	}

	// Create stores
	sessionStore := stores.NewSessionStore(database)
	msgStore := stores.NewMessageStore(database, 0) // 0 = unlimited retention
	kvStore := stores.NewKVStore(database)
	todoStore := stores.NewTodoStore(database)
	hcStore := stores.NewHCStore(database)

	// Start background KV sweep goroutine
	sweepCtx, sweepCancel := context.WithCancel(context.Background())
	a.sweepCancel = sweepCancel
	a.bgWg.Go(func() {
		sweep.Start(sweepCtx, kvStore, 5*time.Minute)
	})

	bus := eventbus.New(64)
	busCtx, busCancel := context.WithCancel(context.Background())
	a.busCancel = busCancel
	a.bgWg.Go(func() {
		bus.Start(busCtx)
		log.Debug().Msg("event bus stopped")
	})

	eventbus.RegisterDebugLogger(bus, log.Logger)
	eventbus.NewNotificationRouter(bus).Register()

	// Create service
	var (
		exec      = &executil.RealExecutor{}
		gitExec   = git.NewExecutor(cfg.GitPath, exec)
		svcLogger = log.With().Str("component", "hive").Logger()
	)

	sessionSvc := NewSessionService(sessionStore, gitExec, cfg, bus, exec, renderer, svcLogger, os.Stdout, os.Stderr)

	// Create all plugin instances, collect availability info for doctor,
	// then register with the manager.
	type configuredPlugin struct {
		plugin   plugins.Plugin
		disabled bool
	}

	isDisabled := func(flag *bool) bool {
		return flag != nil && !*flag
	}

	shellPool := plugins.NewWorkerPool(cfg.Plugins.ShellWorkers)
	commandSet := plugins.NewCommandSet(config.DefaultUserCommands(), cfg.UserCommands)

	allPlugins := []configuredPlugin{
		{plugin: github.New(cfg.Plugins.GitHub, kvStore), disabled: isDisabled(cfg.Plugins.GitHub.Enabled)},
		{plugin: lazygit.New(cfg.Plugins.LazyGit), disabled: isDisabled(cfg.Plugins.LazyGit.Enabled)},
		{plugin: neovim.New(cfg.Plugins.Neovim), disabled: isDisabled(cfg.Plugins.Neovim.Enabled)},
		{plugin: contextdir.New(cfg.Plugins.ContextDir, cfg.DataDir), disabled: isDisabled(cfg.Plugins.ContextDir.Enabled)},
		{plugin: claude.New(cfg.Plugins.Claude, kvStore), disabled: isDisabled(cfg.Plugins.Claude.Enabled)},
		{plugin: plugintmux.New(cfg.Plugins.Tmux), disabled: isDisabled(cfg.Plugins.Tmux.Enabled)},
	}

	pluginInfos := make([]doctor.PluginInfo, len(allPlugins))
	for i, candidate := range allPlugins {
		p := candidate.plugin
		pluginInfos[i] = doctor.PluginInfo{
			Name:      p.Name(),
			Available: p.Available(),
			Disabled:  candidate.disabled,
		}
	}

	pluginMgr := plugins.NewManager(shellPool, commandSet)
	for _, candidate := range allPlugins {
		pluginMgr.Register(candidate.plugin)
	}

	// Initialize plugins (errors are logged but don't stop startup)
	if err := pluginMgr.InitAll(ctx); err != nil {
		log.Warn().Err(err).Msg("plugin initialization error")
	}

	// Populate App fields in place. Build is set separately from Opts.Version.
	a.Sessions = sessionSvc
	a.Messages = NewMessageService(msgStore, cfg, bus)
	a.Context = NewContextService(cfg, sessionSvc.git)
	a.Doctor = NewDoctorService(sessionSvc.sessions, cfg, pluginInfos)
	a.Todos = NewTodoService(todoStore, bus, cfg, svcLogger.With().Str("component", "todos").Logger())
	a.Honeycomb = NewHoneycombService(hcStore, svcLogger.With().Str("component", "honeycomb").Logger())
	a.Bus = bus
	a.Plugins = pluginMgr
	a.CommandSet = commandSet
	a.Config = cfg
	a.DB = database
	a.KV = kvStore
	a.Renderer = renderer

	a.fullBooted = true
	a.minBooted = true // FullBootstrap is a superset
	return nil
}

// MinimalBootstrap performs the bare minimum required by the detached
// timer-fire child: config load, db.Open, and MigrateFromJSON. No plugins,
// no script extract, no sweep goroutine, no event bus. Idempotent.
//
// PRECONDITION: a.Opts must be set.
func (a *App) MinimalBootstrap(ctx context.Context) error {
	if a.minBooted {
		return nil
	}
	if a.Opts.DataDir == "" {
		return nil
	}

	cfg, err := config.Load(a.Opts.ConfigPath, a.Opts.DataDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	dbOpts := db.OpenOptions{
		MaxOpenConns: cfg.Database.MaxOpenConns,
		MaxIdleConns: cfg.Database.MaxIdleConns,
		BusyTimeout:  cfg.Database.BusyTimeout,
	}
	database, err := db.Open(cfg.DataDir, dbOpts)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	if err := stores.MigrateFromJSON(ctx, database, cfg.DataDir); err != nil {
		if closeErr := database.Close(); closeErr != nil {
			return fmt.Errorf("migrate from JSON: %w (close also failed: %w)", err, closeErr)
		}
		return fmt.Errorf("migrate from JSON: %w", err)
	}

	a.Config = cfg
	a.DB = database
	a.minBooted = true
	return nil
}

// Shutdown reverses FullBootstrap and MinimalBootstrap. Idempotent.
func (a *App) Shutdown() error {
	if a.busCancel != nil {
		a.busCancel()
		a.busCancel = nil
	}
	if a.sweepCancel != nil {
		a.sweepCancel()
		a.sweepCancel = nil
	}
	if a.Plugins != nil {
		a.Plugins.CloseAll()
	}
	var dbErr error
	if a.DB != nil {
		dbErr = a.DB.Close()
		a.DB = nil
	}
	a.bgWg.Wait()
	if a.LogCloser != nil {
		a.LogCloser()
		a.LogCloser = nil
	}
	a.fullBooted = false
	a.minBooted = false
	return dbErr
}
