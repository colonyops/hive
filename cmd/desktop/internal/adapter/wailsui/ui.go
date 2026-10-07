package wailsui

import (
	"context"
	"embed"
	"fmt"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/e2e"
	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	wailsnotify "github.com/wailsapp/wails/v3/pkg/services/notifications"
	"github.com/wailsapp/wails/v3/pkg/updater"
)

// UI is the Wails driving adapter as one object: the native shell, the bound
// services, and the event subscriptions that wake the frontend.
//
// It is built in two steps because the core needs two things from it before
// it exists. New makes the notifier and the focus-backed notification gate —
// the driven ports app.Config asks for — and Mount does everything that needs
// a built core.
type UI struct {
	logger        zerolog.Logger
	mock          string
	settingsStore *settings.Store

	focus         *FocusState
	notifications *NotificationService
	native        *wailsnotify.NotificationService

	trayIcon      []byte
	trayIconLinux []byte
	app           *application.App
	window        *application.WebviewWindow
	tray          *MenuBarTray
	updater       *UpdaterService
	cancelEvents  func()
}

// Build identifies the running binary. It is passed in because the ldflags
// that populate it bind to package main.
type Build struct {
	Version string
	Commit  string
	Date    string
}

// MountOptions is everything the native shell needs that the core does not
// carry: the embedded assets and icons, and the build identity.
type MountOptions struct {
	Assets   embed.FS
	AppIcon  []byte
	TrayIcon []byte
	// TrayIconLinux is the white-rendered tray mark: Linux panels take raw
	// pixmaps, not tintable templates (see applyTrayIcon).
	TrayIconLinux []byte
	Build         Build
	// Terminal carries the per-run bearer token and WebSocket path the terminal
	// bootstrap hands the webview. Zero when no terminal transport was mounted.
	Terminal TerminalTransport
	// PopupTerminal carries the same for the ephemeral pop-up terminal (ADR
	// 0048). Zero when its stream was not mounted.
	PopupTerminal PopupTerminalTransport
	// Agents carries the same for the Agents area's control plane and the
	// shared ptyterm stream a workspace session rides (ADR ptyterm-terminals-are-caller-addressed, ADR a-workspace-declares-its-own-authority).
	// Zero when its stream was not mounted.
	Agents AgentsTransport
	// AutoUpdate seeds the updater's initial toggle from settings.yaml.
	AutoUpdate bool
	// UpdateChannel is the resolved release channel to follow.
	UpdateChannel func(defaultChannel string) string
}

// New builds the parts of the adapter the core depends on. appIcon is
// materialized into the notification attachment cache here, which is why it
// is needed this early.
//
// Mock and server builds deliberately skip the native notification service:
// e2e verifies preference persistence without an OS bus, banner, or
// permission prompt, and the frontend still gets a descriptive unavailable
// binding.
func New(mock string, settingsStore *settings.Store, appIcon []byte, logger zerolog.Logger) *UI {
	ui := &UI{
		logger:        logger,
		mock:          mock,
		settingsStore: settingsStore,
		focus:         NewFocusState(),
		notifications: NewUnavailableNotificationService(fmt.Errorf("native notifications unavailable in desktop mock mode")),
	}
	if mock != "" {
		return ui
	}

	ui.native = wailsnotify.New()
	notifier, err := NewNotifier(ui.native, appIcon)
	if err != nil {
		logger.Warn().Err(err).Msg("native notifications unavailable")
		ui.notifications = NewUnavailableNotificationService(err)
		return ui
	}
	ui.notifications = NewNotificationService(notifier)
	return ui
}

// Notifier is the driven port a flow's notify terminal delivers through.
func (u *UI) Notifier() dispatch.SystemNotifier { return NewFlowNotifier(u.notifications) }

