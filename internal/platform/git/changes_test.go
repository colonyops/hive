package git_test

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/platform/git"
	"github.com/colonyops/hive/pkg/executil"
)

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return string(bytes.TrimSpace(out))
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

// changedFiles leaves out the stamps, which hold the test's own clock.
func changedFiles(t *testing.T, g *git.Executor, dir, ref string) []git.ChangedFile {
	t.Helper()
	files, err := g.ChangedFiles(t.Context(), dir, git.WorkingTree(ref))
	require.NoError(t, err)
	for i := range files {
		files[i].Stamp = ""
	}
	return files
}

func newRepo(t *testing.T) (string, *git.Executor) {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	runGit(t, dir, "config", "commit.gpgsign", "false")
	writeFile(t, dir, "keep.txt", "one\ntwo\nthree\n")
	writeFile(t, dir, "gone.txt", "bye\n")
	writeFile(t, dir, "moved.txt", "a\nb\nc\nd\ne\nf\n")
	writeFile(t, dir, "image.bin", "\x00\x01")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-qm", "base")
	return dir, git.NewExecutor("git", &executil.RealExecutor{})
}

func TestChangedFilesReportsEachStatus(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)

	writeFile(t, dir, "keep.txt", "one\nchanged\nthree\nfour\n")
	require.NoError(t, os.Remove(filepath.Join(dir, "gone.txt")))
	runGit(t, dir, "mv", "moved.txt", "renamed.txt")
	writeFile(t, dir, "image.bin", "\x00\x02")
	writeFile(t, dir, "staged.txt", "new\n")
	runGit(t, dir, "add", "staged.txt")
	writeFile(t, dir, "with space/and\nnewline.txt", "x\n")

	files := changedFiles(t, g, dir, "HEAD")
	assert.Equal(t, []git.ChangedFile{
		{Path: "gone.txt", Status: git.FileStatusDeleted, Deletions: 1},
		{Path: "image.bin", Status: git.FileStatusModified, Binary: true},
		{Path: "keep.txt", Status: git.FileStatusModified, Additions: 2, Deletions: 1},
		{Path: "renamed.txt", OldPath: "moved.txt", Status: git.FileStatusRenamed},
		{Path: "staged.txt", Status: git.FileStatusAdded, Additions: 1},
		{Path: "with space/and\nnewline.txt", Status: git.FileStatusUntracked},
	}, files)
}

func TestChangedFilesSkipsIgnoredFiles(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	writeFile(t, dir, ".gitignore", "*.log\n")
	writeFile(t, dir, "debug.log", "noise\n")

	files := changedFiles(t, g, dir, "HEAD")
	assert.Equal(t, []git.ChangedFile{{Path: ".gitignore", Status: git.FileStatusUntracked}}, files)
}

func TestChangedFilesShowsNoPhantomRowForATouchedFile(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	later := time.Now().Add(time.Hour)
	require.NoError(t, os.Chtimes(filepath.Join(dir, "keep.txt"), later, later))

	files, err := g.ChangedFiles(t.Context(), dir, git.WorkingTree("HEAD"))
	require.NoError(t, err)
	assert.Empty(t, files)
}

// The read runs beside an agent, which may hold the index at any moment.
func TestChangedFilesWritesNothingUnderDotGit(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	later := time.Now().Add(time.Hour)
	require.NoError(t, os.Chtimes(filepath.Join(dir, "keep.txt"), later, later))
	index := filepath.Join(dir, ".git", "index")
	before, err := os.ReadFile(index)
	require.NoError(t, err)

	_, err = g.ChangedFiles(t.Context(), dir, git.WorkingTree("HEAD"))
	require.NoError(t, err)

	after, err := os.ReadFile(index)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	assert.NoFileExists(t, filepath.Join(dir, ".git", "index.lock"))
}

func TestChangedFilesRunsNoExternalProgramsFromTheRepoConfig(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	marker := filepath.Join(t.TempDir(), "ran")
	script := filepath.Join(t.TempDir(), "hook.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\ntouch "+marker+"\n"), 0o700)) //nolint:gosec // the test runs this script
	runGit(t, dir, "config", "core.fsmonitor", script)
	runGit(t, dir, "config", "diff.external", script)
	writeFile(t, dir, "keep.txt", "changed\n")

	_, err := g.ChangedFiles(t.Context(), dir, git.WorkingTree("HEAD"))
	require.NoError(t, err)
	assert.NoFileExists(t, marker)
}

