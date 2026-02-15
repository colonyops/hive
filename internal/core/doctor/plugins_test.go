package doctor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginCheck_AllAvailable(t *testing.T) {
	plugins := []PluginInfo{
		{Name: "github", Available: true},
		{Name: "beads", Available: true},
	}

	check := NewPluginCheck(plugins)
	result := check.Run(context.Background())

	assert.Equal(t, "Plugins", result.Name)
	require.Len(t, result.Items, 2)
	assert.Equal(t, StatusPass, result.Items[0].Status)
	assert.Equal(t, "github", result.Items[0].Label)
	assert.Equal(t, StatusPass, result.Items[1].Status)
	assert.Equal(t, "beads", result.Items[1].Label)
}

func TestPluginCheck_SomeUnavailable(t *testing.T) {
	plugins := []PluginInfo{
		{Name: "github", Available: true},
		{Name: "beads", Available: false},
	}

	check := NewPluginCheck(plugins)
	result := check.Run(context.Background())

	require.Len(t, result.Items, 2)
	assert.Equal(t, StatusPass, result.Items[0].Status)
	assert.Equal(t, StatusWarn, result.Items[1].Status)
	assert.Contains(t, result.Items[1].Detail, "not available")
}

func TestPluginCheck_DisabledPlugin(t *testing.T) {
	plugins := []PluginInfo{
		{Name: "github", Available: true},
		{Name: "beads", Available: false, Disabled: true},
	}

	check := NewPluginCheck(plugins)
	result := check.Run(context.Background())

	require.Len(t, result.Items, 2)
	assert.Equal(t, StatusPass, result.Items[0].Status)
	assert.Equal(t, StatusPass, result.Items[1].Status)
	assert.Equal(t, "disabled", result.Items[1].Detail)
}

func TestPluginCheck_NoPlugins(t *testing.T) {
	check := NewPluginCheck(nil)
	result := check.Run(context.Background())

	require.Len(t, result.Items, 1)
	assert.Equal(t, StatusPass, result.Items[0].Status)
	assert.Equal(t, "No plugins", result.Items[0].Label)
}
