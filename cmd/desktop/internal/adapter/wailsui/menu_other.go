//go:build !darwin || server

package wailsui

// Only macOS gets a default application menu from Wails, so only there do the
// zoom chords have an accelerator to give back. Installing one elsewhere would
// add a menu bar to the Linux window and register its accelerators with GTK.
func (u *UI) buildMenu() {}
