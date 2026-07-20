package main

import (
	"context"
	"embed"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/commands"
	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/eventbus"
	"github.com/colonyops/hive/internal/core/git"
	coredb "github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/data/stores"
	"github.com/colonyops/hive/internal/desktop"
	"github.com/colonyops/hive/internal/desktop/auth"
	"github.com/colonyops/hive/internal/desktop/feed"
	"github.com/colonyops/hive/internal/desktop/pipeline"
	"github.com/colonyops/hive/internal/desktop/pipeline/actions"
	"github.com/colonyops/hive/internal/desktop/pipeline/flow"
	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
	"github.com/colonyops/hive/internal/github"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/scripts"
	"github.com/colonyops/hive/pkg/executil"
	"github.com/colonyops/hive/pkg/tmpl"
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
	// buildFlowsStore); actions:updated fires after an actions.yml reload.
	// All are wake-up signals: the frontend re-reads the relevant service on
	// receipt.
	application.RegisterEvent[string]("auth:updated")
	application.RegisterEvent[int64]("log:appended")
	application.RegisterEvent[string]("flows:updated")
	application.RegisterEvent[string]("actions:updated")
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
	case "feed", "pipeline":
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
// backed by a Refs adapter over actionStore. It also starts a FlowsWatcher
// that reloads the store and wakes the frontend on any flows/*.yaml change,
// including the app's own SaveFlow/SaveLayout writes. A watcher that fails to
// start degrades to no hot-reload: the app still works, edits just need a
// restart to pick up.
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

// emitActionsUpdated wakes frontend consumers after a successful catalog
// change or a watcher reload. Service mutations call it only after success.
func emitActionsUpdated() {
	if app := application.Get(); app != nil {
		app.Event.Emit("actions:updated", "changed")
	}
}

// buildActionStore constructs the actions.ActionStore over
// desktop.ActionsPath(), loading it eagerly (rather than waiting for the
// first lazy List/Get) so a broken actions.yml is logged at startup instead
// of only surfacing silently as "no actions found" the first time something
// asks. It also starts an ActionsWatcher so hand edits to actions.yml apply
// live, matching flows hot-reload posture. A watcher that fails to start
// degrades to no hot-reload: the app still works, edits just need a restart
// to pick up.
func buildActionStore(logger zerolog.Logger) (*actions.ActionStore, *actions.ActionsWatcher) {
	path := desktop.ActionsPath()
	if _, err := actions.SeedDefaultsIfMissing(path); err != nil {
		logger.Warn().Err(err).Msg("actions seed failed")
	}
	store := actions.NewActionStore(path)
	if err := store.Reload(); err != nil {
		logger.Warn().Err(err).Msg("actions.yml load failed; using last-good (likely empty) action set")
	}

	watcher, err := actions.NewActionsWatcher(path, func() {
		if err := store.Reload(); err != nil {
			logger.Warn().Err(err).Msg("actions.yml reload failed")
		}
		emitActionsUpdated()
	}, logger)
	if err != nil {
		logger.Warn().Err(err).Msg("actions.yml hot-reload unavailable")
		return store, nil
	}
	return store, watcher
}

// hiveActionRuntime owns the Hive dependencies needed by desktop actions.
// The desktop pipeline keeps its own database, while sessions and internal
// events intentionally use Hive's shared state and event bus.
type hiveActionRuntime struct {
	db     *coredb.DB
	cancel context.CancelFunc

	launcher  pipeline.SessionLauncher
	publisher pipeline.EventPublisher
}

func (r *hiveActionRuntime) Close() {
	r.cancel()
	if err := r.db.Close(); err != nil {
		log.Printf("close hive action database: %v", err)
	}
}

func buildHiveActionRuntime(logger zerolog.Logger) (*hiveActionRuntime, error) {
	dataDir := filepath.Dir(desktop.StateDir())
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create hive data directory: %w", err)
	}

	configPath := os.Getenv("HIVE_CONFIG")
	if configPath == "" {
		configPath = commands.DefaultConfigPath()
	}
	cfg, err := config.Load(configPath, dataDir)
	if err != nil {
		return nil, fmt.Errorf("load hive config for actions: %w", err)
	}
	if err := scripts.EnsureExtracted(dataDir, "desktop"); err != nil {
		logger.Warn().Err(err).Msg("extract hive action scripts failed")
	}

	database, err := coredb.Open(dataDir, coredb.OpenOptions{
		MaxOpenConns: cfg.Database.MaxOpenConns,
		MaxIdleConns: cfg.Database.MaxIdleConns,
		BusyTimeout:  cfg.Database.BusyTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("open hive action database: %w", err)
	}
	if err := stores.MigrateFromJSON(context.Background(), database, dataDir); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("migrate hive action data: %w", err)
	}

	bus := eventbus.New(64)
	busCtx, cancel := context.WithCancel(context.Background())
	go bus.Start(busCtx)

	profile := cfg.Agents.DefaultProfile()
	renderer := tmpl.New(tmpl.Config{
		ScriptPaths:  scripts.ScriptPaths(dataDir),
		AgentCommand: profile.CommandOrDefault(cfg.Agents.Default),
		AgentWindow:  cfg.Agents.Default,
		AgentFlags:   profile.ShellFlags(),
	})
	exec := &executil.RealExecutor{}
	sessions := hive.NewSessionService(
		stores.NewSessionStore(database),
		git.NewExecutor(cfg.GitPath, exec),
		cfg,
		bus,
		exec,
		renderer,
		logger.With().Str("component", "hive-actions").Logger(),
		io.Discard,
		io.Discard,
	)

	return &hiveActionRuntime{
		db:        database,
		cancel:    cancel,
		launcher:  pipeline.NewHiveSessionLauncher(sessions),
		publisher: pipeline.NewEventBusPublisher(bus),
	}, nil
}

