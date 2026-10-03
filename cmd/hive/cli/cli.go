// Package cli is the hive command line program. It is a package, not a main
// package, because two programs run it: cmd/hive and the wrapper at the
// module root.
package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/cmd/hive/internal/app"
	"github.com/colonyops/hive/cmd/hive/internal/commands"
	"github.com/colonyops/hive/cmd/hive/internal/config"
	"github.com/colonyops/hive/cmd/hive/internal/plugins"
	"github.com/colonyops/hive/cmd/hive/internal/plugins/claude"
	"github.com/colonyops/hive/cmd/hive/internal/plugins/contextdir"
	"github.com/colonyops/hive/cmd/hive/internal/plugins/github"
	"github.com/colonyops/hive/cmd/hive/internal/plugins/lazygit"
	"github.com/colonyops/hive/cmd/hive/internal/plugins/neovim"
	plugintmux "github.com/colonyops/hive/cmd/hive/internal/plugins/tmux"
	"github.com/colonyops/hive/cmd/hive/internal/styles"
	"github.com/colonyops/hive/cmd/hive/internal/sweep"
	"github.com/colonyops/hive/cmd/hive/internal/theme"
	hiveconfig "github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/doctor"
	"github.com/colonyops/hive/internal/hive/events"
	"github.com/colonyops/hive/internal/hive/session/scripts"
	tmuxexec "github.com/colonyops/hive/internal/platform/tmux/exec"
	"github.com/colonyops/hive/internal/store"
	"github.com/colonyops/hive/internal/store/db"
	"github.com/colonyops/hive/pkg/buildinfo"
	"github.com/colonyops/hive/pkg/executil"
	"github.com/colonyops/hive/pkg/logutils"
)

var (
	// Build information. Populated at build-time via -ldflags flag, which
	// cmd/hive/.goreleaser.yml points at this package. When installed via
	// `go install module@version`, buildinfo.Resolve reads these from
	// runtime/debug.BuildInfo instead.

	version = "dev"
	commit  = "HEAD"
	date    = "now"
)

func build() string {
	info := buildinfo.Resolve(version, commit, date)

	short := info.Commit
	if len(short) > 7 {
		short = short[:7]
	}

	return fmt.Sprintf("%s (%s) %s", info.Version, short, info.Date)
}

func hiveBuildInfo() app.BuildInfo {
	info := buildinfo.Resolve(version, commit, date)
	return app.BuildInfo{Version: info.Version, Commit: info.Commit, Date: info.Date}
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

// Main runs the hive CLI and exits the process.
func Main() {
	ctx := context.Background()

	var (
		logCloser   func()
		hiveApp     = &app.App{}
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
				Sources:     cli.EnvVars(hiveconfig.EnvConfig),
				Value:       hiveconfig.DefaultConfigPath(),
				Destination: &flags.ConfigPath,
			},
			&cli.StringFlag{
				Name:        "data-dir",
				Usage:       "path to data directory",
				Sources:     cli.EnvVars(hiveconfig.EnvDataDir),
				Value:       hiveconfig.DefaultDataDir(),
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
				hiveApp.Build = hiveBuildInfo()
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

			// Apply configured theme (validation ensures name is valid)
			palette, _ := theme.Get(cfg.TUI.Theme)
			styles.SetTheme(palette)

			database, err = hive.OpenDB(ctx, cfg.DataDir, cfg.Database)
			if err != nil {
				return ctx, err
			}
			kvStore := store.NewKVStore(database)

			// Start background KV sweep goroutine
			sweepCtx, cancel := context.WithCancel(context.Background())
			sweepCancel = cancel
			bgWg.Go(func() {
				sweep.Start(sweepCtx, kvStore, 5*time.Minute)
			})

			bus := events.New(64)
			busCtx, cancel := context.WithCancel(context.Background())
			busCancel = cancel
			bgWg.Go(func() {
				bus.Start(busCtx)
				log.Debug().Msg("event bus stopped")
			})

			events.RegisterDebugLogger(bus, log.Logger)
			hive.NewNotificationRouter(bus).Register()

			var (
				exec      = &executil.RealExecutor{}
				svcLogger = log.With().Str("component", "hive").Logger()
			)

			tmuxClient := tmuxexec.NewDefault(svcLogger.With().Str("component", "tmux").Logger())
			engine, err := hive.New(&cfg.Config, hive.Ports{
				DB:         database,
				Bus:        bus,
				Executor:   exec,
				Mux:        tmuxClient,
				PaneSource: tmuxClient,
				DataDir:    flags.DataDir,
				Styler:     styles.CLIOutputStyler{},
				Stdout:     os.Stdout,
				Stderr:     os.Stderr,
				Logger:     svcLogger,
			})
			if err != nil {
				return ctx, err
			}

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
			*hiveApp = *app.NewApp(engine, cfg, tmuxClient, pluginMgr, commandSet, kvStore, pluginInfos)
			hiveApp.Build = hiveBuildInfo()
			hiveApp.Sources = app.BuildSourceRegistry(cfg, exec, kvStore, svcLogger)

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
