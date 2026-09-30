package wailsui

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"math"
	"runtime"
)

// trayUnreadBlue is macOS's system blue, the colour Mail uses for its unread
// dot.
var trayUnreadBlue = color.NRGBA{R: 0x0A, G: 0x84, B: 0xFF, A: 0xFF}

// The menu row marks are 16pt, the size macOS gives a menu item image. A read
// item gets a transparent mark of the same size, not none: macOS only indents
// a row's title past the image column when the row has an image, so a mix of
// marked and unmarked rows would not line up.
var (
	trayUnreadMark = menuMark(true)
	trayBlankMark  = menuMark(false)
)

func menuMark(unread bool) []byte {
	scale := 1
	if runtime.GOOS == "darwin" {
		scale = 2
	}
	size := 16 * scale
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	if unread {
		fillCircle(img, float64(size)/2, float64(size)/2, 3.5*float64(scale), trayUnreadBlue)
	}
	return encodePNG(img, scale)
}

// coverage is how much of the pixel at (x, y) lies inside the circle, for an
// antialiased edge.
func coverage(x, y int, cx, cy, r float64) float64 {
	d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
	return math.Max(0, math.Min(1, r+0.5-d))
}

func fillCircle(img *image.NRGBA, cx, cy, r float64, c color.NRGBA) {
	for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
		for x := img.Rect.Min.X; x < img.Rect.Max.X; x++ {
			a := coverage(x, y, cx, cy, r)
			if a == 0 {
				continue
			}
			out := c
			out.A = uint8(math.Round(float64(c.A) * a))
			img.SetNRGBA(x, y, out)
		}
	}
}

// encodePNG writes img, declaring 72×scale DPI when scale > 1. macOS sizes
// an image built from PNG data by that DPI, so a 2x bitmap renders at its
// point size and stays sharp on a Retina display.
func encodePNG(img image.Image, scale int) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	if scale == 1 {
		return buf.Bytes()
	}
	return withPixelDensity(buf.Bytes(), uint32(math.Round(72*float64(scale)/0.0254)))
}

// withPixelDensity inserts a pHYs chunk after IHDR, which image/png cannot
// write. The signature is 8 bytes and IHDR is always 25.
func withPixelDensity(data []byte, pixelsPerMeter uint32) []byte {
	const ihdrEnd = 8 + 25
	chunk := make([]byte, 0, 21)
	chunk = binary.BigEndian.AppendUint32(chunk, 9)
	chunk = append(chunk, "pHYs"...)
	chunk = binary.BigEndian.AppendUint32(chunk, pixelsPerMeter)
	chunk = binary.BigEndian.AppendUint32(chunk, pixelsPerMeter)
	chunk = append(chunk, 1)
	chunk = binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(chunk[4:]))

	out := make([]byte, 0, len(data)+len(chunk))
	out = append(out, data[:ihdrEnd]...)
	out = append(out, chunk...)
	return append(out, data[ihdrEnd:]...)
}
