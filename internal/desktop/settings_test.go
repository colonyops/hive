package desktop

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadSettings(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		want     time.Duration
		wantErr  bool
	}{
		{name: "missing file", want: time.Minute},
		{name: "valid interval", contents: "poll_interval: 2m\n", want: 2 * time.Minute},
		{name: "below floor is clamped", contents: "poll_interval: 10s\n", want: MinPollInterval},
		{name: "invalid duration", contents: "poll_interval: fast\n", wantErr: true},
		{name: "malformed yaml", contents: "poll_interval: [\n", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv(EnvConfigPath, filepath.Join(root, "config", "profiles.yaml"))
			if tt.contents != "" {
				require.NoError(t, os.MkdirAll(filepath.Dir(SettingsPath()), 0o755))
				require.NoError(t, os.WriteFile(SettingsPath(), []byte(tt.contents), 0o600))
			}

			settings, err := LoadSettings()
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			interval, err := settings.PollIntervalOrDefault(time.Minute)
			require.NoError(t, err)
			require.Equal(t, tt.want, interval)
		})
	}
}

func TestSaveSettingsRoundTrip(t *testing.T) {
	root := t.TempDir()
	t.Setenv(EnvConfigPath, filepath.Join(root, "nested", "config", "profiles.yaml"))
	want := Settings{PollInterval: "2m"}

	require.NoError(t, SaveSettings(want))
	require.FileExists(t, SettingsPath())
	got, err := LoadSettings()
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestAutoUpdateOrDefault(t *testing.T) {
	enabled := true
	disabled := false
	tests := []struct {
		name     string
		settings Settings
		want     bool
	}{
		{name: "unset defaults to true", settings: Settings{}, want: true},
		{name: "explicit true", settings: Settings{AutoUpdate: &enabled}, want: true},
		{name: "explicit false", settings: Settings{AutoUpdate: &disabled}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.settings.AutoUpdateOrDefault())
		})
	}
}

func TestNotificationSettingsOrDefault(t *testing.T) {
	enabled := true
	disabled := false
	tests := []struct {
		name     string
		settings Settings
		want     bool
		resolve  func(Settings) bool
	}{
		{name: "notifications unset defaults to true", settings: Settings{}, want: true, resolve: Settings.NotificationsEnabledOrDefault},
		{name: "notifications explicit false", settings: Settings{NotificationsEnabled: &disabled}, want: false, resolve: Settings.NotificationsEnabledOrDefault},
		{name: "system notifications unset defaults to true", settings: Settings{}, want: true, resolve: Settings.SystemNotificationsEnabledOrDefault},
		{name: "system notifications explicit false", settings: Settings{SystemNotificationsEnabled: &disabled}, want: false, resolve: Settings.SystemNotificationsEnabledOrDefault},
		{name: "notification sound explicit true", settings: Settings{NotificationSound: &enabled}, want: true, resolve: Settings.NotificationSoundOrDefault},
		{name: "notification sound explicit false", settings: Settings{NotificationSound: &disabled}, want: false, resolve: Settings.NotificationSoundOrDefault},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.resolve(tt.settings))
		})
	}
}

func TestSettingsAutoUpdateRoundTrip(t *testing.T) {
	root := t.TempDir()
	t.Setenv(EnvConfigPath, filepath.Join(root, "config", "profiles.yaml"))

	// Unset auto_update in the file resolves to the default (on).
	require.NoError(t, os.MkdirAll(filepath.Dir(SettingsPath()), 0o755))
	require.NoError(t, os.WriteFile(SettingsPath(), []byte("poll_interval: 2m\n"), 0o600))
	got, err := LoadSettings()
	require.NoError(t, err)
	require.Nil(t, got.AutoUpdate)
	require.True(t, got.AutoUpdateOrDefault())

	// An explicit false persists and round-trips as false.
	disabled := false
	require.NoError(t, SaveSettings(Settings{PollInterval: "2m", AutoUpdate: &disabled}))
	got, err = LoadSettings()
	require.NoError(t, err)
	require.NotNil(t, got.AutoUpdate)
	require.False(t, got.AutoUpdateOrDefault())
	require.Equal(t, "2m", got.PollInterval)
}
