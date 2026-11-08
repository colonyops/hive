package git

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"syscall"

	"github.com/colonyops/hive/pkg/executil"
)

const (
	worktreeSide = ""
	indexSide    = ":"
)

// ErrFileTooLarge is a file version past the limit FileVersions was given.
var ErrFileTooLarge = errors.New("file too large")

// FileVersions is a file's bytes on each side of a diff. A side the file is
// absent from is nil: Before for an added file, After for a deleted one.
type FileVersions struct {
	Before []byte
	After  []byte
}

// FileVersions reads both versions of file across diff, each at most limit
// bytes. A rename reads its old path on the before side.
func (e *Executor) FileVersions(ctx context.Context, dir string, diff DiffRange, file ChangedFile, limit int) (FileVersions, error) {
	var versions FileVersions
	if file.Status != FileStatusAdded && file.Status != FileStatusUntracked {
		path := file.Path
		if file.OldPath != "" {
			path = file.OldPath
		}
		before, err := e.fileAt(ctx, dir, diff.before, path, limit)
		if err != nil {
			return FileVersions{}, err
		}
		versions.Before = before
	}
	if file.Status != FileStatusDeleted {
		after, err := e.fileAt(ctx, dir, diff.after, file.Path, limit)
		if err != nil {
			return FileVersions{}, err
		}
		versions.After = after
	}
	return versions, nil
}

func (e *Executor) fileAt(ctx context.Context, dir, side, path string, limit int) ([]byte, error) {
	if side == worktreeSide {
		return readWorktreeFile(filepath.Join(dir, path), limit)
	}
	spec := side + ":" + path
	if side == indexSide {
		spec = ":" + path
	}
	stdout := &executil.HeadWriter{Max: limit}
	if err := e.capture(ctx, dir, "cat-file", stdout, e.gitPath, slices.Concat(readFlags, []string{"cat-file", "blob", spec})...); err != nil {
		return nil, err
	}
	if stdout.Truncated() {
		return nil, ErrFileTooLarge
	}
	// An empty blob is still a version of the file, so it must not read as absent.
	return append([]byte{}, stdout.Bytes()...), nil
}

func readWorktreeFile(path string, limit int) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, ErrNoTextDiff
	}
	if info.Size() > int64(limit) {
		return nil, ErrFileTooLarge
	}
	// O_NONBLOCK because the path can turn into a FIFO after the Lstat.
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0) //nolint:gosec // git listed this path inside the checkout
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	content, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(content) > limit {
		return nil, ErrFileTooLarge
	}
	return content, nil
}
