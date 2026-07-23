package main

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/internal/commands"
	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/doctor"
	"github.com/colonyops/hive/internal/core/eventbus"
	"github.com/colonyops/hive/internal/core/git"
	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/data/stores"
	"github.com/colonyops/hive/internal/hive"
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
	"github.com/colonyops/hive/pkg/logutils"
	"github.com/colonyops/hive/pkg/tmpl"
)

var (
	// Build information. Populated at build-time via -ldflags flag.
	// When installed via `go install module@version`, init() populates
	// these from runtime/debug.BuildInfo instead.

	version = "dev"
	commit  = "HEAD"
	date    = "now"
)

func build() string {
	v, c, d := resolvedBuildInfo()

	short := c
	if len(c) > 7 {
		short = c[:7]
	}

	return fmt.Sprintf("%s (%s) %s", v, short, d)
}

func resolvedBuildInfo() (string, string, string) {
	v, c, d := version, commit, date

	// When installed via `go install module@version`, ldflags aren't set
	// so version remains "dev". Fall back to runtime/debug.BuildInfo which
	// Go populates automatically with the module version and VCS metadata.
	if v != "dev" {
		return v, c, d
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return v, c, d
	}

	if mv := info.Main.Version; mv != "" && mv != "(devel)" {
		v = mv
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			c = s.Value
		case "vcs.time":
			d = s.Value
		}
	}

	return v, c, d
}

// isShellCompletion reports whether the process was invoked for shell
// completion. It mirrors urfave/cli's own detection: --generate-shell-completion
// must be the last argument with no "--" preceding it. Also matches the
// "completion" subcommand used to generate static completion scripts.
func isShellCompletion(args []string) bool {
	if len(args) < 2 {
		return false
	}

	// Static script generation: `hive completion bash`
	if args[1] == "completion" {
		return true
	}

	// Dynamic completion: last arg is the flag, and no "--" precedes it
	last := args[len(args)-1]
	if last != "--generate-shell-completion" {
		return false
	}
	for _, arg := range args[1 : len(args)-1] {
		if arg == "--" {
			return false
		}
	}
	return true
}

// isInitCommand reports whether the subcommand is "init", scanning past any
// leading flags. hive init must run before any config exists, so Before()
// skips full app initialisation when this returns true.
//
// Only --flag=value style global flags are handled correctly here; space-separated
// --flag value pairs would cause the value to be mistaken for a subcommand. Use
// --flag=value syntax when passing global flags before init.
func isInitCommand(args []string) bool {
	if len(args) == 0 {
		return false
	}
	for _, arg := range args[1:] {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return arg == "init"
	}
	return false
}

