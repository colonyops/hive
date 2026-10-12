// Package app wires the relay: store, services, HTTP server, and the plugs
// manager that runs them.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"syscall"
	"time"

	"github.com/hay-kot/appkit/plugs"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/relay/internal/config"
	"github.com/colonyops/hive/cmd/relay/internal/httpapi"
	"github.com/colonyops/hive/cmd/relay/internal/mail"
	"github.com/colonyops/hive/cmd/relay/internal/mcpsrv"
	"github.com/colonyops/hive/cmd/relay/internal/relay"
	"github.com/colonyops/hive/cmd/relay/internal/store/memory"
	"github.com/colonyops/hive/cmd/relay/internal/store/sqlite"
	"github.com/colonyops/hive/internal/platform/secrets"
)

type Options struct {
	Plugs []plugs.Option
	// Stdout receives the --mint key and, with the log sender, every mail.
	Stdout   io.Writer
	OnListen func(addr net.Addr)
	Version  string
	// NoReload skips the SIGHUP handler, which tests cannot share a process
	// signal with.
	NoReload bool
}

// Run serves the relay until ctx is done or a plugin fails. With an empty
// cfg.PublicURL, ingress URLs point at the bound address.
func Run(ctx context.Context, cfg config.Config, log zerolog.Logger, opts Options) (err error) {
	stdout := opts.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	version := opts.Version
	if version == "" {
		version = "dev"
	}

	store, err := OpenStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := store.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("close store: %w", cerr))
		}
	}()

	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Listen, err)
	}
	defer func() { _ = ln.Close() }()

	publicURL := cfg.PublicURL
	if publicURL == "" {
		publicURL = "http://" + ln.Addr().String()
	}

	if cfg.MintFor != "" && len(cfg.AllowedEmails) == 0 {
		log.Info().Str("email", cfg.MintFor).Msg("allowlist is empty; admitting the --mint email")
	}
	allow := relay.NewAllowlist(allowedEmails(cfg), cfg.UserEmails())
	if len(cfg.AllowedEmails) == 0 && len(cfg.Users) == 0 && cfg.MintFor == "" {
		log.Warn().Msg("the allowlist and the user list are empty, so every request is refused; set allowed_emails or users")
	}
	signIn := relay.NewSignIn(store, allow, cfg.UserHashes(), store.Outbox(), publicURL, cfg.SMTP.Enabled())
	sender, err := newSender(cfg.SMTP, stdout)
	if err != nil {
		return err
	}

	auth := relay.NewAuth(store, allow.Allow)
	core := httpapi.Core{
		Auth:    auth,
		SignIn:  signIn,
		Sources: relay.NewSources(store, cfg.Limits, publicURL),
		Ingress: relay.NewIngress(store, cfg.Limits),
		Feed:    relay.NewFeed(store),
		Store:   store,
		Limits:  cfg.Limits,
		Version: version,
	}
	core.MCP = mcpsrv.New(mcpsrv.Core{
		Auth:    core.Auth,
		SignIn:  core.SignIn,
		Sources: core.Sources,
		Feed:    core.Feed,
		Store:   core.Store,
		Limits:  core.Limits,
		Version: core.Version,
	}, log).Handler()
	handler := httpapi.New(core, cfg, log)

	if cfg.MintFor != "" {
		key, _, err := auth.MintToken(ctx, cfg.MintFor, "mint", relay.TokenAPI)
		if err != nil {
			return fmt.Errorf("mint api key: %w", err)
		}
		if _, err := fmt.Fprintln(stdout, key); err != nil {
			return fmt.Errorf("print api key: %w", err)
		}
	}

	log.Info().Str("addr", ln.Addr().String()).Str("store", cfg.Store).Str("public_url", publicURL).Msg("relay listening")
	if opts.OnListen != nil {
		opts.OnListen(ln.Addr())
	}

	mgr := plugs.New(append([]plugs.Option{
		plugs.WithTimeout(10 * time.Second),
		plugs.WithLogger(plugsLogger{log}),
		plugs.WithMaxAttempts(3),
	}, opts.Plugs...)...)
	mgr.Add(plugs.PluginFunc("http", func(ctx context.Context) error {
		return httpapi.Serve(ctx, ln, cfg.Limits, handler, log)
	}))
	sweeper := relay.NewSweeper(store, cfg.Limits, log)
	mgr.Add(plugs.PluginFunc("sweeper", func(ctx context.Context) error {
		return sweeper.Run(ctx, time.Minute)
	}))
	mailer := mail.NewMailer(store.Outbox(), sender, log)
	mgr.Add(plugs.PluginFunc("mailer", func(ctx context.Context) error {
		return mailer.Run(ctx, 5*time.Second)
	}))
	if !opts.NoReload {
		mgr.Add(plugs.PluginFunc("reload", func(ctx context.Context) error {
			reloadOnHangup(ctx, cfg, allow, signIn, log)
			return nil
		}))
	}
	return mgr.Start(ctx)
}

