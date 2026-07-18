package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

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
		Name:        "Hive Desktop",
		Description: "Hive desktop application",
		Services: []application.Service{
			application.NewService(NewFeedService()),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ActivationPolicy: application.ActivationPolicyAccessory,
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	}
	app := application.New(options)

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Hive",
		Width:            1360,
		Height:           864,
		Frameless:        true,
		Hidden:           true,
		HideOnFocusLost:  true,
		BackgroundColour: application.NewRGB(6, 7, 15),
		URL:              "/",
	})

	trayMenu := app.NewMenu()
	tray := app.SystemTray.New()
	trayMenu.Add("Show Hive").OnClick(func(*application.Context) {
		tray.ShowWindow()
	})
	trayMenu.AddSeparator()
	trayMenu.Add("Quit").OnClick(func(*application.Context) {
		app.Quit()
	})

	// Wails accepts one PNG for template icons. Prefer the retina asset.
	tray.SetTemplateIcon(templateTrayIcon()).AttachWindow(window).SetMenu(trayMenu)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
