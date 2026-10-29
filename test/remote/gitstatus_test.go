//go:build remote

package remote

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/session"
)

func TestSessionAPI_ReadsGitStatusOnTheServer(t *testing.T) {
	created := serverA.CreateSession(t, "git-status")
	api := serverA.API(t)

	statuses, err := api.GitStatuses(t.Context(), []string{created.ID})
	require.NoError(t, err)
	clean := statuses[created.ID]
	require.True(t, clean.Resolved, "read error: %v", clean.Err)
	assert.Equal(t, "main", clean.Branch)
	assert.False(t, clean.Dirty)

	serverA.Shell(t, "touch "+shellQuote(created.Path+"/new-file")).Ok(t)

	// The daemon shares a read between clients for a few seconds.
	assert.Eventually(t, func() bool {
		statuses, err := api.GitStatuses(t.Context(), []string{created.ID})
		return err == nil && statuses[created.ID].Dirty
	}, 15*time.Second, time.Second, "the untracked file makes the checkout dirty")
	serverA.Shell(t, "rm -f "+shellQuote(created.Path+"/new-file")).Ok(t)
}

func TestSessionAPI_PreviewsThePaneOnTheServer(t *testing.T) {
	created := serverA.CreateSession(t, "preview")
	api := serverA.API(t)
	sess, err := api.GetSession(t.Context(), created.ID)
	require.NoError(t, err)
	require.Eventually(t, api.StatusAvailable, 10*time.Second, 200*time.Millisecond)

	assert.Eventually(t, func() bool {
		st := api.FetchBatch(t.Context(), []*session.Session{&sess}, nil)[sess.ID]
		return st.PaneContent != ""
	}, 20*time.Second, 500*time.Millisecond, "the preview carries the fake agent's screen")
}
