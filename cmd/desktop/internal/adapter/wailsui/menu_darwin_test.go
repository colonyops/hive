//go:build darwin && !server

package wailsui

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestCollapseSeparators(t *testing.T) {
	menu := application.NewMenu()
	menu.Add("Reload")
	menu.AddSeparator()
	menu.AddSeparator()
	menu.AddSeparator()
	menu.Add("Toggle Full Screen")

	collapseSeparators(menu)

	require.Equal(t, "Reload", menu.ItemAt(0).Label())
	require.True(t, menu.ItemAt(1).IsSeparator())
	require.Equal(t, "Toggle Full Screen", menu.ItemAt(2).Label())
	require.Nil(t, menu.ItemAt(3))
}

func TestRemoveRolesTakesTheZoomItemsOutOfTheViewMenu(t *testing.T) {
	menu := application.NewMenu()
	menu.AddRole(application.ViewMenu)

	removeRoles(menu, zoomRoles)

	for _, role := range zoomRoles {
		require.Nil(t, menu.FindByRole(role))
	}
	require.NotNil(t, menu.FindByRole(application.ToggleFullscreen))

	view := menu.FindByRole(application.ViewMenu).GetSubmenu()
	for i := 1; view.ItemAt(i) != nil; i++ {
		require.False(t, view.ItemAt(i).IsSeparator() && view.ItemAt(i-1).IsSeparator(), "adjacent separators at %d", i)
	}
}
