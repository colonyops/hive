package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"github.com/hay-kot/hive/internal/commands"
	"github.com/hay-kot/hive/internal/core/config"
	"github.com/hay-kot/hive/internal/core/git"
	"github.com/hay-kot/hive/internal/core/styles"
	"github.com/hay-kot/hive/internal/data/db"
	"github.com/hay-kot/hive/internal/data/stores"
	"github.com/hay-kot/hive/internal/hive"
	"github.com/hay-kot/hive/internal/hive/plugins"
	"github.com/hay-kot/hive/internal/hive/plugins/beads"
	"github.com/hay-kot/hive/internal/hive/plugins/claude"
	"github.com/hay-kot/hive/internal/hive/plugins/contextdir"
	"github.com/hay-kot/hive/internal/hive/plugins/github"
	"github.com/hay-kot/hive/internal/hive/plugins/lazygit"
	"github.com/hay-kot/hive/internal/hive/plugins/neovim"
	plugintmux "github.com/hay-kot/hive/internal/hive/plugins/tmux"
	"github.com/hay-kot/hive/internal/hive/scripts"
	"github.com/hay-kot/hive/pkg/executil"
	"github.com/hay-kot/hive/pkg/logutils"
	"github.com/hay-kot/hive/pkg/tmpl"
)

var (
	// Build information. Populated at build-time via -ldflags flag.
	version = "dev"
	commit  = "HEAD"
	date    = "now"
)

func build() string {
	short := commit
	if len(commit) > 7 {
		short = commit[:7]
	}

	return fmt.Sprintf("%s (%s) %s", version, short, date)
}

func main() {
	ctx := context.Background()

	var (
		logCloser func()
		hiveApp   = &hive.App{}
		database  *db.DB
		pluginMgr *plugins.Manager
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
		Version: build(),
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
				Value:       commands.DefaultConfigPath(),
				Destination: &flags.ConfigPath,
			},
			&cli.StringFlag{
				Name:        "data-dir",
				Usage:       "path to data directory",
				Sources:     cli.EnvVars("HIVE_DATA_DIR"),
				Value:       commands.DefaultDataDir(),
				Destination: &flags.DataDir,
			},
		},
		Before: func(ctx context.Context, c *cli.Command) (context.Context, error) {
			// Always log to a file; use explicit path or default to <datadir>/hive.log
			logFile := flags.LogFile
			if logFile == "" {
				logFile = filepath.Join(flags.DataDir, "hive.log")
			}

			logger, closer, err := logutils.New(flags.LogLevel, logFile)
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

			// Create service
			var (
				exec      = &executil.RealExecutor{}
				gitExec   = git.NewExecutor(cfg.GitPath, exec)
				svcLogger = log.With().Str("component", "hive").Logger()
			)

			sessionSvc := hive.NewSessionService(sessionStore, gitExec, cfg, exec, renderer, svcLogger, os.Stdout, os.Stderr)

			// Create plugin manager and register plugins
			pluginMgr = plugins.NewManager(cfg.Plugins)
			pluginMgr.Register(github.New(cfg.Plugins.GitHub))
			pluginMgr.Register(beads.New(cfg.Plugins.Beads))
			pluginMgr.Register(lazygit.New(cfg.Plugins.LazyGit))
			pluginMgr.Register(neovim.New(cfg.Plugins.Neovim))
			pluginMgr.Register(contextdir.New(cfg.Plugins.ContextDir, cfg.DataDir))
			pluginMgr.Register(claude.New(cfg.Plugins.Claude))
			pluginMgr.Register(plugintmux.New(cfg.Plugins.Tmux))

			// Initialize plugins (errors are logged but don't stop startup)
			if err := pluginMgr.InitAll(ctx); err != nil {
				log.Warn().Err(err).Msg("plugin initialization error")
			}

			// Populate the pre-allocated App struct (commands already hold a pointer to it)
			*hiveApp = *hive.NewApp(
				sessionSvc,
				msgStore,
				cfg,
				nil, // terminal manager created in TUI command
				pluginMgr,
				database,
				renderer,
			)

			return ctx, nil
		},
		After: func(ctx context.Context, c *cli.Command) error {
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

			// Close log file
			if logCloser != nil {
				logCloser()
			}
			return nil
		},
	}

	tuiCmd := commands.NewTuiCmd(flags, hiveApp)

	app = commands.NewNewCmd(flags, hiveApp).Register(app)
	app = commands.NewLsCmd(flags, hiveApp).Register(app)
	app = commands.NewPruneCmd(flags, hiveApp).Register(app)
	app = commands.NewDoctorCmd(flags, hiveApp).Register(app)
	app = commands.NewBatchCmd(flags, hiveApp).Register(app)
	app = commands.NewCtxCmd(flags, hiveApp).Register(app)
	app = commands.NewMsgCmd(flags, hiveApp).Register(app)
	app = commands.NewDocCmd(flags, hiveApp).Register(app)
	app = commands.NewSessionCmd(flags, hiveApp).Register(app)
	app = commands.NewReviewCmd(flags, hiveApp).Register(app)

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
