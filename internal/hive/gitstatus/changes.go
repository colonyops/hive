package gitstatus

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"time"

	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/platform/git"
)

const (
	maxChangedFiles = 1000
	maxDiffLines    = 1500
	maxLineBytes    = 4000
	// A diff read covers a batch of files, so it gets longer than a list read.
	diffReadTimeout = 15 * time.Second
)

// ENUM(branch, uncommitted)
type DiffBase string

// Changes is what differs in a session's checkout against a base. Base is the
// base the read used, which is uncommitted whenever BranchAvailable is false:
// on the default branch, or when the default branch or merge base can't be
// resolved, as in a shallow clone.
type Changes struct {
	Resolved        bool
	Branch          string
	DefaultBranch   string
	Base            DiffBase
	BranchAvailable bool
	Files           []git.ChangedFile
	Truncated       bool
}

// ReadChanges answers the zero Changes for a session with no checkout, as
// ReadSession does: there is nothing to read, which is not a failure. Files
// stops at maxChangedFiles.
func (s *Service) ReadChanges(ctx context.Context, sess session.Session, base DiffBase) (Changes, error) {
	if sess.State != session.StateActive || sess.Path == "" {
		return Changes{}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	changes, ref, err := s.resolveBase(ctx, sess.Path, base)
	if err != nil {
		return Changes{}, err
	}

	files, err := s.git.ChangedFiles(ctx, sess.Path, ref)
	if err != nil {
		return Changes{}, err
	}
	if len(files) > maxChangedFiles {
		files = files[:maxChangedFiles]
		changes.Truncated = true
	}
	changes.Files = files
	return changes, nil
}

func (s *Service) resolveBase(ctx context.Context, dir string, base DiffBase) (Changes, string, error) {
	branch, err := s.git.Branch(ctx, dir)
	if err != nil {
		return Changes{}, "", err
	}
	defaultBranch, mergeBase := s.branchBase(ctx, dir, branch)

	changes := Changes{
		Resolved:        true,
		Branch:          branch,
		DefaultBranch:   defaultBranch,
		Base:            DiffBaseUncommitted,
		BranchAvailable: mergeBase != "",
	}
	ref := "HEAD"
	if changes.BranchAvailable && base != DiffBaseUncommitted {
		changes.Base = DiffBaseBranch
		ref = mergeBase
	}
	return changes, ref, nil
}

// branchBase returns an empty merge base when the branch has nothing to compare
// against, which callers show as the uncommitted view rather than as a failure.
func (s *Service) branchBase(ctx context.Context, dir, branch string) (defaultBranch, mergeBase string) {
	defaultBranch, err := s.git.DefaultBranch(ctx, dir)
	if err != nil {
		s.logger.Debug().Err(err).Str("path", dir).Msg("git default branch lookup failed")
		return "", ""
	}
	if defaultBranch == branch {
		return defaultBranch, ""
	}
	mergeBase, err = s.git.MergeBase(ctx, dir, defaultBranch)
	if err != nil {
		s.logger.Debug().Err(err).Str("path", dir).Msg("git merge base lookup failed")
		return defaultBranch, ""
	}
	return defaultBranch, mergeBase
}

// FileDiff is one file's diff. At most one state field is set, and with none
// set Text is the unified diff, which is empty for a rename with no edits.
type FileDiff struct {
	Path string
	Text string
	// Binary means a binary file, and Truncated a diff past the caps.
	Binary    bool
	Truncated bool
	// Unavailable means a path with no text to show: a FIFO, a symlink, or a
	// nested repository.
	Unavailable bool
	// Gone means the path is no longer in the change list, which is routine: an
	// agent can revert a file between the list and this read.
	Gone bool
	// Error is why this one file failed, so it can't blank the rest of a batch.
	Error string
}

// ReadFileDiffs returns one FileDiff per path, in order. It reads only paths in
// the current change list. A session with no checkout reports every path Gone.
func (s *Service) ReadFileDiffs(ctx context.Context, sess session.Session, base DiffBase, paths []string) ([]FileDiff, error) {
	diffs := make([]FileDiff, len(paths))
	for i, path := range paths {
		diffs[i] = FileDiff{Path: path, Gone: true}
	}
	if sess.State != session.StateActive || sess.Path == "" {
		return diffs, nil
	}
	ctx, cancel := context.WithTimeout(ctx, diffReadTimeout)
	defer cancel()

	_, ref, err := s.resolveBase(ctx, sess.Path, base)
	if err != nil {
		return nil, err
	}
	files, err := s.git.ChangedFiles(ctx, sess.Path, ref)
	if err != nil {
		return nil, err
	}
	listed := make(map[string]git.ChangedFile, len(files))
	for _, file := range files {
		listed[file.Path] = file
	}
	for i, path := range paths {
		if file, ok := listed[path]; ok {
			diffs[i] = s.readFileDiff(ctx, sess.Path, ref, file)
		}
	}
	return diffs, nil
}

func (s *Service) readFileDiff(ctx context.Context, dir, ref string, file git.ChangedFile) FileDiff {
	diff := FileDiff{Path: file.Path}
	if file.Binary {
		diff.Binary = true
		return diff
	}
	// Decided from the counts so a generated file never buffers its patch.
	if file.Additions+file.Deletions > maxDiffLines {
		diff.Truncated = true
		return diff
	}
	text, err := s.git.FileDiff(ctx, dir, ref, file)
	switch {
	case err == nil:
		diff.Text, diff.Truncated = clipDiff(text)
	case errors.Is(err, git.ErrBinaryDiff):
		diff.Binary = true
	case errors.Is(err, git.ErrDiffTooLarge):
		diff.Truncated = true
	case errors.Is(err, git.ErrNoTextDiff):
		diff.Unavailable = true
	case errors.Is(err, fs.ErrNotExist):
		diff.Gone = true
	default:
		s.logger.Debug().Err(err).Str("path", dir).Str("file", file.Path).Msg("git file diff failed")
		diff.Error = err.Error()
	}
	return diff
}

// clipDiff caps the lines and their length. The line cap counts context too, so
// it allows twice the changed-line cap: a file under that cap with many small
// hunks must still show. Invalid UTF-8 is replaced because protobuf strings
// refuse it, and a Latin-1 file must not fail the whole read.
func clipDiff(text string) (string, bool) {
	lines := strings.Split(text, "\n")
	if len(lines) > 2*maxDiffLines {
		return "", true
	}
	for i, line := range lines {
		if len(line) > maxLineBytes {
			lines[i] = line[:maxLineBytes]
		}
	}
	return strings.ToValidUTF8(strings.Join(lines, "\n"), "\uFFFD"), false
}
