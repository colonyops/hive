//go:build remote

package remote

import (
	"fmt"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/domain/terminal"
	"github.com/colonyops/hive/internal/hive/status"
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

func TestSessionAPI_PreviewsThePaneOnTheServer(t *testing.T) {
	created := serverA.CreateSession(t, "preview")
	api := serverA.API(t)
	sess, err := api.GetSession(t.Context(), created.ID)
	require.NoError(t, err)
	require.Eventually(t, api.StatusAvailable, 10*time.Second, 200*time.Millisecond)

	assert.Eventually(t, func() bool {
		st := api.FetchBatch(t.Context(), []*session.Session{&sess}, nil)[sess.ID]
		return strings.Contains(st.PaneContent, "Completed the requested change")
	}, 20*time.Second, 500*time.Millisecond, "the preview carries the fake agent's screen")
}
