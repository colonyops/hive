package sourcemark

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func encodePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func TestSetNormalizesToSquarePNG(t *testing.T) {
	s := NewStore(t.TempDir())
	hash, err := s.Set(encodePNG(t, 200, 80)) // wide, non-square
	require.NoError(t, err)
	require.True(t, ValidHash(hash))

	data, ok, err := s.Get(hash)
	require.NoError(t, err)
	require.True(t, ok)

	img, format, err := image.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	assert.Equal(t, "png", format, "stored format is canonical PNG regardless of input")
	assert.Equal(t, image.Rect(0, 0, markSide, markSide), img.Bounds())
}

// A wide logo is letterboxed, not cropped: the corners are transparent margin
// while the center carries the fitted image.
func TestSetFitsWithoutCropping(t *testing.T) {
	s := NewStore(t.TempDir())
	hash, err := s.Set(encodePNG(t, 240, 60)) // 4:1 wide
	require.NoError(t, err)

	data, _, err := s.Get(hash)
	require.NoError(t, err)
	img, _, err := image.Decode(bytes.NewReader(data))
	require.NoError(t, err)

	corner, _ := color.RGBAModel.Convert(img.At(2, 2)).(color.RGBA)
	assert.Zero(t, corner.A, "the top-left corner is transparent margin, not cropped image")
	center, _ := color.RGBAModel.Convert(img.At(markSide/2, markSide/2)).(color.RGBA)
	assert.NotZero(t, center.A, "the center carries the fitted image")
}

func TestSetIsDeterministic(t *testing.T) {
	raw := encodePNG(t, 64, 64)
	a, err := NewStore(t.TempDir()).Set(raw)
	require.NoError(t, err)
	b, err := NewStore(t.TempDir()).Set(raw)
	require.NoError(t, err)
	assert.Equal(t, a, b, "the same image hashes to the same reference")
}

func TestSetAcceptsJPEG(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 50, 50)), nil))
	_, err := NewStore(t.TempDir()).Set(buf.Bytes())
	require.NoError(t, err)
}

func TestSetRejectsBadInput(t *testing.T) {
	s := NewStore(t.TempDir())
	_, err := s.Set(nil)
	require.ErrorIs(t, err, ErrEmpty)
	_, err = s.Set([]byte("not an image"))
	require.ErrorIs(t, err, ErrUnsupported)
	_, err = s.Set(make([]byte, MaxInputBytes+1))
	require.ErrorIs(t, err, ErrTooLarge)
}

func TestGetMissingReadsAbsent(t *testing.T) {
	s := NewStore(t.TempDir())

	_, ok, err := s.Get("00000000000000000000000000000000")
	require.NoError(t, err)
	assert.False(t, ok, "a well-formed hash with no file reads as absent")
}

func TestGetRejectsMalformedHash(t *testing.T) {
	s := NewStore(t.TempDir())
	for _, hash := range []string{"", "../etc/passwd", "not-hex", "ABCDEF0123456789ABCDEF0123456789", "0123"} {
		_, ok, err := s.Get(hash)
		require.NoError(t, err)
		assert.False(t, ok, "malformed hash %q reads as absent, never an error or a traversal", hash)
	}
}

func TestValidHash(t *testing.T) {
	assert.True(t, ValidHash("0123456789abcdef0123456789abcdef"))
	assert.False(t, ValidHash("0123456789ABCDEF0123456789abcdef"), "uppercase is rejected")
	assert.False(t, ValidHash("0123456789abcdef0123456789abcde"), "too short")
	assert.False(t, ValidHash("0123456789abcdef0123456789abcdeff"), "too long")
	assert.False(t, ValidHash("../../../../../../../../etc/passw"), "traversal shapes are not hex")
}