func TestChangedFilesReadsAWorktree(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	worktree := filepath.Join(t.TempDir(), "wt")
	runGit(t, dir, "worktree", "add", "-q", "-b", "feat/x", worktree)
	writeFile(t, worktree, "keep.txt", "one\ntwo\nthree\nfour\n")

	files := changedFiles(t, g, worktree, "HEAD")
	assert.Equal(t, []git.ChangedFile{{Path: "keep.txt", Status: git.FileStatusModified, Additions: 1}}, files)
}

func TestChangedFilesReportsAgainstAMergeBase(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	runGit(t, dir, "checkout", "-qb", "feat/x")
	writeFile(t, dir, "committed.txt", "one\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-qm", "work")
	writeFile(t, dir, "keep.txt", "one\ntwo\nthree\nfour\n")

	base, err := g.MergeBase(t.Context(), dir, "main")
	require.NoError(t, err)
	files := changedFiles(t, g, dir, base)
	assert.Equal(t, []git.ChangedFile{
		{Path: "committed.txt", Status: git.FileStatusAdded, Additions: 1},
		{Path: "keep.txt", Status: git.FileStatusModified, Additions: 1},
	}, files)
}

func TestChangedFilesReturnsGitsMessageWithNoCommits(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	g := git.NewExecutor("git", &executil.RealExecutor{})

	_, err := g.ChangedFiles(t.Context(), dir, git.WorkingTree("HEAD"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HEAD")
}

// A stale local main must not inflate the diff: a commit already on the remote
// is not this branch's work.
func TestMergeBasePrefersTheRemoteBranch(t *testing.T) {
	t.Parallel()
	upstream, g := newRepo(t)
	dir := t.TempDir()
	runGit(t, dir, "clone", "-q", upstream, ".")
	runGit(t, dir, "config", "commit.gpgsign", "false")

	writeFile(t, upstream, "upstream.txt", "x\n")
	runGit(t, upstream, "add", ".")
	runGit(t, upstream, "commit", "-qm", "upstream work")
	runGit(t, dir, "fetch", "-q", "origin")
	remoteHead := runGit(t, dir, "rev-parse", "origin/main")
	runGit(t, dir, "checkout", "-qb", "feat/x", "origin/main")

	base, err := g.MergeBase(t.Context(), dir, "main")
	require.NoError(t, err)
	assert.Equal(t, remoteHead, base)
}

func TestMergeBaseUsesTheLocalBranchWithNoRemote(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	head := runGit(t, dir, "rev-parse", "HEAD")
	runGit(t, dir, "checkout", "-qb", "feat/x")
	writeFile(t, dir, "new.txt", "x\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-qm", "work")

	base, err := g.MergeBase(t.Context(), dir, "main")
	require.NoError(t, err)
	assert.Equal(t, head, base)
}

func TestMergeBaseFailsWithNoCommonHistory(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	runGit(t, dir, "checkout", "-q", "--orphan", "island")
	runGit(t, dir, "rm", "-rqf", ".")
	writeFile(t, dir, "island.txt", "x\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-qm", "island")

	_, err := g.MergeBase(t.Context(), dir, "main")
	require.Error(t, err)
}

func TestChangedFilesStampMovesWhenAnEditKeepsTheCounts(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	stamp := func() string {
		files, err := g.ChangedFiles(t.Context(), dir, git.WorkingTree("HEAD"))
		require.NoError(t, err)
		require.Len(t, files, 1)
		return files[0].Stamp
	}
	writeFile(t, dir, "keep.txt", "one\nedit\nthree\n")
	first := stamp()
	require.NotEmpty(t, first)

	writeFile(t, dir, "keep.txt", "one\nagain\nthree\n")
	later := time.Now().Add(time.Hour)
	require.NoError(t, os.Chtimes(filepath.Join(dir, "keep.txt"), later, later))
	assert.NotEqual(t, first, stamp())
}

func diffOf(t *testing.T, g *git.Executor, dir string, file git.ChangedFile) string {
	t.Helper()
	text, err := g.FileDiff(t.Context(), dir, git.WorkingTree("HEAD"), file)
	require.NoError(t, err)
	return text
}

func TestFileDiffReadsEachStatus(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	writeFile(t, dir, "keep.txt", "one\nchanged\nthree\n")
	require.NoError(t, os.Remove(filepath.Join(dir, "gone.txt")))
	runGit(t, dir, "mv", "moved.txt", "renamed.txt")
	writeFile(t, dir, "renamed.txt", "a\nb\nc\nd\ne\nedited\n")
	writeFile(t, dir, "[id].txt", "brackets\n")
	runGit(t, dir, "add", "[[]id].txt")

	assert.Contains(t, diffOf(t, g, dir, git.ChangedFile{Path: "keep.txt", Status: git.FileStatusModified}), "+changed\n")
	assert.Contains(t, diffOf(t, g, dir, git.ChangedFile{Path: "gone.txt", Status: git.FileStatusDeleted}), "-bye\n")
	renamed := diffOf(t, g, dir, git.ChangedFile{Path: "renamed.txt", OldPath: "moved.txt", Status: git.FileStatusRenamed})
	assert.Contains(t, renamed, "rename from moved.txt")
	assert.Contains(t, renamed, "+edited\n")
	assert.Contains(t, diffOf(t, g, dir, git.ChangedFile{Path: "[id].txt", Status: git.FileStatusAdded}), "+brackets\n")
}

func TestFileDiffIgnoresColorAndPrefixConfig(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	runGit(t, dir, "config", "color.ui", "always")
	runGit(t, dir, "config", "diff.noprefix", "true")
	writeFile(t, dir, "keep.txt", "one\nchanged\nthree\n")

	text := diffOf(t, g, dir, git.ChangedFile{Path: "keep.txt", Status: git.FileStatusModified})
	assert.NotContains(t, text, "\x1b[")
	assert.Contains(t, text, "@@ -1,3 +1,3 @@")
}

func TestFileDiffRunsNoExternalProgramsAndWritesNoIndex(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	marker := filepath.Join(t.TempDir(), "ran")
	script := filepath.Join(t.TempDir(), "hook.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\ntouch "+marker+"\n"), 0o700)) //nolint:gosec // the test runs this script
	runGit(t, dir, "config", "core.fsmonitor", script)
	runGit(t, dir, "config", "diff.external", script)
	writeFile(t, dir, "keep.txt", "one\nchanged\nthree\n")
	later := time.Now().Add(time.Hour)
	require.NoError(t, os.Chtimes(filepath.Join(dir, "gone.txt"), later, later))
	index := filepath.Join(dir, ".git", "index")
	before, err := os.ReadFile(index)
	require.NoError(t, err)

	diffOf(t, g, dir, git.ChangedFile{Path: "keep.txt", Status: git.FileStatusModified})
	diffOf(t, g, dir, git.ChangedFile{Path: "gone.txt", Status: git.FileStatusModified})

	assert.NoFileExists(t, marker)
	after, err := os.ReadFile(index)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestFileDiffStopsAtTheByteCap(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	writeFile(t, dir, "keep.txt", strings.Repeat("x", 3<<20)+"\n")

	_, err := g.FileDiff(t.Context(), dir, git.WorkingTree("HEAD"), git.ChangedFile{Path: "keep.txt", Status: git.FileStatusModified})
	require.ErrorIs(t, err, git.ErrDiffTooLarge)
}

func TestFileDiffBuildsAnUntrackedFile(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	untracked := func(path string) (string, error) {
		return g.FileDiff(t.Context(), dir, git.WorkingTree("HEAD"), git.ChangedFile{Path: path, Status: git.FileStatusUntracked})
	}

	writeFile(t, dir, "two.txt", "a\nb\n")
	text, err := untracked("two.txt")
	require.NoError(t, err)
	assert.Equal(t, "@@ -0,0 +1,2 @@\n+a\n+b\n", text)

	writeFile(t, dir, "open.txt", "a")
	text, err = untracked("open.txt")
	require.NoError(t, err)
	assert.Equal(t, "@@ -0,0 +1,1 @@\n+a\n\\ No newline at end of file\n", text)

	writeFile(t, dir, "empty.txt", "")
	text, err = untracked("empty.txt")
	require.NoError(t, err)
	assert.Empty(t, text)

	writeFile(t, dir, "data.bin", "\x00\x01")
	_, err = untracked("data.bin")
	require.ErrorIs(t, err, git.ErrBinaryDiff)

	writeFile(t, dir, "big.txt", strings.Repeat("x", 3<<20))
	_, err = untracked("big.txt")
	require.ErrorIs(t, err, git.ErrDiffTooLarge)

	_, err = untracked("vanished.txt")
	require.ErrorIs(t, err, fs.ErrNotExist)
}

func TestFileDiffGivesNoTextForAnUntrackedNonFile(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	require.NoError(t, syscall.Mkfifo(filepath.Join(dir, "pipe"), 0o600))
	require.NoError(t, os.Symlink("keep.txt", filepath.Join(dir, "link")))
	inner := filepath.Join(dir, "inner")
	require.NoError(t, os.Mkdir(inner, 0o750))
	runGit(t, inner, "init", "-q")

	files, err := g.ChangedFiles(t.Context(), dir, git.WorkingTree("HEAD"))
	require.NoError(t, err)
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	assert.Contains(t, paths, "inner/")

	for _, path := range []string{"pipe", "link", "inner/"} {
		_, err := g.FileDiff(t.Context(), dir, git.WorkingTree("HEAD"), git.ChangedFile{Path: path, Status: git.FileStatusUntracked})
		require.ErrorIs(t, err, git.ErrNoTextDiff, path)
	}
}

func TestDiffStatsCountsCommittedAndUncommittedWork(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	runGit(t, dir, "remote", "add", "origin", dir)
	runGit(t, dir, "fetch", "-q", "origin")
	runGit(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	runGit(t, dir, "checkout", "-qb", "feat/x")
	writeFile(t, dir, "committed.txt", "one\ntwo\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-qm", "work")
	writeFile(t, dir, "keep.txt", "one\ntwo\nthree\nfour\n")

	additions, deletions, err := g.DiffStats(t.Context(), dir)
	require.NoError(t, err)
	assert.Equal(t, 3, additions)
	assert.Equal(t, 0, deletions)
}

func TestDiffStatsFallsBackToHEADWithNoDefaultBranch(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	writeFile(t, dir, "keep.txt", "one\ntwo\nthree\nfour\n")

	additions, _, err := g.DiffStats(t.Context(), dir)
	require.NoError(t, err)
	assert.Equal(t, 1, additions)
}

func TestDiffStatsReturnsAFailedDiff(t *testing.T) {
	t.Parallel()
	_, _, err := git.NewExecutor("git", &executil.RealExecutor{}).DiffStats(t.Context(), t.TempDir())
	require.Error(t, err)
}

func TestStagedAndUnstagedSplitTheWorkingTree(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	writeFile(t, dir, "keep.txt", "one\ntwo\nthree\nstaged\n")
	runGit(t, dir, "add", "keep.txt")
	writeFile(t, dir, "gone.txt", "bye\nunstaged\n")
	writeFile(t, dir, "fresh.txt", "new\n")

	staged, err := g.ChangedFiles(t.Context(), dir, git.Staged())
	require.NoError(t, err)
	require.Len(t, staged, 1)
	assert.Equal(t, "keep.txt", staged[0].Path)
	text, err := g.FileDiff(t.Context(), dir, git.Staged(), staged[0])
	require.NoError(t, err)
	assert.Contains(t, text, "+staged")

	unstaged, err := g.ChangedFiles(t.Context(), dir, git.Unstaged())
	require.NoError(t, err)
	paths := make([]string, 0, len(unstaged))
	for _, file := range unstaged {
		paths = append(paths, file.Path)
	}
	assert.Equal(t, []string{"fresh.txt", "gone.txt"}, paths)
	text, err = g.FileDiff(t.Context(), dir, git.Unstaged(), unstaged[1])
	require.NoError(t, err)
	assert.Contains(t, text, "+unstaged")
}

func TestStagedStampFollowsTheIndexNotTheFile(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	writeFile(t, dir, "keep.txt", "one\ntwo\nthree\nfour\n")
	runGit(t, dir, "add", "keep.txt")
	before, err := g.ChangedFiles(t.Context(), dir, git.Staged())
	require.NoError(t, err)
	require.NotEmpty(t, before[0].Stamp)

	writeFile(t, dir, "keep.txt", "edited on disk only\n")
	after, err := g.ChangedFiles(t.Context(), dir, git.Staged())
	require.NoError(t, err)
	assert.Equal(t, before[0].Stamp, after[0].Stamp)

	writeFile(t, dir, "keep.txt", "one\ntwo\nthree\nfive\n")
	runGit(t, dir, "add", "keep.txt")
	restaged, err := g.ChangedFiles(t.Context(), dir, git.Staged())
	require.NoError(t, err)
	assert.NotEqual(t, before[0].Stamp, restaged[0].Stamp)
}

func TestCommitShowsOneCommitAndIgnoresTheWorkingTree(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	writeFile(t, dir, "keep.txt", "one\ntwo\nthree\ncommitted\n")
	runGit(t, dir, "commit", "-qam", "second")
	writeFile(t, dir, "keep.txt", "dirty\n")
	writeFile(t, dir, "fresh.txt", "new\n")

	head, err := g.ResolveCommit(t.Context(), dir, "HEAD")
	require.NoError(t, err)
	diff, err := g.Commit(t.Context(), dir, head)
	require.NoError(t, err)
	files, err := g.ChangedFiles(t.Context(), dir, diff)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "keep.txt", files[0].Path)
	assert.Equal(t, head, files[0].Stamp)
	text, err := g.FileDiff(t.Context(), dir, diff, files[0])
	require.NoError(t, err)
	assert.Contains(t, text, "+committed")
	assert.NotContains(t, text, "dirty")
}

func TestCommitShowsARootCommitAsAdditions(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)

	root, err := g.ResolveCommit(t.Context(), dir, "HEAD")
	require.NoError(t, err)
	diff, err := g.Commit(t.Context(), dir, root)
	require.NoError(t, err)
	files, err := g.ChangedFiles(t.Context(), dir, diff)
	require.NoError(t, err)
	assert.Len(t, files, 4)
	assert.Equal(t, git.FileStatusAdded, files[0].Status)
}

