package dispatch

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/git"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/pkg/executil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type oneSessionManagement struct {
	SessionManagement
	session session.Session
}

func (m oneSessionManagement) GetSession(context.Context, string) (session.Session, error) {
	return m.session, nil
}

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
}

func (g stubGit) Branch(context.Context, string) (string, error) { return g.branch, g.branchErr }
func (g stubGit) IsClean(context.Context, string) (bool, error)  { return g.clean, g.cleanErr }

func (g stubGit) HasUnpushedCommits(context.Context, string) (bool, error) {
	return g.unpushed, g.unpushedErr
}

func (g stubGit) DiffStats(context.Context, string) (int, int, error) {
	return g.additions, g.deletions, g.diffErr
}

func activeSession() session.Session {
	return session.Session{ID: "s1", Path: "/tmp/review-81", Remote: "git@github.com:acme/site.git", State: session.StateActive}
}

func TestSessionGitStatusReportsTheCheckoutAndItsRemoteCoordinates(t *testing.T) {
	t.Parallel()

	manager := NewHiveSessionManager(oneSessionManagement{session: activeSession()}, nil, nil, stubGit{
		branch: "feat/bar", clean: false, unpushed: true, additions: 42, deletions: 7,
	}, 0)

	got, err := manager.SessionGitStatus(t.Context(), "s1")
	require.NoError(t, err)
	assert.Equal(t, SessionGitStatus{
		Path: "/tmp/review-81", Branch: "feat/bar", Dirty: true, Unpushed: true,
		Additions: 42, Deletions: 7, Host: "github.com", Owner: "acme", Repo: "site", Resolved: true,
	}, got)
}

// The delete confirmation assumes dirty when git fails, because over-warning is
// the safe side of that decision. A status badge has no such safe side: an
// unread checkout must not render as one with uncommitted work.
func TestSessionGitStatusReportsAFailedReadInsteadOfAssumingDirty(t *testing.T) {
	t.Parallel()

	manager := NewHiveSessionManager(oneSessionManagement{session: activeSession()}, nil, nil, stubGit{
		branch: "feat/bar", cleanErr: errors.New("git status: exit 128"),
	}, 0)

	got, err := manager.SessionGitStatus(t.Context(), "s1")
	require.NoError(t, err)
	assert.False(t, got.Dirty)
	assert.Equal(t, "git status: exit 128", got.Error)
	assert.True(t, got.Resolved, "the branch resolved, so the rest of the bar still has something to show")
}

// Branch is the read every other read needs a working checkout for, so its
// failure stands for the whole status rather than leaving a half-filled one.
func TestSessionGitStatusReportsNothingResolvedWhenBranchFails(t *testing.T) {
	t.Parallel()

	manager := NewHiveSessionManager(oneSessionManagement{session: activeSession()}, nil, nil, stubGit{
		branchErr: errors.New("git branch: not a repository"),
	}, 0)

	got, err := manager.SessionGitStatus(t.Context(), "s1")
	require.NoError(t, err)
	assert.False(t, got.Resolved)
	assert.Empty(t, got.Branch)
	assert.Equal(t, "git branch: not a repository", got.Error)
}

// A recycled session has no checkout left, which is not a failure — there is
// simply nothing to read.
func TestSessionGitStatusIsEmptyForASessionWithNoCheckout(t *testing.T) {
	t.Parallel()

	recycled := activeSession()
	recycled.State = session.StateRecycled
	manager := NewHiveSessionManager(oneSessionManagement{session: recycled}, nil, nil, stubGit{branch: "feat/bar"}, 0)

	got, err := manager.SessionGitStatus(t.Context(), "s1")
	require.NoError(t, err)
	assert.Equal(t, SessionGitStatus{}, got)
}

