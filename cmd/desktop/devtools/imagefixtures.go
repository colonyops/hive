package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/image/bmp"
)

const imageFixtureDir = "image-fixtures"

// imageFixture is one file the Changes view should draw as an image. base is
// what the fixture commit holds, nil for a file that only the working tree
// adds. edit is what the working tree holds after, nil for a deleted file.
type imageFixture struct {
	name string
	base []byte
	edit []byte
	// renameTo moves the file in the working tree, staged so git sees a rename.
	renameTo string
}

// writeImageFixtures commits a set of images to dir and then changes them in
// the working tree, so the Changes view's Uncommitted target shows every kind
// of image change at once: modified, resized, added, deleted, renamed, an SVG
// with a text diff, and a file that claims to be an image and is not.
func writeImageFixtures(dir string) ([]string, error) {
	if err := gitIn(dir, "rev-parse", "--is-inside-work-tree"); err != nil {
		return nil, fmt.Errorf("%s is not a git checkout: %w", dir, err)
	}
	if out, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output(); err != nil {
		return nil, err
	} else if len(bytes.TrimSpace(out)) > 0 {
		return nil, errors.New("the checkout has uncommitted changes; commit or stash them first so the fixture commit holds only fixtures")
	}

	fixtures := imageFixtureSet()
	for _, f := range fixtures {
		if f.base != nil {
			if err := writeFixture(dir, f.name, f.base); err != nil {
				return nil, err
			}
		}
	}
	if err := gitIn(dir, "add", "--", imageFixtureDir); err != nil {
		return nil, err
	}
	if err := gitIn(dir, "commit", "-q", "-m", "test: add image diff fixtures"); err != nil {
		return nil, err
	}

	var changed []string
	for _, f := range fixtures {
		path := filepath.Join(imageFixtureDir, f.name)
		switch {
		case f.renameTo != "":
			to := filepath.Join(imageFixtureDir, f.renameTo)
			if err := gitIn(dir, "mv", "--", path, to); err != nil {
				return nil, err
			}
			path = to
			if f.edit != nil {
				if err := os.WriteFile(filepath.Join(dir, path), f.edit, 0o600); err != nil {
					return nil, err
				}
			}
		case f.edit == nil:
			if err := os.Remove(filepath.Join(dir, path)); err != nil {
				return nil, err
			}
		default:
			if err := writeFixture(dir, f.name, f.edit); err != nil {
				return nil, err
			}
		}
		changed = append(changed, path)
	}
	return changed, nil
}

func imageFixtureSet() []imageFixture {
	return []imageFixture{
		{name: "icon.png", base: encodePNG(pixelIcon(color.RGBA{0x3b, 0x82, 0xf6, 0xff}, false)), edit: encodePNG(pixelIcon(color.RGBA{0xf9, 0x73, 0x16, 0xff}, true))},
		{name: "photo.jpg", base: encodeJPEG(landscape(480, 270, false)), edit: encodeJPEG(landscape(480, 270, true))},
		{name: "badge.png", base: encodePNG(badge(64, 0.35)), edit: encodePNG(badge(128, 0.45))},
		{name: "overlay.png", base: encodePNG(translucent(240, 160, 0)), edit: encodePNG(translucent(240, 160, 20))},
		{name: "banner-added.png", edit: encodePNG(landscape(640, 160, true))},
		{name: "spinner-deleted.gif", base: encodeGIF(pixelIcon(color.RGBA{0x10, 0xb9, 0x81, 0xff}, false))},
		{name: "screenshot-old.bmp", base: encodeBMP(landscape(320, 200, false)), renameTo: "screenshot-renamed.bmp"},
		{name: "logo.svg", base: []byte(logoSVG("#6366f1", "hive")), edit: []byte(logoSVG("#ec4899", "Hive"))},
		{name: "corrupt.png", base: encodePNG(pixelIcon(color.RGBA{0x64, 0x74, 0x8b, 0xff}, false)), edit: []byte("\x00\x01this is not a png\x00")},
	}
}

