// Package desktop holds code that exists purely for the Hive desktop app.
// Subpackages implement the desktop's service backends (auth, feed); the
// desktop/ main package is thin Wails wiring over them. Anything reusable
// beyond the desktop (the GitHub client, session/core logic) does not
// belong here.
package desktop

import (
	"os"
	"path/filepath"
)

// EnvMockMode selects deterministic offline backends instead of live
// GitHub: "feed" starts authenticated with fixture data (the e2e default),
// "onboarding" starts signed out with a self-granting fake device flow.
const EnvMockMode = "HIVE_DESKTOP_MOCK"

// MockMode returns the requested mock mode, or "" for live backends.
func MockMode() string {
	return os.Getenv(EnvMockMode)
}

// StateDir is where the desktop app persists its state (profiles,
// read-state). It follows the CLI's data-dir convention: HIVE_DATA_DIR,
// then XDG_DATA_HOME, then ~/.local/share — with a desktop/ subdirectory
// keeping app state apart from CLI state.
func StateDir() string {
	if dir := os.Getenv("HIVE_DATA_DIR"); dir != "" {
		return filepath.Join(dir, "desktop")
	}
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, _ := os.UserHomeDir()
		dataHome = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataHome, "hive", "desktop")
}
