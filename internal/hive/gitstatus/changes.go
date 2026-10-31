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
	// Rendering is virtualized, so lines cost little: this keeps a diff readable
	// and git's maxDiffBytes keeps it bounded.
	maxDiffLines = 20000
	maxLineBytes = 4000
	// A diff read covers a batch of files, so it gets longer than a list read.
	diffReadTimeout = 15 * time.Second
	// A diff read reuses a list this recent. It outlasts the review view's 5s poll,
	// so an open view reads each list once. A file reverted since then reads as Gone.
	listReuse   = 10 * time.Second
	maxCommits  = 200
	maxBranches = 1000
)

// ENUM(branch, uncommitted, staged, unstaged, commit)
type DiffBase string

// DiffTarget says what a read compares. Against is the full ref a branch read
// compares with, and empty means the default branch. Commit is the commit a
// commit read shows, and empty means HEAD.
type DiffTarget struct {
	Base    DiffBase
	Against string
	Commit  string
}

// Changes is what differs in a session's checkout against a target. Base is the
// base the read used, which is uncommitted when a branch read has nothing to
// compare with: on the default branch, or when the default branch or merge base
// can't be resolved, as in a shallow clone.
//
// Against is the chosen ref a branch read used. It is empty for the default
// branch, including when a chosen ref no longer resolves. Commit is the full id
// a commit read showed. Commits lists what the branch added, newest first, or
// HEAD's recent history when it has no merge base.
type Changes struct {
	Resolved        bool
	Branch          string
	DefaultBranch   string
	Base            DiffBase
	BranchAvailable bool
	Against         string
	Commit          string
	Commits         []git.CommitInfo
	Files           []git.ChangedFile
	Truncated       bool
}

