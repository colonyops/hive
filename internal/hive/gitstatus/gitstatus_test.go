package gitstatus_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/gitstatus"
	"github.com/colonyops/hive/internal/platform/git"
	"github.com/colonyops/hive/pkg/executil"
)

// stubGit answers each read from a field, with a matching error field so a
// single failing read can be isolated.
type stubGit struct {
	branch      string
	branchErr   error
	clean       bool
	cleanErr    error
	unpushed    bool
	unpushedErr error
	additions   int
	deletions   int
	diffErr     error
	unpushedHit *atomic.Int32
}

func (g stubGit) Branch(context.Context, string) (string, error) { return g.branch, g.branchErr }
func (g stubGit) IsClean(context.Context, string) (bool, error)  { return g.clean, g.cleanErr }

func (g stubGit) HasUnpushedCommits(context.Context, string) (bool, error) {
	if g.unpushedHit != nil {
		g.unpushedHit.Add(1)
	}
	return g.unpushed, g.unpushedErr
}

func (g stubGit) DiffStats(context.Context, string) (int, int, error) {
	return g.additions, g.deletions, g.diffErr
}

func activeSession() session.Session {
	return session.Session{ID: "s1", Path: "/tmp/review-81", Remote: "git@github.com:acme/site.git", State: session.StateActive}
}

func TestReadSessionReportsTheCheckoutAndItsRemoteCoordinates(t *testing.T) {
	t.Parallel()

	svc := gitstatus.NewService(stubGit{branch: "feat/bar", clean: false, unpushed: true, additions: 42, deletions: 7}, 1)

	got := svc.ReadSession(t.Context(), activeSession())
	assert.Equal(t, gitstatus.Status{
		Path: "/tmp/review-81", Branch: "feat/bar", Dirty: true, Unpushed: true,
		Additions: 42, Deletions: 7, Host: "github.com", Owner: "acme", Repo: "site", Resolved: true,
	}, got)
}

// The delete confirmation assumes dirty when git fails, because over-warning is
// the safe side of that decision. A status badge has no such safe side: an
// unread checkout must not render as one with uncommitted work.
func TestReadSessionReportsAFailedReadInsteadOfAssumingDirty(t *testing.T) {
	t.Parallel()

	svc := gitstatus.NewService(stubGit{branch: "feat/bar", cleanErr: errors.New("git status: exit 128")}, 1)

	got := svc.ReadSession(t.Context(), activeSession())
	assert.False(t, got.Dirty)
	require.Error(t, got.Error())
	assert.Equal(t, "git status: exit 128", got.Error().Error())
	assert.True(t, got.Resolved, "the branch resolved, so the rest of the bar still has something to show")
}

// Branch is the read every other read needs a working checkout for, so its
// failure stands for the whole status rather than leaving a half-filled one.
func TestReadSessionReportsNothingResolvedWhenBranchFails(t *testing.T) {
	t.Parallel()

	svc := gitstatus.NewService(stubGit{branchErr: errors.New("git branch: not a repository")}, 1)

	got := svc.ReadSession(t.Context(), activeSession())
	assert.False(t, got.Resolved)
	assert.Empty(t, got.Branch)
	assert.EqualError(t, got.Error(), "git branch: not a repository")
}

// A branch with no upstream fails the unpushed read on every poll. It must not
// displace a failure that says the checkout itself could not be read.
func TestReadSessionPrefersACheckoutFailureOverTheUnpushedOne(t *testing.T) {
	t.Parallel()

	svc := gitstatus.NewService(stubGit{
		branch:      "feat/bar",
		unpushedErr: errors.New("no upstream"),
		diffErr:     errors.New("git diff: exit 128"),
	}, 1)

	got := svc.ReadSession(t.Context(), activeSession())
	require.EqualError(t, got.Error(), "git diff: exit 128")
	assert.EqualError(t, got.UnpushedErr, "no upstream")
}

// A recycled session has no checkout left, which is not a failure: there is
// simply nothing to read.
func TestReadSessionIsEmptyForASessionWithNoCheckout(t *testing.T) {
	t.Parallel()

	recycled := activeSession()
	recycled.State = session.StateRecycled
	svc := gitstatus.NewService(stubGit{branch: "feat/bar"}, 1)

	assert.Equal(t, gitstatus.Status{}, svc.ReadSession(t.Context(), recycled))
}

