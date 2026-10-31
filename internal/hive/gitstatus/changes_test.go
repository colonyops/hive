package gitstatus_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/gitstatus"
	"github.com/colonyops/hive/internal/platform/git"
)

func TestReadChangesPicksTheRefForTheBase(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name         string
		git          stubGit
		base         gitstatus.DiffBase
		against      string
		commit       string
		wantDiff     git.DiffRange
		wantAgainst  string
		wantCommit   string
		wantBase     gitstatus.DiffBase
		wantBranchOK bool
	}{
		{
			name:         "branch reads against the merge base",
			git:          stubGit{branch: "feat/x", defaultBranch: "main", mergeBase: "abc"},
			base:         gitstatus.DiffBaseBranch,
			wantDiff:     git.WorkingTree("abc"),
			wantBase:     gitstatus.DiffBaseBranch,
			wantBranchOK: true,
		},
		{
			name:         "uncommitted reads against HEAD and still offers branch",
			git:          stubGit{branch: "feat/x", defaultBranch: "main", mergeBase: "abc"},
			base:         gitstatus.DiffBaseUncommitted,
			wantDiff:     git.WorkingTree("HEAD"),
			wantBase:     gitstatus.DiffBaseUncommitted,
			wantBranchOK: true,
		},
		{
			name:     "the default branch has only uncommitted",
			git:      stubGit{branch: "main", defaultBranch: "main", mergeBase: "abc"},
			base:     gitstatus.DiffBaseBranch,
			wantDiff: git.WorkingTree("HEAD"),
			wantBase: gitstatus.DiffBaseUncommitted,
		},
		{
			name:     "an unknown default branch falls back",
			git:      stubGit{branch: "feat/x", defaultBranchErr: errors.New("no origin/HEAD")},
			base:     gitstatus.DiffBaseBranch,
			wantDiff: git.WorkingTree("HEAD"),
			wantBase: gitstatus.DiffBaseUncommitted,
		},
		{
			name:         "staged reads the index",
			git:          stubGit{branch: "feat/x", defaultBranch: "main", mergeBase: "abc"},
			base:         gitstatus.DiffBaseStaged,
			wantDiff:     git.Staged(),
			wantBase:     gitstatus.DiffBaseStaged,
			wantBranchOK: true,
		},
		{
			name:         "unstaged reads the working tree against the index",
			git:          stubGit{branch: "feat/x", defaultBranch: "main", mergeBase: "abc"},
			base:         gitstatus.DiffBaseUnstaged,
			wantDiff:     git.Unstaged(),
			wantBase:     gitstatus.DiffBaseUnstaged,
			wantBranchOK: true,
		},
		{
			name:         "a commit read with no commit shows HEAD",
			git:          stubGit{branch: "feat/x", defaultBranch: "main", mergeBase: "abc"},
			base:         gitstatus.DiffBaseCommit,
			wantDiff:     git.WorkingTree("commit:full-HEAD"),
			wantBase:     gitstatus.DiffBaseCommit,
			wantCommit:   "full-HEAD",
			wantBranchOK: true,
		},
		{
			name:         "a commit read shows the commit it names",
			git:          stubGit{branch: "feat/x", defaultBranch: "main", mergeBase: "abc"},
			base:         gitstatus.DiffBaseCommit,
			commit:       "c1",
			wantDiff:     git.WorkingTree("commit:full-c1"),
			wantBase:     gitstatus.DiffBaseCommit,
			wantCommit:   "full-c1",
			wantBranchOK: true,
		},
		{
			name:         "a commit that no longer resolves gives way to HEAD",
			git:          stubGit{branch: "feat/x", defaultBranch: "main", mergeBase: "abc", commitErr: errors.New("gone")},
			base:         gitstatus.DiffBaseCommit,
			commit:       "c1",
			wantDiff:     git.WorkingTree("commit:full-HEAD"),
			wantBase:     gitstatus.DiffBaseCommit,
			wantCommit:   "full-HEAD",
			wantBranchOK: true,
		},
		{
			name:         "a chosen ref sets the merge base",
			git:          stubGit{branch: "feat/x", defaultBranch: "main", mergeBase: "abc", mergeBaseRef: "def"},
			base:         gitstatus.DiffBaseBranch,
			against:      "refs/heads/release",
			wantDiff:     git.WorkingTree("def"),
			wantBase:     gitstatus.DiffBaseBranch,
			wantAgainst:  "refs/heads/release",
			wantBranchOK: true,
		},
		{
			name:         "a chosen ref on the default branch can still compare",
			git:          stubGit{branch: "main", defaultBranch: "main", mergeBaseRef: "def"},
			base:         gitstatus.DiffBaseBranch,
			against:      "refs/remotes/origin/release",
			wantDiff:     git.WorkingTree("def"),
			wantBase:     gitstatus.DiffBaseBranch,
			wantAgainst:  "refs/remotes/origin/release",
			wantBranchOK: true,
		},
		{
			name:         "a chosen ref that no longer resolves gives way to the default branch",
			git:          stubGit{branch: "feat/x", defaultBranch: "main", mergeBase: "abc", mergeBaseRefErr: errors.New("gone")},
			base:         gitstatus.DiffBaseBranch,
			against:      "refs/heads/release",
			wantDiff:     git.WorkingTree("abc"),
			wantBase:     gitstatus.DiffBaseBranch,
			wantBranchOK: true,
		},
		{
			name:         "a chosen value that is not a branch ref is ignored",
			git:          stubGit{branch: "feat/x", defaultBranch: "main", mergeBase: "abc", mergeBaseRef: "def"},
			base:         gitstatus.DiffBaseBranch,
			against:      "--output=/tmp/x",
			wantDiff:     git.WorkingTree("abc"),
			wantBase:     gitstatus.DiffBaseBranch,
			wantBranchOK: true,
		},
		{
			name:     "a failed merge base falls back",
			git:      stubGit{branch: "feat/x", defaultBranch: "main", mergeBaseErr: errors.New("shallow")},
			base:     gitstatus.DiffBaseBranch,
			wantDiff: git.WorkingTree("HEAD"),
			wantBase: gitstatus.DiffBaseUncommitted,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var diff git.DiffRange
			tt.git.onDiff = func(d git.DiffRange) { diff = d }

			got, err := gitstatus.NewService(zerolog.Nop(), tt.git, 1).ReadChanges(t.Context(), activeSession(), gitstatus.DiffTarget{Base: tt.base, Against: tt.against, Commit: tt.commit})
			require.NoError(t, err)
			assert.True(t, got.Resolved)
			assert.Equal(t, tt.wantDiff, diff)
			assert.Equal(t, tt.wantBase, got.Base)
			assert.Equal(t, tt.wantAgainst, got.Against)
			assert.Equal(t, tt.wantCommit, got.Commit)
			assert.Equal(t, tt.wantBranchOK, got.BranchAvailable)
		})
	}
}

