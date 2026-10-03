package config

import (
	"os"
	"path/filepath"

	"github.com/colonyops/hive/pkg/pathutil"
)

var configNames = []string{"config.yaml", "config.yml", "hive.yaml", "hive.yml"}

// DefaultConfigDir returns $XDG_CONFIG_HOME/hive (falling back to
// ~/.config/hive).
func DefaultConfigDir() string {
	return filepath.Join(pathutil.XDGConfigHome(), "hive")
}

// DefaultConfigPath probes for config files with supported extensions
// (config.yaml, config.yml, hive.yaml, hive.yml) and returns the first
// match. Returns empty string when no file is found.
func DefaultConfigPath() string {
	dir := DefaultConfigDir()
	for _, name := range configNames {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// DefaultDataDir returns the default data directory using XDG_DATA_HOME.
func DefaultDataDir() string {
	return filepath.Join(pathutil.XDGDataHome(), "hive")
}
