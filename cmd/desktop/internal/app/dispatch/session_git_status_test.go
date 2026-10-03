package dispatch

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/domain/session"
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

// The read rules (a failed read never claims dirty, a failed branch stands
// for the whole status, coordinates per host) are pinned in hive/gitstatus.
// This pins that the seam carries the failure through as text.
func TestSessionGitStatusCarriesAFailedReadAsText(t *testing.T) {
	t.Parallel()

	manager := NewHiveSessionManager(oneSessionManagement{session: activeSession()}, nil, nil, stubGit{
		branch: "feat/bar", cleanErr: errors.New("git status: exit 128"),
	}, 0)

	got, err := manager.SessionGitStatus(t.Context(), "s1")
	require.NoError(t, err)
	assert.False(t, got.Dirty)
	assert.Equal(t, "git status: exit 128", got.Error)
	assert.True(t, got.Resolved)
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