func TestReadChangesListsTheCommitsTheBranchAdded(t *testing.T) {
	t.Parallel()

	var since string
	commits := []git.CommitInfo{{ID: "c2", Subject: "second"}, {ID: "c1", Subject: "first"}}
	svc := gitstatus.NewService(zerolog.Nop(), stubGit{
		branch: "feat/x", defaultBranch: "main", mergeBase: "abc", commits: commits,
		onSince: func(s string) { since = s },
	}, 1)

	got, err := svc.ReadChanges(t.Context(), activeSession(), gitstatus.DiffTarget{Base: gitstatus.DiffBaseUncommitted})
	require.NoError(t, err)
	assert.Equal(t, "abc", since)
	assert.Equal(t, commits, got.Commits)
}

func TestReadChangesSurvivesAFailedCommitList(t *testing.T) {
	t.Parallel()

	svc := gitstatus.NewService(zerolog.Nop(), stubGit{
		branch: "feat/x", defaultBranch: "main", mergeBase: "abc", commitsErr: errors.New("no log"),
	}, 1)

	got, err := svc.ReadChanges(t.Context(), activeSession(), gitstatus.DiffTarget{Base: gitstatus.DiffBaseBranch})
	require.NoError(t, err)
	assert.True(t, got.Resolved)
	assert.Empty(t, got.Commits)
}

