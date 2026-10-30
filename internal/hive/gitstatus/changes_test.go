package gitstatus_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

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
		wantRef      string
		wantBase     gitstatus.DiffBase
		wantBranchOK bool
	}{
		{
			name:         "branch reads against the merge base",
			git:          stubGit{branch: "feat/x", defaultBranch: "main", mergeBase: "abc"},
			base:         gitstatus.DiffBaseBranch,
			wantRef:      "abc",
			wantBase:     gitstatus.DiffBaseBranch,
			wantBranchOK: true,
		},
		{
			name:         "uncommitted reads against HEAD and still offers branch",
			git:          stubGit{branch: "feat/x", defaultBranch: "main", mergeBase: "abc"},
			base:         gitstatus.DiffBaseUncommitted,
			wantRef:      "HEAD",
			wantBase:     gitstatus.DiffBaseUncommitted,
			wantBranchOK: true,
		},
		{
			name:     "the default branch has only uncommitted",
			git:      stubGit{branch: "main", defaultBranch: "main", mergeBase: "abc"},
			base:     gitstatus.DiffBaseBranch,
			wantRef:  "HEAD",
			wantBase: gitstatus.DiffBaseUncommitted,
		},
		{
			name:     "an unknown default branch falls back",
			git:      stubGit{branch: "feat/x", defaultBranchErr: errors.New("no origin/HEAD")},
			base:     gitstatus.DiffBaseBranch,
			wantRef:  "HEAD",
			wantBase: gitstatus.DiffBaseUncommitted,
		},
		{
			name:     "a failed merge base falls back",
			git:      stubGit{branch: "feat/x", defaultBranch: "main", mergeBaseErr: errors.New("shallow")},
			base:     gitstatus.DiffBaseBranch,
			wantRef:  "HEAD",
			wantBase: gitstatus.DiffBaseUncommitted,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var ref string
			tt.git.onRef = func(r string) { ref = r }

			got, err := gitstatus.NewService(zerolog.Nop(), tt.git, 1).ReadChanges(t.Context(), activeSession(), tt.base)
			require.NoError(t, err)
			assert.True(t, got.Resolved)
			assert.Equal(t, tt.wantRef, ref)
			assert.Equal(t, tt.wantBase, got.Base)
			assert.Equal(t, tt.wantBranchOK, got.BranchAvailable)
		})
	}
}

func TestReadChangesIsEmptyWithNoCheckout(t *testing.T) {
	t.Parallel()

	svc := gitstatus.NewService(zerolog.Nop(), stubGit{branchErr: errors.New("must not run")}, 1)
	for _, sess := range []session.Session{
		{ID: "recycled", Path: "/tmp/x", State: session.StateRecycled},
		{ID: "no path", State: session.StateActive},
	} {
		got, err := svc.ReadChanges(t.Context(), sess, gitstatus.DiffBaseBranch)
		require.NoError(t, err)
		assert.Equal(t, gitstatus.Changes{}, got)
	}
}

func TestReadChangesReturnsAFailedRead(t *testing.T) {
	t.Parallel()

	svc := gitstatus.NewService(zerolog.Nop(), stubGit{branch: "main", defaultBranch: "main", filesErr: errors.New("bad HEAD")}, 1)
	_, err := svc.ReadChanges(t.Context(), activeSession(), gitstatus.DiffBaseBranch)
	require.ErrorContains(t, err, "bad HEAD")
}

func TestReadChangesStopsAtTheFileCap(t *testing.T) {
	t.Parallel()

	files := make([]git.ChangedFile, 1001)
	for i := range files {
		files[i] = git.ChangedFile{Path: fmt.Sprintf("f%d", i), Status: git.FileStatusUntracked}
	}
	svc := gitstatus.NewService(zerolog.Nop(), stubGit{branch: "main", defaultBranch: "main", files: files}, 1)

	got, err := svc.ReadChanges(t.Context(), activeSession(), gitstatus.DiffBaseBranch)
	require.NoError(t, err)
	assert.Len(t, got.Files, 1000)
	assert.True(t, got.Truncated)
}

func TestReadChangesAppliesTheReadTimeout(t *testing.T) {
	t.Parallel()

	svc := gitstatus.NewService(zerolog.Nop(), stubGit{branch: "main", defaultBranch: "main", block: true}, 1)
	_, err := svc.ReadChanges(t.Context(), activeSession(), gitstatus.DiffBaseBranch)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
