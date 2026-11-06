package applets

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetSurface(t *testing.T) {
	edit := func(t *testing.T, body, surface string, on bool) Surfaces {
		t.Helper()
		out, err := SetSurface([]byte(body), surface, on)
		require.NoError(t, err)
		a, err := Parse("thing", out, "")
		require.NoError(t, err, string(out))
		assert.Empty(t, a.Warnings, string(out))
		return a.Surfaces
	}
	t.Run("adding to a file with no list keeps the pop-up it had", func(t *testing.T) {
		got := edit(t, minimal, SurfaceTab, true)
		assert.True(t, got.ShowsTab())
		assert.True(t, got.ShowsPopup())
	})
	t.Run("turning off the only surface leaves the applet nowhere", func(t *testing.T) {
		got := edit(t, minimal, SurfacePopup, false)
		assert.False(t, got.ShowsPopup())
		assert.False(t, got.ShowsTab())
	})
	t.Run("off keeps a surface's settings, and on brings them back", func(t *testing.T) {
		body := minimal + "\nsurfaces:\n  popup: {width: 640, height: 400}\n  titlebar:\n    entry: bar.html\n    width: 64\n"
		off, err := SetSurface([]byte(body), SurfaceTitlebar, false)
		require.NoError(t, err)
		a, err := Parse("thing", off, "")
		require.NoError(t, err)
		assert.False(t, a.Surfaces.ShowsTitlebar())
		assert.Equal(t, "bar.html", a.Surfaces.Titlebar.Entry)

		on, err := SetSurface(off, SurfaceTitlebar, true)
		require.NoError(t, err)
		a, err = Parse("thing", on, "")
		require.NoError(t, err)
		assert.True(t, a.Surfaces.ShowsTitlebar())
		assert.Equal(t, 64, a.Surfaces.Titlebar.Width)
		assert.Equal(t, 640, a.Surfaces.Popup.Width)
	})
	t.Run("turns a page on and off", func(t *testing.T) {
		on := edit(t, minimal+"\nsurfaces:\n  tab: {}\n", SurfacePage, true)
		assert.True(t, on.ShowsPage())
		assert.True(t, on.ShowsTab())
		assert.False(t, edit(t, minimal+"\nsurfaces:\n  page: {}\n", SurfacePage, false).ShowsPage())
	})
	t.Run("a bare key takes the switch", func(t *testing.T) {
		assert.False(t, edit(t, minimal+"\nsurfaces:\n  tab:\n", SurfaceTab, false).ShowsTab())
	})
	t.Run("keeps the file's comments", func(t *testing.T) {
		out, err := SetSurface([]byte("# mine\n"+minimal+"\nsurfaces:\n  # the panel\n  popup: {}\n"), SurfaceTab, true)
		require.NoError(t, err)
		assert.Contains(t, string(out), "# mine")
		assert.Contains(t, string(out), "# the panel")
	})
	for name, tc := range map[string]struct {
		body, surface, want string
	}{
		"a title bar with no page":      {minimal, SurfaceTitlebar, "no title bar page"},
		"a surface that does not exist": {minimal, "sidebar", "no surface"},
		"an alias it would also change": {minimal + "\nx: &shared {}\nsurfaces: {tab: *shared}\n", SurfaceTab, "edit it by hand"},
	} {
		t.Run("refuses "+name, func(t *testing.T) {
			_, err := SetSurface([]byte(tc.body), tc.surface, true)
			assert.ErrorContains(t, err, tc.want)
		})
	}
}
