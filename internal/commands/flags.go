package commands

import (
	"os"
	"path/filepath"
)

type Flags struct {
	LogLevel     string
	LogFile      string
	ConfigPath   string
	DataDir      string
	ProfilerPort int
}

var configNames = []string{"config.yaml", "config.yml", "hive.yaml", "hive.yml"}

func defaultConfigDir() string {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, _ := os.UserHomeDir()
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "hive")
}

// defaultPluginsDir returns the directory where hive discovers Lua plugins.
// This must stay in lockstep with config.LuaPluginConfig.ResolvedEntry()'s
// directory portion so the scaffold and the loader look in the same place.
//
//nolint:unused // consumed by the upcoming `hive plugin init` scaffold command
func defaultPluginsDir() string {
	return filepath.Join(defaultConfigDir(), "plugins")
}

// DefaultConfigPath probes for config files with supported extensions
// (config.yaml, config.yml, hive.yaml, hive.yml) and returns the first
// match. Returns empty string when no file is found.
func DefaultConfigPath() string {
	dir := defaultConfigDir()
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
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, _ := os.UserHomeDir()
		dataHome = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataHome, "hive")
}