// Gate is the driven port that answers whether a notification may be
// delivered, and where. It holds the window's focus state, because the
// automatic delivery mode means "a banner only when I am looking elsewhere"
// and only this side of the app knows both halves.
//
// It reads settings through app.NewSettingsService rather than u.settingsStore
// directly: policy resolution belongs to the core, and this driven port has
// to exist before app.New builds core.Settings itself (New takes this Gate as
// one of Config's two driven ports), so it gets its own settings-only view
// over the same store instead of waiting for one.
func (u *UI) Gate() dispatch.NotificationGate {
	return NewNotificationGate(app.NewSettingsService(u.settingsStore), u.focus, u.logger)
}

// Mount builds the Wails application over core: the bound services, the
// window and its hooks, the tray, the updater, and the event subscriptions.
// Nothing is running when it returns — call Run.
func (u *UI) Mount(ctx context.Context, core *app.App, opts MountOptions) {
	// The updater service goes in the Services slice, but its engine only
	// exists after application.New, so the live Updater is attached below.
	// core.Settings.SetUpdatesEnabled is the settings mutation -- the adapter
	// has none of its own.
	u.updater = NewUpdaterService(opts.Build.Version, opts.AutoUpdate, DefaultUpdateCheckInterval, core.Settings.SetUpdatesEnabled, u.logger)

	// The tray refresh is published before the flows subscription can fire it:
	// the flows watcher calls subscribers from its own goroutine.
	u.cancelEvents = Subscribe(ctx, core.Events, u.refreshTray)

	u.trayIcon = opts.TrayIcon
	u.trayIconLinux = opts.TrayIconLinux
	u.app = application.New(u.options(core, opts))
	u.attachUpdater(opts)
	u.buildMenu()
	u.buildWindow()
	u.buildTray(core)
}

func (u *UI) options(core *app.App, opts MountOptions) application.Options {
	services := []application.Service{
		application.NewService(NewGitHubService(core.GitHub)),
		application.NewService(NewGrafanaService(core.Grafana)),
		application.NewService(NewPostHogService(core.PostHog)),
		application.NewService(NewGiteaService(core.Gitea)),
		application.NewService(NewIntegrationsService(core.Integrations)),
		application.NewService(NewPipelineService(core.Inbox)),
		application.NewService(NewSourcesService(core.Sources)),
		application.NewService(NewSessionService(core.Sessions)),
		application.NewService(NewFlowsService(core.Flows)),
		application.NewService(NewActionsService(core.Actions)),
		application.NewService(NewActivityService(core.Activity)),
		application.NewService(NewJobService(core.Jobs)),
		application.NewService(NewActionRunService(core.ActionRuns)),
		application.NewService(NewTasksService(core.Tasks)),
		application.NewService(NewSystemService(core.System, opts.Build.Version, opts.Build.Commit, opts.Build.Date)),
		application.NewService(NewHiveConfigService(core.HiveConfig)),
		application.NewService(NewAnalyticsService(core.Analytics)),
		application.NewService(NewSettingsService(core.Settings)),
		application.NewService(NewMenuBarService(core.MenuBar)),
		application.NewService(NewWebhookService(core.Webhooks)),
		application.NewService(NewPromptsService(core.Prompts)),
		application.NewService(NewReportService(core.Report)),
		application.NewService(NewReleaseNotesService(core.ReleaseNotes, opts.Build.Version)),
		application.NewService(NewPerfService(core.Perf)),
		application.NewService(NewObservabilityService(core.Observability)),
		application.NewService(NewDevToolsService(core.DevTools)),
		application.NewService(NewTerminalService(core.Terminals, core.Webhooks, opts.Terminal)),
		application.NewService(NewPopupTerminalService(core.PopupTerminals, core.Webhooks, opts.PopupTerminal)),
		application.NewService(NewAgentsService(core.AgentWorkspaces, core.Webhooks, opts.Agents)),
		application.NewService(u.updater),
	}
	if u.native != nil {
		services = append(services, application.NewService(u.native))
	}
	services = append(services,
		application.NewService(u.notifications),
		application.NewService(NewWindowService(u.focus)),
	)

	// Test-only /_e2e/reset harness (nil outside the Docker e2e mock modes).
	// Built this late deliberately: app.New has seeded actions.yml and mock
	// seeding has run, so the captured config baseline is the post-boot state
	// a reset must restore.
	reset := e2e.NewStateResetHarnessForInstance(core.PipelineDB(), core.Stores, core.HiveConn(), u.mock, core.RuntimePaths(), u.logger)

	return application.Options{
		Name:        "Hive",
		Description: "Hive desktop application",
		Icon:        opts.AppIcon,
		Services:    services,
		// Every error a bound method returns reaches the frontend as the
		// thrown exception's `cause`, carrying the Kind the core assigned. It
		// is what lets the UI tell "sign in again" from "we broke" without
		// matching on error text.
		MarshalError: MarshalError,
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(opts.Assets),
			Middleware: e2e.SmokeMiddleware(core.PipelineDB(), core.Stores, core.HiveConn(), reset, core.PublishLogAppended),
		},
		Mac: application.MacOptions{
			ActivationPolicy: application.ActivationPolicyRegular,
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		Linux: application.LinuxOptions{
			// The window-closing hook below hides rather than destroys, so the
			// app already survives a close. This is the belt-and-braces peer of
			// the Mac option above: whatever route destroys the last window,
			// Hive keeps running in the tray until Quit.
			DisableQuitOnLastWindowClosed: true,
			// Matches the binary name so window managers group windows with the
			// .desktop entry a tarball install may add by hand.
			ProgramName: "hive-desktop",
		},
	}
}

