package theme

import (
	"image/color"
	"reflect"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGet(t *testing.T) {
	_, ok := Get(Default)
	assert.True(t, ok, "the default theme must exist")

	_, ok = Get("no-such-theme")
	assert.False(t, ok)
}

func TestNames(t *testing.T) {
	names := Names()
	require.Len(t, names, len(themes))
	assert.True(t, sort.StringsAreSorted(names))
	assert.Contains(t, names, Default)
}

func TestPalettesSetEveryColor(t *testing.T) {
	for name, palette := range themes {
		v := reflect.ValueOf(palette)
		for i := range v.NumField() {
			assert.Falsef(t, v.Field(i).IsNil(), "theme %q has no %s", name, v.Type().Field(i).Name)
		}
	}
}

func TestHex(t *testing.T) {
	assert.Equal(t, color.RGBA{R: 0x7a, G: 0xa2, B: 0xf7, A: 0xff}, hex("#7aa2f7"))
	assert.Equal(t, hex("#7e9cd8"), hex("#7E9CD8"))

	for _, malformed := range []string{"", "7aa2f7", "#7aa2f", "#7aa2f7f", "#gggggg"} {
		assert.Panicsf(t, func() { hex(malformed) }, "hex(%q)", malformed)
	}
}
