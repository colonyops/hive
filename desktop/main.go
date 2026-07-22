package main

import (
	"context"
	"embed"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/commands"
	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/eventbus"
	"github.com/colonyops/hive/internal/core/git"
	coredb "github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/data/stores"
	"github.com/colonyops/hive/internal/desktop"
	"github.com/colonyops/hive/internal/desktop/activity"
	"github.com/colonyops/hive/internal/desktop/auth"
	"github.com/colonyops/hive/internal/desktop/feed"
	"github.com/colonyops/hive/internal/desktop/jobs"
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
	"github.com/wailsapp/wails/v3/pkg/updater"
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
	application.RegisterEvent[string]("jobs:updated")
	// activity:appended carries the new event's id after any subsystem (or the
	// frontend, via ActivityService.Record) appends to the activity log. The
	// Activity view re-reads its latest page and advances its unseen marker.
	application.RegisterEvent[int64]("activity:appended")
	// update:available carries the latest UpdateInfo when a self-update check
	// finds a newer desktop release; update:none fires when the check confirms
	// the app is current. The title bar reacts to update:available.
	application.RegisterEvent[UpdateInfo]("update:available")
	application.RegisterEvent[UpdateInfo]("update:none")
	return struct{}{}
}

// buildSourceFetcher builds the GitHub fetch layer the pipeline producer polls
// through, or nil in a mock mode (where the producer is skipped anyway — see
// buildPipelineProducer). Now that a profile is a flow, there is no profiles
// config to load or hot-reload here: source config lives in the flow's
// github-source nodes, and the producer enumerates them from the flow store.
func buildSourceFetcher(logger zerolog.Logger) *feed.LiveProvider {
	if desktop.MockMode() != "" {
		return nil
	}
	return feed.NewLiveProvider(github.NewClient(), github.NewKeychainStore(), logger)
}

// buildPipelineProducer starts the pipeline event-log producer over every
// enabled github-source node across all flows (via flows), or returns nil when
// there is nothing to poll (mock mode, so fetcher is nil).
func buildPipelineProducer(db *pipelinedb.DB, fetcher *feed.LiveProvider, flows pipeline.FlowLister, recorder activity.Recorder, interval time.Duration, logger zerolog.Logger) *pipeline.Producer {
	if fetcher == nil {
		return nil
	}
	producer := pipeline.NewProducer(db, pipeline.NewFlowSourceLister(fetcher, flows), interval, emitLogAppended, logger)
	producer.SetRecorder(recorder)
	producer.SetPrefetcher(fetcher)
	producer.SetSourceAdapter(pipeline.NewGithubSourceAdapter(fetcher))
	return producer
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
	case "feed", "pipeline", "action-smoke":
		return auth.NewMockBackend(true, onChange)
	case "onboarding":
		return auth.NewMockBackend(false, onChange)
	default:
		return auth.NewLiveBackend(github.NewClient(), github.NewKeychainStore(), onChange)
	}
}

// emitActivityAppended pushes the activity:appended wake-up (carrying the new
// event's id) to the frontend after any subsystem records an activity event.
// Safe to call from any goroutine once the app is running.
func emitActivityAppended(id int64) {
	if app := application.Get(); app != nil {
		app.Event.Emit("activity:appended", id)
	}
}

