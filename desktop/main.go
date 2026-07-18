package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

//go:embed build/icons/tray-templateTemplate.png
var trayIcon []byte

//go:embed build/icons/tray-templateTemplate@2x.png
var trayIcon2x []byte

var _ = registerEvents()

func registerEvents() struct{} {
	application.RegisterEvent[string]("feed:updated")
	return struct{}{}
}

func templateTrayIcon() []byte {
	if len(trayIcon2x) > 0 {
		return trayIcon2x
	}
	return trayIcon
}

func main() {
	options := application.Options{
		Name:        "Hive",
		Description: "Hive desktop application",
		Icon:        appIcon,
		Services: []application.Service{
			application.NewService(NewFeedService()),
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
	window.OnWindowEvent(events.Common.WindowClosing, func(e *application.WindowEvent) {
		e.Cancel()
		window.Hide()
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

	// Wails accepts one PNG for template icons. Prefer the retina asset.
	app.SystemTray.New().SetTemplateIcon(templateTrayIcon()).SetMenu(trayMenu)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
