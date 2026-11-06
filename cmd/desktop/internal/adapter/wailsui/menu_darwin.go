//go:build darwin && !server

package wailsui

import "github.com/wailsapp/wails/v3/pkg/application"

var zoomRoles = []application.Role{
	application.ResetZoom,
	application.ZoomIn,
	application.ZoomOut,
}

// buildMenu installs the default application menu without its zoom items. A
// menu key equivalent is consumed before the webview sees the keydown, so the
// frontend keymap can only bind ⌘+/⌘-/⌘0 once the menu lets go of them (ADR
// the-zoom-chords-step-the-terminal-text-size-instead-of-magnifying-the-webview).
// The items go rather than staying clickable: magnification scales the window
// without reflowing it, which is the behaviour the chords were taken away from.
func (u *UI) buildMenu() {
	menu := application.DefaultApplicationMenu()
	removeRoles(menu, zoomRoles)
	replaceCloseItem(menu, u.closeFront)
	if window := menu.FindByRole(application.WindowMenu); window != nil {
		window.GetSubmenu().AddSeparator()
		window.GetSubmenu().Add("Diagnostics…").OnClick(func(*application.Context) { u.openDiagnostics() })
	}
	u.app.Menu.SetApplicationMenu(menu)
}

// closeFront is ⌘W from the menu. AppKit runs the item whenever the webview lets the key
// through, which a page in an applet frame always does: its keydown reaches the app only
// after the page had its turn. So the main window asks the frontend, which closes the tab
// in front or the window itself. Any other window closes as before.
func (u *UI) closeFront() {
	current := u.app.Window.Current()
	if current == nil {
		return
	}
	if current.Name() != u.window.Name() {
		current.Close()
		return
	}
	emitWindowCloseRequest()
}

// emitWindowCloseRequest asks the main window's frontend to close what is in front.
func emitWindowCloseRequest() {
	if app := application.Get(); app != nil {
		app.Event.Emit("window:close-request", "close")
	}
}

// replaceCloseItem swaps the File menu's Close role for a plain item on the same chord.
// On macOS a role item runs its AppKit selector (performClose:) and never its Go callback,
// so the role cannot be given a new action.
func replaceCloseItem(menu *application.Menu, closeFront func()) {
	file := menu.FindByRole(application.FileMenu)
	if file == nil {
		return
	}
	submenu := file.GetSubmenu()
	if role := submenu.FindByRole(application.CloseWindow); role != nil {
		submenu.RemoveMenuItem(role)
	}
	submenu.Add("Close").SetAccelerator("CmdOrCtrl+w").OnClick(func(*application.Context) { closeFront() })
}

func removeRoles(menu *application.Menu, roles []application.Role) {
	for _, role := range roles {
		if item := menu.FindByRole(role); item != nil {
			menu.RemoveMenuItem(item)
		}
	}
	if view := menu.FindByRole(application.ViewMenu); view != nil {
		collapseSeparators(view.GetSubmenu())
	}
}

// The zoom items sat between two separators, and AppKit draws both.
func collapseSeparators(menu *application.Menu) {
	for i := 1; menu.ItemAt(i) != nil; {
		if menu.ItemAt(i).IsSeparator() && menu.ItemAt(i-1).IsSeparator() {
			menu.RemoveMenuItem(menu.ItemAt(i))
			continue
		}
		i++
	}
}
