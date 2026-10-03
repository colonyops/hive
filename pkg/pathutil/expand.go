package pathutil

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandHome expands a leading ~ to the user's home directory.
// Only expands "~" or "~/..." -- not "~username/..." forms. A path whose home
// cannot be resolved is returned unchanged.
func ExpandHome(path string) string {
	expanded, err := ExpandHomeE(path)
	if err != nil {
		return path
	}
	return expanded
}

// ExpandHomeE is ExpandHome for callers that must report an unresolvable home
// directory instead of using the literal path. The error is the one
// os.UserHomeDir returned, unwrapped, so callers phrase it themselves.
func ExpandHomeE(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, path[1:]), nil
}

// XDGConfigHome returns $XDG_CONFIG_HOME, or ~/.config when it is unset.
func XDGConfigHome() string {
	return xdgHome("XDG_CONFIG_HOME", ".config")
}

// XDGDataHome returns $XDG_DATA_HOME, or ~/.local/share when it is unset.
func XDGDataHome() string {
	return xdgHome("XDG_DATA_HOME", filepath.Join(".local", "share"))
}

func xdgHome(env, fallback string) string {
	if dir := os.Getenv(env); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, fallback)
}