func main() {
	ctx := context.Background()

	var (
		logCloser   func()
		hiveApp     = &hive.App{}
		database    *db.DB
		pluginMgr   *plugins.Manager
		sweepCancel context.CancelFunc
		busCancel   context.CancelFunc
		bgWg        sync.WaitGroup // tracks background goroutines for clean shutdown
	)

	flags := &commands.Flags{}

	app := &cli.Command{
		Name:      "hive",
		Usage:     "Manage multiple AI agent sessions",
		UsageText: "hive [global options] command [command options]",
		Description: `Hive creates isolated git environments for running multiple AI agents in parallel.

Instead of managing worktrees manually, hive handles cloning, recycling, and
spawning terminal sessions with your preferred AI tool.

Run 'hive' with no arguments to open the interactive session manager.
Run 'hive new' to create a new session from the current repository.`,
		Version:               build(),
		EnableShellCompletion: true,

		ConfigureShellCompletionCommand: commands.ConfigureCompletionCommand,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "log-level",
				Usage:       "log level (debug, info, warn, error, fatal, panic)",
				Sources:     cli.EnvVars("HIVE_LOG_LEVEL"),
				Value:       "info",
				Destination: &flags.LogLevel,
			},
			&cli.StringFlag{
				Name:        "log-file",
				Usage:       "path to log file (defaults to <data-dir>/hive.log)",
				Sources:     cli.EnvVars("HIVE_LOG_FILE"),
				Destination: &flags.LogFile,
			},
			&cli.StringFlag{
				Name:        "config",
				Aliases:     []string{"c"},
				Usage:       "path to config file",
				Sources:     cli.EnvVars("HIVE_CONFIG"),
				Value:       config.DefaultConfigPath(),
				Destination: &flags.ConfigPath,
			},
			&cli.StringFlag{
				Name:        "data-dir",
				Usage:       "path to data directory",
				Sources:     cli.EnvVars("HIVE_DATA_DIR"),
				Value:       config.DefaultDataDir(),
				Destination: &flags.DataDir,
			},
		},
		Before: func(ctx context.Context, c *cli.Command) (context.Context, error) {
			// Skip heavy initialization during shell completion. The
			// completion handler only needs the command tree (already
			// registered) to suggest subcommands and flags.
			if isShellCompletion(os.Args) {
				return ctx, nil
			}
			if isInitCommand(os.Args) {
				v, c, d := resolvedBuildInfo()
				hiveApp.Build = hive.BuildInfo{Version: v, Commit: c, Date: d}
				return ctx, nil
			}

			// Always log to a file; use explicit path or default to <datadir>/hive.log
			logger, closer, err := logutils.New(flags.LogLevel, flags.ResolvedLogFile())
			if err != nil {
				return ctx, fmt.Errorf("setup logger: %w", err)
			}
			log.Logger = logger
			logCloser = closer

			// Extract bundled scripts (non-fatal on failure)
			if err := scripts.EnsureExtracted(flags.DataDir, version); err != nil {
				log.Warn().Err(err).Msg("failed to extract bundled scripts")
			}

			cfg, err := config.Load(flags.ConfigPath, flags.DataDir)
			if err != nil {
				return ctx, fmt.Errorf("load config: %w", err)
			}

			// Create template renderer
			agentProfile := cfg.Agents.DefaultProfile()
			renderer := tmpl.New(tmpl.Config{
				ScriptPaths:  scripts.ScriptPaths(flags.DataDir),
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
			database, err = db.Open(cfg.DataDir, dbOpts)
			if err != nil {
				return ctx, fmt.Errorf("open database: %w", err)
			}

			// Migrate from JSON files if they exist
			if err := stores.MigrateFromJSON(ctx, database, cfg.DataDir); err != nil {
				return ctx, fmt.Errorf("migrate from JSON: %w", err)
			}

			// Create stores
			sessionStore := stores.NewSessionStore(database)
			msgStore := stores.NewMessageStore(database, 0) // 0 = unlimited retention
			kvStore := stores.NewKVStore(database)
			todoStore := stores.NewTodoStore(database)
			hcStore := stores.NewHCStore(database)

			// Start background KV sweep goroutine
			sweepCtx, cancel := context.WithCancel(context.Background())
			sweepCancel = cancel
			bgWg.Go(func() {
				sweep.Start(sweepCtx, kvStore, 5*time.Minute)
			})

			bus := eventbus.New(64)
			busCtx, cancel := context.WithCancel(context.Background())
			busCancel = cancel
			bgWg.Go(func() {
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

			sessionSvc := hive.NewSessionService(sessionStore, gitExec, cfg, bus, exec, renderer, svcLogger, os.Stdout, os.Stderr)

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
				{plugin: claude.New(cfg.Plugins.Claude), disabled: isDisabled(cfg.Plugins.Claude.Enabled)},
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

			pluginMgr = plugins.NewManager(shellPool, commandSet)
			for _, candidate := range allPlugins {
				pluginMgr.Register(candidate.plugin)
			}

			// Initialize plugins (errors are logged but don't stop startup)
			if err := pluginMgr.InitAll(ctx); err != nil {
				log.Warn().Err(err).Msg("plugin initialization error")
			}

			// Populate the pre-allocated App struct (commands already hold a pointer to it)
			*hiveApp = *hive.NewApp(
				sessionSvc,
				msgStore,
				todoStore,
				hcStore,
				cfg,
				bus,
				nil, // terminal manager created in TUI command
				pluginMgr,
				commandSet,
				database,
				kvStore,
				renderer,
				pluginInfos,
				svcLogger,
			)
			resolvedVersion, resolvedCommit, resolvedDate := resolvedBuildInfo()
			hiveApp.Build = hive.BuildInfo{
				Version: resolvedVersion,
				Commit:  resolvedCommit,
				Date:    resolvedDate,
			}
			hiveApp.Sources = hive.BuildSourceRegistry(cfg, exec, kvStore, svcLogger)

			return ctx, nil
		},
		After: func(ctx context.Context, c *cli.Command) error {
			if busCancel != nil {
				busCancel()
			}

			// Stop background sweep
			if sweepCancel != nil {
				sweepCancel()
			}

			// Close plugins
			if pluginMgr != nil {
				pluginMgr.CloseAll()
			}

			// Close database connection
			if database != nil {
				if err := database.Close(); err != nil {
					log.Error().Err(err).Msg("failed to close database")
					return err
				}
			}

			// Wait for background goroutines to finish before closing the
			// log file so they don't write to a closed file descriptor.
			bgWg.Wait()

			// Close log file
			if logCloser != nil {
				logCloser()
			}
			return nil
		},
	}

	tuiCmd := commands.NewTuiCmd(flags, hiveApp)

	app = commands.NewNewCmd(flags, hiveApp).Register(app)
	app = commands.NewPruneCmd(flags, hiveApp).Register(app)
	app = commands.NewDoctorCmd(flags, hiveApp).Register(app)
	app = commands.NewBatchCmd(flags, hiveApp).Register(app)
	app = commands.NewCtxCmd(flags, hiveApp).Register(app)
	app = commands.NewMsgCmd(flags, hiveApp).Register(app)
	app = commands.NewDocCmd(flags, hiveApp).Register(app)
	app = commands.NewSessionCmd(flags, hiveApp).Register(app)
	app = commands.NewReviewCmd(flags, hiveApp).Register(app)
	app = commands.NewTodoCmd(flags, hiveApp).Register(app)
	app = commands.NewConfigCmd(flags, hiveApp).Register(app)
	app = commands.NewDetectCmd(flags, hiveApp).Register(app)
	app = commands.NewHoneycombCmd(flags, hiveApp).Register(app)
	app = commands.NewWorkspaceCmd(flags, hiveApp).Register(app)
	app = commands.NewInitCmd(flags, hiveApp).Register(app)
	app = commands.NewExperimentalCmd(flags, hiveApp).Register(app)

	// Register TUI flags on root command
	app.Flags = append(app.Flags, tuiCmd.Flags()...)

	// Set TUI as default action when no subcommand is provided
	app.Action = func(ctx context.Context, c *cli.Command) error {
		if c.Args().Len() > 0 {
			return fmt.Errorf("unknown command %q. Run 'hive --help' for usage", c.Args().First())
		}
		return tuiCmd.Run(ctx, c)
	}

	exitCode := 0
	runErr := app.Run(ctx, os.Args)
	if runErr != nil {
		fmt.Println()
		fmt.Println(runErr.Error())
		exitCode = 1
	}

	os.Exit(exitCode)
}
