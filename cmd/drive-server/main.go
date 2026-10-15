// Command hive-drive-server serves Hive Drive documents to many users.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/urfave/cli/v3"
	"golang.org/x/term"

	"github.com/colonyops/hive/cmd/drive-server/internal/app"
	"github.com/colonyops/hive/cmd/drive-server/internal/config"
	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/pkg/buildinfo"
)

var (
	version = "dev"
	commit  = "dev"
	date    = "dev"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "hive-drive-server:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return newApp().Run(ctx, os.Args)
}

func newApp() *cli.Command {
	info := buildinfo.Resolve(version, commit, date)
	return &cli.Command{
		Name:     "hive-drive-server",
		Usage:    "Share Hive Drive documents with other people",
		Version:  fmt.Sprintf("%s (%s) %s", info.Version, info.Commit, info.Date),
		Commands: []*cli.Command{serveCommand(info), accountsCommand()},
	}
}

func serveCommand(info buildinfo.Info) *cli.Command {
	return &cli.Command{
		Name:  "serve",
		Usage: "Run the server",
		Flags: append(configFlags(),
			&cli.StringFlag{Name: "listen", Usage: "address to listen on, default :8080 (HIVE_DRIVE_SERVER_LISTEN)"},
			&cli.StringFlag{Name: "max-document-size", Usage: "largest document body, default 5MiB (HIVE_DRIVE_SERVER_MAX_DOCUMENT_SIZE)"},
			&cli.StringFlag{Name: "trusted-proxies", Usage: "comma-separated CIDRs whose X-Forwarded-For is honoured (HIVE_DRIVE_SERVER_TRUSTED_PROXIES)"},
			&cli.BoolFlag{Name: "dev-log-mail", Usage: "print mail to stdout instead of sending it; for development only"},
			&cli.StringFlag{Name: "log-level", Usage: "debug, info, warn, or error", Value: "info", Sources: cli.EnvVars("HIVE_DRIVE_SERVER_LOG_LEVEL")},
		),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			level, err := zerolog.ParseLevel(cmd.String("log-level"))
			if err != nil {
				return fmt.Errorf("log-level: %w", err)
			}
			log := newLogger(level)

			cfg, warnings, err := loadConfig(cmd)
			for _, w := range warnings {
				log.Warn().Msg(w)
			}
			if err != nil {
				return err
			}
			return app.Run(ctx, cfg, log, app.Options{Info: info})
		},
	}
}

// configFlags are the flags every command shares. Admin commands open
// control.db directly, so they need the data directory and policies the
// server runs with.
func configFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "base-url", Usage: "public URL of this server (HIVE_DRIVE_SERVER_BASE_URL)"},
		&cli.StringFlag{Name: "data-dir", Usage: "directory for the databases, default /data (HIVE_DRIVE_SERVER_DATA_DIR)"},
		&cli.StringFlag{Name: "config-dir", Usage: "directory holding drive-server.yaml, default /config (HIVE_DRIVE_SERVER_CONFIG_DIR)"},
	}
}

func loadConfig(cmd *cli.Command) (config.Config, []string, error) {
	cfg, err := config.Load(os.Getenv, config.Flags{
		BaseURL:         cmd.String("base-url"),
		Listen:          cmd.String("listen"),
		DataDir:         cmd.String("data-dir"),
		ConfigDir:       cmd.String("config-dir"),
		MaxDocumentSize: cmd.String("max-document-size"),
		TrustedProxies:  cmd.String("trusted-proxies"),
		DevLogMail:      cmd.Bool("dev-log-mail"),
	})
	if err != nil {
		return config.Config{}, nil, err
	}
	warnings, err := cfg.Validate()
	if err != nil {
		return config.Config{}, warnings, fmt.Errorf("invalid config: %w", err)
	}
	return cfg, warnings, nil
}

// withServices runs fn over the services of an opened control.db.
func withServices(fn func(ctx context.Context, cmd *cli.Command, svc *app.Services) error) cli.ActionFunc {
	return func(ctx context.Context, cmd *cli.Command) (err error) {
		cfg, warnings, err := loadConfig(cmd)
		for _, w := range warnings {
			fmt.Fprintln(os.Stderr, "warning:", w)
		}
		if err != nil {
			return err
		}
		svc, err := app.OpenServices(ctx, cfg)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, svc.Close()) }()
		return fn(ctx, cmd, svc)
	}
}

func accountsCommand() *cli.Command {
	return &cli.Command{
		Name:  "accounts",
		Usage: "Manage accounts and their tokens",
		Commands: []*cli.Command{
			{
				Name:  "list",
				Usage: "List accounts and the policy that admits each",
				Flags: configFlags(),
				Action: withServices(func(ctx context.Context, _ *cli.Command, svc *app.Services) error {
					return svc.WriteAccounts(ctx, os.Stdout)
				}),
			},
			{
				Name:      "delete",
				Usage:     "Delete an account, its tokens, and its sessions",
				ArgsUsage: "<email>",
				Flags:     append(configFlags(), &cli.BoolFlag{Name: "yes", Usage: "confirm the deletion"}),
				Action: withServices(func(ctx context.Context, cmd *cli.Command, svc *app.Services) error {
					email := cmd.Args().First()
					if email == "" {
						return errors.New("accounts delete: email is required")
					}
					if !cmd.Bool("yes") {
						return errors.New("accounts delete: pass --yes to confirm")
					}
					return svc.Accounts.Delete(ctx, email)
				}),
			},
			{
				Name:  "token",
				Usage: "Manage API tokens",
				Commands: []*cli.Command{{
					Name:      "create",
					Usage:     "Create an API token, creating the account if its email matches a policy",
					ArgsUsage: "<email>",
					Flags: append(configFlags(),
						&cli.StringFlag{Name: "scope", Usage: "read or push", Required: true},
						&cli.StringFlag{Name: "name", Usage: "label for the token", Value: "cli"},
						&cli.StringFlag{Name: "expires", Usage: "lifetime, like 30d or 720h; default never"},
					),
					Action: withServices(createToken),
				}},
			},
		},
	}
}

func createToken(ctx context.Context, cmd *cli.Command, svc *app.Services) error {
	email := cmd.Args().First()
	if email == "" {
		return errors.New("accounts token create: email is required")
	}
	scope, err := core.ParseScope(cmd.String("scope"))
	if err != nil {
		return fmt.Errorf("scope: %w", err)
	}
	ttl, err := app.ParseTTL(cmd.String("expires"))
	if err != nil {
		return err
	}
	secret, _, err := svc.CreateToken(ctx, email, scope, cmd.String("name"), ttl)
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "This token is shown once. Store it now.")
	_, err = fmt.Fprintln(os.Stdout, secret)
	return err
}

func newLogger(level zerolog.Level) zerolog.Logger {
	if term.IsTerminal(int(os.Stderr.Fd())) {
		return zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).Level(level).With().Timestamp().Logger()
	}
	return zerolog.New(os.Stderr).Level(level).With().Timestamp().Logger()
}