// A remote on another forge reports its coordinates like any other. Which
// forge serves that host is the app layer's question, so answering it here
// would decide it twice.
func TestSessionGitStatusReportsCoordinatesForAnyHostedRemote(t *testing.T) {
	t.Parallel()

	elsewhere := activeSession()
	// Cased, because the host is what the forge match and the pull-request
	// cache are keyed on and a remote may be written any way.
	elsewhere.Remote = "git@Gitea.Example.Test:acme/site.git"
	manager := NewHiveSessionManager(oneSessionManagement{session: elsewhere}, nil, nil, stubGit{branch: "feat/bar"}, 0)

	got, err := manager.SessionGitStatus(t.Context(), "s1")
	require.NoError(t, err)
	assert.Equal(t, "feat/bar", got.Branch)
	assert.Equal(t, "gitea.example.test", got.Host)
	assert.Equal(t, "acme", got.Owner)
	assert.Equal(t, "site", got.Repo)
}

// A remote naming no host has no coordinates to read: owner/repo parsing is
// only the last two path segments, which a local path also has.
func TestSessionGitStatusLeavesCoordinatesEmptyForAHostlessRemote(t *testing.T) {
	t.Parallel()

	local := activeSession()
	local.Remote = "/srv/git/acme/site.git"
	manager := NewHiveSessionManager(oneSessionManagement{session: local}, nil, nil, stubGit{branch: "feat/bar"}, 0)

	got, err := manager.SessionGitStatus(t.Context(), "s1")
	require.NoError(t, err)
	assert.Equal(t, "feat/bar", got.Branch)
	assert.Empty(t, got.Host)
	assert.Empty(t, got.Owner)
	assert.Empty(t, got.Repo)
}

// The stubs above pin the seam's shape; this pins the git invocations behind
// it, which is the half a stub cannot check. It also covers the shape a
// session commonly has before its first push: no upstream and no
// origin/<default>, so "unpushed" is genuinely unanswerable while everything
// else still reads.
func TestSessionGitStatusAgainstARealCheckout(t *testing.T) {
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

	manager := NewHiveSessionManager(
		oneSessionManagement{session: session.Session{
			ID: "s1", Path: dir, Remote: "https://github.com/acme/site", State: session.StateActive,
		}},
		nil,
		nil,
		git.NewExecutor("git", &executil.RealExecutor{}),
		0,
	)

	got, err := manager.SessionGitStatus(t.Context(), "s1")
	require.NoError(t, err)
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
	// Said out loud rather than reported as "nothing to push", which is the
	// claim the zero value would otherwise make.
	assert.NotEmpty(t, got.Error)
	assert.False(t, got.Unpushed)
}

// rebindingManagement rebinds the manager from inside GetSession, which is the
// interleaving Rebind's contract is about: a reload landing between two reads
// of one call.
type rebindingManagement struct {
	SessionManagement
	session session.Session
	rebind  func()
}

func (m rebindingManagement) GetSession(context.Context, string) (session.Session, error) {
	m.rebind()
	return m.session, nil
}

func TestSessionGitStatusReadsOneSnapshotAcrossARebind(t *testing.T) {
	t.Parallel()

	var manager *HiveSessionManager
	manager = NewHiveSessionManager(rebindingManagement{session: activeSession(), rebind: func() {
		manager.Rebind(oneSessionManagement{session: activeSession()}, nil, stubGit{branch: "after"}, 0)
	}}, nil, nil, stubGit{branch: "before"}, 0)

	got, err := manager.SessionGitStatus(t.Context(), "s1")
	require.NoError(t, err)
	assert.Equal(t, "before", got.Branch, "the git executor paired with the session service that answered")

	got, err = manager.SessionGitStatus(t.Context(), "s1")
	require.NoError(t, err)
	assert.Equal(t, "after", got.Branch, "the next call sees the rebound config")
}

func TestSessionStatusesReportTheRebindedPollInterval(t *testing.T) {
	t.Parallel()

	manager := NewHiveSessionManager(oneSessionManagement{}, nil, nil, stubGit{}, 5*time.Second)
	before, err := manager.SessionStatuses(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 5*time.Second, before.PollInterval)

	manager.Rebind(oneSessionManagement{}, nil, stubGit{}, 9*time.Second)

	after, err := manager.SessionStatuses(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 9*time.Second, after.PollInterval)
}
