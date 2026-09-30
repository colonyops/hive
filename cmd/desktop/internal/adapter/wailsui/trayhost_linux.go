//go:build linux

package wailsui

import "github.com/godbus/dbus/v5"

// statusNotifierWatcher is the well-known bus name a desktop's tray host claims.
// KDE's StatusNotifierItem spec is what GNOME (via the AppIndicator extension),
// KDE, XFCE, and Cinnamon all implement, and it is what wails' Linux tray
// backend exports onto.
const statusNotifierWatcher = "org.kde.StatusNotifierWatcher"

// trayHostAvailable reports whether the session has somewhere for a tray icon
// to appear. wails exports its StatusNotifierItem regardless of whether anyone
// is listening and logs failures without telling the application, so the only
// reliable answer is to ask the bus directly whether a host owns the name.
//
// Vanilla GNOME — Fedora Workstation, Debian's GNOME — ships no tray host
// unless the user installs the AppIndicator extension; Ubuntu enables it by
// default. Hive hides its window on close and relies on the tray to bring it
// back, so on a session with no host that would strand the app running with no
// window and no way to reach it.
//
// Every failure answers "no". Being wrong that way costs close-to-tray; being
// wrong the other way strands the user.
func trayHostAvailable() bool {
	conn, err := dbus.SessionBus()
	if err != nil || conn == nil {
		return false
	}
	// The connection is shared with wails' own tray export — never close it.
	var owned bool
	if err := conn.BusObject().Call(
		"org.freedesktop.DBus.NameHasOwner", 0, statusNotifierWatcher,
	).Store(&owned); err != nil {
		return false
	}
	return owned
}
