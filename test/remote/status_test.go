//go:build remote

package remote

import (
	"fmt"
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/domain/terminal"
	"github.com/colonyops/hive/internal/hive/gitstatus"
	"github.com/colonyops/hive/internal/hive/status"
	"github.com/colonyops/hive/internal/platform/git"
)

func TestSessionAPI_ReportsTmuxPresenceFromTheServer(t *testing.T) {
	created := serverA.CreateSession(t, "watched")
	api := serverA.API(t)
	sess, err := api.GetSession(t.Context(), created.ID)
	require.NoError(t, err)

	// The daemon client learns that status is available from its first Hello,
	// which StatusAvailable starts in the background.
	require.Eventually(t, api.StatusAvailable, 10*time.Second, 200*time.Millisecond)

	statusOf := func() status.TerminalStatus {
		return api.FetchBatch(t.Context(), []*session.Session{&sess}, nil)[sess.ID]
	}
	require.Eventually(t, func() bool {
		st := statusOf()
		return st.PresenceKnown && st.SessionPresent
	}, 20*time.Second, 500*time.Millisecond, "the server's tmux session shows as present")

	serverA.Shell(t, "tmux kill-session -t "+shellQuote(sess.Name)).Ok(t)

	assert.Eventually(t, func() bool {
		st := statusOf()
		return st.PresenceKnown && !st.SessionPresent
	}, 20*time.Second, 500*time.Millisecond, "the killed tmux session shows as missing")
}

func TestSessionAPI_ReportsTheAgentStatusFromTheServer(t *testing.T) {
	created := serverA.CreateSession(t, "agent-status")
	api := serverA.API(t)
	sess, err := api.GetSession(t.Context(), created.ID)
	require.NoError(t, err)
	require.Eventually(t, api.StatusAvailable, 10*time.Second, 200*time.Millisecond)

	var last status.TerminalStatus
	waitFor := func(want terminal.Status) {
		t.Helper()
		ok := assert.Eventually(t, func() bool {
			last = api.FetchBatch(t.Context(), []*session.Session{&sess}, nil)[sess.ID]
			return last.Status == want
		}, 20*time.Second, 500*time.Millisecond, "status %s", want)
		if !ok {
			t.Logf("last status: %+v", last)
		}
	}

	waitFor(terminal.StatusReady)
	assert.Equal(t, "claude", last.Tool)

	stateFile := "/tmp/fake-claude/" + path.Base(sess.Path)
	for _, step := range []struct {
		snapshot string
		want     terminal.Status
	}{
		{snapshot: "approval-permission-dialog", want: terminal.StatusApproval},
		{snapshot: "working-spinner", want: terminal.StatusActive},
	} {
		serverA.Shell(t, fmt.Sprintf("mkdir -p /tmp/fake-claude && echo %s > %s", shellQuote(step.snapshot), shellQuote(stateFile))).Ok(t)
		waitFor(step.want)
	}
}

func TestSessionAPI_ReadsChangesFromTheServer(t *testing.T) {
	created := serverA.CreateSession(t, "changes")
	serverA.Shell(t, "echo changed > "+path.Join(created.Path, "only-on-the-server.txt")).Ok(t)

	changes, err := serverA.API(t).SessionChanges(t.Context(), created.ID, gitstatus.DiffTarget{Base: gitstatus.DiffBaseUncommitted})
	require.NoError(t, err)
	require.True(t, changes.Resolved)
	require.Len(t, changes.Files, 1)
	assert.Equal(t, "only-on-the-server.txt", changes.Files[0].Path)
	assert.Equal(t, git.FileStatusUntracked, changes.Files[0].Status)
}

