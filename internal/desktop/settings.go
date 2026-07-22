package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// settingsFileName is the desktop settings file, resolved under the desktop
// config root next to profiles.yaml, flows/, and actions.yml.
const settingsFileName = "settings.yaml"

// MinPollInterval is the floor for the configured poll interval. It matches
// GitHub's notifications polling contract.
const MinPollInterval = 60 * time.Second

// Settings holds user-tunable desktop behavior. Zero-valued fields mean use
// the application's default.
type Settings struct {
	PollInterval string `yaml:"poll_interval,omitempty"`
	// AutoUpdate toggles the desktop app's self-update checks. It is a pointer
	// so an absent key (nil) is distinguishable from an explicit `false`: unset
	// means "use the default" (on), matching the omitempty convention used for
	// PollInterval. Resolve it through AutoUpdateOrDefault rather than reading
	// the pointer directly.
	AutoUpdate *bool `yaml:"auto_update,omitempty"`
	// NotificationsEnabled controls Hive notifications. An absent key defaults
	// to enabled so upgrades preserve the prior behavior.
	NotificationsEnabled *bool `yaml:"notifications_enabled,omitempty"`
	// SystemNotificationsEnabled controls OS banners while Hive is unfocused.
	// It defaults to enabled when absent.
	SystemNotificationsEnabled *bool `yaml:"system_notifications_enabled,omitempty"`
	// NotificationSound controls sound for OS notification banners. It defaults
	// to enabled when absent.
	NotificationSound *bool `yaml:"notification_sound,omitempty"`
}

// SettingsPath is the settings.yaml location under the desktop config root.
func SettingsPath() string {
	return filepath.Join(ConfigDir(), settingsFileName)
}

// LoadSettings reads settings.yaml. Missing settings are equivalent to all
// defaults; malformed YAML and invalid duration values are reported so callers
// can warn before falling back to defaults.
func LoadSettings() (Settings, error) {
	data, err := os.ReadFile(SettingsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return Settings{}, nil
		}
		return Settings{}, fmt.Errorf("read desktop settings: %w", err)
	}

	var settings Settings
	if err := yaml.Unmarshal(data, &settings); err != nil {
		return Settings{}, fmt.Errorf("parse desktop settings: %w", err)
	}
	if _, err := settings.PollIntervalOrDefault(MinPollInterval); err != nil {
		return Settings{}, fmt.Errorf("parse desktop settings poll interval: %w", err)
	}
	return settings, nil
}

// SaveSettings writes settings.yaml, creating its parent directory.
func SaveSettings(settings Settings) error {
	path := SettingsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create desktop settings dir: %w", err)
	}
	data, err := yaml.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal desktop settings: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write desktop settings: %w", err)
	}
	return nil
}

// AutoUpdateOrDefault resolves AutoUpdate, defaulting to true (auto-update on)
// when the key is absent from settings.yaml.
func (s Settings) AutoUpdateOrDefault() bool {
	if s.AutoUpdate == nil {
		return true
	}
	return *s.AutoUpdate
}

// NotificationsEnabledOrDefault resolves NotificationsEnabled, defaulting to
// true when the key is absent from settings.yaml.
func (s Settings) NotificationsEnabledOrDefault() bool {
	if s.NotificationsEnabled == nil {
		return true
	}
	return *s.NotificationsEnabled
}

// SystemNotificationsEnabledOrDefault resolves SystemNotificationsEnabled,
// defaulting to true when the key is absent from settings.yaml.
func (s Settings) SystemNotificationsEnabledOrDefault() bool {
	if s.SystemNotificationsEnabled == nil {
		return true
	}
	return *s.SystemNotificationsEnabled
}

// NotificationSoundOrDefault resolves NotificationSound, defaulting to true
// when the key is absent from settings.yaml.
func (s Settings) NotificationSoundOrDefault() bool {
	if s.NotificationSound == nil {
		return true
	}
	return *s.NotificationSound
}

// PollIntervalOrDefault resolves PollInterval. Hand-edited values below the
// floor are tolerated and clamped; callers that accept user input should
// reject them before saving.
func (s Settings) PollIntervalOrDefault(fallback time.Duration) (time.Duration, error) {
	if s.PollInterval == "" {
		return fallback, nil
	}
	interval, err := time.ParseDuration(s.PollInterval)
	if err != nil {
		return 0, err
	}
	if interval < MinPollInterval {
		return MinPollInterval, nil
	}
	return interval, nil
}
