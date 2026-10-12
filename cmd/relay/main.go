// Command hive-relay is Hive Relay, the hosted feed Hive Desktop pulls.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"text/tabwriter"

	"github.com/rs/zerolog"
	"github.com/urfave/cli/v3"
	"golang.org/x/term"

	"github.com/colonyops/hive/cmd/relay/internal/app"
	"github.com/colonyops/hive/cmd/relay/internal/config"
	"github.com/colonyops/hive/cmd/relay/internal/relay"
	"github.com/colonyops/hive/pkg/buildinfo"
)

var (
	version = "dev"
	commit  = "dev"
	date    = "dev"
)

func main() {
	if err := newApp().Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "hive-relay:", err)
		os.Exit(1)
	}
}

func newApp() *cli.Command {
	info := buildinfo.Resolve(version, commit, date)
	return &cli.Command{
		Name:    "hive-relay",
		Usage:   "A hosted feed: store any HTTP request as delivered and serve it by cursor",
		Version: fmt.Sprintf("%s (%s) %s", info.Version, info.Commit, info.Date),
		Commands: []*cli.Command{
			{
				Name:  "serve",
				Usage: "Run the relay server",
				Flags: append(config.Flags(), &cli.StringFlag{
					Name:  config.FlagMint,
					Usage: "memory store only: mint an API key for `EMAIL` at start and print it once",
				}),
				Action: func(ctx context.Context, cmd *cli.Command) error {
					cfg, log, err := load(cmd)
					if err != nil {
						return err
					}
					if err := cfg.RequirePublicURL(); err != nil {
						return err
					}
					return app.Run(ctx, cfg, log, app.Options{Version: info.Version})
				},
			},
			{
				Name:  "tokens",
				Usage: "Manage API keys on the host",
				Commands: []*cli.Command{{
					Name:  "create",
					Usage: "Mint an API key for an email and print it once",
					Flags: append(config.Flags(),
						&cli.StringFlag{Name: "email", Required: true},
						&cli.StringFlag{Name: "name", Required: true},
					),
					Action: tokensCreate,
				}},
			},
			{
				Name:  "accounts",
				Usage: "Inspect and remove accounts on the host",
				Commands: []*cli.Command{
					{Name: "list", Usage: "List accounts", Flags: config.Flags(), Action: accountsList},
					{
						Name:   "delete",
						Usage:  "Delete an account and everything it owns",
						Flags:  append(config.Flags(), &cli.StringFlag{Name: "email", Required: true}),
						Action: accountsDelete,
					},
				},
			},
			{
				Name:  "hash-password",
				Usage: "Hash a password for the users list in the config file",
				Action: func(context.Context, *cli.Command) error {
					return errors.New("hash-password arrives in Phase 4")
				},
			},
		},
	}
}

func load(cmd *cli.Command) (config.Config, zerolog.Logger, error) {
	cfg, err := config.Load(cmd)
	if err != nil {
		return config.Config{}, zerolog.Nop(), err
	}
	level, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		return config.Config{}, zerolog.Nop(), err
	}
	return cfg, newLogger(level), nil
}

func newLogger(level zerolog.Level) zerolog.Logger {
	if term.IsTerminal(int(os.Stderr.Fd())) {
		return zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).Level(level).With().Timestamp().Logger()
	}
	return zerolog.New(os.Stderr).Level(level).With().Timestamp().Logger()
}

// openPersistentStore opens the store for an operator command. Those commands
// write to the store a running server reads, so the memory store, which lives
// only inside one process, has nothing for them to act on.
func openPersistentStore(ctx context.Context, cmd *cli.Command) (relay.Store, error) {
	cfg, _, err := load(cmd)
	if err != nil {
		return nil, err
	}
	if cfg.Store == config.StoreMemory {
		return nil, fmt.Errorf("%s needs --store sqlite: the memory store lives inside the server process, so there is nothing here to act on", cmd.FullName())
	}
	return app.OpenStore(ctx, cfg)
}

func tokensCreate(ctx context.Context, cmd *cli.Command) (err error) {
	store, err := openPersistentStore(ctx, cmd)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, store.Close()) }()

	key, _, err := relay.NewAuth(store, nil).MintToken(ctx, cmd.String("email"), cmd.String("name"), relay.TokenAPI)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(cmd.Root().Writer, key)
	return err
}

func accountsList(ctx context.Context, cmd *cli.Command) (err error) {
	store, err := openPersistentStore(ctx, cmd)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, store.Close()) }()

	accts, err := store.Accounts().List(ctx)
	if err != nil {
		return err
	}
	slices.SortFunc(accts, func(a, b relay.Account) int { return a.CreatedAt.Compare(b.CreatedAt) })
	tw := tabwriter.NewWriter(cmd.Root().Writer, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "EMAIL\tSOURCES\tBYTES\tCREATED")
	for _, a := range accts {
		n, err := store.Sources().CountByEmail(ctx, a.Email)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%d\t%s\n", a.Email, n, a.BytesUsed, a.CreatedAt.Format("2006-01-02"))
	}
	return tw.Flush()
}

func accountsDelete(ctx context.Context, cmd *cli.Command) (err error) {
	store, err := openPersistentStore(ctx, cmd)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, store.Close()) }()

	email := relay.NormalizeEmail(cmd.String("email"))
	acct, err := store.Accounts().Get(ctx, email)
	if errors.Is(err, relay.ErrNotFound) {
		return fmt.Errorf("no account for %s", email)
	}
	if err != nil {
		return err
	}
	sources, err := store.Sources().CountByEmail(ctx, email)
	if err != nil {
		return err
	}
	tokens, err := store.Tokens().ListByEmail(ctx, email)
	if err != nil {
		return err
	}
	if err := store.Accounts().Delete(ctx, email); err != nil {
		return err
	}
	_, err = fmt.Fprintf(cmd.Root().Writer, "deleted %s: %d sources, %d tokens, %d bytes\n", email, sources, len(tokens), acct.BytesUsed)
	return err
}
