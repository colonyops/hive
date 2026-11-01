package app

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/applets"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

func TestEverySurfaceHasAListInSettings(t *testing.T) {
	var cfg settings.AppletsSettings
	for _, surface := range applets.Surfaces {
		_, ok := cfg.Surface(surface.Name)
		assert.True(t, ok, surface.Name)
	}
}

func TestForgetAppletClearsEveryChoiceForThatIDAndNoOther(t *testing.T) {
	service := &SettingsService{store: settings.NewStore(filepath.Join(t.TempDir(), "settings.yaml"))}
	ctx := t.Context()
	require.NoError(t, service.SetAppletSurface(ctx, "titlebar", []string{"radio", "radio-2"}))
	require.NoError(t, service.SetAppletDisabled("radio", true))
	require.NoError(t, service.SetAppletDisabled("radio-2", true))
	require.NoError(t, service.SetKeybindings(ctx, map[string][]string{
		"applet.radio":      {"mod+r"},
		"applet.radio.next": {"mod+n"},
		"applet.radio-2":    {"mod+2"},
		"view.go-inbox":     {"mod+i"},
	}))

	require.NoError(t, service.ForgetApplet("radio"))

	placed, err := service.AppletSurface(ctx, "titlebar")
	require.NoError(t, err)
	assert.Equal(t, []string{"radio-2"}, placed)
	disabled, err := service.AppletDisabled("radio")
	require.NoError(t, err)
	assert.False(t, disabled)
	disabled, err = service.AppletDisabled("radio-2")
	require.NoError(t, err)
	assert.True(t, disabled)
	bindings, err := service.Keybindings(ctx)
	require.NoError(t, err)
	assert.Equal(t, map[string][]string{"applet.radio-2": {"mod+2"}, "view.go-inbox": {"mod+i"}}, bindings)
}

func TestAppletSurfaceRefusesANameItDoesNotKnow(t *testing.T) {
	service := &SettingsService{store: settings.NewStore(filepath.Join(t.TempDir(), "settings.yaml"))}
	_, err := service.AppletSurface(t.Context(), "nowhere")
	require.Error(t, err)
	require.Error(t, service.SetAppletSurface(t.Context(), "nowhere", nil))
}
