// Package osopen opens files and directories in the host OS's default
// application or file manager. It is the reusable home for the "open" /
// "reveal in folder" primitives the desktop app's System settings needs.
package osopen

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Open launches path in the OS default application (a .log in the text editor,
// a directory in the file manager).
func Open(path string) error {
	if path == "" {
		return errors.New("osopen: empty path")
	}
	name, args := openCmd(runtime.GOOS, path)
	if name == "" {
		return fmt.Errorf("osopen: unsupported platform %q", runtime.GOOS)
	}
	if err := exec.Command(name, args...).Run(); err != nil {
		return fmt.Errorf("osopen: open %q: %w", path, err)
	}
	return nil
}

// Reveal shows path in the OS file manager, selecting it within its containing
// folder where the platform supports it (Finder, Explorer). On Linux, which
// has no universal reveal-and-select, it opens the containing directory.
func Reveal(path string) error {
	if path == "" {
		return errors.New("osopen: empty path")
	}
	name, args := revealCmd(runtime.GOOS, path)
	if name == "" {
		return fmt.Errorf("osopen: unsupported platform %q", runtime.GOOS)
	}
	err := exec.Command(name, args...).Run()
	// Windows Explorer exits non-zero (1) even on a successful /select, so a
	// bare ExitError there is expected and not a real failure.
	if err != nil {
		var exitErr *exec.ExitError
		if runtime.GOOS == "windows" && errors.As(err, &exitErr) {
			return nil
		}
		return fmt.Errorf("osopen: reveal %q: %w", path, err)
	}
	return nil
}

// openCmd returns the command and args to open path in the default app for the
// given GOOS, or an empty name for unsupported platforms. Split out as a pure
// function so the platform matrix is unit-testable without spawning processes.
func openCmd(goos, path string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", []string{path}
	case "windows":
		// The empty "" is start's window-title argument; without it a quoted
		// path would be consumed as the title instead of the target.
		return "cmd", []string{"/c", "start", "", path}
	case "linux", "freebsd", "openbsd", "netbsd":
		return "xdg-open", []string{path}
	default:
		return "", nil
	}
}

// revealCmd returns the command and args to reveal path in the file manager for
// the given GOOS. Pure, for the same testing reason as openCmd.
func revealCmd(goos, path string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", []string{"-R", path}
	case "windows":
		return "explorer", []string{"/select," + path}
	case "linux", "freebsd", "openbsd", "netbsd":
		return "xdg-open", []string{filepath.Dir(path)}
	default:
		return "", nil
	}
}
