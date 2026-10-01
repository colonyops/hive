package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/pkg/tmpl"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/activity"
	"github.com/colonyops/hive/cmd/desktop/internal/app/agentws"
	"github.com/colonyops/hive/cmd/desktop/internal/app/canvas"
	"github.com/colonyops/hive/cmd/desktop/internal/app/credentials"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	datastores "github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/cmd/desktop/internal/app/execenv"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
	"github.com/colonyops/hive/cmd/desktop/internal/app/ingest"
	"github.com/colonyops/hive/cmd/desktop/internal/app/profileimg"
	"github.com/colonyops/hive/cmd/desktop/internal/app/ptyterm"
	"github.com/colonyops/hive/cmd/desktop/internal/app/report"
	"github.com/colonyops/hive/cmd/desktop/internal/app/runtime"
	"github.com/colonyops/hive/cmd/desktop/internal/app/runtime/js"
	"github.com/colonyops/hive/cmd/desktop/internal/app/schedule"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sourcemark"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
	execsource "github.com/colonyops/hive/cmd/desktop/internal/app/sources/exec"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/gitea"
	ghsource "github.com/colonyops/hive/cmd/desktop/internal/app/sources/github"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/github/ghclient"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/grafana"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/posthog"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/rss"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/webhook"
	"github.com/colonyops/hive/cmd/desktop/internal/app/terminalimg"
	"github.com/colonyops/hive/cmd/desktop/internal/app/tmuxbin"
	"github.com/colonyops/hive/cmd/desktop/internal/app/tmuxcc"
	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/eventbus"
	"github.com/colonyops/hive/internal/core/git"
	"github.com/colonyops/hive/internal/core/multiplexer"
	coreterminal "github.com/colonyops/hive/internal/core/terminal"
	terminalstatus "github.com/colonyops/hive/internal/core/terminal/status"
	terminaltmux "github.com/colonyops/hive/internal/core/terminal/tmux"
	coredb "github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/data/stores"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/scripts"
	tmuxadapter "github.com/colonyops/hive/internal/integration/multiplexer/tmux"
)

// Config is everything App needs that it cannot resolve itself.
type Config struct {
	Settings      settings.Settings
	SettingsStore *settings.Store
	Paths         settings.Paths
	MockMode      string
	Logger        zerolog.Logger

	// CredentialKeyringService isolates development credentials from the installed app.
	CredentialKeyringService string

	// GUI-owned dependencies enter through consumer-defined driven ports.
	Notifier dispatch.SystemNotifier
	Gate     dispatch.NotificationGate

	// Build stamps the running binary into report bundles.
	Build report.Build

	// TelemetryRuntime is the startup result for exporters constructed before
	// App so their log writer can participate in logger construction.
	TelemetryRuntime TelemetryRuntime
}

// App is the headless core. Driving adapters hold *App and the concrete
// types on it; there are no driving-port interfaces.
type App struct {
	// The per-domain services. Driving adapters call these — never the
	// unexported domain stores further down, which is what they are built
	// over.
	Inbox      *InboxService
	Sessions   *SessionsService
	Flows      *FlowsService
	Actions    *ActionsService
	Settings   *SettingsService
	MenuBar    *MenuBarService
	System     *SystemService
	HiveConfig *HiveConfigService
	Webhooks   *WebhookService
	GitHub     *GitHubService
	Gitea      *GiteaService
	Grafana    *GrafanaService
	PostHog    *PostHogService
	// Integrations lists the connector registry with each entry's connection
	// state. Generic; GitHub, Gitea, Grafana and PostHog above are the
	// provider-specific acquisition halves.
	Integrations *IntegrationsService
	Activity     *ActivityService
	Jobs         *JobService
	Prompts      *PromptsService
	Skills       *SkillsService
	Report       *ReportService
	// ReleaseNotes serves the changelog embedded in this binary and remembers
	// which version's notes the user has seen.
	ReleaseNotes   *ReleaseNotesService
	Terminals      *TerminalsService
	TerminalImages *TerminalImagesService
	// Perf records UI spans to a JSONL file when development.perf.enabled is
	// on. Always non-nil; a disabled recorder is a no-op (ADR ui-performance-spans-are-recorded-to-jsonl).
	Perf *PerfService
	// Observability owns process sampling and telemetry startup status.
	Observability *ObservabilityService
	DevTools      *DevToolsService
	// Sources runs ingestion on demand — the refresh a user pressed, and the
	// one a flow edit earns.
	Sources *SourcesService

	PopupTerminals  *PopupTerminalsService
	AgentWorkspaces *AgentWorkspacesService
	Tasks           *TasksService
	Canvas          *CanvasService
	Schedules       *SchedulesService

	// Events is the typed bus adapters project into transport-specific events.
	// Stores is exposed only so the e2e harness can seed fixtures.
	Events *events.Bus
	Stores *datastores.Stores

	// db is retained for whole-database maintenance and the e2e-only PipelineDB
	// seam.
	db *queries.DB

	// Domain stores stay private so callers cannot bypass service error mapping
	// and event publication.
	actionStore         *actions.ActionStore
	flowStore           *flow.FlowStore
	agentWorkspaceStore *agentws.Store
	// agentWorkspaceRootProblem carries EnsureRoot's error, verbatim, when the
	// configured agent-workspace root could not be created or opened.
	// openAgentWorkspaces sets it; the Agents area is what surfaces it to the
	// user (phase 6) rather than silently creating a second root elsewhere.
	agentWorkspaceRootProblem string
	fetchers                  *ghsource.Fetchers
	credentials               credentials.Store

	// gitHubConnection acquires and releases GitHub credentials. It is one
	// connector's, not the app's: nothing here is gated on it holding one.
	gitHubConnection ghsource.Connection

	// grafanaFetchers hands out one per-stack fetcher; grafanaAuth connects and
	// disconnects stacks. Like GitHub, nothing is gated on them.
	grafanaFetchers *grafana.Fetchers
	grafanaAuth     *grafana.Authenticator

	// posthogFetchers hands out one per-project fetcher; posthogAuth connects
	// and disconnects projects. Like GitHub, nothing is gated on them.
	posthogFetchers *posthog.Fetchers
	posthogAuth     *posthog.Authenticator

	// giteaFetchers hands out one per-account fetcher; giteaAuth connects and
	// disconnects instances. Like GitHub, nothing is gated on them.
	giteaFetchers *gitea.Fetchers
	giteaAuth     *gitea.Authenticator

	// rssFetchers is the shared feed client and its per-URL window cache.
	// There is no auth beside it: the connector reads public feeds.
	rssFetchers *rss.Fetchers

	// sources resolves the current flow set into live connector instances.
	// Both ingress paths go through it — the poll producer takes its
	// pull-mode instances, the webhook listener its push-mode ones — so a
	// connector is constructed one way regardless of how it delivers.
	sources *ingest.Resolver

	// Background subsystems, owned here so main.go stops holding them.
	// Uniform lifecycle through a plugs manager was evaluated and declined
	// for now — see the note on Close.
	producer *ingest.Producer
	engine   *runtime.Engine
	// scripts is the one script-language registry in the process. The engine's
	// runners and a dry run's throwaway runner resolve `function` nodes through
	// the same one, so a flow cannot execute differently depending on which
	// asked for it.
	scripts *runtime.ScriptRegistry
	outputs *dispatch.Worker
	// dispatcher is shared with the worker rather than private to it: a
	// terminal action runs the same executors without a durable command
	// behind it, and a second dispatcher would be a second executor map to
	// keep in step.
	dispatcher  *dispatch.Dispatcher
	retention   *ingest.Maintenance
	webhook     *webhook.Listener
	webhookHost string
	webhookPort int

	// Hive integration: sessions and internal events use Hive's own shared
	// state and event bus, while this app keeps its own database.
	launcher  *dispatch.HiveSessionLauncher
	sessions  *dispatch.HiveSessionManager
	hiveDB    *coredb.DB
	honeycomb *dispatch.HiveHoneycomb

	// hiveBus and hiveDataDir are what a reload needs and must not rebuild:
	// the bus subscribers already hold, and the resolved data directory.
	hiveBus     *eventbus.EventBus
	hiveDataDir string
	// reloadMu serializes ReloadHiveRuntime. Each adapter swap is atomic on
	// its own; the mutex is what makes the four swaps of one reload land
	// together, so two overlapping saves cannot leave the launcher on one
	// config and the manager on another.
	reloadMu sync.Mutex

	// Full agent command lines are atomic because ReloadHiveRuntime replaces
	// them while AgentWorkspacesService can read them.
	agentCommands atomic.Pointer[map[string]string]

	// hiveConfig is where the Hive config resolved on the last load. Atomic
	// for the same reason agentCommands is: a reload re-resolves it (HIVE_CONFIG
	// can change, and a first run turns an absent path into a real file) while
	// the settings screens are reading it.
	hiveConfig atomic.Pointer[HiveConfigLocation]

	// scheduler launches a workspace's scheduled chats when they come due, and
	// catches up the ones that fell due while the app was closed.
	scheduler *schedule.Scheduler

	// terminals owns one tmux control-mode client per attached session slug.
	// Its context is the app's lifetime, not a request's (ADR terminal-transport).
	terminals *tmuxcc.Manager

	// popupTerminals owns the ephemeral terminals a pop-up opens (ADR ephemeral-popup-terminals).
	// They are this process's children, so unlike tmux's they end with Close.
	popupTerminals *ptyterm.Manager

	// tmux is the one place the tmux binary is discovered, shared by the
	// terminal's control clients and Hive's session spawning (ADR tmux-discovery).
	tmux *tmuxbin.Resolver

	// execEnv is the environment every command the app spawns on the user's
	// behalf runs in — session hooks, git, shell actions (ADR subprocess-environment).
	execEnv *execenv.Resolver

	// pollInterval is the validated, clamped interval the producer polls on.
	pollInterval time.Duration

	logger    zerolog.Logger
	mock      string
	publisher *dispatch.HiveMessagePublisher

	// ctx is the application's lifetime, not a request's. Background
	// callbacks wired at construction — the config watchers, the GitHub
	// connection's change hook — publish with it, and Close cancels it. This is
	// the one type in the core that legitimately holds a context: it is the
	// thing whose lifetime that context represents.
	ctx    context.Context
	cancel context.CancelFunc

	settings               settings.Settings
	settingsStore          *settings.Store
	paths                  settings.Paths
	flowsWatcher           *flow.FlowsWatcher
	actionsWatcher         *actions.ActionsWatcher
	agentWorkspacesWatcher *agentws.Watcher
	hiveBusCancel          context.CancelFunc
}

