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
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sync/errgroup"

	"github.com/colonyops/hive/pkg/executil"
)

// ENUM(modified, added, deleted, renamed, untracked)
type FileStatus string

// ChangedFile is one path that differs from a diff's base. OldPath is set for
// a rename. Untracked files carry no line counts, so a list's totals match
// DiffStats and a poll reads no file contents.
//
// Stamp changes when the file's diff across this range can. Line counts alone
// can't tell a reader that its diff is stale: replacing one line with another
// leaves them the same, and an untracked file has none.
type ChangedFile struct {
	Path      string
	OldPath   string
	Status    FileStatus
	Additions int
	Deletions int
	Binary    bool
	Stamp     string
}

// FileDiff reports these instead of a diff.
var (
	ErrBinaryDiff   = errors.New("binary file")
	ErrNoTextDiff   = errors.New("not a regular file")
	ErrDiffTooLarge = errors.New("diff too large")
)

// maxDiffBytes keeps one generated file from buffering in memory.
const (
	maxDiffBytes      = 2 << 20
	binarySniffLength = 8 << 10
	// A batch past this is read file by file, so its memory stays bounded.
	maxBatchDiffBytes = 32 << 20
	maxFileReads      = 4
)

// Polled reads run beside an agent in the checkout, so they must not write
// under .git. The flag core.fsmonitor=false also keeps a repo's hook from
// running on each poll. Clean filters stay on: git status runs them on the
// status bar's poll, and without them every LFS file reads as changed.
//
// Paths are literal because a file can be named [id].vue, which a pathspec would
// read as a glob.
var readFlags = []string{"--no-optional-locks", "--literal-pathspecs", "-c", "core.fsmonitor=false", "-c", "core.quotePath=false"}

// color.ui=always would otherwise break the parsers, and config can change the
// prefixes and the submodule format that fileHeader relies on.
var diffFlags = []string{"--no-ext-diff", "--no-textconv", "--no-color", "--src-prefix=a/", "--dst-prefix=b/", "--submodule=short"}

const readErrorBytes = 500

// DiffRange is the two sides of a diff. Build one with WorkingTree, Staged,
// Unstaged or Commit.
type DiffRange struct {
	args      []string
	untracked bool
	staged    bool
	commit    string
}

// WorkingTree compares the working tree with ref. It lists untracked files.
func WorkingTree(ref string) DiffRange {
	return DiffRange{args: []string{ref}, untracked: true}
}

// Staged compares the index with HEAD.
func Staged() DiffRange {
	return DiffRange{args: []string{"--cached", "HEAD"}, staged: true}
}

// Unstaged compares the working tree with the index. It lists untracked files.
func Unstaged() DiffRange {
	return DiffRange{untracked: true}
}

// CommitRange compares a commit with its first parent, or with the empty tree for a
// root commit. It takes the id ResolveCommit returned.
func (e *Executor) CommitRange(ctx context.Context, dir, commit string) (DiffRange, error) {
	from := commit + "^"
	if _, err := e.read(ctx, dir, "rev-parse", "--verify", "--quiet", "--end-of-options", from+"^{commit}"); err != nil {
		// hash-object writes nothing without -w, and it names the empty tree in the
		// repository's own hash.
		out, err := e.read(ctx, dir, "hash-object", "-t", "tree", os.DevNull)
		if err != nil {
			return DiffRange{}, err
		}
		from = strings.TrimSpace(string(out))
	}
	return DiffRange{args: []string{from, commit}, commit: commit}, nil
}

// read returns stdout alone: git writes warnings to stderr, and those must not
// reach a parser.
func (e *Executor) read(ctx context.Context, dir string, args ...string) ([]byte, error) {
	var stdout bytes.Buffer
	if err := e.capture(ctx, dir, args[0], &stdout, e.gitPath, slices.Concat(readFlags, args)...); err != nil {
		return nil, err
	}
	return stdout.Bytes(), nil
}