// attachUpdater configures self-update only for published release builds:
// ReleaseChannel rejects source builds ("dev") and pseudo-versions, so the
// engine stays nil there and the service degrades to Available:false with a
// no-op ticker. A published build follows its own channel (a beta build
// tracks beta, per docs/decisions/0004) unless settings.yaml overrides it.
//
// The check ticker it starts is deliberately not rooted at this call's
// context: SetAutoUpdate can start it again long after Mount returned, so it
// outlives every context that could be threaded here. Stop, which main's
// shutdown calls, is its teardown.
//
//nolint:contextcheck // see above; the ticker's lifetime is Stop, not a call
func (u *UI) attachUpdater(opts MountOptions) {
	channel, ok := ReleaseChannel(opts.Build.Version)
	if !ok {
		return
	}
	provider := NewManifestProvider(DefaultManifestBaseURL, opts.UpdateChannel(channel))
	if err := u.app.Updater.Init(updater.Config{
		CurrentVersion: opts.Build.Version,
		Providers:      []updater.Provider{provider},
	}); err != nil {
		u.logger.Warn().Err(err).Msg("desktop auto-update unavailable; updater init failed")
		return
	}
	u.updater.Attach(u.app.Updater)
}

func (u *UI) buildWindow() {
	u.window = u.app.Window.NewWithOptions(application.WebviewWindowOptions{
		EnableFileDrop:   true,
		Title:            "Hive",
		Width:            1360,
		Height:           864,
		BackgroundColour: application.NewRGB(16, 19, 24),
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

	u.window.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		if target := event.Context().DropTargetDetails(); target != nil {
			emitTerminalFilesDropped(TerminalFilesDropped{Target: target.ElementID, Paths: event.Context().DroppedFiles()})
		}
	})

	// The sole owner of notification activation: a click brings the existing
	// window forward, and — for a notification a flow's notify node sent —
	// publishes which item it came from. Routing to that item stays in the
	// frontend, and out of the notification binding.
	if u.native != nil {
		u.native.OnNotificationResponse(func(result wailsnotify.NotificationResult) {
			u.reveal()
			if activation, ok := NotificationActivationFrom(result); ok {
				emitNotificationActivated(activation)
			}
		})
	}

	u.window.OnWindowEvent(events.Common.WindowFocus, func(*application.WindowEvent) {
		if u.focus.Set(true) {
			emitWindowFocus()
		}
	})
	u.window.OnWindowEvent(events.Common.WindowLostFocus, func(*application.WindowEvent) {
		if u.focus.Set(false) {
			emitWindowBlur()
		}
	})

	// Closing the window keeps the app running in the dock and tray; it can be
	// reopened from either. Quitting is done via Cmd+Q or the tray menu.
	//
	// This must be a hook, not OnWindowEvent: hooks run synchronously before
	// listeners, so Cancel() reliably aborts Wails' own window-destroy
	// listener, which otherwise races this callback in a separate goroutine.
	//
	// Hiding is only safe when there is a tray to restore the window from. A
	// Linux session with no StatusNotifier host — vanilla GNOME without the
	// AppIndicator extension — has none, and hiding there would leave Hive
	// running with no window, no tray, and no way back. Closing quits instead.
	closeHidesWindow := trayHostAvailable()
	if !closeHidesWindow {
		u.logger.Info().Msg("no system tray host detected; closing the window will quit Hive")
	}
	u.window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if !closeHidesWindow {
			u.app.Quit()
			return
		}
		u.window.Hide()
		if u.focus.Set(false) {
			emitWindowBlur()
		}
		e.Cancel()
	})

	u.app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(*application.ApplicationEvent) {
		u.window.Show()
	})
}

