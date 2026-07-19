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

// EnvConfigPath overrides the profiles config file location, mirroring how
// HIVE_CONFIG overrides the CLI config file.
const EnvConfigPath = "HIVE_DESKTOP_CONFIG"

// EnvFlowsDir overrides the flows/*.yaml directory location, mirroring how
// EnvConfigPath overrides the profiles config file.
const EnvFlowsDir = "HIVE_DESKTOP_FLOWS"

// MockMode returns the requested mock mode, or "" for live backends.
func MockMode() string {
	return os.Getenv(EnvMockMode)
}

// StateDir is where the desktop app persists its app-local state
// (read-state). It follows the CLI's data-dir convention: HIVE_DATA_DIR,
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

// ConfigPath is the profiles config file — user-editable, dotfiles-managed
// YAML, deliberately separate from StateDir so config can live in a dotfiles
// repo while state stays local. It follows the CLI's config convention:
// XDG_CONFIG_HOME, then ~/.config, with a desktop/ subdirectory.
func ConfigPath() string {
	if path := os.Getenv(EnvConfigPath); path != "" {
		return path
	}
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, _ := os.UserHomeDir()
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "hive", "desktop", "profiles.yaml")
}

// FlowsDir is where the desktop pipeline's flow definitions
// (flows/<id>.yaml, plus each flow's sibling flows/<id>.ui.yaml layout)
// live: a "flows" directory next to the profiles config, so both are
// user-editable, dotfiles-managed YAML under the same root. It follows the
// same override convention as ConfigPath: EnvFlowsDir wins outright over
// the derived location.
func FlowsDir() string {
	if dir := os.Getenv(EnvFlowsDir); dir != "" {
		return dir
	}
	return filepath.Join(filepath.Dir(ConfigPath()), "flows")
}

// EnvActionsPath overrides the actions.yml file location, mirroring how
// EnvFlowsDir overrides the flows directory.
const EnvActionsPath = "HIVE_DESKTOP_ACTIONS"

// ActionsPath is the actions.yml file location: launch-session/shell/
// publish-event action definitions consumed by the desktop pipeline's
// output worker (see internal/desktop/pipeline/actions) and, in a later
// phase, the detail-pane action picker. The design doc calls this
// ".hive/actions.yml" (repo-scoped), but the desktop app's config is global
// rather than repo-scoped — there is no single repo it belongs to — so it
// lives next to the desktop profiles/flows config instead. EnvActionsPath
// overrides the derived location outright, mirroring EnvFlowsDir/
// EnvConfigPath.
func ActionsPath() string {
	if path := os.Getenv(EnvActionsPath); path != "" {
		return path
	}
	return filepath.Join(filepath.Dir(ConfigPath()), "actions.yml")
}