func TestReadBranchesIsEmptyWithNoCheckout(t *testing.T) {
	t.Parallel()

	svc := gitstatus.NewService(zerolog.Nop(), stubGit{branches: []string{"refs/heads/main"}}, 1)

	got, err := svc.ReadBranches(t.Context(), session.Session{ID: "r", State: session.StateRecycled})
	require.NoError(t, err)
	assert.Empty(t, got)

	got, err = svc.ReadBranches(t.Context(), activeSession())
	require.NoError(t, err)
	assert.Equal(t, []string{"refs/heads/main"}, got)
}

func TestReadChangesIsEmptyWithNoCheckout(t *testing.T) {
	t.Parallel()

	svc := gitstatus.NewService(zerolog.Nop(), stubGit{branchErr: errors.New("must not run")}, 1)
	for _, sess := range []session.Session{
		{ID: "recycled", Path: "/tmp/x", State: session.StateRecycled},
		{ID: "no path", State: session.StateActive},
	} {
		got, err := svc.ReadChanges(t.Context(), sess, gitstatus.DiffTarget{Base: gitstatus.DiffBaseBranch})
		require.NoError(t, err)
		assert.Equal(t, gitstatus.Changes{}, got)
	}
}

func TestReadChangesReturnsAFailedRead(t *testing.T) {
	t.Parallel()

	svc := gitstatus.NewService(zerolog.Nop(), stubGit{branch: "main", defaultBranch: "main", filesErr: errors.New("bad HEAD")}, 1)
	_, err := svc.ReadChanges(t.Context(), activeSession(), gitstatus.DiffTarget{Base: gitstatus.DiffBaseBranch})
	require.ErrorContains(t, err, "bad HEAD")
}

func TestReadChangesStopsAtTheFileCap(t *testing.T) {
	t.Parallel()

	files := make([]git.ChangedFile, 1001)
	for i := range files {
		files[i] = git.ChangedFile{Path: fmt.Sprintf("f%d", i), Status: git.FileStatusUntracked}
	}
	svc := gitstatus.NewService(zerolog.Nop(), stubGit{branch: "main", defaultBranch: "main", files: files}, 1)

	got, err := svc.ReadChanges(t.Context(), activeSession(), gitstatus.DiffTarget{Base: gitstatus.DiffBaseBranch})
	require.NoError(t, err)
	assert.Len(t, got.Files, 1000)
	assert.True(t, got.Truncated)
}

func TestReadChangesAppliesTheReadTimeout(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		svc := gitstatus.NewService(zerolog.Nop(), stubGit{branch: "main", defaultBranch: "main", block: true}, 1)
		_, err := svc.ReadChanges(t.Context(), activeSession(), gitstatus.DiffTarget{Base: gitstatus.DiffBaseBranch})
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})
}

