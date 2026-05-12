package main

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/internal/commands"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/pkg/logutils"
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

func main() {
	ctx := context.Background()

	hiveApp := &hive.App{}
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
			// Skip all initialization during shell completion. The
			// completion handler only needs the command tree (already
			// registered) to suggest subcommands and flags.
			if isShellCompletion(os.Args) {
				return ctx, nil
			}

			// Logger setup runs for every command — cheap, and every
			// command needs the option to log.
			logFile := commands.EffectiveLogFile(flags)
			logger, closer, err := logutils.New(flags.LogLevel, logFile)
			if err != nil {
				return ctx, fmt.Errorf("setup logger: %w", err)
			}
			log.Logger = logger
			hiveApp.LogCloser = closer

			// Pin resolved bootstrap inputs onto the app. Per-command
			// Before hooks invoke FullBootstrap / MinimalBootstrap to
			// perform the heavy initialization.
			resolvedVersion, resolvedCommit, resolvedDate := resolvedBuildInfo()
			hiveApp.Opts = hive.BootstrapOptions{
				DataDir:    flags.DataDir,
				ConfigPath: flags.ConfigPath,
				LogFile:    logFile,
				LogLevel:   flags.LogLevel,
				Version:    resolvedVersion,
			}
			hiveApp.Build = hive.BuildInfo{
				Version: resolvedVersion,
				Commit:  resolvedCommit,
				Date:    resolvedDate,
			}

			return ctx, nil
		},
		After: func(ctx context.Context, c *cli.Command) error {
			if err := hiveApp.Shutdown(); err != nil {
				log.Error().Err(err).Msg("failed to close database")
				return err
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
	app = commands.NewHoneycombCmd(flags, hiveApp).Register(app)
	app = commands.NewWorkspaceCmd(flags, hiveApp).Register(app)
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
