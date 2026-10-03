package config

import (
	"os"
	"path/filepath"

	"github.com/colonyops/hive/pkg/pathutil"
)

const (
	// EnvConfig names the config file, overriding the probe of the config
	// directory.
	EnvConfig  = "HIVE_CONFIG"
	EnvDataDir = "HIVE_DATA_DIR"
	// EnvDesktopDataDir names the hive data directory for Hive Desktop only. It
	// wins over EnvDataDir so a development desktop can point at a hive.db
	// other than the one the shell's hive CLI uses.
	EnvDesktopDataDir = "HIVE_DESKTOP_HIVE_DATA_DIR"
)

var configNames = []string{"config.yaml", "config.yml", "hive.yaml", "hive.yml"}

// DefaultConfigDir returns $XDG_CONFIG_HOME/hive (falling back to
// ~/.config/hive).
func DefaultConfigDir() string {
	return filepath.Join(pathutil.XDGConfigHome(), "hive")
}

// ConfigPathIn returns the first config file present in dir, probing
// config.yaml, config.yml, hive.yaml and hive.yml in that order. When none is
// present it returns dir/config.yaml, the file a writer creates.
func ConfigPathIn(dir string) string {
	for _, name := range configNames {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return filepath.Join(dir, configNames[0])
}

// DefaultConfigPath returns the config file present in DefaultConfigDir, or
// "" when there is none.
func DefaultConfigPath() string {
	path := ConfigPathIn(DefaultConfigDir())
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

// DefaultDataDir returns the default data directory using XDG_DATA_HOME.
func DefaultDataDir() string {
	return filepath.Join(pathutil.XDGDataHome(), "hive")
}

// ResolveDataDir returns the hive data directory the environment names:
// EnvDesktopDataDir, then EnvDataDir, then fallback. getenv is a parameter
// because a desktop launched from the Dock reads these from the login shell,
// not from its own environment.
func ResolveDataDir(getenv func(string) string, fallback string) string {
	for _, name := range []string{EnvDesktopDataDir, EnvDataDir} {
		if dir := getenv(name); dir != "" {
			return dir
		}
	}
	return fallback
}
