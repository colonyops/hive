//go:build remote

package remote

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/status"
)

// The fixture agent is bash, which the status rules do not detect as an agent,
// so this test checks only that the tmux session on the server is present.
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