func (e *Executor) capture(ctx context.Context, dir, operation string, stdout io.Writer, cmd string, args ...string) error {
	stderr := &executil.HeadWriter{Max: readErrorBytes}
	if err := e.exec.RunDirStream(ctx, dir, stdout, stderr, cmd, args...); err != nil {
		if text := strings.TrimSpace(stderr.String()); text != "" {
			return fmt.Errorf("git %s: %s: %w", operation, text, err)
		}
		return fmt.Errorf("git %s: %w", operation, err)
	}
	return nil
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
func (e *Executor) readDiff(ctx context.Context, dir, indexEnv string, stdout io.Writer, args ...string) error {
	argv := slices.Concat([]string{indexEnv, e.gitPath}, readFlags, []string{"diff"}, diffFlags, args)
	return e.capture(ctx, dir, "diff", stdout, "env", argv...)
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
	return e.MergeBaseRef(ctx, dir, ref)
}

// MergeBaseRef returns the commit where HEAD left a full ref, exactly as named.
func (e *Executor) MergeBaseRef(ctx context.Context, dir, ref string) (string, error) {
	out, err := e.read(ctx, dir, "merge-base", "--end-of-options", ref, "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ResolveCommit returns the full id of rev, or fails when it names no commit.
func (e *Executor) ResolveCommit(ctx context.Context, dir, rev string) (string, error) {
	out, err := e.read(ctx, dir, "rev-parse", "--verify", "--quiet", "--end-of-options", rev+"^{commit}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// CommitInfo is one commit Commits lists.
type CommitInfo struct {
	ID      string
	Subject string
	Author  string
	Unix    int64
}

// Commits lists the commits reachable from HEAD and not from since, newest
// first. An empty since lists HEAD's history.
func (e *Executor) Commits(ctx context.Context, dir, since string, limit int) ([]CommitInfo, error) {
	revisions := "HEAD"
	if since != "" {
		revisions = since + "..HEAD"
	}
	// log.showSignature would otherwise write gpg's output between the records.
	out, err := e.read(ctx, dir, "log", "--no-show-signature", "--no-color", "-z", "--format=%H%x00%s%x00%an%x00%ct",
		fmt.Sprintf("--max-count=%d", limit), "--end-of-options", revisions, "--")
	if err != nil {
		return nil, err
	}
	fields := nulFields(out)
	commits := make([]CommitInfo, 0, len(fields)/4)
	for i := 0; i+3 < len(fields); i += 4 {
		// git wrote these digits, so a failed parse can only read as zero.
		unix, _ := strconv.ParseInt(fields[i+3], 10, 64)
		commits = append(commits, CommitInfo{ID: fields[i], Subject: fields[i+1], Author: fields[i+2], Unix: unix})
	}
	return commits, nil
}

// Branches lists the full refs of local and remote branches, most recently
// committed first. A symbolic ref such as origin/HEAD only repeats a branch.
func (e *Executor) Branches(ctx context.Context, dir string, limit int) ([]string, error) {
	out, err := e.read(ctx, dir, "for-each-ref", "--sort=-committerdate", fmt.Sprintf("--count=%d", limit),
		"--format=%(refname)%00%(symref)", "refs/heads", "refs/remotes")
	if err != nil {
		return nil, err
	}
	var refs []string
	for line := range strings.Lines(string(out)) {
		ref, symref, _ := strings.Cut(strings.TrimSuffix(line, "\n"), "\x00")
		if symref == "" {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}

// ChangedFiles lists what differs across diff, sorted by path, plus untracked
// files when diff includes the working tree.
func (e *Executor) ChangedFiles(ctx context.Context, dir string, diff DiffRange) ([]ChangedFile, error) {
	var names, stats bytes.Buffer
	err := e.withScratchIndex(ctx, dir, func(indexEnv string) error {
		nameArgs := slices.Concat([]string{"--name-status", "-z", "--find-renames"}, diff.args, []string{"--"})
		if err := e.readDiff(ctx, dir, indexEnv, &names, nameArgs...); err != nil {
			return err
		}
		statArgs := slices.Concat([]string{"--numstat", "-z", "--find-renames"}, diff.args, []string{"--"})
		return e.readDiff(ctx, dir, indexEnv, &stats, statArgs...)
	})
	if err != nil {
		return nil, err
	}

	files := parseNameStatus(names.Bytes())
	counts := parseNumstat(stats.Bytes())
	for i := range files {
		if count, ok := counts[files[i].Path]; ok {
			files[i].Additions, files[i].Deletions, files[i].Binary = count.additions, count.deletions, count.binary
		}
	}
	if diff.untracked {
		untracked, err := e.read(ctx, dir, "ls-files", "--others", "--exclude-standard", "-z")
		if err != nil {
			return nil, err
		}
		for _, path := range nulFields(untracked) {
			files = append(files, ChangedFile{Path: path, Status: FileStatusUntracked})
		}
	}
	slices.SortFunc(files, func(a, b ChangedFile) int { return strings.Compare(a.Path, b.Path) })
	stamp, err := e.stamper(ctx, dir, diff)
	if err != nil {
		return nil, err
	}
	for i := range files {
		files[i].Stamp = stamp(files[i].Path)
	}
	return files, nil
}

// stamper picks what moves when a file's diff does. A commit's diff never moves,
// and a staged diff follows the index rather than the file on disk.
func (e *Executor) stamper(ctx context.Context, dir string, diff DiffRange) (func(path string) string, error) {
	switch {
	case diff.commit != "":
		return func(string) string { return diff.commit }, nil
	case diff.staged:
		out, err := e.read(ctx, dir, "ls-files", "--stage", "-z")
		if err != nil {
			return nil, err
		}
		blobs := make(map[string]string)
		for _, entry := range nulFields(out) {
			meta, path, ok := strings.Cut(entry, "\t")
			if ok {
				blobs[path] = meta
			}
		}
		return func(path string) string { return blobs[path] }, nil
	default:
		return func(path string) string { return fileStamp(dir, path) }, nil
	}
}

func fileStamp(dir, path string) string {
	info, err := os.Lstat(filepath.Join(dir, path))
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d-%d", info.ModTime().UnixNano(), info.Size())
}

// FileDiff returns the unified diff of one file across diff, or ErrBinaryDiff,
// ErrNoTextDiff or ErrDiffTooLarge. An untracked file that has vanished returns
// an error matching fs.ErrNotExist.
func (e *Executor) FileDiff(ctx context.Context, dir string, diff DiffRange, file ChangedFile) (string, error) {
	if file.Status == FileStatusUntracked {
		return untrackedDiff(dir, file.Path)
	}
	paths := []string{file.Path}
	if file.OldPath != "" {
		paths = []string{file.OldPath, file.Path}
	}
	stdout := &executil.HeadWriter{Max: maxDiffBytes}
	err := e.withScratchIndex(ctx, dir, func(indexEnv string) error {
		args := slices.Concat([]string{"--find-renames"}, diff.args, []string{"--"}, paths)
		return e.readDiff(ctx, dir, indexEnv, stdout, args...)
	})
	if err != nil {
		return "", err
	}
	if stdout.Truncated() {
		return "", ErrDiffTooLarge
	}
	return stdout.String(), nil
}

// FileDiffResult is the outcome of one file in FileDiffs. Err holds what FileDiff
// would return.
type FileDiffResult struct {
	Text string
	Err  error
}

// FileDiffs returns the diff of each file, in order. One git diff covers the
// tracked files, because a process and an index copy for each file costs far more
// than the diff itself. A file that run cannot answer is read on its own: a path
// git quotes, a file no longer changed, or every file when the run fails or passes
// maxBatchDiffBytes. So a failure stays with the one file it belongs to.
func (e *Executor) FileDiffs(ctx context.Context, dir string, diff DiffRange, files []ChangedFile) []FileDiffResult {
	results := make([]FileDiffResult, len(files))
	sections := e.trackedSections(ctx, dir, diff, files)

	var reads errgroup.Group
	reads.SetLimit(maxFileReads)
	for i, file := range files {
		if section, ok := sections[fileHeader(file)]; ok && file.Status != FileStatusUntracked {
			if len(section) > maxDiffBytes {
				results[i].Err = ErrDiffTooLarge
			} else {
				results[i].Text = section
			}
			continue
		}
		reads.Go(func() error {
			results[i].Text, results[i].Err = e.FileDiff(ctx, dir, diff, file)
			return nil
		})
	}
	_ = reads.Wait() // Each read keeps its error in its own result.
	return results
}

// trackedSections runs one git diff over files and splits it by file header. It
// answers nil when the run fails or overflows, which sends every file to its own
// read, where a real failure is reported against that file.
func (e *Executor) trackedSections(ctx context.Context, dir string, diff DiffRange, files []ChangedFile) map[string]string {
	var paths []string
	for _, file := range files {
		if file.Status == FileStatusUntracked {
			continue
		}
		if file.OldPath != "" {
			paths = append(paths, file.OldPath)
		}
		paths = append(paths, file.Path)
	}
	if len(paths) == 0 {
		return nil
	}
	stdout := &executil.HeadWriter{Max: maxBatchDiffBytes}
	err := e.withScratchIndex(ctx, dir, func(indexEnv string) error {
		args := slices.Concat([]string{"--find-renames"}, diff.args, []string{"--"}, paths)
		return e.readDiff(ctx, dir, indexEnv, stdout, args...)
	})
	if err != nil || stdout.Truncated() {
		return nil
	}
	return splitPatch(stdout.String())
}

func fileHeader(file ChangedFile) string {
	from := file.OldPath
	if from == "" {
		from = file.Path
	}
	return "diff --git a/" + from + " b/" + file.Path
}

// patchSectionStart matches what can start a file's section. A line of a hunk
// always starts with a marker, so only a real header can match. During a merge,
// an unmerged path gets a combined diff or the "Unmerged path" line.
var patchSectionStart = regexp.MustCompile(`(?m)^(?:diff --git |diff --cc |diff --combined |\* Unmerged path )`)

// splitPatch keys each file's section by its header line. A file that changes
// between a regular file and a symlink has two sections under one header, a
// deletion and a creation, so sections under one header join.
func splitPatch(patch string) map[string]string {
	sections := make(map[string]string)
	starts := patchSectionStart.FindAllStringIndex(patch, -1)
	for i, start := range starts {
		end := len(patch)
		if i+1 < len(starts) {
			end = starts[i+1][0]
		}
		section := patch[start[0]:end]
		header, _, _ := strings.Cut(section, "\n")
		sections[header] += section
	}
	return sections
}

// untrackedDiff writes the diff itself. git diff --no-index hangs on a FIFO and
// exits 1 for both a success and a vanished file.
func untrackedDiff(dir, path string) (string, error) {
	full := filepath.Join(dir, path)
	info, err := os.Lstat(full)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", ErrNoTextDiff
	}
	// O_NONBLOCK because the path can turn into a FIFO after the Lstat, and
	// opening one would otherwise wait for a writer.
	f, err := os.OpenFile(full, os.O_RDONLY|syscall.O_NONBLOCK, 0) //nolint:gosec // git listed this path inside the checkout
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	if info, err = f.Stat(); err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", ErrNoTextDiff
	}
	content, err := io.ReadAll(io.LimitReader(f, maxDiffBytes+1))
	if err != nil {
		return "", err
	}
	if bytes.IndexByte(content[:min(len(content), binarySniffLength)], 0) >= 0 {
		return "", ErrBinaryDiff
	}
	if len(content) > maxDiffBytes {
		return "", ErrDiffTooLarge
	}
	return additionsPatch(string(content)), nil
}

func additionsPatch(content string) string {
	if content == "" {
		return ""
	}
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	var patch strings.Builder
	fmt.Fprintf(&patch, "@@ -0,0 +1,%d @@\n", len(lines))
	for _, line := range lines {
		patch.WriteString("+" + line + "\n")
	}
	if !strings.HasSuffix(content, "\n") {
		patch.WriteString("\\ No newline at end of file\n")
	}
	return patch.String()
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