// New builds the core: the store, the domain stores and their watchers, the
// Hive action runtime, and the background subsystems. Nothing is running when
// it returns — call Start.
func New(ctx context.Context, cfg Config) (*App, error) {
	if cfg.Paths.SettingsPath == "" {
		b, _ := settings.LoadBootstrap()
		cfg.Paths = settings.ResolvePaths(b, settings.ResolveOptions{MockMode: cfg.MockMode})
	}
	if cfg.SettingsStore == nil {
		cfg.SettingsStore = settings.NewStore(cfg.Paths.SettingsPath)
	}
	runCtx, cancel := context.WithCancel(ctx)

	a := &App{
		logger:        cfg.Logger,
		Events:        events.New(cfg.Logger),
		mock:          cfg.MockMode,
		settings:      cfg.Settings,
		settingsStore: cfg.SettingsStore,
		paths:         cfg.Paths,
		ctx:           runCtx,
		cancel:        cancel,
	}

	a.pollInterval = cfg.Settings.Polling.Interval.Duration()
	a.tmux = tmuxbin.NewResolver(cfg.Settings.Paths.Tmux)
	a.execEnv = execenv.NewResolver(execenv.Options{Logger: cfg.Logger})

	// Mock modes get an in-memory credential store: a keychain read can
	// prompt, and a fixture run that prompts is a fixture run that hangs.
	a.credentials = buildCredentialStore(cfg.MockMode, cfg.Paths.CredentialsIndexPath, cfg.CredentialKeyringService)

	// One client template backs both the fetch layer and the connect flow, so a
	// development instance pointed at cmd/desktop/devserver never splits its traffic
	// between the proxy and real GitHub. The API base is the ADR devserver-github-proxy dev
	// override and is empty in shipped builds; the OAuth base is never
	// redirected, so the device flow still reaches github.com.
	gitHubOpts := []ghclient.Option{ghclient.WithLogger(cfg.Logger)}
	if apiBase := cfg.Settings.GitHubAPIBase(); apiBase != "" {
		gitHubOpts = append(gitHubOpts, ghclient.WithAPIBase(apiBase))
	}
	gitHubClient := ghclient.NewClient(gitHubOpts...)

	if cfg.MockMode == "" {
		a.fetchers = ghsource.NewFetchers(gitHubClient, a.credentials, cfg.Logger)
		a.fetchers.SetSearchTTL(a.pollInterval)
	}

	dbOptions := queries.DefaultOpenOptions()
	dbOptions.PauseCommit = cfg.Settings.Development.Debug.PauseCommit.Duration()
	dbOptions.Logger = cfg.Logger
	db, err := queries.Open(ctx, cfg.Paths.StateDir, dbOptions)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open desktop store: %w", err)
	}
	compactPipelineStoreAtStartup(ctx, db, queries.DatabasePath(cfg.Paths.StateDir), cfg.Logger)
	a.db = db
	a.Stores = datastores.New(db, datastores.Options{Logger: cfg.Logger})

	a.Activity = newActivityService(a.Stores.ActivityEvents, a.Events, cfg.Logger)
	a.Jobs = newJobService(a.Stores.Jobs, a.Events, cfg.Logger)
	if a.fetchers != nil {
		a.fetchers.SetRecorder(a.Activity)
	}

	a.terminals = tmuxcc.NewManager(runCtx, tmuxcc.ManagerOptions{
		Logger:  cfg.Logger,
		Binary:  func() (string, error) { return a.resolveTmuxBinary(runCtx) },
		Environ: a.execEnv.Environ,
	})
	if err := a.openHiveRuntime(runCtx, cfg); err != nil {
		_ = a.db.Close()
		cancel()
		return nil, err
	}
	a.popupTerminals = ptyterm.NewManager(ptyterm.ManagerOptions{Environ: a.execEnv.Environ})

	a.openActions(cfg.Paths.ActionsPath, cfg.Logger)
	a.openFlows(cfg.Paths.FlowsDir, cfg.Logger)
	a.openAgentWorkspaces(cfg.Paths.AgentWorkspacesDir, cfg.Logger)
	a.actionStore.SetUsageChecker(newActionUsage(a.flowStore, a.Stores.OutputCommands))

	a.gitHubConnection = buildGitHubConnection(cfg.MockMode, gitHubClient, a.credentials, func() {
		// Every connection transition drops this provider's fetch caches
		// before anything is notified: a different account must never be
		// served items fetched with the previous token. Fetchers is already
		// GitHub's alone, so invalidating all of them is exactly this
		// provider's scope — and over-invalidating costs a refetch, where
		// under-invalidating serves another account's items.
		if a.fetchers != nil {
			a.fetchers.InvalidateAll()
		}
		a.Events.Publish(a.ctx, events.ConnectionUpdated{Provider: ghsource.Provider})
	})

	// Grafana's client is built per tick from a stack URL and token, so unlike
	// GitHub there is no mock-mode fetch template to gate on — the fetcher
	// registry and its stack store are always wired.
	grafanaStacks := grafana.NewStackStore(filepath.Join(cfg.Paths.StateDir, "grafana-stacks.json"))
	a.grafanaFetchers = grafana.NewFetchers(grafanaStacks, a.credentials, cfg.Logger)
	a.grafanaAuth = grafana.NewAuthenticator(a.credentials, grafanaStacks, cfg.Logger, func(credentials.Ref) {
		// Drop every stack's cooldown so a freshly connected account isn't held
		// back by its predecessor's rate limit, then announce so Integrations re-reads.
		a.grafanaFetchers.InvalidateAll()
		a.Events.Publish(a.ctx, events.ConnectionUpdated{Provider: grafana.Provider})
	})

	// PostHog binds a host and project to the account at connect time, the same
	// shape as a Grafana stack, so its registry and binding store are likewise
	// always wired rather than gated on a mock-mode fetch template.
	posthogProjects := posthog.NewProjectStore(filepath.Join(cfg.Paths.StateDir, "posthog-projects.json"))
	a.posthogFetchers = posthog.NewFetchers(posthogProjects, a.credentials, cfg.Logger)
	a.posthogAuth = posthog.NewAuthenticator(a.credentials, posthogProjects, cfg.Logger, func(credentials.Ref) {
		a.posthogFetchers.InvalidateAll()
		a.Events.Publish(a.ctx, events.ConnectionUpdated{Provider: posthog.Provider})
	})

	// Gitea binds a host to the account at connect time, the same shape again,
	// so its registry and binding store are likewise always wired.
	giteaInstances := gitea.NewInstanceStore(filepath.Join(cfg.Paths.StateDir, "gitea-instances.json"))
	a.giteaFetchers = gitea.NewFetchers(giteaInstances, a.credentials, cfg.Logger)
	a.giteaAuth = gitea.NewAuthenticator(a.credentials, giteaInstances, cfg.Logger, func(credentials.Ref) {
		a.giteaFetchers.InvalidateAll()
		a.Events.Publish(a.ctx, events.ConnectionUpdated{Provider: gitea.Provider})
	})

	// Feeds need no account, so the client is wired unconditionally and has
	// no connection callback to drop its cache on.
	a.rssFetchers = rss.NewFetchers(cfg.Logger)

	a.retention = ingest.NewMaintenance(db, queries.DefaultRetentionPolicy(), ingest.DefaultRetentionInterval, cfg.Logger)
	a.scripts = runtime.NewScriptRegistry()
	a.scripts.Register(js.New(runtime.NewScriptPool(0)))
	a.engine = a.buildEngine(cfg.Logger)
	a.sources = a.buildSources(cfg.Logger)
	a.producer = a.buildProducer(cfg.Logger)
	a.openWebhook(runCtx, cfg)

	a.Sources = newSourcesService(a.producer, a.fetchers, a.rssFetchers)
	a.Settings = newSettingsService(SettingsDeps{Store: cfg.SettingsStore, Producer: a.producer, Fetchers: a.fetchers, LookPath: a.execEnv.LookPath})
	a.TerminalImages = &TerminalImagesService{store: terminalimg.NewStore(filepath.Join(cfg.Paths.StateDir, "assets", "terminal-images"))}
	profileImages := profileimg.NewStore(filepath.Join(cfg.Paths.StateDir, "assets", "profiles"))
	sourceMarks := sourcemark.NewStore(filepath.Join(cfg.Paths.StateDir, "assets", "webhookmarks"))
	a.Flows = newFlowsService(FlowsDeps{
		Flows:    a.flowStore,
		Stores:   a.Stores,
		Creds:    a.credentials,
		Images:   profileImages,
		Marks:    sourceMarks,
		Scripts:  a.scripts,
		Settings: a.settingsStore,
		Events:   a.Events,
	})
	if err := a.Flows.EnsureProfile(ctx); err != nil {
		return nil, fmt.Errorf("create the default profile: %w", err)
	}
	a.Actions = newActionsService(a.actionStore, a.Events)
	a.System = newSystemService(systemOptions{Paths: cfg.Paths, HiveConfig: a.HiveConfigLocation})
	a.HiveConfig = newHiveConfigService(hiveConfigOptions{
		Location:     a.HiveConfigLocation,
		LookPath:     a.execEnv.LookPath,
		DefaultAgent: defaultAgentEnvReader{env: a.execEnv},
		Check: func(path string) error {
			_, err := config.Load(path, a.hiveDataDir)
			return err
		},
		Reload: a.ReloadHiveRuntime,
	})
	a.ReleaseNotes = NewReleaseNotesService(cfg.Paths, cfg.Logger)
	a.Webhooks = newWebhookService(WebhookDeps{Settings: cfg.SettingsStore, Captures: a.Stores.WebhookCaptures, Listener: a.webhook, Host: a.webhookHost, Port: a.webhookPort})
	a.GitHub = newGitHubService(a.gitHubConnection)
	a.Gitea = newGiteaService(a.giteaAuth)
	a.Grafana = newGrafanaService(a.grafanaAuth)
	a.PostHog = newPostHogService(a.posthogAuth)
	a.Integrations = newIntegrationsService(a.credentials)
	a.Prompts = newPromptsService(cfg.Paths, cfg.SettingsStore, a.Webhooks)
	a.Skills = newSkillsService(a.Prompts)
	// The retired global installer left an index beside the state dir. Nothing
	// reads it, so drop it once; the SKILL.md files it tracked stay where they
	// are, valid but frozen (ADR skills-are-declared-by-a-workspace).
	_ = os.Remove(filepath.Join(cfg.Paths.StateDir, "skills.json"))
	a.Report = newReportService(cfg.Paths, cfg.SettingsStore, cfg.Build, cfg.Logger)
	a.Perf = newPerfService(openPerfRecorder(cfg.Settings.Development.Perf.Enabled, cfg.Paths.StateDir, cfg.Logger), cfg.Logger)
	a.Observability = newObservabilityService(cfg.SettingsStore, cfg.Settings.Telemetry, cfg.TelemetryRuntime)
	a.DevTools = newDevToolsService(cfg.Settings.Development.DevTools.Enabled)
	a.Canvas = newCanvasService(CanvasDeps{
		Store:    canvas.NewStore(cfg.Paths.AgentWorkspacesDir),
		Sessions: a.Stores.AgentSessions,
		Events:   a.Events,
	})
	a.AgentWorkspaces = newAgentWorkspacesService(AgentWorkspacesDeps{
		Store:           a.agentWorkspaceStore,
		Terminals:       a.terminals,
		Stores:          a.Stores,
		Skills:          a.Skills,
		ProfileCommands: a.profileCommands,
		RootProblem:     a.agentWorkspaceRootProblem,
		ExecEnv:         a.execEnv,
		EditorCommand:   a.Settings,
		MCPBase:         a,
		EndDelay:        a.Settings,
		Events:          a.Events,
		Logger:          cfg.Logger,
	})
	a.outputs = a.buildOutputWorker(cfg)
	a.Inbox = newInboxService(InboxDeps{Items: a.Stores.InboxItems, Commands: a.Stores.OutputCommands, NodeRuns: a.Stores.NodeRuns, Catalog: a.actionStore, Worker: a.outputs})
	a.MenuBar = newMenuBarService(MenuBarDeps{
		Settings: cfg.SettingsStore,
		Flows:    a.flowStore,
		Items:    a.Stores.InboxItems,
		Catalog:  a.actionStore,
		Polls:    a.lastTicker(),
		Events:   a.Events,
	})
	a.Sessions = newSessionsService(SessionsDeps{
		Launcher: a.launcher, WorkspaceLauncher: a.AgentWorkspaces,
		Manager: a.sessions, Statuses: a.sessions, Git: a.sessions, Tmux: a.terminals,
		AgentWindows: a.terminals, AgentCommands: a.profileCommands,
		Jobs: a.Jobs, Items: a.Stores.InboxItems, Links: a.Stores.ItemSessions, Catalog: a.actionStore, Dispatcher: a.dispatcher,
		Recorder: a.Activity, Events: a.Events, Logger: cfg.Logger,
		PullRequests: newSessionPullRequests(
			newGitHubForge(gitHubClient, a.credentials),
			newGiteaForge(gitea.NewPullRequests(giteaInstances, a.credentials, a.giteaFetchers)),
		),
		ExecEnv:         a.execEnv,
		EditorCommand:   a.Settings,
		DefaultAgentEnv: defaultAgentEnvReader{env: a.execEnv},
	})
	a.Terminals = newTerminalsService(TerminalsDeps{Manager: a.terminals, Starter: a.Sessions, Home: os.UserHomeDir, Logger: cfg.Logger})
	a.PopupTerminals = newPopupTerminalsService(PopupTerminalsDeps{Manager: a.popupTerminals, Terminals: a.Terminals, Directory: a.Sessions, Catalog: a.actionStore})
	// a.honeycomb holding a nil *dispatch.HiveHoneycomb would otherwise pass a
	// non-nil taskSource whose nil-guard never fires — the explicit check keeps
	// Tasks answering unavailable instead.
	var tasks taskSource
	if a.honeycomb != nil {
		tasks = a.honeycomb
	}
	a.Tasks = newTasksService(tasks)

	// After AgentWorkspaces: the scheduler launches chats through it, and the
	// service reads the same workspace store the scheduler takes its specs
	// from.
	a.scheduler = a.buildScheduler(cfg.Logger)
	a.Schedules = newSchedulesService(SchedulesDeps{
		Workspaces: a.agentWorkspaceStore,
		Schedules:  a.Stores.Schedules,
		Sessions:   a.Stores.AgentSessions,
		Scheduler:  a.scheduler,
		Logger:     cfg.Logger,
	})
	// Schedules are saved with the manifest, so the write that reaches the
	// running loop is the workspace editor's, not a route of its own.
	a.AgentWorkspaces.OnSchedulesChanged = func(string) { a.scheduler.Reload() }

	return a, nil
}

