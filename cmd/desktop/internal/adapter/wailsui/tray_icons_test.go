package wailsui

import (
	"bytes"
	"image"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decodeNRGBA(t *testing.T, data []byte) *image.NRGBA {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	nrgba, ok := img.(*image.NRGBA)
	require.True(t, ok)
	return nrgba
}

func TestMenuMarksShareOneSizeSoRowsAlign(t *testing.T) {
	unread := decodeNRGBA(t, trayUnreadMark)
	blank := decodeNRGBA(t, trayBlankMark)

	assert.Equal(t, unread.Bounds(), blank.Bounds())
	center := unread.Bounds().Dx() / 2
	assert.Equal(t, trayUnreadBlue, unread.NRGBAAt(center, center))
	assert.Zero(t, blank.NRGBAAt(center, center).A)
}

func TestWithPixelDensityWritesAValidChunk(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	data := encodePNG(img, 2)

	assert.Equal(t, "pHYs", string(data[37:41]))
	_, err := png.Decode(bytes.NewReader(data))
	require.NoError(t, err, "a bad chunk CRC fails the decode")
}
