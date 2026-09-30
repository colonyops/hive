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
	u.app.Menu.SetApplicationMenu(menu)
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
