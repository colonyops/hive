// Command devserver fronts the GitHub API for local development: it caches
// responses so several desktop instances share one rate-limit budget, rewrites
// them from a config-driven overlay so workflows can be simulated against real
// data, and pushes webhook payloads at a running instance's local listener.
//
// It is development tooling: loopback only, no credentials of its own, and
// nothing in the shipped app depends on it. See README.md and ADR devserver-github-proxy.
package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/cmd/desktop/internal/devproxy"
)

//go:embed dashboard.html
var dashboardHTML []byte

// browserChromePaths are requests a browser makes on its own behalf when it
// loads the dashboard. Without a route here they fall through to the proxy,
// reach api.github.com, and inflate the very stats the dashboard reports. The
// desktop client never requests these, so a path list cannot misclassify a real
// API call the way sniffing the caller could.
var browserChromePaths = []string{
	"/favicon.ico",
	"/.well-known/appspecific/com.chrome.devtools.json",
}

// options holds the parsed flags. Each is bound with Destination so its name is
// declared once, rather than restated as a lookup string wherever it is read.
type options struct {
	config          string
	listen          string
	upstream        string
	ttl             time.Duration
	standbyPoll     time.Duration
	shutdownTimeout time.Duration
	logLevel        string
}

func main() {
	// run has a pointer receiver on purpose: opts.run resolves to (&opts).run,
	// so the flag parser writes through to the same struct the action reads. A
	// value receiver would bind a copy taken before anything is parsed.
	var opts options

	cmd := &cli.Command{
		Name:  "devserver",
		Usage: "cache and simulate the GitHub API for desktop development",
		Description: `Runs a loopback proxy in front of the GitHub API. Responses are cached and shared
across every desktop instance pointed at it, so N instances and their restarts
cost one upstream request per unique call per TTL. Config-driven overlays rewrite
those responses to simulate lifecycle events, and a webhook pusher delivers
synthetic payloads to a running instance. A dashboard drives both.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "config",
				Aliases:     []string{"c"},
				Usage:       "path to a config file",
				DefaultText: devproxy.RepoConfigPath,
				Destination: &opts.config,
			},
			&cli.StringFlag{
				Name:        "listen",
				Usage:       "override the configured bind address",
				Destination: &opts.listen,
			},
			&cli.StringFlag{
				Name:        "upstream",
				Usage:       "override the configured GitHub API base URL",
				Destination: &opts.upstream,
			},
			&cli.DurationFlag{
				Name:        "ttl",
				Usage:       "override the configured cache TTL",
				Destination: &opts.ttl,
			},
			&cli.DurationFlag{
				Name:        "standby-poll",
				Value:       2 * time.Second,
				Usage:       "how often a standby launch retries the port",
				Destination: &opts.standbyPoll,
			},
			&cli.DurationFlag{
				Name:        "shutdown-timeout",
				Value:       5 * time.Second,
				Usage:       "how long in-flight requests get to finish on shutdown",
				Destination: &opts.shutdownTimeout,
			},
			&cli.StringFlag{
				Name:        "log-level",
				Value:       "info",
				Usage:       "trace, debug, info, warn, or error",
				Destination: &opts.logLevel,
			},
		},
		Action: opts.run,
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "devserver:", err)
		os.Exit(1)
	}
}

// newHandler wires devserver's routes. Ordering is by ServeMux specificity, not
// registration order: the proxy takes "/" and everything else wins over it.
func newHandler(control *Control, proxy *Proxy, logger zerolog.Logger) http.Handler {
	noContent := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	mux := http.NewServeMux()
	// {$} matches the root path exactly, leaving every other path to the proxy.
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, err := w.Write(dashboardHTML); err != nil {
			logger.Debug().Err(err).Msg("writing dashboard")
		}
	})
	mux.Handle("/_ctl/", control.Handler())
	for _, path := range browserChromePaths {
		mux.Handle("GET "+path, noContent)
	}
	mux.Handle("/", proxy)
	return mux
}

// awaitListener binds addr, standing by until it is free. The proxy is a
// singleton (ADR devserver-github-proxy), so the first process to bind serves every worktree and
// the rest park here instead of exiting: when the live one stops, a standby
// takes over and the worktrees still pointed at the port keep working.
//
// The bind is the arbiter, not the probe. Two launches can both find the port
// free and exactly one wins, and the loser just goes back to waiting — so there
// is no race to close. The probe only identifies the occupant, which is what
// keeps a stranger on the port fatal rather than something we wait on forever.
func awaitListener(ctx context.Context, addr string, poll time.Duration, logger zerolog.Logger) (net.Listener, error) {
	var announced string
	for {
		listener, err := net.Listen("tcp", addr)
		if err == nil {
			if announced != "" {
				logger.Info().Str("listen", addr).Msg("port released; taking over")
			}
			return listener, nil
		}
		if !errors.Is(err, syscall.EADDRINUSE) {
			return nil, fmt.Errorf("listen on %s: %w", addr, err)
		}

		// Something that answers and is not devserver is fatal. A port held by
		// something that answers nothing is waited on instead: that is also what
		// a devserver looks like between binding and serving, and waiting on the
		// wrong thing forever is recoverable where exiting on our own is not.
		status := devproxy.Probe(ctx, devproxy.BaseURL(addr))
		if status == devproxy.StatusForeign {
			return nil, fmt.Errorf("%s is in use by something that is not devserver", addr)
		}
		msg := "devserver is already running; standing by to take over when it stops"
		if status == devproxy.StatusAbsent {
			msg = "port is held by something that does not answer the devserver probe; standing by"
		}
		// Announce on change rather than per poll, so an unattended standby stays
		// quiet but never leaves the reason for the wait unsaid.
		if announced != msg {
			announced = msg
			logger.Info().Str("listen", addr).Msg(msg)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(poll):
		}
	}
}

func (o *options) run(ctx context.Context, _ *cli.Command) error {
	configPath, err := ResolveConfigPath(o.config)
	if err != nil {
		return err
	}
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return err
	}
	if o.listen != "" {
		cfg.Listen = o.listen
	}
	if o.upstream != "" {
		cfg.Upstream = o.upstream
	}
	if o.ttl > 0 {
		cfg.Cache.TTL = o.ttl
	}

	level, err := zerolog.ParseLevel(o.logLevel)
	if err != nil {
		return fmt.Errorf("invalid log level %q: %w", o.logLevel, err)
	}
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.TimeOnly}).
		Level(level).With().Timestamp().Logger()

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Win the port before opening anything: a process standing by must hold no
	// cache handle, and binding is what decides which launch is the live one.
	listener, err := awaitListener(ctx, cfg.Listen, o.standbyPoll, logger)
	if errors.Is(err, context.Canceled) {
		return nil // interrupted while standing by
	}
	if err != nil {
		return err
	}

	cache, err := OpenCache(cfg.Cache.Path, cfg.Cache.TTL)
	if err != nil {
		return err
	}
	defer cache.Close() //nolint:errcheck // best-effort close on shutdown

	store := NewStore(cfg.Overlays)
	proxy := NewProxy(cfg.Upstream, cache, store, logger)
	pusher := NewPusher(cfg.Webhooks, logger)
	control := NewControl(cfg, store, cache, proxy, pusher, logger)

	server := &http.Server{
		Addr:              cfg.Listen,
		Handler:           newHandler(control, proxy, logger),
		ReadHeaderTimeout: 5 * time.Second,
	}

	logger.Info().
		Str("config", configPath).
		Str("listen", cfg.Listen).
		Str("upstream", cfg.Upstream).
		Str("cache", cfg.Cache.Path).
		Dur("ttl", cfg.Cache.TTL).
		Int("overlays", len(cfg.Overlays)).
		Int("targets", len(cfg.Webhooks.Targets)).
		Msg("devserver started")
	if len(cfg.Overlays) > 0 {
		logger.Warn().Int("overlays", len(cfg.Overlays)).
			Msg("config seeds overlays; connected instances see rewritten data from the first request")
	}
	logger.Info().Msgf("dashboard: http://%s", cfg.Listen)
	logger.Info().Msgf("point a desktop instance at it: %s=http://%s mise run desktop:dev",
		devproxy.EnvAPIBase, cfg.Listen)

	errs := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err
		}
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		logger.Info().Msg("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), o.shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
		return nil
	}
}
