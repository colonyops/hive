// Package profileimg stores a profile's (flow's) avatar as a normalized
// square PNG under the app data dir — one file per flow id. It is app-local
// state, not dotfiles config: the flow YAML records only a content-hash
// reference, and a missing file falls back to the letter chip the rail draws
// today.
//
// Normalization — center-crop to a square, downscale, canonical PNG
// re-encode — lives here rather than in a caller so every setter produces the
// same stored shape: the Wails settings view today, an HTTP or MCP surface
// later.
package profileimg

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"golang.org/x/image/draw"

	_ "image/gif"  // register GIF decoder for image.Decode
	_ "image/jpeg" // register JPEG decoder for image.Decode

	_ "golang.org/x/image/webp" // register WebP decoder for image.Decode
)

const (
	// MaxInputBytes caps a raw upload before decoding, so a hostile or huge
	// file is rejected cheaply rather than decoded into memory.
	MaxInputBytes = 8 << 20

	// maxInputSide rejects an image whose decoded dimensions are implausibly
	// large — a decode-bomb guard well above any real avatar source.
	maxInputSide = 12000

	// storedSide is the square edge of the stored PNG. The rail renders it at
	// ~38px; 128 stays crisp on a HiDPI display while keeping the file tiny.
	storedSide = 128
)

var (
	// ErrEmpty is returned when raw carries no bytes.
	ErrEmpty = errors.New("profileimg: image is empty")
	// ErrTooLarge is returned when raw exceeds MaxInputBytes or decodes to
	// implausible dimensions.
	ErrTooLarge = errors.New("profileimg: image is too large")
	// ErrUnsupported is returned when raw cannot be decoded as a known image
	// format (PNG, JPEG, GIF, WebP).
	ErrUnsupported = errors.New("profileimg: unsupported image format")
)

// Store keeps one normalized avatar PNG per flow id under dir.
type Store struct {
	dir string
}

// NewStore returns a store rooted at dir (typically <StateDir>/assets/profiles).
// The directory is created on first write.
func NewStore(dir string) *Store { return &Store{dir: dir} }

// Set normalizes raw into a square PNG, writes it as <id>.png, and returns the
// stored bytes' content hash — the value the flow YAML records. Replacing an
// existing image overwrites it.
func (s *Store) Set(id string, raw []byte) (string, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", ErrEmpty
	}
	if len(raw) > MaxInputBytes {
		return "", ErrTooLarge
	}

	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return "", ErrUnsupported
	}
	b := src.Bounds()
	if b.Dx() > maxInputSide || b.Dy() > maxInputSide {
		return "", ErrTooLarge
	}

	out, err := encodeSquarePNG(src)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(out)
	hash := hex.EncodeToString(sum[:16])
	if err := writeFileAtomic(s.path(id), out); err != nil {
		return "", err
	}
	return hash, nil
}

// Get returns the stored PNG for id, or ok=false when none is stored.
func (s *Store) Get(id string) (data []byte, ok bool, err error) {
	data, err = os.ReadFile(s.path(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("profileimg: read %s: %w", id, err)
	}
	return data, true, nil
}

// Delete removes id's stored image. A missing file is not an error, so a
// delete of a profile that never had one — or a retried cleanup — succeeds.
func (s *Store) Delete(id string) error {
	if err := os.Remove(s.path(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("profileimg: delete %s: %w", id, err)
	}
	return nil
}

func (s *Store) path(id string) string {
	return filepath.Join(s.dir, id+".png")
}

// encodeSquarePNG center-crops src to a square and scales it to storedSide,
// returning canonical PNG bytes. A source already square and sized is still
// re-encoded, so the stored format never depends on the input's.
func encodeSquarePNG(src image.Image) ([]byte, error) {
	b := src.Bounds()
	side := min(b.Dx(), b.Dy())
	offX := b.Min.X + (b.Dx()-side)/2
	offY := b.Min.Y + (b.Dy()-side)/2
	crop := image.Rect(offX, offY, offX+side, offY+side)

	dst := image.NewRGBA(image.Rect(0, 0, storedSide, storedSide))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, crop, draw.Src, nil)

	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, dst); err != nil {
		return nil, fmt.Errorf("profileimg: encode png: %w", err)
	}
	return buf.Bytes(), nil
}

func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("profileimg: create dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("profileimg: write %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("profileimg: replace %s: %w", filepath.Base(path), err)
	}
	return nil
}