// buildOutputWorker constructs the output worker over db and actionStore.
// Mock modes do not start its background loop because no fixture flow emits
// output commands, but they retain this worker for explicit detail-pane
// confirmation RPCs. That keeps the configured action path real in e2e while
// avoiding a background shell action from compromising fixture determinism.
func buildOutputWorker(db *pipelinedb.DB, actionStore *actions.ActionStore, launcher pipeline.SessionLauncher, publisher pipeline.EventPublisher, logger zerolog.Logger) *pipeline.Worker {
	dispatcher := pipeline.NewDispatcher(map[string]pipeline.Executor{
		"launch-session": pipeline.NewLaunchSessionExecutor(launcher),
		"shell":          pipeline.NewShellExecutor(logger),
		"publish-event":  pipeline.NewPublishEventExecutor(publisher),
	})
	return pipeline.NewWorker(db, actionStore, dispatcher, pipeline.DefaultOutputWorkerInterval, logger)
}

func main() {
	fetcher := buildSourceFetcher()

	pipelineLogger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	pipelineDB, err := pipelinedb.Open(desktop.StateDir(), pipelinedb.DefaultOpenOptions())
	if err != nil {
		log.Fatal(err)
	}
	actionRuntime, err := buildHiveActionRuntime(pipelineLogger)
	if err != nil {
		log.Fatal(err)
	}

	// Mock "feed" mode has no live producer (buildSourceFetcher/
	// buildPipelineProducer both no-op in mock mode), so the sidebar would
	// otherwise be empty offline: seed a fixed set of feed_item rows for the
	// fixture flow desktop/e2e/fixtures/flows/frontend-triage.yaml serves
	// (see desktop/mockseed.go and desktop/e2e/scripts/serve.sh's
	// HIVE_DESKTOP_FLOWS).
	if desktop.MockMode() == "feed" {
		seedMockFeedItemsOrWarn(pipelineDB, pipelineLogger)
	}

	actionsLogger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	actionStore, actionsWatcher := buildActionStore(actionsLogger)
	if actionsWatcher != nil {
		actionsWatcher.Start()
	}

	// The flows store must exist before the producer and retention maintenance:
	// both resolve enabled flow IDs live from it.
	flowsLogger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	flowsStore, flowsWatcher := buildFlowsStore(actionStore, flowsLogger)
	actionStore.SetUsageChecker(actionUsageChecker{flows: flowsStore, db: pipelineDB})
	if flowsWatcher != nil {
		flowsWatcher.Start()
	}

	outputWorker := buildOutputWorker(pipelineDB, actionStore, actionRuntime.launcher, actionRuntime.publisher, pipelineLogger)
	if desktop.MockMode() == "" {
		outputWorker.Start()
	}

	maintenance := pipeline.NewMaintenance(
		pipelineDB,
		flowsStore,
		pipelinedb.DefaultRetentionPolicy(),
		pipeline.DefaultRetentionInterval,
		pipelineLogger,
	)
	maintenance.Start()

	producer := buildPipelineProducer(pipelineDB, fetcher, flowsStore, pipelineLogger)
	if producer != nil {
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
			application.NewService(NewPipelineService(pipelineDB, actionStore, outputWorker)),
			application.NewService(NewFlowsService(flowsStore)),
			application.NewService(NewActionsService(actionStore, emitActionsUpdated)),
		},
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(assets),
			Middleware: sourceToCommitSmokeMiddleware(pipelineDB),
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

	shutdown := func() {
		maintenance.Stop()
		actionRuntime.Close()
	}
	if err := app.Run(); err != nil {
		shutdown()
		log.Fatal(err)
	}
	shutdown()
}
