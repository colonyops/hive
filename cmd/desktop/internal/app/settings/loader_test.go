package settings

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmigrate "github.com/colonyops/hive/internal/config/migrate"
)

func TestLoadSettingsMigratesUnversionedFileAndRoundTrips(t *testing.T) {
	path := isolateSettings(t)
	require.NoError(t, os.WriteFile(path, []byte("polling:\n  interval: 2m\n"), 0o600))

	current := configmigrate.SettingsSet.Current

	cfg, err := LoadSettings()
	require.NoError(t, err)
	assert.Equal(t, current, cfg.Version)
	assert.Equal(t, current, DefaultSettings().Version)

	store := NewStore(path)
	saved, err := store.Update(func(cfg *Settings) error {
		cfg.Appearance.Theme = "dark"
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, current, saved.Version)

	reloaded, err := store.Effective()
	require.NoError(t, err)
	assert.Equal(t, current, reloaded.Version)
}

func TestLoadSettingsRejectsNewerVersionAndLeavesFileUntouched(t *testing.T) {
	path := isolateSettings(t)
	contents := fmt.Sprintf("version: %d\npolling:\n  interval: 2m\n", configmigrate.SettingsSet.Current+1)
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))

	_, err := LoadSettings()
	require.Error(t, err)
	require.NotPanics(t, func() { _, _ = LoadSettings() })

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, contents, string(raw), "the load path is pure Apply and must never write")
}
