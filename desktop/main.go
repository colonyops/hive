package main

import (
	"embed"
	"fmt"
	"log"
	"os"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/desktop"
	"github.com/colonyops/hive/internal/desktop/auth"
	"github.com/colonyops/hive/internal/desktop/feed"
	"github.com/colonyops/hive/internal/desktop/migrate"
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
	// auth:updated carries the new auth state string; log:appended carries the
	// pipeline event log's new tail offset after a producer tick appends at
	// least one row; flows:updated fires after a flows/*.yaml directory reload
	// (an external edit, or the app's own SaveFlow/SaveLayout — see
	// buildFlowsStore). All are wake-up signals: the frontend re-reads the
	// relevant service on receipt.
	application.RegisterEvent[string]("auth:updated")
	application.RegisterEvent[int64]("log:appended")
	application.RegisterEvent[string]("flows:updated")
	return struct{}{}
}

// buildSourceFetcher builds the GitHub fetch layer the pipeline producer polls
// through, or nil in a mock mode (where the producer is skipped anyway — see
// buildPipelineProducer). Now that a profile is a flow, there is no profiles
// config to load or hot-reload here: source config lives in the flow's
// github-source nodes, and the producer enumerates them from the flow store.
func buildSourceFetcher() *feed.LiveProvider {
	if desktop.MockMode() != "" {
		return nil
	}
	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	return feed.NewLiveProvider(github.NewClient(), github.NewKeychainStore(), logger)
}

// buildPipelineProducer starts the pipeline event-log producer over every
// enabled github-source node across all flows (via flows), or returns nil when
// there is nothing to poll (mock mode, so fetcher is nil).
func buildPipelineProducer(db *pipelinedb.DB, fetcher *feed.LiveProvider, flows pipeline.FlowLister, logger zerolog.Logger) *pipeline.Producer {
	if fetcher == nil {
		return nil
	}
	return pipeline.NewProducer(db, pipeline.NewFlowSourceLister(fetcher, flows), feed.DefaultPollInterval, emitLogAppended, logger)
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
func buildFlowsStore(actionStore *actions.ActionStore, logger zerolog.Logger) (*flow.FlowStore, *flow.FlowsWatcher) {
	dir := desktop.FlowsDir()
	store := flow.NewFlowStore(dir, newActionsRefs(actionStore))

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

// runMigrationIfRequested handles the one-time `--migrate-profiles[=dry|write]`
// flag: it converts the legacy profiles.yaml into per-profile flows/*.yaml and
// exits, never starting the app. Inert (returns false) when the flag is absent,
// so a normal launch is unaffected. `--force` overwrites existing flow files.
func runMigrationIfRequested() bool {
	var requested, write, force bool
	for _, arg := range os.Args[1:] {
		switch arg {
		case "--migrate-profiles", "--migrate-profiles=dry":
			requested = true
		case "--migrate-profiles=write":
			requested, write = true, true
		case "--force":
			force = true
		}
	}
	if !requested {
		return false
	}
	report, err := migrate.Convert(desktop.ConfigPath(), desktop.FlowsDir(), migrate.Options{Write: write, Force: force})
	if err != nil {
		fmt.Fprintln(os.Stderr, "migrate-profiles:", err)
		os.Exit(1)
	}
	fmt.Print(report.Format())
	return true
}

func main() {
	if runMigrationIfRequested() {
		return
	}

	// The poller, watcher, and pipeline producer live for the whole
	// process; they die with it, so there are no Stop/Close calls here (and
	// log.Fatal below would skip a defer anyway).
	fetcher := buildSourceFetcher()

	pipelineLogger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	pipelineDB, err := pipelinedb.Open(desktop.StateDir(), pipelinedb.DefaultOpenOptions())
	if err != nil {
		log.Fatal(err)
	}

	actionsLogger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	actionStore, actionsWatcher := buildActionStore(actionsLogger)
	if actionsWatcher != nil {
		actionsWatcher.Start()
	}
	if outputWorker := buildOutputWorker(pipelineDB, actionStore, pipelineLogger); outputWorker != nil {
		outputWorker.Start()
	}

	// The flows store must exist before the producer: the producer enumerates
	// its github-source nodes to decide what to poll.
	flowsLogger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	flowsStore, flowsWatcher := buildFlowsStore(actionStore, flowsLogger)
	if flowsWatcher != nil {
		flowsWatcher.Start()
	}

	if producer := buildPipelineProducer(pipelineDB, fetcher, flowsStore, pipelineLogger); producer != nil {
		producer.Start()
	}

	// Every auth transition drops the fetch cache before the frontend is
	// notified: a different account must never be served items fetched with
	// the previous token.
	onAuthChange := func() {
		if fetcher != nil {
			fetcher.Invalidate()
		}
		emitAuthUpdated()
	}

	options := application.Options{
		Name:        "Hive",
		Description: "Hive desktop application",
		Icon:        appIcon,
		Services: []application.Service{
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
