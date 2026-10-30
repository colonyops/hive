package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/colonyops/hive/pkg/executil"
)

// ENUM(modified, added, deleted, renamed, untracked)
type FileStatus string

// ChangedFile is one path that differs from a diff's base. OldPath is set for
// a rename. Untracked files carry no line counts, so a list's totals match
// DiffStats and a poll reads no file contents.
type ChangedFile struct {
	Path      string
	OldPath   string
	Status    FileStatus
	Additions int
	Deletions int
	Binary    bool
}

// Polled reads run beside an agent in the checkout, so they must not write
// under .git. The flag core.fsmonitor=false also keeps a repo's hook from
// running on each poll. Clean filters stay on: git status runs them on the
// status bar's poll, and without them every LFS file reads as changed.
var readFlags = []string{"--no-optional-locks", "-c", "core.fsmonitor=false"}

// color.ui=always would otherwise break the parsers.
var diffFlags = []string{"--no-ext-diff", "--no-textconv", "--no-color"}

const readErrorBytes = 500

// read returns stdout alone: git writes warnings to stderr, and those must not
// reach a parser.
func (e *Executor) read(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return e.capture(ctx, dir, args[0], e.gitPath, slices.Concat(readFlags, args)...)
}

func (e *Executor) capture(ctx context.Context, dir, operation, cmd string, args ...string) ([]byte, error) {
	var stdout bytes.Buffer
	stderr := &executil.HeadWriter{Max: readErrorBytes}
	if err := e.exec.RunDirStream(ctx, dir, &stdout, stderr, cmd, args...); err != nil {
		if text := strings.TrimSpace(stderr.String()); text != "" {
			return nil, fmt.Errorf("git %s: %s: %w", operation, text, err)
		}
		return nil, fmt.Errorf("git %s: %w", operation, err)
	}
	return stdout.Bytes(), nil
}

// withScratchIndex runs read with a private copy of dir's index and passes the
// environment assignment that selects it.
//
// git diff takes index.lock and rewrites the index when it meets a touched
// file, even under --no-optional-locks, and a lock held at that moment fails an
// agent's own git command. diff.autoRefreshIndex=false stops the write but
// reports a touched file as modified with an empty diff. A copy gives the right
// answer and leaves the write somewhere nothing else looks.
func (e *Executor) withScratchIndex(ctx context.Context, dir string, read func(indexEnv string) error) error {
	out, err := e.read(ctx, dir, "rev-parse", "--git-path", "index")
	if err != nil {
		return err
	}
	source := strings.TrimSpace(string(out))
	if !filepath.IsAbs(source) {
		source = filepath.Join(dir, source)
	}

	scratch, err := os.MkdirTemp("", "hive-git-index-")
	if err != nil {
		return fmt.Errorf("create scratch index: %w", err)
	}
	defer func() { _ = os.RemoveAll(scratch) }()

	index := filepath.Join(scratch, "index")
	// A repo with nothing added has no index, which git reads as an empty one.
	if err := copyFile(source, index); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("copy index: %w", err)
	}
	return read("GIT_INDEX_FILE=" + index)
}

func copyFile(source, dest string) error {
	in, err := os.Open(source) //nolint:gosec // git named this path
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dest) //nolint:gosec // inside a directory this process just created
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// readDiff runs git diff against the scratch index. The env program sets the
// variable because executil.Executor has no per-command environment.
func (e *Executor) readDiff(ctx context.Context, dir, indexEnv string, args ...string) ([]byte, error) {
	argv := slices.Concat([]string{indexEnv, e.gitPath}, readFlags, []string{"diff"}, diffFlags, args)
	return e.capture(ctx, dir, "diff", "env", argv...)
}

// MergeBase returns the commit where HEAD left branch. It prefers
// origin/<branch>, because a checkout's local default branch goes stale and
// would inflate the diff. A bare clone's worktree has no remote refs, so there
// it uses the local branch.
func (e *Executor) MergeBase(ctx context.Context, dir, branch string) (string, error) {
	ref := "refs/heads/" + branch
	remote := "refs/remotes/origin/" + branch
	if _, err := e.read(ctx, dir, "rev-parse", "--verify", "--quiet", remote+"^{commit}"); err == nil {
		ref = remote
	}
	out, err := e.read(ctx, dir, "merge-base", ref, "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ChangedFiles lists the working tree against ref, plus untracked files,
// sorted by path.
func (e *Executor) ChangedFiles(ctx context.Context, dir, ref string) ([]ChangedFile, error) {
	var names, stats []byte
	err := e.withScratchIndex(ctx, dir, func(indexEnv string) error {
		var err error
		if names, err = e.readDiff(ctx, dir, indexEnv, "--name-status", "-z", "--find-renames", ref, "--"); err != nil {
			return err
		}
		stats, err = e.readDiff(ctx, dir, indexEnv, "--numstat", "-z", "--find-renames", ref, "--")
		return err
	})
	if err != nil {
		return nil, err
	}
	untracked, err := e.read(ctx, dir, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}

	files := parseNameStatus(names)
	counts := parseNumstat(stats)
	for i := range files {
		if count, ok := counts[files[i].Path]; ok {
			files[i].Additions, files[i].Deletions, files[i].Binary = count.additions, count.deletions, count.binary
		}
	}
	for _, path := range nulFields(untracked) {
		files = append(files, ChangedFile{Path: path, Status: FileStatusUntracked})
	}
	slices.SortFunc(files, func(a, b ChangedFile) int { return strings.Compare(a.Path, b.Path) })
	return files, nil
}

func nulFields(out []byte) []string {
	if len(out) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
}

// parseNameStatus reads diff --name-status -z: "M\0path\0", or "R100\0old\0new\0"
// for a rename. A type change or an unmerged path reads as modified.
func parseNameStatus(out []byte) []ChangedFile {
	fields := nulFields(out)
	files := make([]ChangedFile, 0, len(fields)/2)
	for i := 0; i+1 < len(fields); i += 2 {
		file := ChangedFile{Path: fields[i+1], Status: FileStatusModified}
		switch fields[i][0] {
		case 'A':
			file.Status = FileStatusAdded
		case 'D':
			file.Status = FileStatusDeleted
		case 'R':
			if i+2 >= len(fields) {
				return files
			}
			file.Status, file.OldPath, file.Path = FileStatusRenamed, fields[i+1], fields[i+2]
			i++
		}
		files = append(files, file)
	}
	return files
}

type lineCount struct {
	additions, deletions int
	binary               bool
}

// parseNumstat reads diff --numstat -z, keyed by the new path. An entry is
// "add\tdel\tpath\0", or "add\tdel\t\0old\0new\0" for a rename. A binary file
// reports "-" for both counts.
func parseNumstat(out []byte) map[string]lineCount {
	fields := nulFields(out)
	counts := make(map[string]lineCount, len(fields))
	for i := 0; i < len(fields); i++ {
		parts := strings.SplitN(fields[i], "\t", 3)
		if len(parts) != 3 {
			continue
		}
		path := parts[2]
		if path == "" {
			if i+2 >= len(fields) {
				return counts
			}
			path = fields[i+2]
			i += 2
		}
		if parts[0] == "-" {
			counts[path] = lineCount{binary: true}
			continue
		}
		// git wrote these digits, so a failed parse can only read as zero.
		additions, _ := strconv.Atoi(parts[0])
		deletions, _ := strconv.Atoi(parts[1])
		counts[path] = lineCount{additions: additions, deletions: deletions}
	}
	return counts
}
