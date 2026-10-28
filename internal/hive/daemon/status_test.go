package daemon_test

import (
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/domain/terminal"
	"github.com/colonyops/hive/internal/hive/daemonclient"
	"github.com/colonyops/hive/internal/hive/hivetest"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/hive/status"
)

func newStatusClient(t *testing.T, pollInterval time.Duration) *daemonclient.Client {
	t.Helper()
	d := runDaemonWith(t, t.TempDir(), daemonOpts{paneSource: hivetest.PaneSource{}, pollInterval: pollInterval})
	client := daemonclient.New(daemonclient.Options{Dialer: daemonclient.UnixDialer(d.socket), Logger: zerolog.Nop()})
	require.Eventually(t, client.StatusAvailable, 5*time.Second, 10*time.Millisecond, "a daemon with a pane source reports status available")
	return client
}

func createActive(t *testing.T, client *daemonclient.Client, name string) *session.Session {
	t.Helper()
	sess, err := client.CreateSession(t.Context(), sessionsvc.CreateOptions{Name: name, Remote: remote, SkipSpawn: true})
	require.NoError(t, err)
	return sess
}

func TestStatusesBeforeTheFirstTickAreLoadingPlaceholders(t *testing.T) {
	client := newStatusClient(t, time.Hour)
	asked := createActive(t, client, "asked")
	createActive(t, client, "other")
	root := status.RootRepoTarget{Path: "/work/site"}

	got := client.FetchBatch(t.Context(), []*session.Session{asked}, []status.RootRepoTarget{root})

	assert.Len(t, got, 2, "only the session and root asked for")
	for _, key := range []string{asked.ID, status.RootStatusKey(root.Path)} {
		require.Contains(t, got, key)
		assert.True(t, got[key].IsLoading, key)
		assert.False(t, got[key].PresenceKnown, "a placeholder must not read as an ended terminal: %s", key)
	}
}

func TestStatusesAfterATickCarryTheFetchedStatus(t *testing.T) {
	client := newStatusClient(t, 20*time.Millisecond)
	sess := createActive(t, client, "agent")

	var got status.TerminalStatus
	require.Eventually(t, func() bool {
		got = client.FetchBatch(t.Context(), []*session.Session{sess}, nil)[sess.ID]
		return !got.IsLoading
	}, 5*time.Second, 20*time.Millisecond)

	assert.True(t, got.PresenceKnown)
	assert.False(t, got.SessionPresent, "the fake pane source lists no panes")
	assert.Equal(t, terminal.StatusMissing, got.Status)
}
