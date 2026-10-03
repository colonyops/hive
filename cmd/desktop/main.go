// Command hive-desktop is the desktop app's entrypoint. It does four things:
// apply the bootstrap overrides, build the headless core, mount the Wails
// adapter over it, and run. Everything else belongs to cmd/desktop/internal/app (what the
// app does) or cmd/desktop/internal/adapter/wailsui (how a window talks to it).
package main

import (
	"context"
	"embed"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/colonyops/hive/cmd/desktop/internal/adapter/httpapi"
	"github.com/colonyops/hive/cmd/desktop/internal/adapter/mcpsrv"
	"github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui"
	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/agentws"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
	"github.com/colonyops/hive/cmd/desktop/internal/app/report"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
	"github.com/colonyops/hive/cmd/desktop/internal/app/telemetry"
	configmigrate "github.com/colonyops/hive/internal/config/migrate"
	"github.com/colonyops/hive/internal/platform/credentials"
	"github.com/colonyops/hive/internal/platform/execenv"
	"github.com/colonyops/hive/internal/platform/observe"
	"github.com/colonyops/hive/internal/platform/secrets"
	"github.com/colonyops/hive/pkg/buildinfo"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

// Wails accepts a single PNG for template icons; embed the retina asset.
//
//go:embed build/icons/tray-templateTemplate@2x.png
var trayIcon []byte

// The same mark rendered white: Linux panels take raw pixmaps, not tintable
// templates (see wailsui.applyTrayIcon).
//
//go:embed build/linux/tray-icon.png
var trayIconLinux []byte

func main() {
	bootstrap, err := settings.LoadBootstrap()
	if err != nil {
		log.Fatal(err)
	}
	paths := settings.ResolvePaths(bootstrap, settings.ResolveOptions{})
	level, err := settings.ResolveLogLevel()
	if err != nil {
		log.Fatal(err)
	}
	logger, logCloser, logErr := settings.NewLogger(paths.LogFile, level)
	if logErr != nil {
		logger.Warn().Err(logErr).Msg("desktop log file unavailable; logging to stderr only")
	}

	backupDir := filepath.Join(paths.StateDir, "migration-backups")
	if _, _, err := configmigrate.MigrateFile(configmigrate.SettingsSet, paths.SettingsPath, backupDir, &logger); err != nil {
		log.Fatal(err) // preserve settings' fail-startup semantics
	}

	settingsStore := settings.NewStore(paths.SettingsPath)
	cfg, err := settingsStore.Effective()
	if err != nil {
		log.Fatal(err)
	}
	if err := cfg.UnknownKeys(); err != nil {
		logger.Warn().Err(err).Msg("settings.yaml has keys this build ignores")
	}
	// Cancelled by shutdown rather than deferred: log.Fatal below would skip a
	// defer, and shutdown is the one path both exits take.
	ctx, cancel := context.WithCancel(context.Background())

	// Mock mode can select an isolated flows directory, so finalize the path
	// snapshot only after settings and environment precedence are resolved.
	initialLogPath := paths.LogFile
	// The resolver is shared with the app so the login shell is probed once.
	execEnv := execenv.NewResolver(execenv.Options{Logger: logger})
	paths = settings.ResolvePaths(bootstrap, settings.ResolveOptions{
		MockMode:           cfg.MockMode(),
		AgentWorkspacesDir: cfg.AgentWorkspaces.Dir,
		Getenv:             func(name string) string { return execEnv.Getenv(ctx, name) },
	})

	build := buildinfo.Resolve(version, commit, date)
	environment := telemetryEnvironment(build.Version)

	// Built before the final logger because its log bridge is one of that
	// logger's writer arms. A bad configuration disables telemetry rather than
	// failing startup: nothing else depends on it.
	telOpts := telemetry.Options{
		Export: cfg.Telemetry.Enabled,
		Profiles: telemetry.ProfilesOptions{
			Enabled:     cfg.Telemetry.Profiles.Enabled,
			HTTPTimeout: telemetryFlushGrace,
		},
		Scrape:      cfg.Development.Metrics.Enabled,
		Version:     build.Version,
		Environment: environment,
		HostID:      cfg.Telemetry.HostID,
	}
	// Only when export is on: resolving a reference can prompt for approval,
	// which a disabled section must never do.
	if cfg.Telemetry.Enabled {
		telOpts.Endpoint = resolveSetting("telemetry.endpoint", cfg.Telemetry.Endpoint, &logger)
		telOpts.User = resolveSetting("telemetry.instance_id", cfg.Telemetry.InstanceID, &logger)
		telOpts.Token = resolveSetting("telemetry.token", cfg.Telemetry.Token, &logger)
	}
	if cfg.Telemetry.Profiles.Enabled {
		telOpts.Profiles.Endpoint = resolveSetting("telemetry.profiles.endpoint", cfg.Telemetry.Profiles.Endpoint, &logger)
		telOpts.Profiles.User = resolveSetting("telemetry.profiles.user", cfg.Telemetry.Profiles.User, &logger)
		telOpts.Profiles.Token = resolveSetting("telemetry.profiles.token", cfg.Telemetry.Profiles.Token, &logger)
	}
	tel, telErr := telemetry.New(ctx, telOpts)
	if telErr != nil {
		tel = telemetry.Off()
	}

	if paths.LogFile != initialLogPath || len(tel.LogWriters()) > 0 {
		logCloser()
		logger, logCloser, logErr = settings.NewLogger(paths.LogFile, level, tel.LogWriters()...)
		if logErr != nil {
			logger.Warn().Err(logErr).Msg("desktop log file unavailable; logging to stderr only")
		}
	}
	switch {
	case telErr != nil:
		logger.Error().Err(telErr).Msg("telemetry is configured but unusable; continuing without it")
	case tel.Enabled():
		logger.Info().
			Bool("export", cfg.Telemetry.Enabled).
			Bool("profiles", cfg.Telemetry.Profiles.Enabled).
			Bool("scrape", cfg.Development.Metrics.Enabled).
			Str("environment", environment).
			Str("version", build.Version).
			Msg("telemetry enabled")
	}

	settingsStore = settings.NewStore(paths.SettingsPath)
	backupDir = filepath.Join(paths.StateDir, "migration-backups")

	// Migrate flows/*.yaml and actions.yml in place before app.New constructs the
	// stores and starts the watchers. Non-fatal: mirror each type's last-good
	// semantics rather than failing startup.
	if err := flow.MigrateDir(paths.FlowsDir, backupDir, &logger); err != nil {
		logger.Warn().Err(err).Msg("flow migration sweep failed")
	}
	if _, _, err := configmigrate.MigrateFile(configmigrate.ActionsSet, paths.ActionsPath, backupDir, &logger); err != nil {
		logger.Warn().Err(err).Msg("actions.yml migration failed; using last-good")
	}
	if err := agentws.MigrateRoot(paths.AgentWorkspacesDir, backupDir, &logger); err != nil {
		logger.Warn().Err(err).Msg("agent workspace migration sweep failed; using last-good")
	}

	// A redirected API base means every item this run shows may be stale or
	// deliberately rewritten by cmd/desktop/devserver. That is invisible in the UI, so
	// it is worth a line in the log before anything fetches.
	if base := cfg.GitHubAPIBase(); base != "" {
		logger.Warn().
			Str("api_base", base).
			Bool("from_env", cfg.EnvironmentOverridden(settings.EnvGitHubAPIBase)).
			Msg("GitHub API base overridden; not talking to api.github.com")
	}

	// One span per startup phase, so "the app is slow to open" resolves to
	// which phase without further instrumentation.
	startupCtx, startupSpan := tracer.Start(ctx, "app.startup", trace.WithAttributes(
		attribute.String("build.commit", build.Commit),
		attribute.String("build.date", build.Date),
	))

	// The adapter is built first because the core takes two driven ports from
	// it — where a notification is delivered, and whether it may be.
	ui := wailsui.New(cfg.MockMode(), settingsStore, appIcon, logger)

	_, coreSpan := tracer.Start(startupCtx, "app.core.new")
	telemetryRuntime := app.TelemetryRuntime{
		OTLPRunning:     telErr == nil && cfg.Telemetry.Enabled,
		ProfilesRunning: telErr == nil && cfg.Telemetry.Profiles.Enabled,
	}
	if telErr != nil {
		telemetryRuntime.StartError = telErr.Error()
	}
	core, err := app.New(ctx, app.Config{
		Settings:                 cfg,
		SettingsStore:            settingsStore,
		Paths:                    paths,
		MockMode:                 cfg.MockMode(),
		Logger:                   logger,
		ExecEnv:                  execEnv,
		CredentialKeyringService: os.Getenv(credentials.EnvKeyringService),
		Notifier:                 ui.Notifier(),
		Gate:                     ui.Gate(),
		Build:                    report.Build{Version: build.Version, Commit: build.Commit, Date: build.Date},
		TelemetryRuntime:         telemetryRuntime,
	})
	coreSpan.End()
	if err != nil {
		log.Fatal(err)
	}
	ui.SeedMock(startupCtx, core)

	// The terminal-guarded surfaces are the parts of the API that authenticate,
	// so their token and CORS allowlist are minted here and handed to the
	// adapters that need them — the core carries neither (ADR terminal-transport). One token
	// covers all three, because the agent control plane and the PTY stream a
	// workspace session rides sit under the same token-guarded /api/terminal/
	// prefix as the tmux/pop-up terminal surface (ADR a-workspace-declares-its-own-authority).
	terminalToken, err := httpapi.MintTerminalToken()
	if err != nil {
		log.Fatal(err)
	}
	origins := webviewOrigins()

	// The agent HTTP API shares the loopback HTTP server with the webhook
	// listener (ADR agent-http-api); mount it before Start whenever that server is up.
	if core.MountAPI(httpapi.PathPrefix, httpapi.New(core, logger, httpapi.Options{
		TerminalToken: terminalToken,
		Origins:       origins,
	}).Handler()) {
		logger.Info().Msg("agent HTTP API mounted at /api/")
	}

	// The MCP server is the agent-facing surface (ADR mcp-replaces-the-agent-facing-http-api); what stays on
	// /api/ is the frontend's terminal control planes and the liveness probe.
	// It needs no token — it spawns nothing — and no teardown branch: the
	// server is stateless, so no session outlives a request.
	if core.MountAPI(mcpsrv.PathPrefix, mcpsrv.New(core, logger, mcpsrv.Options{Version: build.Version}).Handler()) {
		logger.Info().Str("path", mcpsrv.PathPrefix).Msg("agent MCP server mounted")
	}
	// The canvas MCP server is a separate mount and catalogue entry, so a
	// workspace can enable the canvas without the app-control tool set
	// (ADR canvases-are-named-files-in-the-workspace-folder-served-over-their-own-mcp-entry).
	if core.MountAPI(mcpsrv.CanvasPathPrefix, mcpsrv.NewCanvas(core, logger, mcpsrv.Options{Version: build.Version}).Handler()) {
		logger.Info().Str("path", mcpsrv.CanvasPathPrefix).Msg("canvas MCP server mounted")
	}
	terminal := wailsui.TerminalTransport{}
	popupTerminal := wailsui.PopupTerminalTransport{}
	agents := wailsui.AgentsTransport{}
	// A workspace session rides this same tmux stream a hive session's terminal
	// does — it is a tmux session too, just not a hive one
	// (ADR agent-workspace-sessions-are-tmux-sessions) — addressed by the agentws-<terminal id> name AgentWorkspacesService
	// gives it rather than a hive slug. There is no agent-specific stream.
	if path, handler := httpapi.TerminalStreamHandler(core, terminalToken, origins, logger); core.MountAPI(path, handler) {
		terminal = wailsui.TerminalTransport{Token: terminalToken, StreamPath: path}
		agents = wailsui.AgentsTransport{Token: terminalToken, StreamPath: path}
		logger.Info().Str("path", path).Msg("terminal WebSocket stream mounted")
	}
	// The ptyterm data plane. It carries one terminal per socket rather than a
	// session's window set (ADR terminal-renderer-claimed-on-activation), and is addressed by an id a caller may
	// supply as well as one this process mints (ADR ptyterm-terminals-are-caller-addressed). Pop-ups only — an
	// agent workspace session rides the tmux stream above since ADR agent-workspace-sessions-are-tmux-sessions.
	if path, handler := httpapi.PTYStreamHandler(core, terminalToken, origins, logger); core.MountAPI(path, handler) {
		popupTerminal = wailsui.PopupTerminalTransport{Token: terminalToken, StreamPath: path}
		logger.Info().Str("path", path).Msg("ptyterm WebSocket stream mounted")
	}
	// pprof shares the same server when enabled (ADR pprof-debug-endpoint).
	if cfg.Development.Pprof.Enabled && core.MountAPI(httpapi.PprofPathPrefix, httpapi.PprofHandler(tel.CPUProfileHandler())) {
		logger.Info().Str("path", httpapi.PprofPathPrefix).Msg("pprof debug endpoint mounted")
	}
	// The metrics scrape rides the same server on the same terms.
	if h := tel.MetricsHandler(); h != nil && core.MountAPI(telemetry.MetricsPath, h) {
		logger.Info().Str("path", telemetry.MetricsPath).Msg("metrics endpoint mounted")
	}

	_, mountSpan := tracer.Start(startupCtx, "app.ui.mount")
	ui.Mount(ctx, core, wailsui.MountOptions{
		Assets:        assets,
		AppIcon:       appIcon,
		TrayIcon:      trayIcon,
		TrayIconLinux: trayIconLinux,
		Build:         wailsui.Build{Version: build.Version, Commit: build.Commit, Date: build.Date},
		Terminal:      terminal,
		PopupTerminal: popupTerminal,
		Agents:        agents,
		AutoUpdate:    cfg.Updates.Enabled,
		UpdateChannel: func(buildChannel string) string {
			if cfg.Updates.Channel == "" {
				return buildChannel
			}
			return cfg.Updates.Channel
		},
	})

	mountSpan.End()

	// Background work starts after the adapter is mounted: the flows watcher
	// calls event subscribers from its own goroutine, and the tray subscriber
	// has to exist before it can fire.
	_, startSpan := tracer.Start(startupCtx, "app.core.start")
	err = core.Start(ctx)
	startSpan.End()
	startupSpan.End()
	if err != nil {
		log.Fatal(err)
	}

	// Registered as a shutdown hook and called again after Run, because which
	// of the two fires is the platform's business: on macOS Quit is [NSApp
	// terminate:], which runs the hooks and exits without Run ever returning,
	// while the server build returns from Run normally. Once, so the pair is
	// exactly one teardown.
	shutdown := sync.OnceFunc(func() {
		ui.Close()
		cancel()
		if err := core.Close(); err != nil {
			logger.Warn().Err(err).Msg("core shutdown reported an error")
		}
		// After the core, so a shutdown log line still reaches the exporter, and
		// on its own context because ctx is already cancelled.
		flushCtx, flushCancel := context.WithTimeout(context.Background(), telemetryFlushGrace)
		if err := tel.Shutdown(flushCtx); err != nil {
			logger.Warn().Err(err).Msg("telemetry shutdown reported an error")
		}
		flushCancel()
		logCloser()
	})
	ui.OnShutdown(shutdown)
	quitOnSignal(ui.Quit, logger, logCloser)

	if err := ui.Run(); err != nil {
		shutdown()
		log.Fatal(err)
	}
	shutdown()
}

// tracer emits the startup trace. It resolves against the global provider, so
// it is valid at init and upgrades in place once telemetry.New registers one.
var tracer = observe.Tracer("/desktop")

// telemetryFlushGrace is short on purpose: an unreachable backend must not be
// able to hold up quitting, and losing the last batch costs less than a hang.
const telemetryFlushGrace = 2 * time.Second

// resolveSetting reads a value that may be a secret reference. A literal
// resolves to itself, so this is safe on a setting that is ordinarily written
// out. A failure resolves to empty, which telemetry reports as a missing
// setting rather than failing startup.
func resolveSetting(name, ref string, logger *zerolog.Logger) string {
	if ref == "" {
		return ""
	}
	value, err := secrets.Resolve(ref)
	if err != nil {
		logger.Error().Err(err).Str("setting", name).Msg("telemetry setting could not be resolved")
		return ""
	}
	return value
}

// telemetryEnvironment separates a working tree's signals from a release's. A
// published build reports its release channel; a plain `go build`, a dev-task
// binary, or a pseudo-version reports "source".
func telemetryEnvironment(version string) string {
	if channel, ok := wailsui.ReleaseChannel(version); ok {
		return channel
	}
	return "source"
}

// shutdownGrace bounds a signal-triggered teardown end to end. App.Close
// budgets three seconds each for the terminal clients and the HTTP drain and
// then closes two databases, so this is that worst case with headroom — not a
// target anything is expected to reach.
const shutdownGrace = 10 * time.Second

// quitOnSignal answers SIGINT, SIGTERM and SIGHUP by asking for the same quit
// the tray's Quit item asks for, which reaches the teardown above by whichever
// of its two routes the platform takes. Without it the process dies on Go's
// default disposition with the databases mid-write and the tmux control
// clients still attached.
//
// os/signal is wired here rather than in cmd/desktop/internal/app because the disposition
// belongs to the process, not the core — App.Close documents why the core must
// register none of its own.
//
// The watchdog is the part that earns its keep. quit hands work to the main
// thread and the teardown then joins terminal clients and drains an HTTP
// server, so either can wedge; past the grace period, or on a second signal,
// the process exits anyway. An app you cannot kill is worse than one that
// skipped its teardown.
func quitOnSignal(quit func(), logger zerolog.Logger, flush func()) {
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		sig := <-signals
		logger.Info().Str("signal", sig.String()).Msg("signal received; shutting down")

		go func() {
			select {
			case next := <-signals:
				logger.Warn().Str("signal", next.String()).Msg("second signal; exiting without finishing shutdown")
			case <-time.After(shutdownGrace):
				logger.Error().Dur("grace", shutdownGrace).Msg("shutdown did not finish in time; exiting")
			}
			flush()
			os.Exit(1)
		}()

		quit()
	}()
}

// webviewOrigins is the CORS allowlist for the terminal surface: the packaged
// webview's own origin, plus the dev servers when they are running. In dev the
// webview may load from the Vite server or from the Wails dev server that
// proxies it, and Wails builds its URLs with localhost while the servers bind
// 127.0.0.1 — so both hosts are listed for both ports. All are loopback; the
// bearer token is the actual gate.
func webviewOrigins() []string {
	origins := []string{"wails://localhost"}
	for _, portKey := range []string{"WAILS_VITE_PORT", "WAILS_SERVER_PORT"} {
		port := os.Getenv(portKey)
		if port == "" {
			continue
		}
		for _, host := range []string{"localhost", "127.0.0.1"} {
			origins = append(origins, "http://"+host+":"+port)
		}
	}
	return origins
}
