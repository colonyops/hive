package git_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
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

	files, err := g.ChangedFiles(t.Context(), dir, "HEAD")
	require.NoError(t, err)
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

	files, err := g.ChangedFiles(t.Context(), dir, "HEAD")
	require.NoError(t, err)
	assert.Equal(t, []git.ChangedFile{{Path: ".gitignore", Status: git.FileStatusUntracked}}, files)
}

func TestChangedFilesShowsNoPhantomRowForATouchedFile(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	later := time.Now().Add(time.Hour)
	require.NoError(t, os.Chtimes(filepath.Join(dir, "keep.txt"), later, later))

	files, err := g.ChangedFiles(t.Context(), dir, "HEAD")
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

	_, err = g.ChangedFiles(t.Context(), dir, "HEAD")
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

	_, err := g.ChangedFiles(t.Context(), dir, "HEAD")
	require.NoError(t, err)
	assert.NoFileExists(t, marker)
}

func TestChangedFilesReadsAWorktree(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	worktree := filepath.Join(t.TempDir(), "wt")
	runGit(t, dir, "worktree", "add", "-q", "-b", "feat/x", worktree)
	writeFile(t, worktree, "keep.txt", "one\ntwo\nthree\nfour\n")

	files, err := g.ChangedFiles(t.Context(), worktree, "HEAD")
	require.NoError(t, err)
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
	files, err := g.ChangedFiles(t.Context(), dir, base)
	require.NoError(t, err)
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

	_, err := g.ChangedFiles(t.Context(), dir, "HEAD")
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
