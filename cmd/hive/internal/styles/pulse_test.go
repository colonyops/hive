package styles

import (
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPulseColor_Frame0_ReturnsBaseColor(t *testing.T) {
	base := color.RGBA{R: 200, G: 100, B: 50, A: 255}
	got := PulseColor(base, 0, 20, 0.5)

	r, g, b, _ := got.RGBA()
	assert.EqualValues(t, 200, r>>8, "red channel should match base at frame 0")
	assert.EqualValues(t, 100, g>>8, "green channel should match base at frame 0")
	assert.EqualValues(t, 50, b>>8, "blue channel should match base at frame 0")
}

func TestPulseColor_Midpoint_ReturnsDimmedColor(t *testing.T) {
	base := color.RGBA{R: 200, G: 100, B: 50, A: 255}
	frames := 20
	mid := frames / 2
	got := PulseColor(base, mid, frames, 0.5)

	r, g, b, _ := got.RGBA()
	assert.EqualValues(t, 100, r>>8, "red channel should be halved at midpoint with minBrightness=0.5")
	assert.EqualValues(t, 50, g>>8, "green channel should be halved at midpoint")
	assert.EqualValues(t, 25, b>>8, "blue channel should be halved at midpoint")
}

func TestPulseColor_Symmetric(t *testing.T) {
	base := color.RGBA{R: 200, G: 100, B: 50, A: 255}
	frames := 20

	// Frames equidistant from the midpoint should produce the same color.
	for i := 1; i < frames/2; i++ {
		before := PulseColor(base, i, frames, 0.5)
		after := PulseColor(base, frames-i, frames, 0.5)

		rb, gb, bb, _ := before.RGBA()
		ra, ga, ba, _ := after.RGBA()
		assert.Equal(t, rb, ra, "red channel should match for symmetric frames %d and %d", i, frames-i)
		assert.Equal(t, gb, ga, "green channel should match for symmetric frames %d and %d", i, frames-i)
		assert.Equal(t, bb, ba, "blue channel should match for symmetric frames %d and %d", i, frames-i)
	}
}

func TestPulseColor_FrameWrapping(t *testing.T) {
	base := color.RGBA{R: 200, G: 100, B: 50, A: 255}
	frames := 20

	// frame > frames should wrap via modulo
	wrapped := PulseColor(base, frames+3, frames, 0.5)
	direct := PulseColor(base, 3, frames, 0.5)

	rw, gw, bw, _ := wrapped.RGBA()
	rd, gd, bd, _ := direct.RGBA()
	require.Equal(t, rw, rd, "wrapped frame should equal direct frame (red)")
	require.Equal(t, gw, gd, "wrapped frame should equal direct frame (green)")
	require.Equal(t, bw, bd, "wrapped frame should equal direct frame (blue)")
}
