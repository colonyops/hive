//go:build remote

package remote

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestSessionAPI_GitStatusFailsWhenTheServerStops(t *testing.T) {
	created := serverB.CreateSession(t, "git-status-stop")
	api := serverB.API(t)
	_, err := api.GitStatuses(t.Context(), []string{created.ID})
	require.NoError(t, err)

	serverB.Stop(t)
	started := time.Now()
	_, err = api.GitStatuses(t.Context(), []string{created.ID})

	require.Error(t, err)
	assert.Less(t, time.Since(started), 30*time.Second, "a read has a time limit")
	serverB.Start(t)
}
