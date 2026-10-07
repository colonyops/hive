package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestAnalyticsOmissionAndFalse(t *testing.T) {
	for _, tt := range []struct {
		yaml string
		want bool
	}{
		{"", true}, {"analytics: {}", true}, {"analytics: {enabled: false}", false}, {"analytics: {local: {enabled: false}}", false}, {"analytics: {enabled: true, local: {enabled: true}}", true},
	} {
		t.Run(tt.yaml, func(t *testing.T) {
			var cfg Config
			require.NoError(t, yaml.Unmarshal([]byte(tt.yaml), &cfg))
			require.Equal(t, tt.want, cfg.Analytics.CollectionEnabled())
		})
	}
	for _, value := range []string{"false", "FALSE", "0"} {
		require.True(t, AnalyticsEnvironmentDisabled(value))
	}
	for _, value := range []string{"", "true", "1", "not-a-boolean"} {
		require.False(t, AnalyticsEnvironmentDisabled(value))
	}
}

func TestAnalyticsEditPreservesConfigAndSymlink(t *testing.T) {
	t.Setenv(EnvDefaultAgent, "")
	dir := t.TempDir()
	target := filepath.Join(dir, "target.yaml")
	path := filepath.Join(dir, "config.yaml")
	original := "# keep header\nworkspaces: [/not/a/real/workspace]\nunknown: {secret: preserved} # keep unknown\nagents: {default: claude, claude: {}}\nanalytics:\n  enabled: true # keep gate\n  local:\n    enabled: false\n    future: preserved\n"
	require.NoError(t, os.WriteFile(target, []byte(original), 0o600))
	require.NoError(t, os.Symlink(target, path))
	check := func(candidate string) error { _, err := Load(candidate, dir); return err }
	require.NoError(t, SetAnalyticsEnabled(path, false, check))
	require.NoError(t, SetAnalyticsEnabled(path, true, check))
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	for _, text := range []string{"# keep header", "# keep unknown", "# keep gate", "future: preserved", "/not/a/real/workspace", "secret: preserved"} {
		require.Contains(t, string(data), text)
	}
	cfg, err := Load(path, dir)
	require.NoError(t, err)
	require.True(t, cfg.Analytics.CollectionEnabled())
	_, err = os.Readlink(path)
	require.NoError(t, err)
	before := string(data)
	require.Error(t, SetAnalyticsEnabled(path, false, func(string) error { return os.ErrPermission }))
	data, err = os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, before, string(data))
}

func TestAnalyticsEditMissingAndInvalidFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.yaml")
	require.NoError(t, SetAnalyticsEnabled(path, false, nil))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(data), "enabled: false")
	for _, invalid := range []string{"[not, a, mapping]", "analytics: false", "analytics: {local: false}"} {
		require.NoError(t, os.WriteFile(path, []byte(invalid), 0o600))
		require.Error(t, SetAnalyticsEnabled(path, true, nil))
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, invalid, string(data))
	}
}