func TestResolveCommitRefusesWhatIsNotACommit(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)

	for _, rev := range []string{"no-such-commit", "--output=/tmp/x", "HEAD:keep.txt"} {
		_, err := g.ResolveCommit(t.Context(), dir, rev)
		require.Error(t, err, rev)
	}
}

func TestCommitsListsWhatHEADAddedNewestFirst(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	base := runGit(t, dir, "rev-parse", "HEAD")
	writeFile(t, dir, "keep.txt", "first\n")
	runGit(t, dir, "commit", "-qam", "first")
	writeFile(t, dir, "keep.txt", "second\n")
	runGit(t, dir, "commit", "-qam", "second subject")

	commits, err := g.Commits(t.Context(), dir, base, 10)
	require.NoError(t, err)
	require.Len(t, commits, 2)
	assert.Equal(t, "second subject", commits[0].Subject)
	assert.Equal(t, "first", commits[1].Subject)
	assert.Equal(t, "t", commits[0].Author)
	assert.Equal(t, runGit(t, dir, "rev-parse", "HEAD"), commits[0].ID)
	assert.Positive(t, commits[0].Unix)

	history, err := g.Commits(t.Context(), dir, "", 2)
	require.NoError(t, err)
	assert.Len(t, history, 2)
}

func TestBranchesListsFullRefsWithoutSymbolicOnes(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	runGit(t, dir, "branch", "feat/x")
	head := runGit(t, dir, "rev-parse", "HEAD")
	runGit(t, dir, "update-ref", "refs/remotes/origin/main", head)
	runGit(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")

	refs, err := g.Branches(t.Context(), dir, 100)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"refs/heads/main", "refs/heads/feat/x", "refs/remotes/origin/main"}, refs)
}

func TestMergeBaseRefUsesTheRefItIsGiven(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	base := runGit(t, dir, "rev-parse", "HEAD")
	runGit(t, dir, "branch", "release")
	writeFile(t, dir, "keep.txt", "ahead\n")
	runGit(t, dir, "commit", "-qam", "ahead")

	got, err := g.MergeBaseRef(t.Context(), dir, "refs/heads/release")
	require.NoError(t, err)
	assert.Equal(t, base, got)
}