type pipelineCompactor interface {
	Compact(context.Context, queries.CompactionPolicy) (queries.CompactionResult, error)
}

func compactPipelineStoreAtStartup(ctx context.Context, db pipelineCompactor, path string, logger zerolog.Logger) {
	before, beforeErr := os.Stat(path)
	started := time.Now()
	result, compactErr := db.Compact(ctx, queries.DefaultCompactionPolicy())
	duration := time.Since(started)
	after, afterErr := os.Stat(path)

	if beforeErr != nil {
		logger.Warn().Err(beforeErr).Str("path", path).Msg("pipeline compaction: measure size before maintenance")
	}
	if afterErr != nil {
		logger.Warn().Err(afterErr).Str("path", path).Msg("pipeline compaction: measure size after maintenance")
	}

	if compactErr != nil {
		event := logger.Warn().Err(compactErr).Dur("duration", duration)
		if beforeErr == nil {
			event = event.Int64("before_bytes", before.Size())
		}
		if afterErr == nil {
			event = event.Int64("after_bytes", after.Size())
		}
		event.Msg("pipeline compaction failed; startup continuing")
		return
	}

	ratio := float64(0)
	if result.PageCount > 0 {
		ratio = float64(result.FreelistCount) / float64(result.PageCount)
	}
	if !result.Compacted {
		logger.Debug().
			Int64("page_count", result.PageCount).
			Int64("freelist_count", result.FreelistCount).
			Float64("reclaimable_ratio", ratio).
			Int64("reclaimable_bytes", result.ReclaimableBytes).
			Dur("duration", duration).
			Msg("pipeline compaction skipped")
		return
	}

	event := logger.Info().
		Int64("page_count", result.PageCount).
		Int64("freelist_count", result.FreelistCount).
		Float64("reclaimable_ratio", ratio).
		Int64("reclaimable_bytes", result.ReclaimableBytes).
		Dur("duration", duration)
	if beforeErr == nil {
		event = event.Int64("before_bytes", before.Size())
	}
	if afterErr == nil {
		event = event.Int64("after_bytes", after.Size())
	}
	event.Msg("pipeline database compacted")
}

