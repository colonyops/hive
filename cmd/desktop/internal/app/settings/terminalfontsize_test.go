package settings_test

import (
	"testing"

	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
	"github.com/stretchr/testify/require"
)

func TestTerminalFontSizePx(t *testing.T) {
	cases := []struct {
		name string
		px   int
		want int
	}{
		{"unset", 0, settings.DefaultTerminalFontSizePx},
		{"in range", 15, 15},
		{"upper bound", 64, 64},
		{"above range", 200, settings.MaxTerminalFontSizePx},
		{"below range", 1, settings.MinTerminalFontSizePx},
		{"negative", -4, settings.MinTerminalFontSizePx},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, settings.TerminalFontSizePx(tc.px))
		})
	}
}
