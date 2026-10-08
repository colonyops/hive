package wailsui

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

func (u *UI) openDiagnostics() {
	u.diagnosticsMu.Lock()
	defer u.diagnosticsMu.Unlock()
	if u.diagnosticsWindow == nil {
		window := u.app.Window.NewWithOptions(application.WebviewWindowOptions{
			Title: "Hive Diagnostics", Width: 1100, Height: 800, MinWidth: 700, MinHeight: 480,
			URL: "/?diagnostics=1", BackgroundColour: application.NewRGB(16, 19, 24),
		})
		window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) { window.Hide(); e.Cancel() })
		u.diagnosticsWindow = window
	}
	u.diagnosticsWindow.Show()
	u.diagnosticsWindow.Focus()
}
