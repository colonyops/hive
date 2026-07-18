package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

var _ = registerEvents()

func registerEvents() struct{} {
	application.RegisterEvent[string]("feed:updated")
	return struct{}{}
}

func main() {
	options := application.Options{
		Name:        "hive-desktop",
		Description: "Hive desktop application",
		Services: []application.Service{
			application.NewService(NewFeedService()),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	}
	app := application.New(options)

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Hive",
		Width:            1000,
		Height:           618,
		BackgroundColour: application.NewRGB(6, 7, 15),
		URL:              "/",
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