func TestReadFileDiffsReportsEachFileOnItsOwn(t *testing.T) {
	t.Parallel()

	svc := gitstatus.NewService(zerolog.Nop(), stubGit{
		branch: "main", defaultBranch: "main",
		files: []git.ChangedFile{
			{Path: "ok.go", Status: git.FileStatusModified, Additions: 1},
			{Path: "image.png", Status: git.FileStatusModified, Binary: true},
			{Path: "lock.json", Status: git.FileStatusModified, Additions: 15000, Deletions: 5001},
			{Path: "data.bin", Status: git.FileStatusUntracked},
			{Path: "pipe", Status: git.FileStatusUntracked},
			{Path: "raced.txt", Status: git.FileStatusUntracked},
			{Path: "big.txt", Status: git.FileStatusUntracked},
			{Path: "broken.go", Status: git.FileStatusModified},
		},
		diffs: map[string]string{"ok.go": "@@ -1 +1 @@\n+x\n"},
		diffErrs: map[string]error{
			"data.bin":  git.ErrBinaryDiff,
			"pipe":      git.ErrNoTextDiff,
			"raced.txt": fs.ErrNotExist,
			"big.txt":   git.ErrDiffTooLarge,
			"broken.go": errors.New("exit 128"),
		},
	}, 1)

	got, err := svc.ReadFileDiffs(t.Context(), activeSession(), gitstatus.DiffTarget{Base: gitstatus.DiffBaseUncommitted},
		[]string{"ok.go", "image.png", "lock.json", "data.bin", "pipe", "raced.txt", "big.txt", "broken.go", "../../etc/passwd"})
	require.NoError(t, err)
	assert.Equal(t, []gitstatus.FileDiff{
		{Path: "ok.go", State: gitstatus.FileDiffStateText, Text: "@@ -1 +1 @@\n+x\n"},
		{Path: "image.png", State: gitstatus.FileDiffStateBinary},
		{Path: "lock.json", State: gitstatus.FileDiffStateTruncated},
		{Path: "data.bin", State: gitstatus.FileDiffStateBinary},
		{Path: "pipe", State: gitstatus.FileDiffStateUnavailable},
		{Path: "raced.txt", State: gitstatus.FileDiffStateGone},
		{Path: "big.txt", State: gitstatus.FileDiffStateTruncated},
		{Path: "broken.go", State: gitstatus.FileDiffStateError, Error: "exit 128"},
		{Path: "../../etc/passwd", State: gitstatus.FileDiffStateGone},
	}, got)
}

func TestReadFileDiffsClipsLongLinesAndHugePatches(t *testing.T) {
	t.Parallel()

	long := "+" + strings.Repeat("x", 5000)
	svc := gitstatus.NewService(zerolog.Nop(), stubGit{
		branch: "main", defaultBranch: "main",
		files: []git.ChangedFile{{Path: "long.go", Status: git.FileStatusModified}, {Path: "spread.go", Status: git.FileStatusModified}, {Path: "latin.txt", Status: git.FileStatusModified}},
		diffs: map[string]string{
			"long.go":   long + "\n",
			"spread.go": strings.Repeat(" context\n", 40001),
			"latin.txt": "+caf\xe9\n",
		},
	}, 1)

	got, err := svc.ReadFileDiffs(t.Context(), activeSession(), gitstatus.DiffTarget{Base: gitstatus.DiffBaseBranch}, []string{"long.go", "spread.go", "latin.txt"})
	require.NoError(t, err)
	assert.Len(t, strings.TrimSuffix(got[0].Text, "\n"), 4000)
	assert.Equal(t, gitstatus.FileDiffStateTruncated, got[1].State)
	assert.Empty(t, got[1].Text)
	assert.Equal(t, "+caf\uFFFD\n", got[2].Text)
}

func TestReadFileDiffsReportsEveryPathGoneWithNoCheckout(t *testing.T) {
	t.Parallel()

	svc := gitstatus.NewService(zerolog.Nop(), stubGit{branchErr: errors.New("must not run")}, 1)
	got, err := svc.ReadFileDiffs(t.Context(), session.Session{ID: "r", State: session.StateRecycled}, gitstatus.DiffTarget{Base: gitstatus.DiffBaseBranch}, []string{"a"})
	require.NoError(t, err)
	assert.Equal(t, []gitstatus.FileDiff{{Path: "a", State: gitstatus.FileDiffStateGone}}, got)
}

func TestReadFileDiffsReturnsAFailedList(t *testing.T) {
	t.Parallel()

	svc := gitstatus.NewService(zerolog.Nop(), stubGit{branch: "main", defaultBranch: "main", filesErr: errors.New("bad HEAD")}, 1)
	_, err := svc.ReadFileDiffs(t.Context(), activeSession(), gitstatus.DiffTarget{Base: gitstatus.DiffBaseBranch}, []string{"a"})
	require.ErrorContains(t, err, "bad HEAD")
}

