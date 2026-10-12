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
	"path/filepath"
	"time"

	"github.com/hay-kot/appkit/plugs"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/relay/internal/config"
	"github.com/colonyops/hive/cmd/relay/internal/httpapi"
	"github.com/colonyops/hive/cmd/relay/internal/relay"
	"github.com/colonyops/hive/cmd/relay/internal/store/memory"
	"github.com/colonyops/hive/cmd/relay/internal/store/sqlite"
)

type Options struct {
	Plugs    []plugs.Option
	Stdout   io.Writer
	OnListen func(addr net.Addr)
	Version  string
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

	auth := relay.NewAuth(store, nil)
	core := httpapi.Core{
		Auth:    auth,
		Sources: relay.NewSources(store, cfg.Limits, publicURL),
		Ingress: relay.NewIngress(store, cfg.Limits),
		Feed:    relay.NewFeed(store),
		Store:   store,
		Limits:  cfg.Limits,
		Version: version,
	}
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
	return mgr.Start(ctx)
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
