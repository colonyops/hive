// Command hive-drive-server serves Hive Drive documents to many users.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/urfave/cli/v3"
	"golang.org/x/term"

	"github.com/colonyops/hive/cmd/drive-server/internal/app"
	"github.com/colonyops/hive/cmd/drive-server/internal/config"
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
		Name:    "hive-drive-server",
		Usage:   "Share Hive Drive documents with other people",
		Version: fmt.Sprintf("%s (%s) %s", info.Version, info.Commit, info.Date),
		Commands: []*cli.Command{{
			Name:  "serve",
			Usage: "Run the server",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "base-url", Usage: "public URL of this server (HIVE_DRIVE_SERVER_BASE_URL)"},
				&cli.StringFlag{Name: "listen", Usage: "address to listen on, default :8080 (HIVE_DRIVE_SERVER_LISTEN)"},
				&cli.StringFlag{Name: "data-dir", Usage: "directory for the databases, default /data (HIVE_DRIVE_SERVER_DATA_DIR)"},
				&cli.StringFlag{Name: "config-dir", Usage: "directory holding drive-server.yaml, default /config (HIVE_DRIVE_SERVER_CONFIG_DIR)"},
				&cli.StringFlag{Name: "max-document-size", Usage: "largest document body, default 5MiB (HIVE_DRIVE_SERVER_MAX_DOCUMENT_SIZE)"},
				&cli.StringFlag{Name: "trusted-proxies", Usage: "comma-separated CIDRs whose X-Forwarded-For is honoured (HIVE_DRIVE_SERVER_TRUSTED_PROXIES)"},
				&cli.BoolFlag{Name: "dev-log-mail", Usage: "print mail to stdout instead of sending it; for development only"},
				&cli.StringFlag{Name: "log-level", Usage: "debug, info, warn, or error", Value: "info", Sources: cli.EnvVars("HIVE_DRIVE_SERVER_LOG_LEVEL")},
			},
			Action: func(ctx context.Context, cmd *cli.Command) error {
				level, err := zerolog.ParseLevel(cmd.String("log-level"))
				if err != nil {
					return fmt.Errorf("log-level: %w", err)
				}
				log := newLogger(level)

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
					return err
				}
				warnings, err := cfg.Validate()
				for _, w := range warnings {
					log.Warn().Msg(w)
				}
				if err != nil {
					return fmt.Errorf("invalid config: %w", err)
				}
				return app.Run(ctx, cfg, log, app.Options{Info: info})
			},
		}},
	}
}

func newLogger(level zerolog.Level) zerolog.Logger {
	if term.IsTerminal(int(os.Stderr.Fd())) {
		return zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).Level(level).With().Timestamp().Logger()
	}
	return zerolog.New(os.Stderr).Level(level).With().Timestamp().Logger()
}