// Start runs the background subsystems: the config watchers, the output
// worker, retention, the poll producer, and the webhook listener. Mock modes
// deliberately skip the worker loop and have no producer, so a fixture run
// stays deterministic.
func (a *App) Start(ctx context.Context) error {
	if a.actionsWatcher != nil {
		a.actionsWatcher.Start()
	}
	if a.flowsWatcher != nil {
		a.flowsWatcher.Start()
	}
	if a.agentWorkspacesWatcher != nil {
		a.agentWorkspacesWatcher.Start()
	}
	// After the watcher, so the first pass evaluates the workspace set the
	// watcher is already keeping current. That pass is the catch-up for
	// everything that came due while the app was closed, so it runs in mock
	// modes too -- a fixture root simply declares no schedules.
	a.scheduler.Start(ctx)
	if a.mock == "" {
		a.outputs.Start(ctx)
	}
	a.retention.Start(ctx)
	// The engine starts before anything that can append to the log. Its flow
	// installation is synchronous, so by the time a producer tick, a webhook
	// delivery or a test harness can append, there is a runner ready to route
	// it — no window in which a wake-up has nothing to wake.
	if err := a.engine.Start(ctx); err != nil {
		return fmt.Errorf("start flow engine: %w", err)
	}
	if a.producer != nil {
		a.producer.Start(ctx)
	}
	if a.webhook != nil {
		if err := a.webhook.Start(ctx); err != nil {
			a.logger.Warn().Err(err).Int("port", a.webhookPort).Msg("webhook listener unavailable")
		} else if a.webhookPort == 0 && !a.settings.EnvironmentOverridden(settings.EnvHTTPPort) {
			_, err := a.settingsStore.Update(func(persisted *settings.Settings) error {
				persisted.HTTP.Port = a.webhook.Port()
				return nil
			})
			if err != nil {
				a.Webhooks.setStartError(fmt.Errorf("persist allocated webhook port: %w", err))
				a.logger.Warn().Err(err).Msg("persist allocated webhook port")
			}
		}
	}
	// The login shell probe costs a shell startup and everything the app spawns
	// on the user's behalf waits on it, so it is paid here rather than by
	// whichever click reaches it first. Skipped in mock/e2e runs: a fixture
	// launch must not start the machine's shell.
	if a.mock == "" {
		go func() { _ = a.execEnv.Path(a.ctx) }()
	}
	return nil
}

// RuntimePaths returns the immutable location snapshot used by this process.
func (a *App) RuntimePaths() settings.Paths { return a.paths }

// MCPBaseURL returns this run's loopback base URL, or empty while the server
// is down. App-hosted catalogue entries have no static URL because startup
// allocates the port; callers join this base with RuntimePath
// (ADR mcp-replaces-the-agent-facing-http-api).
func (a *App) MCPBaseURL(ctx context.Context) string {
	if a.Webhooks == nil {
		return ""
	}
	running, port := a.Webhooks.Endpoint(ctx)
	if !running || port == 0 {
		return ""
	}
	return HTTPBaseURLAt(a.Webhooks.Host(), port)
}

// HiveConn exposes the connection to the shared Hive action database
// (sessions, messages) as a plain *sql.DB. Shared hive types stop at this
// method — adapters that need raw access, such as the e2e harness's table
// resets and read-only snapshots, take the stdlib type rather than
// *coredb.DB.
func (a *App) HiveConn() *sql.DB {
	if a.hiveDB == nil {
		return nil
	}
	return a.hiveDB.Conn()
}

