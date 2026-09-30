package terminalimg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	_ "golang.org/x/image/webp"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

const (
	MaxFileBytes   = 20 << 20
	MaxBatchBytes  = 64 << 20
	MaxFiles       = 10
	maxPixels      = 64_000_000
	maxStoredBytes = 1 << 30
)

var (
	ErrInvalid = errors.New("invalid image input")
	ErrQuota   = errors.New("image storage is full")
)

type Store struct {
	dir   string
	mu    sync.Mutex
	quota int64
}

func NewStore(dir string) *Store {
	return &Store{dir: dir, quota: maxStoredBytes}
}

// Prepare preserves the user's files; only clipboard bytes belong in the store.
func (s *Store) Prepare(ctx context.Context, paths []string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := count(len(paths)); err != nil {
		return nil, err
	}
	pastes := make([]string, 0, len(paths))
	total := 0
	for _, path := range paths {
		if !filepath.IsAbs(path) || !safePath(path) {
			return nil, fmt.Errorf("%w: image paths must be absolute and contain no control characters", ErrInvalid)
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return nil, fmt.Errorf("%w: image file is unavailable", ErrInvalid)
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%w: choose a readable image file", ErrInvalid)
		}
		if info.Size() > MaxFileBytes {
			return nil, fmt.Errorf("%w: each image must be at most 20 MiB", ErrInvalid)
		}
		f, err := os.Open(resolved)
		if err != nil {
			return nil, fmt.Errorf("%w: image file is not readable", ErrInvalid)
		}
		raw, err := read(ctx, f)
		_ = f.Close()
		if err != nil {
			return nil, err
		}
		total += len(raw)
		if total > MaxBatchBytes {
			return nil, fmt.Errorf("%w: images must total at most 64 MiB", ErrInvalid)
		}
		if _, err := validate(raw); err != nil {
			return nil, err
		}
		if !safePath(resolved) {
			return nil, fmt.Errorf("%w: image path contains control characters", ErrInvalid)
		}
		pastes = append(pastes, quotePath(resolved))
	}
	return pastes, ctx.Err()
}

// Save commits a whole batch before returning any references. Completed batches
// outlive terminals and the app because external agents can resume those paths.
func (s *Store) Save(ctx context.Context, images []io.Reader) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := count(len(images)); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(s.dir) || !safePath(s.dir) {
		return nil, fmt.Errorf("%w: image storage path is invalid", ErrInvalid)
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return nil, fmt.Errorf("create image storage: %w", err)
	}
	used, err := s.usedBytes()
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(s.dir, ".pending-")
	if err != nil {
		return nil, fmt.Errorf("create image batch: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	names := make([]string, 0, len(images))
	total := 0
	for i, input := range images {
		raw, err := read(ctx, input)
		if err != nil {
			return nil, err
		}
		total += len(raw)
		if total > MaxBatchBytes {
			return nil, fmt.Errorf("%w: images must total at most 64 MiB", ErrInvalid)
		}
		if used+int64(total) > s.quota {
			return nil, fmt.Errorf("%w: remove images you no longer need from %s (1 GiB limit)", ErrQuota, s.dir)
		}
		ext, err := validate(raw)
		if err != nil {
			return nil, err
		}
		name := fmt.Sprintf("image-%d.%s", i+1, ext)
		if err := os.WriteFile(filepath.Join(dir, name), raw, 0o600); err != nil {
			return nil, fmt.Errorf("save image: %w", err)
		}
		names = append(names, name)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	committed := filepath.Join(s.dir, strings.TrimPrefix(filepath.Base(dir), ".pending-"))
	if err := os.Rename(dir, committed); err != nil {
		return nil, fmt.Errorf("commit image batch: %w", err)
	}
	pastes := make([]string, 0, len(names))
	for _, name := range names {
		pastes = append(pastes, quotePath(filepath.Join(committed, name)))
	}
	return pastes, nil
}

func (s *Store) usedBytes() (int64, error) {
	var used int64
	err := filepath.WalkDir(s.dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		used += info.Size()
		return nil
	})
	return used, err
}

func count(n int) error {
	if n < 1 || n > MaxFiles {
		return fmt.Errorf("%w: choose between 1 and 10 images", ErrInvalid)
	}
	return nil
}

func read(ctx context.Context, input io.Reader) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(input, MaxFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read image: %w", err)
	}
	if len(raw) > MaxFileBytes {
		return nil, fmt.Errorf("%w: each image must be at most 20 MiB", ErrInvalid)
	}
	return raw, ctx.Err()
}

func validate(raw []byte) (string, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("%w: choose a PNG, JPEG, GIF or WebP image", ErrInvalid)
	}
	switch format {
	case "png", "jpeg", "gif", "webp":
	default:
		return "", fmt.Errorf("%w: choose a PNG, JPEG, GIF or WebP image", ErrInvalid)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width) > maxPixels/int64(cfg.Height) {
		return "", fmt.Errorf("%w: images must be at most 64 megapixels", ErrInvalid)
	}
	if _, _, err := image.Decode(bytes.NewReader(raw)); err != nil {
		return "", fmt.Errorf("%w: image is corrupt or unsupported", ErrInvalid)
	}
	return format, nil
}

func safePath(path string) bool {
	return utf8.ValidString(path) && !strings.ContainsFunc(path, unicode.IsControl)
}

func quotePath(path string) string {
	var out strings.Builder
	for _, r := range path {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("/._-:", r) {
			out.WriteByte('\\')
		}
		out.WriteRune(r)
	}
	// Each image is a separate paste: Codex parses one path per paste event.
	out.WriteByte(' ')
	return out.String()
}