// allowedEmails is cfg's allowlist, plus the --mint email when the list is
// empty: a dev relay with no list would otherwise refuse its own minted key.
func allowedEmails(cfg config.Config) []string {
	if cfg.MintFor != "" && len(cfg.AllowedEmails) == 0 {
		return []string{relay.NormalizeEmail(cfg.MintFor)}
	}
	return slices.Clone(cfg.AllowedEmails)
}

func newSender(cfg config.SMTP, stdout io.Writer) (mail.Sender, error) {
	if cfg.IsLog() {
		return &mail.Log{Out: stdout}, nil
	}
	password := func() (string, error) {
		if cfg.Password == "" {
			return "", nil
		}
		return secrets.Resolve(cfg.Password)
	}
	s, err := mail.NewSMTP(cfg.URL, password, cfg.From)
	if err != nil {
		return nil, fmt.Errorf("smtp: %w", err)
	}
	return s, nil
}

// reloadOnHangup re-reads the allowlist and the user list on every SIGHUP
// until ctx is done. A bad file is logged and the running lists stay.
func reloadOnHangup(ctx context.Context, cfg config.Config, allow *relay.Allowlist, signIn *relay.SignIn, log zerolog.Logger) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP)
	defer signal.Stop(ch)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ch:
		}
		next, err := cfg.Reload()
		if err != nil {
			log.Error().Err(err).Msg("reload failed; keeping the current allowlist and users")
			continue
		}
		allow.Reload(allowedEmails(next), next.UserEmails())
		signIn.SetUsers(next.UserHashes())
		log.Info().Int("allowed_emails", len(next.AllowedEmails)).Int("users", len(next.Users)).Msg("reloaded allowlist and users")
	}
}

func OpenStore(ctx context.Context, cfg config.Config) (relay.Store, error) {
	switch cfg.Store {
	case config.StoreMemory:
		return memory.New(), nil
	case config.StoreSQLite:
		if err := os.MkdirAll(filepath.Dir(cfg.SQLitePath()), 0o700); err != nil {
			return nil, fmt.Errorf("create data dir: %w", err)
		}
		return sqlite.Open(ctx, cfg.SQLitePath())
	default:
		return nil, fmt.Errorf("unknown store %q", cfg.Store)
	}
}

type plugsLogger struct{ log zerolog.Logger }

func (l plugsLogger) Debug(msg string, kv ...any) { l.log.Debug().Fields(kv).Msg(msg) }
func (l plugsLogger) Info(msg string, kv ...any)  { l.log.Info().Fields(kv).Msg(msg) }
func (l plugsLogger) Warn(msg string, kv ...any)  { l.log.Warn().Fields(kv).Msg(msg) }
func (l plugsLogger) Error(msg string, kv ...any) { l.log.Error().Fields(kv).Msg(msg) }