// Close stops the background subsystems and releases resources. It is the
// single teardown path: the adapter's own shutdown hook covers only what it
// owns (a tray, a window).
//
// Each subsystem's own Stop/Close is idempotent (its own stopOnce; webhook's
// Stop below is the one that gained one, see docs/decisions/0016), so the
// hand-written sequence below — unchanged from before this phase — stays
// safe to call in this reverse-startup order even if something upstream
// already tore part of it down.
//
// A plugs.Manager (docs/architecture.md's "Background lifecycle") was
// evaluated to replace this sequence and declined for now: appkit/plugs
// v0.0.0-20260423210245's Manager.Start unconditionally calls
// signal.NotifyContext, which permanently adds Go's one-time os/signal
// watcher goroutine to the process (confirmed against the stdlib source —
// there is no way to opt out; passing no signals means "watch all signals,"
// not "watch none," per signal.Notify's own documented behavior) and that
// goroutine has nowhere to go before TestAppLifecycle's strict
// before-vs-after goroutine count runs. Revisit once either that test
// tolerates it or a plugs release makes signal registration optional.
func (a *App) Close() error {
	a.cancel()

	if a.Observability != nil {
		if err := a.Observability.close(); err != nil {
			a.logger.Warn().Err(err).Msg("unregister process metrics")
		}
	}

	// Before the terminals: a pass in flight is launching chats through them,
	// and stopping the transport underneath it would fail a launch that has
	// already been recorded as made.
	if a.scheduler != nil {
		a.scheduler.Stop()
	}

	// Before the webhook listener: a terminal WebSocket has hijacked its
	// connection, which http.Server.Shutdown neither tracks nor closes, so the
	// socket has to be brought down by closing the streams behind it first
	// (ADR terminal-transport). The context is a fresh one for the same reason Shutdown's is.
	if a.terminals != nil {
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(a.ctx), 3*time.Second)
		_ = a.terminals.Stop(stopCtx)
		cancel()
	}
	// The pop-up terminals are this process's children rather than another
	// server's, so this is not just a detach: whatever is running in them ends
	// here.
	if a.popupTerminals != nil {
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(a.ctx), 3*time.Second)
		_ = a.popupTerminals.Stop(stopCtx)
		cancel()
	}

	if a.webhook != nil {
		// A fresh, un-cancelled context for the graceful drain: a.ctx may
		// already be cancelled by the line above, and handing a Done context
		// to Shutdown would mean "stop now" instead of "you have this long
		// to drain." WithoutCancel keeps this a context derived from a.ctx
		// rather than a bare root, without inheriting a deadline that may
		// have already passed.
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(a.ctx), 3*time.Second)
		// A shutdown failure only warns, matching Start's own bind-failure
		// policy: a slow or stuck drain must never fail Close outright.
		if err := a.webhook.Stop(stopCtx); err != nil {
			a.logger.Warn().Err(err).Msg("webhook listener shutdown")
		}
		cancel()
	}
	if a.producer != nil {
		a.producer.Stop()
	}
	a.engine.Stop()
	a.retention.Stop()
	a.outputs.Stop()
	if a.flowsWatcher != nil {
		a.flowsWatcher.Close()
	}
	if a.actionsWatcher != nil {
		a.actionsWatcher.Close()
	}
	if a.agentWorkspacesWatcher != nil {
		a.agentWorkspacesWatcher.Close()
	}
	a.Events.Close()

	if a.Perf != nil {
		if closeErr := a.Perf.Close(); closeErr != nil {
			a.logger.Warn().Err(closeErr).Msg("close perf recorder")
		}
	}

	if a.hiveBusCancel != nil {
		a.hiveBusCancel()
	}

	var err error
	if a.hiveDB != nil {
		if closeErr := a.hiveDB.Close(); closeErr != nil {
			err = fmt.Errorf("close hive action database: %w", closeErr)
		}
	}
	if closeErr := a.db.Close(); closeErr != nil && err == nil {
		err = fmt.Errorf("close desktop store: %w", closeErr)
	}
	return err
}

// PipelineDB exposes the raw pipeline database handle to driving adapters
// that need it directly: the e2e harness's table resets and read-only
// snapshots. Everything else reaches persistence through Stores.
func (a *App) PipelineDB() *queries.DB {
	return a.db
}

func buildGitHubConnection(mock string, client *ghclient.Client, creds credentials.Store, onChange func()) ghsource.Connection {
	switch mock {
	case "feed", "pipeline", "action-smoke":
		return ghsource.NewMockConnection(true, creds, onChange)
	case settings.MockOnboarding:
		return ghsource.NewMockConnection(false, creds, onChange)
	default:
		return ghsource.NewLiveConnection(client, creds, onChange)
	}
}

// buildCredentialStore picks the credential backing. Mock modes never touch
// the OS keychain: reading one can prompt, and the e2e harness has no way to
// answer.
func buildCredentialStore(mock, indexPath, keyringService string) credentials.Store {
	if mock != "" {
		return credentials.NewMemoryStore()
	}
	return credentials.NewKeychainStoreWithService(indexPath, keyringService)
}

// openActions loads actions.yml eagerly — rather than waiting for the first
// lazy List/Get — so a broken file is logged at startup instead of surfacing
// silently as "no actions found". A watcher that fails to start degrades to
// no hot-reload: the app still works, edits just need a restart.
func (a *App) openActions(path string, logger zerolog.Logger) {
	if _, err := actions.SeedDefaultsIfMissing(path); err != nil {
		logger.Warn().Err(err).Msg("actions seed failed")
	}
	a.actionStore = actions.NewActionStore(path)
	if err := a.actionStore.Reload(); err != nil {
		logger.Warn().Err(err).Msg("actions.yml load failed; using last-good (likely empty) action set")
	}

	watcher, err := actions.NewActionsWatcher(path, func() {
		if err := a.actionStore.Reload(); err != nil {
			logger.Warn().Err(err).Msg("actions.yml reload failed")
		}
		count := len(a.actionStore.List())
		a.Events.Publish(a.ctx, events.ActionsUpdated{Count: count})
		// A hand edit (or the app's own write) reloaded actions.yml: record
		// the now-effective action count so the change is auditable.
		a.Activity.Record(a.ctx, activity.ConfigReloaded("actions.yml", count))
	}, logger)
	if err != nil {
		logger.Warn().Err(err).Msg("actions.yml hot-reload unavailable")
		return
	}
	a.actionsWatcher = watcher
}

// openFlows constructs the flow store over settings.FlowsDir() and a watcher
// that reloads it on any flows/*.yaml change, including the app's own
// SaveFlow/SaveLayout writes. It must run before the producer, which
// resolves enabled flow ids live from the flow store.
//
// The rail order comes from settings.yaml, which the flow package does not
// read; it is process state the watcher's reloads leave alone. Reordering the
// rail pushes it back through FlowsService.SetOrder, so only a hand edit of
// settings.yaml waits for the next launch.
func (a *App) openFlows(dir string, logger zerolog.Logger) {
	a.flowStore = flow.NewFlowStore(dir, actions.NewRefs(a.actionStore))
	a.flowStore.SetOrder(a.settings.Profiles.Order)

	watcher, err := flow.NewFlowsWatcher(dir, func() {
		if err := a.flowStore.Reload(); err != nil {
			logger.Warn().Err(err).Msg("flows reload failed")
		}
		a.PublishFlowsUpdated("reload")
	}, logger)
	if err != nil {
		logger.Warn().Err(err).Msg("flows hot-reload unavailable")
		return
	}
	a.flowsWatcher = watcher
}

// openAgentWorkspaces ensures the workspace root exists, seeds it — and,
// only when EnsureRoot creates it for the first time, the Hive workspace too
// — then loads it eagerly, with the same warn-and-keep-last-good shape and
// the same publish-even-on-failure behaviour as openActions (app.go:610-637)
// so the UI re-reads and sees the error.
//
// When EnsureRoot fails, root is a configured location Hive cannot reach (an
// unmounted volume, a signed-out iCloud Drive) or one occupied by a file —
// spec §14 says that is reported, not silently replaced with a second empty
// root elsewhere. So nothing past that point may create root or anything
// under it: no seed, no Hive workspace, no watcher (NewWatcher's own
// MkdirAll would recreate exactly what EnsureRoot just refused to). The store
// still gets built — its Reload on a missing root is already a valid, empty
// snapshot — so the rest of the app has something non-nil to read; the
// Agents area (phase 6) is what surfaces the unavailable root to the user.
func (a *App) openAgentWorkspaces(root string, logger zerolog.Logger) {
	created, err := agentws.EnsureRoot(root)
	if err != nil {
		logger.Warn().Err(err).Str("root", root).Str("setting", "agent_workspaces.dir").Msg("agent workspace root unavailable")
		a.agentWorkspaceRootProblem = err.Error()
		a.agentWorkspaceStore = agentws.NewStore(root)
		if err := a.agentWorkspaceStore.Reload(); err != nil {
			logger.Warn().Err(err).Msg("agent workspace root load failed; using last-good (likely empty) workspace set")
		}
		return
	}

	if err := agentws.SeedDefaultsIfMissing(root); err != nil {
		logger.Warn().Err(err).Msg("agent workspace defaults seed failed")
	}
	if created {
		if err := agentws.SeedHiveWorkspace(root); err != nil {
			logger.Warn().Err(err).Msg("hive workspace seed failed")
		}
	}

	a.agentWorkspaceStore = agentws.NewStore(root)
	if err := a.agentWorkspaceStore.Reload(); err != nil {
		logger.Warn().Err(err).Msg("agent workspace root load failed; using last-good (likely empty) workspace set")
	}

	watcher, err := agentws.NewWatcher(root, func() {
		if err := a.agentWorkspaceStore.Reload(); err != nil {
			logger.Warn().Err(err).Msg("agent workspace reload failed")
		}
		count := len(a.agentWorkspaceStore.Statuses())
		a.Events.Publish(a.ctx, events.AgentWorkspacesUpdated{Count: count})
		// A hand edit to a manifest's schedules: list is a schedule change like
		// any other, so the scheduler re-reads on the same signal the UI does.
		a.scheduler.Reload()
	}, logger)
	if err != nil {
		logger.Warn().Err(err).Msg("agent workspace hot-reload unavailable")
		return
	}
	a.agentWorkspacesWatcher = watcher
}

