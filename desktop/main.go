package main

import (
	"embed"
	"log"
	"os"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/desktop"
	"github.com/colonyops/hive/internal/desktop/auth"
	"github.com/colonyops/hive/internal/desktop/feed"
	"github.com/colonyops/hive/internal/desktop/pipeline"
	"github.com/colonyops/hive/internal/desktop/pipeline/actions"
	"github.com/colonyops/hive/internal/desktop/pipeline/flow"
	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
	"github.com/colonyops/hive/internal/github"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

// Wails accepts a single PNG for template icons; embed the retina asset.
//
//go:embed build/icons/tray-templateTemplate@2x.png
var trayIcon []byte

// Package-variable initialization instead of init(): this repo enables
// gochecknoinits.
var _ = registerEvents()

func registerEvents() struct{} {
	// feed:updated carries the profile ID whose data changed; auth:updated
	// carries the new auth state string; config:updated carries "ok" or the
	// config error text after a profiles-config reload; log:appended carries
	// the pipeline event log's new tail offset after a producer tick appends
	// at least one row; flows:updated fires after a flows/*.yaml directory
	// reload (an external edit, or the app's own SaveFlow/SaveLayout — see
	// buildFlowsStore). All are wake-up signals: the frontend re-reads the
	// relevant service on receipt. flows:updated is a wake-up only: the
	// frontend graph runtime performs the actual drain-then-swap of a
	// running flow on receipt (Phase 6) — this phase just keeps
	// FlowsService's view of flows/*.yaml current.
	application.RegisterEvent[string]("feed:updated")
	application.RegisterEvent[string]("auth:updated")
	application.RegisterEvent[string]("config:updated")
	application.RegisterEvent[int64]("log:appended")
	application.RegisterEvent[string]("flows:updated")
	return struct{}{}
}

// buildFeedProvider returns the provider plus, in live mode, a poller that
// pushes feed:updated when a background refresh finds changes and a watcher
// that hot-reloads the profiles config on external edits. Mock modes serve
// static fixtures and need neither.
func buildFeedProvider() (feed.Provider, *feed.Poller, *feed.ConfigWatcher) {
	switch desktop.MockMode() {
	case "feed":
		return feed.NewMockProvider(), nil, nil
	case "onboarding":
		// Empty start so e2e walks the whole first run: auth, then
		// workspace creation, then the fixture feed.
		return feed.NewEmptyMockProvider(), nil, nil
	default:
		logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
		store := feed.NewStore(desktop.ConfigPath(), desktop.StateDir())
		provider := feed.NewLiveProvider(github.NewClient(), github.NewKeychainStore(), store, logger)
		poller := feed.NewPoller(provider, feed.DefaultPollInterval, emitFeedUpdated, logger)
		watcher, err := feed.NewConfigWatcher(desktop.ConfigPath(), func() {
			reloadConfig(store, provider)
		}, logger)
		if err != nil {
			// The app works without hot reload; edits then need a restart.
			logger.Warn().Err(err).Msg("profiles config hot-reload unavailable")
		}
		return provider, poller, watcher
	}
}

// reloadConfig re-reads the profiles config after an on-disk change, drops
// the fetch cache (feed definitions may have changed), and wakes the
// frontend with the outcome. A broken config keeps the last-good profiles;
// the error text rides the event so the UI can say what is wrong.
func reloadConfig(store *feed.Store, provider *feed.LiveProvider) {
	status := "ok"
	if err := store.Reload(); err != nil {
		status = err.Error()
	}
	provider.Invalidate()
	if app := application.Get(); app != nil {
		app.Event.Emit("config:updated", status)
	}
}

// emitFeedUpdated pushes the changed profile's ID to the frontend. Safe to
// call from the poller goroutine once the app is running.
func emitFeedUpdated(profileID string) {
	if app := application.Get(); app != nil {
		app.Event.Emit("feed:updated", profileID)
	}
}

// buildPipelineProducer starts the pipeline event-log producer over
// provider's configured sources, or returns nil when there is nothing to
// poll. Mock modes ("feed", "onboarding") skip it: they serve static
// fixtures with no ticking network fetch, and starting a producer there
// would make e2e non-deterministic for no benefit — this phase ships no
// mock Source. Live mode requires provider to be a *feed.LiveProvider,
// which it always is outside mock modes (see buildFeedProvider).
func buildPipelineProducer(db *pipelinedb.DB, provider feed.Provider, logger zerolog.Logger) *pipeline.Producer {
	if desktop.MockMode() != "" {
		return nil
	}
	live, ok := provider.(*feed.LiveProvider)
	if !ok {
		return nil
	}
	return pipeline.NewProducer(db, pipeline.NewGithubSourceLister(live), feed.DefaultPollInterval, emitLogAppended, logger)
}

// emitLogAppended pushes the pipeline event log's new tail offset to the
// frontend after a producer tick appends at least one row. Safe to call
// from the producer goroutine once the app is running.
func emitLogAppended(nextOffset int64) {
	if app := application.Get(); app != nil {
		app.Event.Emit("log:appended", nextOffset)
	}
}

func buildAuthBackend(onChange func()) auth.Backend {
	switch desktop.MockMode() {
	case "feed":
		return auth.NewMockBackend(true, onChange)
	case "onboarding":
		return auth.NewMockBackend(false, onChange)
	default:
		return auth.NewLiveBackend(github.NewClient(), github.NewKeychainStore(), onChange)
	}
}

// emitAuthUpdated pushes the auth:updated wake-up to the frontend. Safe to
// call from any goroutine once the app is running.
func emitAuthUpdated() {
	if app := application.Get(); app != nil {
		app.Event.Emit("auth:updated", "changed")
	}
}

// buildFlowsStore constructs the flow.FlowStore over desktop.FlowsDir(),
// backed by a Refs adapter over provider and actionStore — this works
// uniformly across mock and live feed providers, so unlike buildFeedProvider
// there is no mock-mode branch here: an empty/tmp flows dir is deterministic
// on its own, and the store never touches GitHub or auth state. It also
// starts a FlowsWatcher that reloads the store and wakes the frontend on any
// flows/*.yaml change, including the app's own SaveFlow/SaveLayout writes
// (the same self-triggering tradeoff feed.ConfigWatcher makes). A watcher
// that fails to start degrades to no hot-reload, matching feed's posture:
// the app still works, edits just need a restart to pick up.
func buildFlowsStore(provider feed.Provider, actionStore *actions.ActionStore, logger zerolog.Logger) (*flow.FlowStore, *flow.FlowsWatcher) {
	dir := desktop.FlowsDir()
	store := flow.NewFlowStore(dir, newFlowRefsAdapter(provider, actionStore))

	watcher, err := flow.NewFlowsWatcher(dir, func() {
		if err := store.Reload(); err != nil {
			logger.Warn().Err(err).Msg("flows reload failed")
		}
		emitFlowsUpdated()
	}, logger)
	if err != nil {
		logger.Warn().Err(err).Msg("flows hot-reload unavailable")
		return store, nil
	}
	return store, watcher
}

// emitFlowsUpdated pushes the flows:updated wake-up to the frontend. Safe to
// call from any goroutine once the app is running.
func emitFlowsUpdated() {
	if app := application.Get(); app != nil {
		app.Event.Emit("flows:updated", "changed")
	}
}

// buildActionStore constructs the actions.ActionStore over
// desktop.ActionsPath(), loading it eagerly (rather than waiting for the
// first lazy List/Get) so a broken actions.yml is logged at startup instead
// of only surfacing silently as "no actions found" the first time something
// asks. It also starts a file watcher (reusing feed.ConfigWatcher, which
// already does exactly this for a single config file) so hand edits to
// actions.yml apply live, matching flows'/profiles' hot-reload posture. A
// watcher that fails to start degrades to no hot-reload: the app still
// works, edits just need a restart to pick up.
func buildActionStore(logger zerolog.Logger) (*actions.ActionStore, *feed.ConfigWatcher) {
	path := desktop.ActionsPath()
	store := actions.NewActionStore(path)
	if err := store.Reload(); err != nil {
		logger.Warn().Err(err).Msg("actions.yml load failed; using last-good (likely empty) action set")
	}

	watcher, err := feed.NewConfigWatcher(path, func() {
		if err := store.Reload(); err != nil {
			logger.Warn().Err(err).Msg("actions.yml reload failed")
		}
	}, logger)
	if err != nil {
		logger.Warn().Err(err).Msg("actions.yml hot-reload unavailable")
		return store, nil
	}
	return store, watcher
}

// buildOutputWorker constructs the output worker over db and actionStore, or
// returns nil in mock mode. Mock modes ("feed", "onboarding") skip it,
// mirroring buildPipelineProducer: they serve static fixtures with no live
// producer feeding the event log, so there is nothing for a frontend graph
// run to commit and no output_command would ever be enqueued in practice —
// but skipping the worker outright keeps e2e fully deterministic rather than
// relying on that being true (e.g. against a shell action actually running
// a command).
//
// launch-session and publish-event get logging stubs (see
// LaunchSessionExecutor/PublishEventExecutor's package docs for why: the
// desktop app doesn't wire the hive session service or a real event bus
// yet). shell gets a real executor — running an author-trusted local
// command has no such missing dependency.
func buildOutputWorker(db *pipelinedb.DB, actionStore *actions.ActionStore, logger zerolog.Logger) *pipeline.Worker {
	if desktop.MockMode() != "" {
		return nil
	}
	dispatcher := pipeline.NewDispatcher(map[string]pipeline.Executor{
		"launch-session": pipeline.NewLaunchSessionExecutor(nil),
		"shell":          pipeline.NewShellExecutor(logger),
		"publish-event":  pipeline.NewPublishEventExecutor(nil),
	})
	return pipeline.NewWorker(db, actionStore, dispatcher, pipeline.DefaultOutputWorkerInterval, logger)
}

func main() {
	// The poller, watcher, and pipeline producer live for the whole
	// process; they die with it, so there are no Stop/Close calls here (and
	// log.Fatal below would skip a defer anyway).
	provider, poller, watcher := buildFeedProvider()
	if poller != nil {
		poller.Start()
	}
	if watcher != nil {
		watcher.Start()
	}

	pipelineLogger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	pipelineDB, err := pipelinedb.Open(desktop.StateDir(), pipelinedb.DefaultOpenOptions())
	if err != nil {
		log.Fatal(err)
	}
	if producer := buildPipelineProducer(pipelineDB, provider, pipelineLogger); producer != nil {
		producer.Start()
	}

	actionsLogger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	actionStore, actionsWatcher := buildActionStore(actionsLogger)
	if actionsWatcher != nil {
		actionsWatcher.Start()
	}
	if outputWorker := buildOutputWorker(pipelineDB, actionStore, pipelineLogger); outputWorker != nil {
		outputWorker.Start()
	}

	flowsLogger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	flowsStore, flowsWatcher := buildFlowsStore(provider, actionStore, flowsLogger)
	if flowsWatcher != nil {
		flowsWatcher.Start()
	}

	// Every auth transition drops the feed cache before the frontend is
	// notified: a different account must never be served items fetched with
	// the previous token.
	onAuthChange := func() {
		if live, ok := provider.(*feed.LiveProvider); ok {
			live.Invalidate()
		}
		emitAuthUpdated()
	}

	options := application.Options{
		Name:        "Hive",
		Description: "Hive desktop application",
		Icon:        appIcon,
		Services: []application.Service{
			application.NewService(NewFeedService(provider)),
			application.NewService(auth.NewService(buildAuthBackend(onAuthChange))),
			application.NewService(NewPipelineService(pipelineDB)),
			application.NewService(NewFlowsService(flowsStore)),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ActivationPolicy: application.ActivationPolicyRegular,
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	}
	app := application.New(options)

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Hive",
		Width:            1360,
		Height:           864,
		BackgroundColour: application.NewRGB(24, 26, 31),
		URL:              "/",
		Mac: application.MacWindow{
			// HiddenInset with an explicit compact toolbar style: the default
			// (Automatic) lets AppKit pick the toolbar height, which drifts
			// across macOS versions. UnifiedCompact pins it — 42pt as measured
			// on macOS Tahoe — so the traffic lights center on the 42px HTML
			// titlebar.
			TitleBar: application.MacTitleBar{
				AppearsTransparent:   true,
				HideTitle:            true,
				FullSizeContent:      true,
				UseToolbar:           true,
				HideToolbarSeparator: true,
				ToolbarStyle:         application.MacToolbarStyleUnifiedCompact,
			},
			InvisibleTitleBarHeight: 42,
		},
	})

	// Closing the window keeps the app running in the dock and tray; it can be
	// reopened from either. Quitting is done via Cmd+Q or the tray menu.
	// This must be a hook, not OnWindowEvent: hooks run synchronously before
	// listeners, so Cancel() reliably aborts Wails' own window-destroy listener,
	// which otherwise races this callback in a separate goroutine.
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		window.Hide()
		e.Cancel()
	})

	app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(*application.ApplicationEvent) {
		window.Show()
	})

	trayMenu := app.NewMenu()
	trayMenu.Add("Show Hive").OnClick(func(*application.Context) {
		window.Show()
		window.Focus()
	})
	trayMenu.AddSeparator()
	trayMenu.Add("Quit").OnClick(func(*application.Context) {
		app.Quit()
	})

	app.SystemTray.New().SetTemplateIcon(trayIcon).SetMenu(trayMenu)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