// buildTray is not rooted at Mount's ctx: like attachUpdater above, the tray
// it builds outlives Mount's call — clicks and event-triggered refreshes fire
// for the rest of the process's life, long past setup, so the tray's calls
// root their own context rather than reuse one that is about to go out of
// scope.
//
//nolint:contextcheck // see above; the tray's lifetime is Close, not a call
func (u *UI) buildTray(core *app.App) {
	u.tray = NewMenuBarTray(TrayDeps{
		App:          u.app,
		Flows:        NewFlowsService(core.Flows),
		MenuBar:      core.MenuBar,
		Inbox:        core.Inbox,
		Sources:      core.Sources,
		Logger:       u.logger,
		TemplateIcon: u.trayIcon,
		LinuxIcon:    u.trayIconLinux,
		Show:         u.reveal,
		Quit:         u.app.Quit,
	})
	u.app.OnShutdown(u.tray.Close)
}

// refreshTray re-renders the tray menu. Not rooted at Subscribe's ctx for the
// same reason as buildTray: events fire long after Mount returns.
//
//nolint:contextcheck // see above; the tray's lifetime is Close, not a call
func (u *UI) refreshTray() {
	if u.tray != nil {
		u.tray.Refresh()
	}
}

func (u *UI) reveal() {
	u.window.Show()
	u.window.Focus()
}

// Run blocks until the app quits.
func (u *UI) Run() error { return u.app.Run() }

// Quit asks the application to terminate. It is what the tray's Quit item
// does; a signal handler takes the same path so every exit runs one teardown.
// It is a no-op before Mount, and on macOS it does not return, so a caller that
// must exit regardless needs its own backstop.
func (u *UI) Quit() {
	if u.app == nil {
		return
	}
	u.app.Quit()
}

// OnShutdown registers work to run while the application is terminating. It is
// how a teardown reaches macOS at all: Quit there is [NSApp terminate:], which
// runs these hooks from applicationShouldTerminate: and then exits the process
// without ever returning from Run. Registering only after Run returns would
// mean every quit on macOS — tray, Cmd+Q, signal — skipped the teardown.
func (u *UI) OnShutdown(f func()) {
	if u.app == nil {
		return
	}
	u.app.OnShutdown(f)
}

// Close stops what the adapter owns. The core's own teardown is App.Close.
func (u *UI) Close() {
	if u.updater != nil {
		u.updater.Stop()
	}
	if u.cancelEvents != nil {
		u.cancelEvents()
	}
}

// SeedMock installs the deterministic inbox rows a fixture run needs. Mock
// mode has no live producer, so nothing else would fill the feed.
func (u *UI) SeedMock(ctx context.Context, core *app.App) {
	if u.mock != "feed" && u.mock != "action-smoke" {
		return
	}
	e2e.SeedMockInboxItemsOrWarn(ctx, core.PipelineDB(), core.Stores.EventLog, u.logger)
}