// ReadChanges answers the zero Changes for a session with no checkout, as
// ReadSession does: there is nothing to read, which is not a failure. Files
// stops at maxChangedFiles.
func (s *Service) ReadChanges(ctx context.Context, sess session.Session, target DiffTarget) (Changes, error) {
	if sess.State != session.StateActive || sess.Path == "" {
		return Changes{}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	changes, list, err := s.readList(ctx, sess.Path, target)
	if err != nil {
		return Changes{}, err
	}

	files := list.files
	if len(files) > maxChangedFiles {
		files = files[:maxChangedFiles]
		changes.Truncated = true
	}
	changes.Files = files
	return changes, nil
}

// changeList is what a diff read needs from a list read.
type changeList struct {
	diff  git.DiffRange
	files []git.ChangedFile
	at    time.Time
}

func (s *Service) readList(ctx context.Context, dir string, target DiffTarget) (Changes, changeList, error) {
	changes, diff, err := s.resolveTarget(ctx, dir, target)
	if err != nil {
		return Changes{}, changeList{}, err
	}
	files, err := s.git.ChangedFiles(ctx, dir, diff)
	if err != nil {
		return Changes{}, changeList{}, err
	}
	list := changeList{diff: diff, files: files, at: time.Now()}

	s.listsMu.Lock()
	defer s.listsMu.Unlock()
	for key, held := range s.lists {
		if time.Since(held.at) >= listReuse {
			delete(s.lists, key)
		}
	}
	s.lists[listKey(dir, target)] = list
	return changes, list, nil
}

func (s *Service) recentList(ctx context.Context, dir string, target DiffTarget) (changeList, error) {
	s.listsMu.Lock()
	list, ok := s.lists[listKey(dir, target)]
	s.listsMu.Unlock()
	if ok && time.Since(list.at) < listReuse {
		return list, nil
	}
	_, list, err := s.readList(ctx, dir, target)
	return list, err
}

func listKey(dir string, target DiffTarget) string {
	return strings.Join([]string{dir, target.Base.String(), target.Against, target.Commit}, "\x00")
}

// ReadBranches lists the full refs a session's checkout can compare against. A
// session with no checkout has none.
func (s *Service) ReadBranches(ctx context.Context, sess session.Session) ([]string, error) {
	if sess.State != session.StateActive || sess.Path == "" {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	return s.git.Branches(ctx, sess.Path, maxBranches)
}

func (s *Service) resolveTarget(ctx context.Context, dir string, target DiffTarget) (Changes, git.DiffRange, error) {
	branch, err := s.git.Branch(ctx, dir)
	if err != nil {
		return Changes{}, git.DiffRange{}, err
	}
	defaultBranch, against, mergeBase := s.branchBase(ctx, dir, branch, target.Against)

	changes := Changes{
		Resolved:        true,
		Branch:          branch,
		DefaultBranch:   defaultBranch,
		Base:            DiffBaseUncommitted,
		BranchAvailable: mergeBase != "",
		Against:         against,
		Commits:         s.commits(ctx, dir, mergeBase),
	}
	diff := git.WorkingTree("HEAD")
	switch target.Base {
	case DiffBaseStaged:
		changes.Base = DiffBaseStaged
		diff = git.Staged()
	case DiffBaseUnstaged:
		changes.Base = DiffBaseUnstaged
		diff = git.Unstaged()
	case DiffBaseCommit:
		commit, commitDiff, err := s.commitDiff(ctx, dir, target.Commit)
		if err != nil {
			return Changes{}, git.DiffRange{}, err
		}
		changes.Base, changes.Commit = DiffBaseCommit, commit
		diff = commitDiff
	case DiffBaseUncommitted:
	default:
		if changes.BranchAvailable {
			changes.Base = DiffBaseBranch
			diff = git.WorkingTree(mergeBase)
		}
	}
	return changes, diff, nil
}

// commitDiff falls back to HEAD for a commit that no longer resolves, which a
// rewritten branch can leave behind.
func (s *Service) commitDiff(ctx context.Context, dir, commit string) (string, git.DiffRange, error) {
	if commit != "" {
		if resolved, err := s.git.ResolveCommit(ctx, dir, commit); err == nil {
			diff, err := s.git.Commit(ctx, dir, resolved)
			return resolved, diff, err
		}
		s.logger.Debug().Str("path", dir).Str("commit", commit).Msg("git commit lookup failed")
	}
	resolved, err := s.git.ResolveCommit(ctx, dir, "HEAD")
	if err != nil {
		return "", git.DiffRange{}, err
	}
	diff, err := s.git.Commit(ctx, dir, resolved)
	return resolved, diff, err
}

// commits is only a picker's contents, so a failure leaves it empty rather than
// failing the read of the changes.
func (s *Service) commits(ctx context.Context, dir, mergeBase string) []git.CommitInfo {
	commits, err := s.git.Commits(ctx, dir, mergeBase, maxCommits)
	if err != nil {
		s.logger.Debug().Err(err).Str("path", dir).Msg("git log failed")
		return nil
	}
	return commits
}

// branchBase returns an empty merge base when the branch has nothing to compare
// against, which callers show as the uncommitted view rather than as a failure.
// A chosen ref that no longer resolves gives way to the default branch.
func (s *Service) branchBase(ctx context.Context, dir, branch, chosen string) (defaultBranch, against, mergeBase string) {
	defaultBranch, err := s.git.DefaultBranch(ctx, dir)
	if err != nil {
		s.logger.Debug().Err(err).Str("path", dir).Msg("git default branch lookup failed")
	}
	// The prefix also keeps a ref from reaching git as an option.
	if strings.HasPrefix(chosen, "refs/heads/") || strings.HasPrefix(chosen, "refs/remotes/") {
		mergeBase, err := s.git.MergeBaseRef(ctx, dir, chosen)
		if err == nil {
			return defaultBranch, chosen, mergeBase
		}
		s.logger.Debug().Err(err).Str("path", dir).Str("ref", chosen).Msg("git merge base lookup failed")
	}
	if defaultBranch == "" || defaultBranch == branch {
		return defaultBranch, "", ""
	}
	mergeBase, err = s.git.MergeBase(ctx, dir, defaultBranch)
	if err != nil {
		s.logger.Debug().Err(err).Str("path", dir).Msg("git merge base lookup failed")
		return defaultBranch, "", ""
	}
	return defaultBranch, "", mergeBase
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
func (s *Service) ReadFileDiffs(ctx context.Context, sess session.Session, target DiffTarget, paths []string) ([]FileDiff, error) {
	diffs := make([]FileDiff, len(paths))
	for i, path := range paths {
		diffs[i] = FileDiff{Path: path, Gone: true}
	}
	if sess.State != session.StateActive || sess.Path == "" {
		return diffs, nil
	}
	ctx, cancel := context.WithTimeout(ctx, diffReadTimeout)
	defer cancel()

	list, err := s.recentList(ctx, sess.Path, target)
	if err != nil {
		return nil, err
	}
	listed := make(map[string]git.ChangedFile, len(list.files))
	for _, file := range list.files {
		listed[file.Path] = file
	}

	var toRead []git.ChangedFile
	var readAt []int
	for i, path := range paths {
		file, ok := listed[path]
		switch {
		case !ok:
		case file.Binary:
			diffs[i] = FileDiff{Path: path, Binary: true}
		// Decided from the counts so a generated file never buffers its patch.
		case file.Additions+file.Deletions > maxDiffLines:
			diffs[i] = FileDiff{Path: path, Truncated: true}
		default:
			toRead = append(toRead, file)
			readAt = append(readAt, i)
		}
	}
	if len(toRead) == 0 {
		return diffs, nil
	}
	for i, result := range s.git.FileDiffs(ctx, sess.Path, list.diff, toRead) {
		diffs[readAt[i]] = s.fileDiffOf(sess.Path, toRead[i], result)
	}
	return diffs, nil
}

func (s *Service) fileDiffOf(dir string, file git.ChangedFile, result git.FileDiffResult) FileDiff {
	diff := FileDiff{Path: file.Path}
	text, err := result.Text, result.Err
	switch {
	case err == nil:
		// The list can be seconds old. A tracked file with no diff has gone back to
		// matching its base. An empty untracked file and a pure rename are still changes.
		if text == "" && file.Status != git.FileStatusUntracked && file.Status != git.FileStatusRenamed {
			diff.Gone = true
			return diff
		}
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
