package styles

import (
	"reflect"
	"testing"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/core/theme"
)

// The theme package parses its colors without lipgloss. The values must stay
// equal to what lipgloss parses, or the rendered output changes.
func TestThemeColorsMatchLipgloss(t *testing.T) {
	palette, ok := theme.Get("tokyo-night")
	require.True(t, ok)

	want := map[string]string{
		"Primary":    "#7aa2f7",
		"Secondary":  "#7dcfff",
		"Foreground": "#c0caf5",
		"Muted":      "#565f89",
		"Background": "#1a1b26",
		"Surface":    "#3b4261",
		"SurfaceLow": "#232433",
		"Success":    "#9ece6a",
		"Warning":    "#e0af68",
		"Error":      "#f7768e",
	}

	v := reflect.ValueOf(palette)
	require.Equal(t, len(want), v.NumField())
	for field, hex := range want {
		assert.Equalf(t, lipgloss.Color(hex), v.FieldByName(field).Interface(), "field %s", field)
	}
}