// Which forge serves a host is the caller's question, so any hosted remote
// reports its coordinates, with the host lowered.
func TestReadSessionReportsCoordinatesForAnyHostedRemote(t *testing.T) {
	t.Parallel()

	elsewhere := activeSession()
	elsewhere.Remote = "git@Gitea.Example.Test:acme/site.git"
	svc := gitstatus.NewService(stubGit{branch: "feat/bar"}, 1)

	got := svc.ReadSession(t.Context(), elsewhere)
	assert.Equal(t, "feat/bar", got.Branch)
	assert.Equal(t, "gitea.example.test", got.Host)
	assert.Equal(t, "acme", got.Owner)
	assert.Equal(t, "site", got.Repo)
}

// owner/repo parsing is only the last two path segments, which a local path
// also has.
func TestReadSessionLeavesCoordinatesEmptyForAHostlessRemote(t *testing.T) {
	t.Parallel()

	local := activeSession()
	local.Remote = "/srv/git/acme/site.git"
	svc := gitstatus.NewService(stubGit{branch: "feat/bar"}, 1)

	got := svc.ReadSession(t.Context(), local)
	assert.Equal(t, "feat/bar", got.Branch)
	assert.Empty(t, got.Host)
	assert.Empty(t, got.Owner)
	assert.Empty(t, got.Repo)
}

// The stubs pin the read logic; this pins the git invocations behind it. It
// also covers a session before its first push: no upstream and no
// origin/<default>, so "unpushed" is unanswerable while everything else reads.
func TestReadSessionAgainstARealCheckout(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	runGit("init", "--initial-branch=main", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\n"), 0o600))
	runGit("add", "a.txt")
	runGit("commit", "-qm", "first")
	runGit("checkout", "-qb", "feat/bar")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\ntwo\nthree\n"), 0o600))

	svc := gitstatus.NewService(git.NewExecutor("git", &executil.RealExecutor{}), 1)
	got := svc.ReadSession(t.Context(), session.Session{
		ID: "s1", Path: dir, Remote: "https://github.com/acme/site", State: session.StateActive,
	})

	assert.True(t, got.Resolved)
	assert.Equal(t, "feat/bar", got.Branch)
	assert.True(t, got.Dirty)
	// No remote at all, so DiffStats falls back to the working tree against
	// HEAD rather than against a default branch it cannot resolve.
	assert.Equal(t, 2, got.Additions)
	assert.Equal(t, 0, got.Deletions)
	assert.Equal(t, "github.com", got.Host)
	assert.Equal(t, "acme", got.Owner)
	assert.Equal(t, "site", got.Repo)
	require.NoError(t, got.Err)
	require.Error(t, got.UnpushedErr, "said out loud rather than reported as nothing to push")
	assert.False(t, got.Unpushed)
}

func TestReadBatchReadsEveryPathWithoutTheUnpushedCheckByDefault(t *testing.T) {
	t.Parallel()

	var unpushedCalls atomic.Int32
	svc := gitstatus.NewService(stubGit{branch: "main", clean: true, additions: 3, unpushedHit: &unpushedCalls}, 2)

	got := svc.ReadBatch(t.Context(), []string{"/a", "/b", "/c"}, gitstatus.Options{})
	require.Len(t, got, 3)
	for _, path := range []string{"/a", "/b", "/c"} {
		assert.Equal(t, gitstatus.Status{Path: path, Branch: "main", Additions: 3, Resolved: true}, got[path])
	}
	assert.Zero(t, unpushedCalls.Load())
}

func TestReadBatchKeepsAFailedPathsError(t *testing.T) {
	t.Parallel()

	svc := gitstatus.NewService(stubGit{branchErr: errors.New("not a repository")}, 0)

	got := svc.ReadBatch(t.Context(), []string{"/gone"}, gitstatus.Options{})
	require.EqualError(t, got["/gone"].Err, "not a repository")
	assert.False(t, got["/gone"].Resolved)
}
