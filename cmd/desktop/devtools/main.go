// Command devtools prepares an isolated worktree-local Hive Desktop
// development instance and writes the environment mise loads before launch.
package main

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/urfave/cli/v3"
)

const envLogLevel = "HIVE_DESKTOP_DEVTOOLS_LOG_LEVEL"

func main() {
	logger := newConsoleLogger(os.Stderr, zerolog.InfoLevel)

	command := &cli.Command{
		Name:        "devtools",
		Usage:       "manage an isolated Hive Desktop development instance",
		Description: "Prepares reusable worktree-local data and config and writes launch.env for mise. Wails runs directly from the mise task with that environment.",
		// Main owns structured error logging and process exit codes.
		ExitErrHandler: func(context.Context, *cli.Command, error) {},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:             "log-level",
				Usage:            "set log verbosity (trace, debug, info, warn, error, fatal, panic, disabled)",
				Value:            zerolog.InfoLevel.String(),
				Sources:          cli.EnvVars(envLogLevel),
				Local:            true,
				ValidateDefaults: true,
				Validator: func(value string) error {
					_, err := zerolog.ParseLevel(value)
					return err
				},
			},
		},
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			level, err := zerolog.ParseLevel(cmd.String("log-level"))
			if err != nil {
				return nil, err
			}
			logger = newConsoleLogger(os.Stderr, level)
			return ctx, nil
		},
		Commands: []*cli.Command{
			{
				Name:        "prepare",
				Usage:       "prepare or reuse this worktree's development instance",
				Description: "Seeds .hive-desktop on first use and writes launch.env with isolated paths and non-conflicting Wails/Vite ports.",
				Action: devtoolsAction(logger, func(tools *devtools) error {
					return tools.withLock(func() error { return tools.prepare(false) })
				}),
			},
			{
				Name:        "fresh",
				Usage:       "delete and reseed this worktree's development instance",
				Description: "Safely removes the marked instance and launch.env, then snapshots installed state and allocates fresh framework ports.",
				Action: devtoolsAction(logger, func(tools *devtools) error {
					return tools.withLock(func() error { return tools.prepare(true) })
				}),
			},
			{
				Name:        "reset",
				Usage:       "remove this worktree's development instance",
				Description: "Safely removes the marked .hive-desktop directory and generated launch.env without starting Wails.",
				Action: devtoolsAction(logger, func(tools *devtools) error {
					return tools.withLock(tools.reset)
				}),
			},
			{
				Name:        "onboarding",
				Usage:       "prepare a blank instance for live onboarding",
				Description: "Recreates an onboarding-only instance with empty data and config and a worktree-specific keychain namespace.",
				Action: func(_ context.Context, cmd *cli.Command) error {
					if cmd.NArg() != 0 {
						return cli.Exit(cmd.Name+" does not accept positional arguments", 2)
					}
					worktree, err := findWorktree()
					if err != nil {
						return err
					}
					tools := newOnboardingDevtools(worktree, logger)
					return tools.withLock(func() error { return tools.prepare(true) })
				},
			},
			{
				Name:      "run",
				Usage:     "run the Wails dev runner and own the session's teardown",
				ArgsUsage: "-- <runner> [args...]",
				Description: "Supervises the dev runner so a closed terminal cannot leave the app running, " +
					"and so the app is asked to shut down before the runner kills it.",
				Action: func(_ context.Context, cmd *cli.Command) error {
					argv := cmd.Args().Slice()
					if len(argv) == 0 {
						return cli.Exit("run needs the dev runner command, e.g. `run -- wails3 dev`", 2)
					}
					return runDevSession(logger, argv)
				},
			},
		},
	}

	if err := command.Run(context.Background(), os.Args); err != nil {
		logger.Error().Err(err).Msg("devtools failed")

		code := 1
		if cliExit, ok := errors.AsType[cli.ExitCoder](err); ok {
			code = cliExit.ExitCode()
		}

		os.Exit(code)
	}
}

func newConsoleLogger(out io.Writer, level zerolog.Level) zerolog.Logger {
	writer := zerolog.ConsoleWriter{Out: out, TimeFormat: time.Kitchen}
	return zerolog.New(writer).With().Timestamp().Logger().Level(level)
}

func devtoolsAction(logger zerolog.Logger, action func(*devtools) error) cli.ActionFunc {
	return func(_ context.Context, cmd *cli.Command) error {
		if cmd.NArg() != 0 {
			return cli.Exit(cmd.Name+" does not accept positional arguments", 2)
		}
		worktree, err := findWorktree()
		if err != nil {
			return err
		}
		return action(newDevtools(worktree, logger))
	}
}
