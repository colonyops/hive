//go:build !linux

package wailsui

// trayHostAvailable reports whether the session has somewhere for a tray icon
// to appear. macOS and Windows both have a menu bar / notification area that is
// always present, so the answer is unconditionally yes and the close-to-tray
// behaviour never needs a fallback. Only Linux can lack a tray host — see
// trayhost_linux.go.
func trayHostAvailable() bool { return true }
