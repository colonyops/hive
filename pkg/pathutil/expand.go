package pathutil

import (
	"os"
	"path/filepath"
)

// ExpandHome expands a leading ~ to the user's home directory.
// Only expands "~" or "~/..." — not "~username/..." forms.
func ExpandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return path
	}
	if len(path) > 1 && path[0] == '~' && path[1] == '/' {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
