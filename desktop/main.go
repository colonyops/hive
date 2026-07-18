package main

import (
	"embed"
	"log"
	"os"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/desktop"
	"github.com/colonyops/hive/internal/desktop/auth"
	"github.com/colonyops/hive/internal/desktop/feed"
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
	// config error text after a profiles-config reload. All are wake-up
	// signals: the frontend re-reads the relevant service on receipt.
	application.RegisterEvent[string]("feed:updated")
	application.RegisterEvent[string]("auth:updated")
	application.RegisterEvent[string]("config:updated")
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

func main() {
	// The poller and watcher live for the whole process; they die with it,
	// so there are no Stop/Close calls here (and log.Fatal below would skip
	// a defer anyway).
	provider, poller, watcher := buildFeedProvider()
	if poller != nil {
		poller.Start()
	}
	if watcher != nil {
		watcher.Start()
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
