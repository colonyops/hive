package profileimg

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
	hash, err := s.Set("triage", encodePNG(t, 200, 80)) // wide, non-square
	require.NoError(t, err)
	require.NotEmpty(t, hash)

	data, ok, err := s.Get("triage")
	require.NoError(t, err)
	require.True(t, ok)

	img, format, err := image.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	assert.Equal(t, "png", format, "stored format is canonical PNG regardless of input")
	assert.Equal(t, image.Rect(0, 0, storedSide, storedSide), img.Bounds())
}

func TestSetIsDeterministic(t *testing.T) {
	raw := encodePNG(t, 64, 64)
	a, err := NewStore(t.TempDir()).Set("p", raw)
	require.NoError(t, err)
	b, err := NewStore(t.TempDir()).Set("p", raw)
	require.NoError(t, err)
	assert.Equal(t, a, b, "the same image hashes to the same reference")
}

func TestSetAcceptsJPEG(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 50, 50)), nil))
	_, err := NewStore(t.TempDir()).Set("p", buf.Bytes())
	require.NoError(t, err)
}

func TestSetRejectsBadInput(t *testing.T) {
	s := NewStore(t.TempDir())
	_, err := s.Set("p", nil)
	require.ErrorIs(t, err, ErrEmpty)
	_, err = s.Set("p", []byte("not an image"))
	require.ErrorIs(t, err, ErrUnsupported)
	_, err = s.Set("p", make([]byte, MaxInputBytes+1))
	require.ErrorIs(t, err, ErrTooLarge)
}

func TestGetMissingAndDeleteIdempotent(t *testing.T) {
	s := NewStore(t.TempDir())

	_, ok, err := s.Get("nope")
	require.NoError(t, err)
	assert.False(t, ok)
	require.NoError(t, s.Delete("nope"), "deleting a missing image is not an error")

	_, err = s.Set("p", encodePNG(t, 32, 32))
	require.NoError(t, err)
	require.NoError(t, s.Delete("p"))
	_, ok, err = s.Get("p")
	require.NoError(t, err)
	assert.False(t, ok)
}
