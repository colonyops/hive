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
	"github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/core/terminal"
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
	LogLevel   string // used when building the child environment; logger init owns it
	Version    string // build version, passed to scripts.EnsureExtracted

	// SkipBootstrap instructs FullBootstrap and MinimalBootstrap to return
	// immediately without initializing anything. Set this in tests that wire
	// App fields directly rather than going through the bootstrap path.
	SkipBootstrap bool
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
	mu          sync.Mutex // guards fullBooted and minBooted
	sweepCancel context.CancelFunc
	busCancel   context.CancelFunc
	bgWg        sync.WaitGroup
	fullBooted  bool
	minBooted   bool
}

// FullBootstrap performs the heavy initialization required by user-facing
// commands: config load, scripts extract, style application, DB open,
// migrations, stores, KV sweep goroutine, event bus, services, plugin
// discovery, plugin init. Idempotent — safe to call from per-command Before
// hooks (subsequent invocations are no-ops).
//
// PRECONDITION: a.Opts must be set by main.go's global Before.
func (a *App) FullBootstrap(ctx context.Context) error {
	a.mu.Lock()
	skip := a.Opts.SkipBootstrap || a.fullBooted
	a.mu.Unlock()
	if skip {
		return nil
	}
	if a.Opts.DataDir == "" {
		return fmt.Errorf("bootstrap: DataDir not configured")
	}

	if err := scripts.EnsureExtracted(a.Opts.DataDir, a.Opts.Version); err != nil {
		log.Warn().Err(err).Msg("failed to extract bundled scripts")
	}

	cfg, err := config.Load(a.Opts.ConfigPath, a.Opts.DataDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	agentProfile := cfg.Agents.DefaultProfile()
	renderer := tmpl.New(tmpl.Config{
		ScriptPaths:  scripts.ScriptPaths(a.Opts.DataDir),
		AgentCommand: agentProfile.CommandOrDefault(cfg.Agents.Default),
		AgentWindow:  cfg.Agents.Default,
		AgentFlags:   agentProfile.ShellFlags(),
	})

	// validation guarantees the theme name is valid; _ discard is safe.
	palette, _ := styles.GetPalette(cfg.TUI.Theme)
	styles.SetTheme(palette)

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

	sessionStore := stores.NewSessionStore(database)
	msgStore := stores.NewMessageStore(database, 0) // 0 = unlimited retention
	kvStore := stores.NewKVStore(database)
	todoStore := stores.NewTodoStore(database)
	hcStore := stores.NewHCStore(database)

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

	var (
		exec      = &executil.RealExecutor{}
		gitExec   = git.NewExecutor(cfg.GitPath, exec)
		svcLogger = log.With().Str("component", "hive").Logger()
	)

	sessionSvc := NewSessionService(sessionStore, gitExec, cfg, bus, exec, renderer, svcLogger, os.Stdout, os.Stderr)

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

	// Errors are logged; plugin failures are non-fatal to startup.
	if err := pluginMgr.InitAll(ctx); err != nil {
		log.Warn().Err(err).Msg("plugin initialization error")
	}

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

	a.mu.Lock()
	a.fullBooted = true
	a.minBooted = true // FullBootstrap is a superset
	a.mu.Unlock()
	return nil
}

// MinimalBootstrap performs the bare minimum required by the detached
// timer-fire child: config load, scripts extract, db.Open, and
// MigrateFromJSON. No plugins, no sweep goroutine, no event bus. Idempotent.
//
// PRECONDITION: a.Opts must be set.
func (a *App) MinimalBootstrap(ctx context.Context) error {
	a.mu.Lock()
	skip := a.Opts.SkipBootstrap || a.minBooted
	a.mu.Unlock()
	if skip {
		return nil
	}
	if a.Opts.DataDir == "" {
		return fmt.Errorf("bootstrap: DataDir not configured")
	}

	// Scripts must be extracted before we look up agentSendPath.
	if err := scripts.EnsureExtracted(a.Opts.DataDir, a.Opts.Version); err != nil {
		log.Warn().Err(err).Msg("failed to extract bundled scripts")
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

	a.mu.Lock()
	a.minBooted = true
	a.mu.Unlock()
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

	// Wait for background goroutines before closing the DB they may be
	// querying, and before closing the log file they may be writing to.
	a.bgWg.Wait()

	var dbErr error
	if a.DB != nil {
		dbErr = a.DB.Close()
		a.DB = nil
	}
	if a.LogCloser != nil {
		a.LogCloser()
		a.LogCloser = nil
	}

	a.mu.Lock()
	a.fullBooted = false
	a.minBooted = false
	a.mu.Unlock()
	return dbErr
}
