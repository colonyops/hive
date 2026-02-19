package commands

import (
	"fmt"
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

// DefaultConfigPath returns the default config file path using XDG_CONFIG_HOME.
// Returns the .yaml path for backwards compatibility with flags/defaults.
// Use FindConfigPath to check for both .yaml and .yml extensions.
func DefaultConfigPath() string {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, _ := os.UserHomeDir()
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "hive", "config.yaml")
}

// FindConfigPath searches for a config file with either .yaml or .yml extension.
// Returns the first existing file, or the .yaml path if neither exists.
// The second return value indicates whether a config file was found.
func FindConfigPath() (path string, found bool) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, _ := os.UserHomeDir()
		configHome = filepath.Join(home, ".config")
	}
	configDir := filepath.Join(configHome, "hive")

	// Check for .yaml first (preferred)
	yamlPath := filepath.Join(configDir, "config.yaml")
	if _, err := os.Stat(yamlPath); err == nil {
		return yamlPath, true
	}

	// Check for .yml
	ymlPath := filepath.Join(configDir, "config.yml")
	if _, err := os.Stat(ymlPath); err == nil {
		return ymlPath, true
	}

	// No config found, return default path
	return yamlPath, false
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