// emitJobsUpdated wakes frontend consumers after any successful job lifecycle
// transition. The payload is intentionally only a wake-up signal.
func emitJobsUpdated() {
	if app := application.Get(); app != nil {
		app.Event.Emit("jobs:updated", "changed")
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
func buildActionStore(recorder activity.Recorder, logger zerolog.Logger) (*actions.ActionStore, *actions.ActionsWatcher) {
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
		// A hand edit (or the app's own write) reloaded actions.yml: record the
		// now-effective action count so the change is auditable.
		if recorder != nil {
			recorder.Record(context.Background(), activity.ConfigReloaded("actions.yml", len(store.List())))
		}
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

	launcher  *pipeline.HiveSessionLauncher
	publisher pipeline.MessagePublisher
}

func (r *hiveActionRuntime) Close() {
	r.cancel()
	if err := r.db.Close(); err != nil {
		log.Printf("close hive action database: %v", err)
	}
}

func buildHiveActionRuntime(recorder activity.Recorder, logger zerolog.Logger) (*hiveActionRuntime, error) {
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

	launcher := pipeline.NewHiveSessionLauncher(sessions)
	launcher.SetRecorder(recorder)

	return &hiveActionRuntime{
		db:        database,
		cancel:    cancel,
		launcher:  launcher,
		publisher: pipeline.NewHiveMessagePublisher(hive.NewMessageService(stores.NewMessageStore(database, cfg.Messaging.MaxMessages), cfg, bus)),
	}, nil
}

// buildOutputWorker constructs the output worker over db and actionStore.
// Mock modes do not start its background loop because no fixture flow emits
// output commands, but they retain this worker for explicit detail-pane
// confirmation RPCs. That keeps the configured action path real in e2e while
// avoiding a background shell action from compromising fixture determinism.
func buildOutputWorker(db *pipelinedb.DB, actionStore *actions.ActionStore, launcher pipeline.SessionLauncher, publisher pipeline.MessagePublisher, recorder activity.Recorder, jobRecorder jobs.Recorder, logger zerolog.Logger) *pipeline.Worker {
	dispatcher := pipeline.NewDispatcher(map[string]pipeline.Executor{
		pipeline.ActionTypeLaunchSession: pipeline.NewLaunchSessionExecutor(launcher),
		"shell":                          pipeline.NewShellExecutor(logger),
		"publish-message":                pipeline.NewPublishMessageExecutor(publisher),
	})
	worker := pipeline.NewWorker(db, actionStore, dispatcher, pipeline.DefaultOutputWorkerInterval, logger)
	worker.SetRecorder(recorder)
	worker.SetJobRecorder(jobRecorder)
	return worker
}

func main() {
	// Seed HIVE_DATA_DIR / HIVE_DESKTOP_CONFIG from the bootstrap pointer file
	// before any path is resolved, so a data/config directory override chosen
	// in System settings applies to a dock-launched app. Must precede
	// StateDir/ConfigPath use below. An explicit env var still wins.
	bootstrapErr := desktop.ApplyBootstrap()

	logger, logCloser, logErr := desktop.NewLogger()
	if logErr != nil {
		logger.Warn().Err(logErr).Msg("desktop log file unavailable; logging to stderr only")
	}
	if bootstrapErr != nil {
		logger.Warn().Err(bootstrapErr).Msg("desktop bootstrap overrides ignored")
	}

	interval := feed.DefaultPollInterval
	settings, err := desktop.LoadSettings()
	if err != nil {
		logger.Warn().Err(err).Msg("desktop settings load failed; using defaults")
	} else if resolved, err := settings.PollIntervalOrDefault(feed.DefaultPollInterval); err != nil {
		logger.Warn().Err(err).Msg("desktop settings poll interval invalid; using defaults")
	} else {
		interval = resolved
		if raw, parseErr := time.ParseDuration(settings.PollInterval); parseErr == nil && raw < desktop.MinPollInterval {
			logger.Warn().Str("configured_interval", settings.PollInterval).Dur("interval", interval).Msg("desktop poll interval below minimum; clamped")
		}
	}

	fetcher := buildSourceFetcher(logger)
	if fetcher != nil {
		fetcher.SetSearchTTL(interval)
	}

	pipelineDB, err := pipelinedb.Open(desktop.StateDir(), pipelinedb.DefaultOpenOptions())
	if err != nil {
		log.Fatal(err)
	}

	// The activity recorder is shared by every subsystem that reports to the
	// Activity view (producer, worker, session launcher, config watcher) and by
	// the ActivityService the frontend reads/writes. It emits activity:appended
	// on each append so open views refresh.
	activityStore := activity.NewStore(pipelineDB, activity.Options{Emit: emitActivityAppended})
	jobStore := jobs.NewStore(pipelineDB, jobs.Options{Emit: func(int64) { emitJobsUpdated() }})
	if fetcher != nil {
		fetcher.SetRecorder(activityStore)
	}

	actionRuntime, err := buildHiveActionRuntime(activityStore, logger)
	if err != nil {
		log.Fatal(err)
	}

	// Mock mode has no live producer, so seed deterministic inbox rows for the
	// fixture flow in desktop/e2e/fixtures/flows/frontend-triage.yaml.
	if desktop.MockMode() == "feed" || desktop.MockMode() == "action-smoke" {
		seedMockInboxItemsOrWarn(pipelineDB, logger)
	}

	actionStore, actionsWatcher := buildActionStore(activityStore, logger)
	if actionsWatcher != nil {
		actionsWatcher.Start()
	}

	// The flows store must exist before the producer and retention maintenance:
	// both resolve enabled flow IDs live from it.
	flowsStore, flowsWatcher := buildFlowsStore(actionStore, logger)
	actionStore.SetUsageChecker(actionUsageChecker{flows: flowsStore, db: pipelineDB})
	if flowsWatcher != nil {
		flowsWatcher.Start()
	}

	outputWorker := buildOutputWorker(pipelineDB, actionStore, actionRuntime.launcher, actionRuntime.publisher, activityStore, jobStore, logger)
	if desktop.MockMode() == "" {
		outputWorker.Start()
	}

	maintenance := pipeline.NewMaintenance(
		pipelineDB,
		flowsStore,
		pipelinedb.DefaultRetentionPolicy(),
		pipeline.DefaultRetentionInterval,
		logger,
	)
	maintenance.Start()

	producer := buildPipelineProducer(pipelineDB, fetcher, flowsStore, activityStore, interval, logger)
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

	// The updater service is created before the app (it goes in the Services
	// slice) but its engine (app.Updater) only exists after application.New, so
	// the live Updater is attached below. Auto-update defaults on; the persisted
	// toggle seeds the initial state.
	updaterVersion, _, _ := resolvedBuildInfo()
	updaterService := NewUpdaterService(updaterVersion, settings.AutoUpdateOrDefault(), defaultUpdateCheckInterval, logger)

	options := application.Options{
		Name:        "Hive",
		Description: "Hive desktop application",
		Icon:        appIcon,
		Services: []application.Service{
			application.NewService(auth.NewService(buildAuthBackend(onAuthChange))),
			application.NewService(NewPipelineService(pipelineDB, actionStore, outputWorker, actionRuntime.launcher)),
			application.NewService(NewFlowsService(flowsStore, pipelineDB)),
			application.NewService(NewActionsService(actionStore, emitActionsUpdated)),
			application.NewService(NewActivityService(activityStore)),
			application.NewService(NewJobService(jobStore)),
			application.NewService(NewSystemService()),
			application.NewService(NewSettingsService(producer, fetcher, logger)),
			application.NewService(updaterService),
		},
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(assets),
			Middleware: desktopSmokeMiddleware(pipelineDB, actionRuntime.db),
		},
		Mac: application.MacOptions{
			ActivationPolicy: application.ActivationPolicyRegular,
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	}
	app := application.New(options)

	// Configure self-update only for real release builds: a dev build has no
	// published release newer than itself, and every release would register as
	// "newer" against the "dev" sentinel. isReleaseVersion rejects dev and
	// pseudo-versions, so the engine stays nil there and the service degrades to
	// Available:false with a no-op ticker.
	if isReleaseVersion(updaterVersion) {
		provider, provErr := newDesktopProvider(desktopRepoSlug, "")
		if provErr != nil {
			logger.Warn().Err(provErr).Msg("desktop auto-update unavailable; provider init failed")
		} else if initErr := app.Updater.Init(updater.Config{
			CurrentVersion: updaterVersion,
			Providers:      []updater.Provider{provider},
		}); initErr != nil {
			logger.Warn().Err(initErr).Msg("desktop auto-update unavailable; updater init failed")
		} else {
			updaterService.attach(app.Updater)
		}
	}

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
		updaterService.stop()
		maintenance.Stop()
		actionRuntime.Close()
		logCloser()
	}
	if err := app.Run(); err != nil {
		shutdown()
		log.Fatal(err)
	}
	shutdown()
}
