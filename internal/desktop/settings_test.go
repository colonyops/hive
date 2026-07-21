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