func writeFixture(dir, name string, data []byte) error {
	path := filepath.Join(dir, imageFixtureDir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func gitIn(dir string, args ...string) error {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput() //nolint:gosec // dev tool; args are fixed
	if err != nil {
		return fmt.Errorf("git %s: %s: %w", args[0], strings.TrimSpace(string(out)), err)
	}
	return nil
}

// pixelIcon is a 16x16 smiley, small enough that the preview draws it pixelated.
func pixelIcon(fill color.RGBA, wink bool) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := range 16 {
		for x := range 16 {
			dx, dy := float64(x)-7.5, float64(y)-7.5
			if dx*dx+dy*dy <= 56 {
				img.Set(x, y, fill)
			}
		}
	}
	dark := color.RGBA{0x11, 0x18, 0x27, 0xff}
	img.Set(5, 5, dark)
	img.Set(5, 6, dark)
	if wink {
		img.Set(9, 6, dark)
		img.Set(10, 6, dark)
	} else {
		img.Set(10, 5, dark)
		img.Set(10, 6, dark)
	}
	for x := 4; x <= 11; x++ {
		img.Set(x, 11, dark)
	}
	img.Set(3, 10, dark)
	img.Set(12, 10, dark)
	return img
}

// landscape is a sky gradient with hills. The edited version adds a sun, so
// a swipe shows one region change.
func landscape(w, h int, sun bool) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		t := float64(y) / float64(h)
		for x := range w {
			hill := 0.65*float64(h) + 0.12*float64(h)*math.Sin(float64(x)/float64(w)*2*math.Pi)
			switch {
			case float64(y) > hill:
				img.Set(x, y, color.RGBA{uint8(40 + 30*t), uint8(120 + 40*t), 60, 0xff})
			default:
				img.Set(x, y, color.RGBA{uint8(90 + 100*t), uint8(150 + 60*t), 0xf0, 0xff})
			}
			if sun {
				dx, dy := float64(x)-0.78*float64(w), float64(y)-0.28*float64(h)
				if dx*dx+dy*dy <= math.Pow(0.11*float64(h), 2) {
					img.Set(x, y, color.RGBA{0xfd, 0xe0, 0x47, 0xff})
				}
			}
		}
	}
	return img
}

// badge is a ring on a transparent square; the edit doubles its size.
func badge(size int, ring float64) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	c := float64(size) / 2
	for y := range size {
		for x := range size {
			d := math.Hypot(float64(x)-c+0.5, float64(y)-c+0.5) / c
			if d <= 1 && d >= 1-ring {
				img.Set(x, y, color.RGBA{0x8b, 0x5c, 0xf6, 0xff})
			}
		}
	}
	return img
}

// translucent fades from opaque to clear, so the backdrop toggle shows
// through it. shift moves the stripes in the edited version.
func translucent(w, h, shift int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			alpha := uint8(255 * (1 - float64(x)/float64(w)))
			c := color.NRGBA{0xef, 0x44, 0x44, alpha}
			if ((x+shift)/20)%2 == 0 {
				c = color.NRGBA{0x0e, 0xa5, 0xe9, alpha}
			}
			img.Set(x, y, c)
		}
	}
	return img
}

func logoSVG(fill, label string) string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="200" height="120" viewBox="0 0 200 120">
  <rect x="4" y="4" width="192" height="112" rx="16" fill="%s"/>
  <polygon points="60,30 85,44 85,72 60,86 35,72 35,44" fill="#fff" opacity="0.9"/>
  <text x="100" y="68" font-family="sans-serif" font-size="28" font-weight="700" fill="#fff">%s</text>
</svg>
`, fill, label)
}

func encodePNG(img image.Image) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func encodeJPEG(img image.Image) []byte {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func encodeGIF(img image.Image) []byte {
	var buf bytes.Buffer
	// draw.Src keeps the flat colors; the default dithers them into noise.
	if err := gif.Encode(&buf, img, &gif.Options{NumColors: 256, Drawer: draw.Src}); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func encodeBMP(img image.Image) []byte {
	var buf bytes.Buffer
	if err := bmp.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}
