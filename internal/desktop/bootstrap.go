package desktop

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// bootstrapFileName is the fixed pointer file the desktop app reads before any
// path resolution. It records user-chosen data/config directory overrides so a
// GUI launched from the dock (which never sees shell env vars) can still be
// pointed at, e.g., an iCloud-synced folder.
const bootstrapFileName = "bootstrap.yaml"

// Bootstrap holds the persisted directory overrides set from the System
// settings screen. Empty fields mean "use the default resolution".
//
// DataDir is the data-dir root (the HIVE_DATA_DIR equivalent) under which
// StateDir and the core hive.db live. ConfigDir is the desktop config root
// (the directory holding profiles.yaml, flows/, and actions.yml).
type Bootstrap struct {
	DataDir   string `yaml:"data_dir,omitempty"`
	ConfigDir string `yaml:"config_dir,omitempty"`
}

// BootstrapPath is the fixed location of the bootstrap pointer file. It is
// deliberately anchored to the default XDG config location and is NEVER
// affected by a config-dir override — otherwise relocating the config dir
// would move the very file that records where the config dir went. It mirrors
// ConfigPath's default-location logic (XDG_CONFIG_HOME, then ~/.config) but
// ignores EnvConfigPath.
func BootstrapPath() string {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, _ := os.UserHomeDir()
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "hive", "desktop", bootstrapFileName)
}

// LoadBootstrap reads the pointer file. A missing file is not an error: it
// returns a zero Bootstrap so first-run and default setups behave identically.
func LoadBootstrap() (Bootstrap, error) {
	data, err := os.ReadFile(BootstrapPath())
	if err != nil {
		if os.IsNotExist(err) {
			return Bootstrap{}, nil
		}
		return Bootstrap{}, fmt.Errorf("read desktop bootstrap: %w", err)
	}
	var b Bootstrap
	if err := yaml.Unmarshal(data, &b); err != nil {
		return Bootstrap{}, fmt.Errorf("parse desktop bootstrap: %w", err)
	}
	return b, nil
}

// SaveBootstrap writes the pointer file, creating its parent directory. An
// entirely empty Bootstrap is still written (an empty file), which reads back
// as "no overrides".
func SaveBootstrap(b Bootstrap) error {
	path := BootstrapPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create desktop bootstrap dir: %w", err)
	}
	data, err := yaml.Marshal(b)
	if err != nil {
		return fmt.Errorf("marshal desktop bootstrap: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write desktop bootstrap: %w", err)
	}
	return nil
}

// ApplyBootstrap seeds HIVE_DATA_DIR and HIVE_DESKTOP_CONFIG from the pointer
// file so the existing StateDir/ConfigPath resolvers honor the overrides with
// no further changes. It must run before any path is resolved (first thing in
// main). An explicit env var always wins: a value already set (dev, e2e, an
// operator's shell) is never overwritten, preserving env precedence.
//
// A read error is returned but is non-fatal for callers — the app can still
// start on defaults; callers should log and continue.
func ApplyBootstrap() error {
	b, err := LoadBootstrap()
	if err != nil {
		return err
	}
	if b.DataDir != "" {
		if _, ok := os.LookupEnv("HIVE_DATA_DIR"); !ok {
			_ = os.Setenv("HIVE_DATA_DIR", b.DataDir)
		}
	}
	if b.ConfigDir != "" {
		if _, ok := os.LookupEnv(EnvConfigPath); !ok {
			// EnvConfigPath points at the profiles.yaml file; its directory is
			// the config root that FlowsDir/ActionsPath derive from.
			_ = os.Setenv(EnvConfigPath, filepath.Join(b.ConfigDir, "profiles.yaml"))
		}
	}
	return nil
}

// DataDir is the effective data-dir root for this process: the parent of
// StateDir. Everything the desktop persists (its state dir, the core hive.db)
// lives beneath it.
func DataDir() string {
	return filepath.Dir(StateDir())
}

// ConfigDir is the effective desktop config root for this process: the
// directory holding profiles.yaml, flows/, and actions.yml.
func ConfigDir() string {
	return filepath.Dir(ConfigPath())
}
