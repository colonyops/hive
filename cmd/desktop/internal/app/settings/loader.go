package settings

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	env "github.com/caarlos0/env/v11"
	"gopkg.in/yaml.v3"

	"github.com/colonyops/hive/cmd/desktop/internal/app/configmigrate"
	"github.com/colonyops/hive/pkg/atomicfile"
)

// Store serializes access to one settings.yaml and keeps environment overrides
// process-local. All mutations go through Update so concurrent UI/background
// writes cannot lose each other.
type Store struct {
	path string
}

var settingsFileMu sync.Mutex

func NewStore(path string) *Store { return &Store{path: path} }

func (s *Store) Effective() (Settings, error) {
	settingsFileMu.Lock()
	defer settingsFileMu.Unlock()
	return loadSettingsAt(s.path, true)
}

func (s *Store) Persisted() (Settings, error) {
	settingsFileMu.Lock()
	defer settingsFileMu.Unlock()
	return loadSettingsAt(s.path, false)
}

// Update atomically applies mutate to persisted settings, then returns the
// effective value after environment overrides are reapplied.
func (s *Store) Update(mutate func(*Settings) error) (Settings, error) {
	settingsFileMu.Lock()
	defer settingsFileMu.Unlock()

	cfg, err := loadSettingsAt(s.path, false)
	if err != nil {
		return Settings{}, err
	}
	if err := mutate(&cfg); err != nil {
		return Settings{}, err
	}
	if err := saveSettingsAt(s.path, cfg); err != nil {
		return Settings{}, err
	}
	return loadSettingsAt(s.path, true)
}

func loadSettingsAt(path string, withEnvironment bool) (Settings, error) {
	cfg := DefaultSettings()
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return Settings{}, fmt.Errorf("read desktop settings: %w", err)
	}
	data, _, err := configmigrate.SettingsSet.Apply(raw)
	if err != nil {
		return Settings{}, fmt.Errorf("migrate desktop settings: %w", err)
	}
	if len(bytes.TrimSpace(data)) > 0 {
		// No KnownFields: a file from a newer build must not keep this build from
		// reaching its updater
		// (ADR a-settings-yaml-key-the-build-does-not-declare-is-ignored-not-rejected).
		if err := yaml.NewDecoder(bytes.NewReader(data)).Decode(&cfg); err != nil {
			return Settings{}, fmt.Errorf("parse desktop settings: %w", err)
		}
		cfg.unknownKeys = checkKnownFields(data)
	}

	// Persisted settings must be valid on their own. An environment override is
	// process-local and must not hide a broken file that a later launch would
	// still be unable to use.
	if err := cfg.Validate(); err != nil {
		return Settings{}, fmt.Errorf("validate persisted desktop settings: %w", err)
	}

	if withEnvironment {
		cfg.overrides = make(map[string]bool)
		if err := env.ParseWithOptions(&cfg, env.Options{OnSet: func(tag string, _ any, isDefault bool) {
			if value, ok := os.LookupEnv(tag); !isDefault && ok && value != "" {
				cfg.overrides[tag] = true
			}
		}}); err != nil {
			return Settings{}, fmt.Errorf("parse desktop environment: %w", err)
		}
		if err := cfg.Validate(); err != nil {
			return Settings{}, fmt.Errorf("validate effective desktop settings: %w", err)
		}
	}
	return cfg, nil
}

func saveSettingsAt(path string, cfg Settings) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate desktop settings: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create desktop settings dir: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal desktop settings: %w", err)
	}

	if err := atomicfile.Write(path, data, 0o600); err != nil {
		return fmt.Errorf("replace desktop settings: %w", err)
	}
	return nil
}

// checkKnownFields runs after a lenient decode has succeeded, so every error
// the strict decode still returns is a key this build does not declare.
func checkKnownFields(data []byte) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var discard Settings
	return decoder.Decode(&discard)
}