// PublishLogAppended announces that the event log grew and wakes the engine to
// route it. The producer and the webhook listener go through the same path;
// this method is exported for the one caller that writes through the store
// directly — the e2e source-to-commit harness, which stands in for a producer
// tick.
func (a *App) PublishLogAppended(nextOffset int64) {
	a.engine.Wake()
	a.Events.Publish(a.ctx, events.LogAppended{NextOffset: nextOffset})
}

// PublishFlowsUpdated announces a change to the flow set, reinstalls the
// engine's runners against it, and drains the sources. The app's own writes go
// through it too, so a save and an external edit are one path.
//
// The drain is what makes an edit visible: without it a new or retyped source
// node produces nothing until the next poll tick, which is up to
// settings.MinPollInterval away and reads as a broken canvas. One deploy earns
// one drain — the flows watcher coalesces a deploy's flow and layout writes
// into a single reload before this is called.
func (a *App) PublishFlowsUpdated(reason string) {
	a.engine.Reload()
	a.Events.Publish(a.ctx, events.FlowsUpdated{Reason: reason})
	a.runSourcesForFlowChange()
}

// runSourcesForFlowChange drains the sources detached from its caller. A tick
// is network I/O, and PublishFlowsUpdated is called from the flows watcher's
// goroutine and from a save RPC — neither may block on a fetch.
func (a *App) runSourcesForFlowChange() {
	if a.Sources == nil {
		return
	}
	go func() {
		if _, err := a.Sources.Run(a.ctx); err != nil {
			a.logger.Debug().Err(err).Msg("source run after flow change unavailable")
		}
	}()
}

// RefreshSources clears fetch caches and runs one producer tick. Engine commits
// are asynchronous, so callers should retry reads briefly. Mock modes return
// KindUnavailable.
func (a *App) RefreshSources(ctx context.Context) (ingest.TickSummary, error) {
	return a.Sources.Refresh(ctx)
}

// MountAPI mounts h onto the loopback webhook listener at prefix so the HTTP API
// shares its port. It reports false when no listener exists. Call before Start.
func (a *App) MountAPI(prefix string, h http.Handler) bool {
	if a.webhook == nil {
		return false
	}
	a.webhook.MountAPI(prefix, h)
	return true
}

// buildEngine wires the flow engine over the store and the live flow set. It
// runs in every mode, including the mock ones: a fixture that seeds inbox
// items also appends the source snapshot they came from, so the engine's
// replay recomputes exactly the membership the fixture declared rather than
// contradicting it.
func (a *App) buildEngine(logger zerolog.Logger) *runtime.Engine {
	return runtime.NewEngine(runtime.EngineOptions{
		Log:      a.Stores.EventLog,
		Items:    a.Stores.InboxItems,
		Commits:  a.Stores.EventLog,
		KV:       a.Stores.NodeKV,
		Flows:    a.flowStore,
		Scripts:  a.scripts,
		Logger:   logger,
		Events:   a.Events,
		Recorder: a.Activity,
	})
}

// buildSources wires the instance half of every source connector's
// declaration. The descriptors are static and live in the registry; the
// factories need dependencies — the GitHub fetcher — and so are built here,
// at the one place that holds them.
//
// TestFactoriesCoverEveryDescriptor fails if a registered connector has no
// factory, which is the failure mode this split trades for: a connector
// declared and not wired is a source node the editor offers and nothing ever
// polls.
func (a *App) buildSources(logger zerolog.Logger) *ingest.Resolver {
	return ingest.NewResolver(a.flowStore, sourceFactories(a.fetchers, a.grafanaFetchers, a.posthogFetchers, a.giteaFetchers, a.rssFetchers, a.execEnv), logger)
}

// sourceFactories is the instance half of the connector registry. It is a
// function of its dependencies rather than a method so the bijection test can
// hold it against the descriptors without standing up an App.
func sourceFactories(fetchers *ghsource.Fetchers, grafanaFetchers *grafana.Fetchers, posthogFetchers *posthog.Fetchers, giteaFetchers *gitea.Fetchers, rssFetchers *rss.Fetchers, env execsource.Environment) map[string]connector.Factory {
	factories := map[string]connector.Factory{
		webhook.Descriptor.Type:    webhook.NewFactory(),
		execsource.Descriptor.Type: execsource.NewFactory(env),
	}
	// Mock modes have no fetchers, so the GitHub connector has nothing to
	// construct instances over and is left out of the map: resolving one logs
	// and skips rather than dereferencing nil.
	if fetchers != nil {
		factories[ghsource.Descriptor.Type] = ghsource.NewFactory(fetchers)
	}
	// Always wired in a real build; the nil guard is only for the bijection test.
	if grafanaFetchers != nil {
		factories[grafana.MetricsDescriptor.Type] = grafana.NewMetricsFactory(grafanaFetchers)
		factories[grafana.AlertsDescriptor.Type] = grafana.NewAlertsFactory(grafanaFetchers)
		factories[grafana.IRMAlertsDescriptor.Type] = grafana.NewIRMAlertsFactory(grafanaFetchers)
	}
	if posthogFetchers != nil {
		factories[posthog.ErrorsDescriptor.Type] = posthog.NewErrorsFactory(posthogFetchers)
		factories[posthog.AlertsDescriptor.Type] = posthog.NewAlertsFactory(posthogFetchers)
	}
	if giteaFetchers != nil {
		factories[gitea.Descriptor.Type] = gitea.NewFactory(giteaFetchers)
	}
	if rssFetchers != nil {
		factories[rss.Descriptor.Type] = rss.NewFactory(rssFetchers)
	}
	return factories
}

// lastTicker keeps mock modes' missing producer a nil interface rather than
// a non-nil one wrapping a nil pointer.
func (a *App) lastTicker() LastTicker {
	if a.producer == nil {
		return nil
	}
	return a.producer
}

// buildProducer starts nothing; it wires the event-log producer over every
// enabled pull-mode source node across all flows. Mock modes have no fetcher
// and therefore no producer.
func (a *App) buildProducer(logger zerolog.Logger) *ingest.Producer {
	if a.fetchers == nil {
		return nil
	}
	producer := ingest.NewProducer(ingest.ProducerDeps{
		Ingester:  a.Stores.InboxItems,
		Snapshots: a.Stores.EventLog,
		Heads:     a.Stores.SourceHeads,
		Sources:   a.sources,
		Interval:  a.pollInterval,
		Notifier:  a,
		Logger:    logger,
	})
	producer.SetRecorder(a.Activity)
	producer.SetDebugPause(a.settings.Development.Debug.PauseIngest.Duration())
	return producer
}

// buildOutputWorker wires the output worker. Mock modes keep the worker for
// explicit detail-pane confirmation RPCs but never start its loop: that keeps
// the configured action path real in e2e while stopping a background shell
// action from compromising fixture determinism.
//
// Actions resolve through FlowNotifyActions rather than the catalog directly:
// a notify node's config lives in its flow, not in actions.yml, so the worker
// resolves those ids from the live flow set and everything else from the
// authored catalog.
func (a *App) buildOutputWorker(cfg Config) *dispatch.Worker {
	a.dispatcher = dispatch.NewDispatcher(outputExecutors(a.launcher, a.AgentWorkspaces, a.publisher, a.observedNotifier(cfg.Notifier), cfg.Gate, a.Stores.InboxItems, a.execEnv, cfg.Logger))
	worker := dispatch.NewWorker(a.Stores.OutputCommands, dispatch.NewFlowNotifyActions(a.flowStore, a.actionStore), a.dispatcher, dispatch.DefaultOutputWorkerInterval, cfg.Logger)
	worker.SetRecorder(a.Activity)
	worker.SetJobRecorder(a.Jobs)
	return worker
}