func TestSessionAPI_ReadsAFileDiffFromTheServer(t *testing.T) {
	created := serverA.CreateSession(t, "diffs")
	serverA.Shell(t, "echo changed > "+path.Join(created.Path, "only-on-the-server.txt")).Ok(t)

	diffs, err := serverA.API(t).SessionFileDiffs(t.Context(), created.ID, gitstatus.DiffTarget{Base: gitstatus.DiffBaseUncommitted},
		[]string{"only-on-the-server.txt", "not-in-the-list.txt"})
	require.NoError(t, err)
	assert.Equal(t, []gitstatus.FileDiff{
		{Path: "only-on-the-server.txt", State: gitstatus.FileDiffStateText, Text: "@@ -0,0 +1,1 @@\n+changed\n"},
		{Path: "not-in-the-list.txt", State: gitstatus.FileDiffStateGone},
	}, diffs)
}

func TestSessionAPI_ReadsAChosenBranchAndCommitFromTheServer(t *testing.T) {
	created := serverA.CreateSession(t, "targets")
	const commit = "git -c user.name=t -c user.email=t@t -c commit.gpgsign=false commit -qm"
	serverA.Shell(t, "cd "+created.Path+" && git branch chosen && "+
		"echo older > older.txt && git add . && "+commit+" older && "+
		"echo newer > newer.txt && git add . && "+commit+" newer").Ok(t)
	api := serverA.API(t)

	const against = "refs/heads/chosen"
	branch := gitstatus.DiffTarget{Base: gitstatus.DiffBaseBranch, Against: against}
	changes, err := api.SessionChanges(t.Context(), created.ID, branch)
	require.NoError(t, err)
	assert.Equal(t, against, changes.Against)
	assert.ElementsMatch(t, []string{"older.txt", "newer.txt"}, []string{changes.Files[0].Path, changes.Files[1].Path})
	diffs, err := api.SessionFileDiffs(t.Context(), created.ID, branch, []string{"older.txt"})
	require.NoError(t, err)
	assert.Contains(t, diffs[0].Text, "+older")

	require.Len(t, changes.Commits, 2)
	older := gitstatus.DiffTarget{Base: gitstatus.DiffBaseCommit, Commit: changes.Commits[1].ID}
	changes, err = api.SessionChanges(t.Context(), created.ID, older)
	require.NoError(t, err)
	assert.Equal(t, older.Commit, changes.Commit)
	require.Len(t, changes.Files, 1)
	assert.Equal(t, "older.txt", changes.Files[0].Path)
	diffs, err = api.SessionFileDiffs(t.Context(), created.ID, older, []string{"older.txt", "newer.txt"})
	require.NoError(t, err)
	assert.Equal(t, gitstatus.FileDiffStateText, diffs[0].State)
	assert.Equal(t, gitstatus.FileDiffStateGone, diffs[1].State)
}

func TestSessionAPI_ReadsACommitAndTheBranchesFromTheServer(t *testing.T) {
	created := serverA.CreateSession(t, "commits")
	serverA.Shell(t, "cd "+created.Path+" && echo committed > on-the-server.txt && git add . && "+
		"git -c user.name=t -c user.email=t@t -c commit.gpgsign=false commit -qm 'server commit' && echo dirty > dirty.txt").Ok(t)

	changes, err := serverA.API(t).SessionChanges(t.Context(), created.ID, gitstatus.DiffTarget{Base: gitstatus.DiffBaseCommit})
	require.NoError(t, err)
	assert.Equal(t, gitstatus.DiffBaseCommit, changes.Base)
	assert.NotEmpty(t, changes.Commit)
	require.NotEmpty(t, changes.Commits)
	assert.Equal(t, "server commit", changes.Commits[0].Subject)
	require.Len(t, changes.Files, 1)
	assert.Equal(t, "on-the-server.txt", changes.Files[0].Path)

	diffs, err := serverA.API(t).SessionFileDiffs(t.Context(), created.ID,
		gitstatus.DiffTarget{Base: gitstatus.DiffBaseCommit, Commit: changes.Commit}, []string{"on-the-server.txt"})
	require.NoError(t, err)
	require.Len(t, diffs, 1)
	assert.Contains(t, diffs[0].Text, "+committed")

	refs, err := serverA.API(t).SessionBranches(t.Context(), created.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, refs)
}