func TestReadFileDiffsReadsABatchInOneCall(t *testing.T) {
	t.Parallel()

	var batches atomic.Int32
	svc := gitstatus.NewService(zerolog.Nop(), stubGit{
		branch: "main", defaultBranch: "main", diffBatch: &batches,
		files: []git.ChangedFile{
			{Path: "a.go", Status: git.FileStatusModified},
			{Path: "b.go", Status: git.FileStatusModified},
			{Path: "image.png", Status: git.FileStatusModified, Binary: true},
		},
		diffs: map[string]string{"a.go": "+a\n", "b.go": "+b\n"},
	}, 1)

	got, err := svc.ReadFileDiffs(t.Context(), activeSession(), gitstatus.DiffTarget{Base: gitstatus.DiffBaseUncommitted}, []string{"b.go", "image.png", "a.go"})
	require.NoError(t, err)
	assert.Equal(t, []gitstatus.FileDiff{{Path: "b.go", State: gitstatus.FileDiffStateText, Text: "+b\n"}, {Path: "image.png", State: gitstatus.FileDiffStateBinary}, {Path: "a.go", State: gitstatus.FileDiffStateText, Text: "+a\n"}}, got)
	assert.Equal(t, int32(1), batches.Load())
}

func TestReadFileDiffsReportsAFileThatStoppedDifferingAsGone(t *testing.T) {
	t.Parallel()

	svc := gitstatus.NewService(zerolog.Nop(), stubGit{
		branch: "main", defaultBranch: "main",
		files: []git.ChangedFile{
			{Path: "reverted.go", Status: git.FileStatusModified},
			{Path: "empty.txt", Status: git.FileStatusUntracked},
			{Path: "moved.go", OldPath: "old.go", Status: git.FileStatusRenamed},
		},
	}, 1)

	got, err := svc.ReadFileDiffs(t.Context(), activeSession(), gitstatus.DiffTarget{Base: gitstatus.DiffBaseUncommitted}, []string{"reverted.go", "empty.txt", "moved.go"})
	require.NoError(t, err)
	assert.Equal(t, []gitstatus.FileDiff{{Path: "reverted.go", State: gitstatus.FileDiffStateGone}, {Path: "empty.txt", State: gitstatus.FileDiffStateText}, {Path: "moved.go", State: gitstatus.FileDiffStateText}}, got)
}

func TestReadFileDiffsReusesARecentListForTheSameTarget(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var lists atomic.Int32
		svc := gitstatus.NewService(zerolog.Nop(), stubGit{
			branch: "main", defaultBranch: "main",
			files:  []git.ChangedFile{{Path: "a.go", Status: git.FileStatusModified}},
			diffs:  map[string]string{"a.go": "+a\n"},
			onDiff: func(git.DiffRange) { lists.Add(1) },
		}, 1)
		staged := gitstatus.DiffTarget{Base: gitstatus.DiffBaseStaged}
		unstaged := gitstatus.DiffTarget{Base: gitstatus.DiffBaseUnstaged}
		read := func(target gitstatus.DiffTarget) {
			_, err := svc.ReadFileDiffs(t.Context(), activeSession(), target, []string{"a.go"})
			require.NoError(t, err)
		}

		_, err := svc.ReadChanges(t.Context(), activeSession(), staged)
		require.NoError(t, err)
		read(staged)
		read(staged)
		assert.Equal(t, int32(1), lists.Load(), "diff reads reuse the list")

		read(unstaged)
		assert.Equal(t, int32(2), lists.Load(), "another target has its own list")

		_, err = svc.ReadChanges(t.Context(), activeSession(), staged)
		require.NoError(t, err)
		assert.Equal(t, int32(3), lists.Load(), "a list read always reads")

		time.Sleep(11 * time.Second)
		read(staged)
		assert.Equal(t, int32(4), lists.Load(), "an old list is read again")
	})
}