// observedNotifier decorates the adapter's delivery port so the core learns
// when a notify terminal's delivery actually reaches the user. dispatch's
// NotifyExecutor calls this port directly and never touches the event bus —
// events carry payloads in the core, not in dispatch, which is why the wrap
// happens here instead: app.go is where Config's driven port enters the
// core. Only a successful delivery publishes: dispatch itself retries a
// failed one rather than recording it, so announcing on error would report a
// notification the user never saw. A nil port stays nil — NotifyExecutor's
// own nil check is what turns "no notifier configured" into a typed failure,
// and wrapping nil here would silently paper over that.
func (a *App) observedNotifier(next dispatch.SystemNotifier) dispatch.SystemNotifier {
	if next == nil {
		return nil
	}
	return systemNotifierFunc(func(ctx context.Context, in dispatch.SystemNotification) error {
		if err := next.Notify(ctx, in); err != nil {
			return err
		}
		profileID, _ := in.Data["profileId"].(string)
		itemID, _ := in.Data["itemId"].(int64)
		// a.ctx, not the incoming ctx, on purpose: every other publish in this
		// file uses the app's own lifetime rather than whatever call
		// triggered it (see the field doc on App.ctx), and a Buffer
		// subscriber's blocking wait should be bounded by "is the app still
		// running", not by a dispatch command's own deadline.
		//nolint:contextcheck // deliberate -- see comment above
		a.Events.Publish(a.ctx, events.NotificationRaised{
			ProfileID: profileID,
			ItemID:    itemID,
			Title:     in.Title,
			Body:      in.Body,
			Severity:  in.Severity,
			InApp:     in.InApp,
		})
		return nil
	})
}

// systemNotifierFunc adapts a plain function to dispatch.SystemNotifier, the
// same shape http.HandlerFunc gives http.Handler.
type systemNotifierFunc func(ctx context.Context, n dispatch.SystemNotification) error

func (f systemNotifierFunc) Notify(ctx context.Context, n dispatch.SystemNotification) error {
	return f(ctx, n)
}

type defaultAgentEnvReader struct{ env *execenv.Resolver }

func (r defaultAgentEnvReader) DefaultAgent(ctx context.Context) string {
	return r.env.Getenv(ctx, config.EnvDefaultAgent)
}

type hiveConfigEnvironment interface {
	Getenv(context.Context, string) string
}

// Hive keeps this list private. The ACL mirrors it so a Dock launch can honor
// XDG_CONFIG_HOME from the login shell instead of probing the launcher's home.
var hiveConfigNames = []string{"config.yaml", "config.yml", "hive.yaml", "hive.yml"}

func resolveHiveConfigLocation(ctx context.Context, env hiveConfigEnvironment) HiveConfigLocation {
	if path := env.Getenv(ctx, "HIVE_CONFIG"); path != "" {
		return HiveConfigLocation{Path: path, EnvironmentOverride: true}
	}
	configDir := config.DefaultConfigDir()
	if configHome := env.Getenv(ctx, "XDG_CONFIG_HOME"); configHome != "" {
		configDir = filepath.Join(configHome, "hive")
	}
	for _, name := range hiveConfigNames {
		path := filepath.Join(configDir, name)
		if _, err := os.Stat(path); err == nil {
			return HiveConfigLocation{Path: path}
		}
	}
	return HiveConfigLocation{Path: filepath.Join(configDir, hiveConfigNames[0])}
}

func resolveHiveDefaultAgent(ctx context.Context, env hiveConfigEnvironment, configured string, profiles []string) string {
	if preferred := environmentAgentOverride(env.Getenv(ctx, config.EnvDefaultAgent), profiles); preferred != "" {
		return preferred
	}
	return configured
}

// environmentAgentOverride answers the HIVE_DEFAULT_AGENT value when hive
// would honour it — it names one of the configured profiles — and ""
// otherwise. Hive's lookup is exact and it ignores an unknown name, so every
// reader of the variable applies this one rule against the same profile list.
func environmentAgentOverride(value string, profiles []string) string {
	preferred := strings.TrimSpace(value)
	if preferred == "" || !slices.Contains(profiles, preferred) {
		return ""
	}
	return preferred
}

// openWebhook constructs the optional loopback listener without binding it.
// Port zero is passed through to net.Listen so the OS allocates without a
// probe/rebind race. Mock instances only claim a listener through an explicit
// port override, keeping parallel e2e lanes isolated.
func (a *App) openWebhook(_ context.Context, cfg Config) {
	a.webhookHost = cfg.Settings.HTTP.Host
	a.webhookPort = cfg.Settings.HTTP.Port
	if !cfg.Settings.HTTP.Enabled {
		return
	}
	if cfg.MockMode != "" && !cfg.Settings.EnvironmentOverridden(settings.EnvHTTPPort) {
		return
	}

	a.webhook = webhook.NewListener(a.Stores.InboxItems, a.Stores.EventLog, a.Stores.WebhookCaptures, a.Stores.InboxItems, a.sources.PushInstances, a.webhookHost, a.webhookPort, a, cfg.Logger)
	a.webhook.SetRecorder(a.Activity)
}

// openHiveRuntime opens the Hive dependencies desktop actions need. The
// desktop keeps its own database, while sessions and internal events
// intentionally use Hive's shared state and event bus.
//
// It splits in two on purpose. The database and the event bus are opened once
// and live for the process: reopening a connection pool underneath in-flight
// queries, or restarting a bus subscribers already hold, buys nothing a
// config edit needs. Everything the Hive config decides is built by
// buildHiveServices, which ReloadHiveRuntime runs again (ADR the-hive-runtime-rebinds-on-a-config-write-instead-of-requiring-a-restart).
func (a *App) openHiveRuntime(ctx context.Context, cfg Config) error {
	dataDir := cfg.Paths.HiveDataDir
	if dataDir == "" {
		dataDir = cfg.Paths.DataDir
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("create hive data directory: %w", err)
	}
	a.hiveDataDir = dataDir

	hiveCfg, err := a.loadHiveConfig(ctx, dataDir)
	if err != nil {
		return err
	}
	if err := scripts.EnsureExtracted(dataDir, "desktop"); err != nil {
		cfg.Logger.Warn().Err(err).Msg("extract hive action scripts failed")
	}

	// coredb.Open takes no context. Threading one means changing the shared
	// internal/data/db package and its CLI callers.
	//nolint:contextcheck // shared signature, see internal/data/db
	database, err := coredb.Open(dataDir, coredb.OpenOptions{
		MaxOpenConns: hiveCfg.Database.MaxOpenConns,
		MaxIdleConns: hiveCfg.Database.MaxIdleConns,
		BusyTimeout:  hiveCfg.Database.BusyTimeout,
	})
	if err != nil {
		return fmt.Errorf("open hive action database: %w", err)
	}
	if err := stores.MigrateFromJSON(ctx, database, dataDir); err != nil {
		_ = database.Close()
		return fmt.Errorf("migrate hive action data: %w", err)
	}
	a.hiveDB = database
	a.honeycomb = dispatch.NewHiveHoneycomb(hive.NewHoneycombService(stores.NewHCStore(database), cfg.Logger.With().Str("component", "hive-hc").Logger()))

	bus := eventbus.New(64)
	busCtx, cancel := context.WithCancel(ctx)
	a.hiveBusCancel = cancel
	go bus.Start(busCtx)
	a.hiveBus = bus

	built := a.buildHiveServices(hiveCfg, database, bus)
	a.agentCommands.Store(&built.agentCommands)
	a.launcher = dispatch.NewHiveSessionLauncher(built.sessions)
	a.launcher.SetRecorder(a.Activity)
	a.launcher.SetItemSessionLinker(a.Stores.ItemSessions, cfg.Logger)
	a.sessions = dispatch.NewHiveSessionManager(built.sessions, built.statuses, hiveSessionWindowSource{terminals: a.terminals}, built.git, built.pollInterval)
	a.publisher = dispatch.NewHiveMessagePublisher(built.messages)
	return nil
}

// Reloads re-resolve HIVE_CONFIG because it can change between calls, and
// first run can turn an absent path into a real file.
func (a *App) loadHiveConfig(ctx context.Context, dataDir string) (*config.Config, error) {
	location := resolveHiveConfigLocation(ctx, a.execEnv)
	a.hiveConfig.Store(&location)
	hiveCfg, err := config.Load(location.Path, dataDir)
	if err != nil {
		return nil, fmt.Errorf("load hive config for actions: %w", err)
	}
	agents := make([]string, 0, len(hiveCfg.Agents.Profiles))
	for agent := range hiveCfg.Agents.Profiles {
		agents = append(agents, agent)
	}
	hiveCfg.Agents.Default = resolveHiveDefaultAgent(ctx, a.execEnv, hiveCfg.Agents.Default, agents)
	return hiveCfg, nil
}

