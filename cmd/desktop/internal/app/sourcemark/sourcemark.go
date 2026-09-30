// Package sourcemark stores a source node's custom feed mark as a normalized
// square PNG under the app data dir, content-addressed by hash. A node's config
// records the hash; a missing file falls back to the node's glyph. See ADR webhook-source-image-marks.
package sourcemark

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
	// MaxInputBytes caps a raw upload before decoding.
	MaxInputBytes = 8 << 20
	// maxInputSide guards against a decode bomb.
	maxInputSide = 12000
	// markSide is the square edge of the stored PNG.
	markSide = 128
	// HashLen is the character length of a mark reference: hex of the first 16
	// bytes of the stored PNG's SHA-256.
	HashLen = 32
)

var (
	ErrEmpty       = errors.New("sourcemark: image is empty")
	ErrTooLarge    = errors.New("sourcemark: image is too large")
	ErrUnsupported = errors.New("sourcemark: unsupported image format")
)

// Store keeps normalized mark PNGs under dir, one per content hash.
type Store struct {
	dir string
}

// NewStore returns a store rooted at dir. The directory is created on first write.
func NewStore(dir string) *Store { return &Store{dir: dir} }

// Set normalizes raw into a square PNG, writes it as <hash>.png, and returns the
// content hash a node's config records.
func (s *Store) Set(raw []byte) (string, error) {
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
	if err := writeFileAtomic(s.path(hash), out); err != nil {
		return "", err
	}
	return hash, nil
}

// Get returns the stored PNG for hash, or ok=false when none is stored. A
// malformed or missing hash reads as absent, not an error.
func (s *Store) Get(hash string) (data []byte, ok bool, err error) {
	if !ValidHash(hash) {
		return nil, false, nil
	}
	data, err = os.ReadFile(s.path(hash))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("sourcemark: read %s: %w", hash, err)
	}
	return data, true, nil
}

func (s *Store) path(hash string) string {
	return filepath.Join(s.dir, hash+".png")
}

// ValidHash reports whether s is lowercase hex of length HashLen — the shape Set
// produces. It also guards Get against path traversal.
func ValidHash(s string) bool {
	if len(s) != HashLen {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// encodeSquarePNG scales src to fit within a markSide square, centers it on a
// transparent canvas of that size, and returns canonical PNG bytes.
func encodeSquarePNG(src image.Image) ([]byte, error) {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	longest := max(sw, sh)
	dw := max(sw*markSide/longest, 1)
	dh := max(sh*markSide/longest, 1)
	offX := (markSide - dw) / 2
	offY := (markSide - dh) / 2

	dst := image.NewRGBA(image.Rect(0, 0, markSide, markSide))
	draw.CatmullRom.Scale(dst, image.Rect(offX, offY, offX+dw, offY+dh), src, b, draw.Src, nil)

	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, dst); err != nil {
		return nil, fmt.Errorf("sourcemark: encode png: %w", err)
	}
	return buf.Bytes(), nil
}

func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("sourcemark: create dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("sourcemark: write %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("sourcemark: replace %s: %w", filepath.Base(path), err)
	}
	return nil
}