// hiveServices is everything the Hive config decides. Its fields are what a
// reload replaces; anything not here is either process-lived (the database,
// the bus) or this app's own (the tmux pool).
func newTmuxRunner(
	binary func(context.Context) (string, error),
	environ func(context.Context) []string,
) tmuxadapter.Runner {
	return tmuxadapter.NewExecRunner(tmuxadapter.ExecRunnerOptions{
		Binary:      binary,
		Environ:     tmuxcc.RunnerEnviron(environ),
		PrepareArgs: tmuxcc.RunnerArgs,
	})
}

func (a *App) resolveTmuxBinary(ctx context.Context) (string, error) {
	return resolveTmuxBinary(ctx, a.tmux.Path, a.execEnv.LookPath)
}

func resolveTmuxBinary(
	ctx context.Context,
	locate func() (string, error),
	lookPath func(context.Context, string) (string, error),
) (string, error) {
	path, err := locate()
	if err == nil || !errors.Is(err, tmuxbin.ErrNotFound) {
		return path, err
	}
	return lookPath(ctx, "tmux")
}

type tmuxSessionRenamer interface {
	RenameSession(ctx context.Context, from, to string) error
}

// hiveMultiplexer keeps the one-shot adapter and Desktop's control-mode client
// on the same session name. A raw rename would leave the control-mode manager
// registered under the old name.
type hiveMultiplexer struct {
	*tmuxadapter.Client
	renamer tmuxSessionRenamer
}

func (m hiveMultiplexer) RenameSession(ctx context.Context, target multiplexer.Target, newName string) error {
	if err := target.ValidateSession(); err != nil {
		return err
	}
	return m.renamer.RenameSession(ctx, target.Session, newName)
}

type hiveServices struct {
	sessions      *hive.SessionService
	statuses      *hive.StatusService
	git           git.Git
	messages      *hive.MessageService
	pollInterval  time.Duration
	agentCommands map[string]string
}

// buildHiveServices constructs the config-derived half of the Hive runtime. It
// takes the database and bus rather than opening them so it can run more than
// once against the same ones.
//
// Nothing here is closed on the way out of a reload, because nothing here owns
// anything to close: the session, status and message services hold the handles
// they were given and spawn per call, and the tmux capture recorder writes one
// file per capture without keeping a handle.
func (a *App) buildHiveServices(hiveCfg *config.Config, database *coredb.DB, bus *eventbus.EventBus) hiveServices {
	profile := hiveCfg.Agents.DefaultProfile()
	renderer := tmpl.New(tmpl.Config{
		ScriptPaths:  scripts.ScriptPaths(a.hiveDataDir),
		AgentCommand: profile.CommandOrDefault(hiveCfg.Agents.Default),
		AgentWindow:  hiveCfg.Agents.Default,
		AgentFlags:   profile.ShellFlags(),
	})
	exec := newEnvExecutor(a.execEnv)
	gitExec := git.NewExecutor(hiveCfg.GitPath, exec)
	serviceLogger := a.logger.With().Str("component", "hive-actions").Logger()
	tmuxBinary := func(ctx context.Context) (string, error) {
		return a.resolveTmuxBinary(ctx)
	}
	tmuxClient := tmuxadapter.New(newTmuxRunner(tmuxBinary, a.execEnv.Environ), serviceLogger.With().Str("component", "tmux").Logger())
	sessionMultiplexer := hiveMultiplexer{Client: tmuxClient, renamer: a.terminals}
	sessions := hive.NewSessionService(
		stores.NewSessionStore(database),
		gitExec,
		hiveCfg,
		bus,
		exec,
		renderer,
		hive.PlainStyler{},
		serviceLogger,
		io.Discard,
		io.Discard,
		sessionMultiplexer,
	)

	var statusService *hive.StatusService
	if a.mock == "" {
		statusOptions := []terminaltmux.Option{
			terminaltmux.WithPaneSource(tmuxClient),
			terminaltmux.WithStatusOptions(terminalstatus.OptionsFromConfig(hiveCfg.Terminal.Status, hiveCfg.Tmux.PollInterval)),
			terminaltmux.WithMissingTolerance(hiveCfg.Terminal.Status.Confirm.Missing.Polls),
		}
		if hiveCfg.Tmux.CaptureRecording.Enabled {
			recorder, recorderErr := terminaltmux.NewJSONCaptureRecorder(hiveCfg.TmuxCaptureRecordingsDir())
			if recorderErr != nil {
				a.logger.Warn().Err(recorderErr).Msg("enable tmux pane capture recording for session status")
			} else {
				statusOptions = append(statusOptions, terminaltmux.WithCaptureRecorder(recorder))
			}
		}
		terminalManager := coreterminal.NewManager([]string{"tmux"})
		terminalManager.Register(terminaltmux.NewFromPreviewMatchers(hiveCfg.Tmux.PreviewWindowMatcher, statusOptions...))
		statusService = hive.NewStatusService(terminalManager, hiveCfg.Git.StatusWorkers)
	}

	return hiveServices{
		sessions:      sessions,
		statuses:      statusService,
		git:           gitExec,
		messages:      hive.NewMessageService(stores.NewMessageStore(database, hiveCfg.Messaging.MaxMessages), hiveCfg, bus),
		pollInterval:  hiveCfg.Tmux.PollInterval,
		agentCommands: agentCommands(hiveCfg),
	}
}

// ReloadHiveRuntime re-reads the Hive config and points the session, status
// and message adapters at services built from it. It is what makes a config
// the app itself just wrote take effect without a relaunch.
//
// A failed load changes nothing: the running services keep serving the config
// they were built from, which is strictly better than a process left with no
// session service because someone saved a typo.
//
// What it does not reload is deliberate and documented in
// docs/architecture.md: the database pool, the event bus, and the honeycomb
// store keep their startup settings, so database: and a changed data dir still
// need a restart.
func (a *App) ReloadHiveRuntime(ctx context.Context) error {
	if a.hiveDB == nil || a.hiveBus == nil {
		return Errorf(KindInternal, "the Hive runtime is not open")
	}
	a.reloadMu.Lock()
	defer a.reloadMu.Unlock()
	hiveCfg, err := a.loadHiveConfig(ctx, a.hiveDataDir)
	if err != nil {
		return Wrap(err, KindInvalid, "reloading the Hive config")
	}
	built := a.buildHiveServices(hiveCfg, a.hiveDB, a.hiveBus)
	a.agentCommands.Store(&built.agentCommands)
	a.launcher.Rebind(built.sessions)
	a.sessions.Rebind(built.sessions, built.statuses, built.git, built.pollInterval)
	a.publisher.Rebind(built.messages)
	return nil
}

// HiveConfigLocation reports where the Hive config resolved on the last load.
// It is a method rather than a field read because a reload replaces it.
func (a *App) HiveConfigLocation() HiveConfigLocation {
	if current := a.hiveConfig.Load(); current != nil {
		return *current
	}
	return HiveConfigLocation{}
}

func (a *App) profileCommands() map[string]string {
	if current := a.agentCommands.Load(); current != nil {
		return *current
	}
	return nil
}

// agentCommands projects hive's agent profiles onto a full command line,
// flags included, for workspace presets and peer agent windows.
//
// Flags used to be dropped here so a workspace could not inherit
// --dangerously-skip-permissions from hive's config. They now cross, because
// the destination changed: a preset is seeded into the manifest once, where
// the user reads and edits it, rather than resolved out of hive.yaml at every
// launch. Workspace launches do not read this map, so a hive config edit cannot
// change what an existing workspace runs — which is the guarantee the old
// seam was reaching for (ADR the-workspace-command-is-a-template, superseding
// ADR a-workspace-declares-its-own-authority §1-2).
func agentCommands(cfg *config.Config) map[string]string {
	commands := make(map[string]string, len(cfg.Agents.Profiles))
	for key, profile := range cfg.Agents.Profiles {
		line := profile.CommandOrDefault(key)
		if flags := profile.ShellFlags(); flags != "" {
			line += " " + flags
		}
		commands[key] = line
	}
	return commands
}
