package daemon_test

import (
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/daemonclient"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
)

const remote = "https://github.com/acme/site"

func newClient(t *testing.T) *daemonclient.Client {
	t.Helper()
	d := runDaemon(t, t.TempDir(), false)
	return daemonclient.New(daemonclient.Options{Dialer: daemonclient.UnixDialer(d.socket), Logger: zerolog.Nop()})
}

func TestClientDrivesTheSessionAPIThroughTheDaemon(t *testing.T) {
	client := newClient(t)
	ctx := t.Context()

	var progress strings.Builder
	created, err := client.CreateSession(ctx, sessionsvc.CreateOptions{Name: "review", Remote: remote, SkipSpawn: true, Progress: &progress})
	require.NoError(t, err)
	assert.Equal(t, "review", created.Name)
	assert.Contains(t, progress.String(), "Session created: review\n")

	listed, err := client.ListSessions(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, created.ID, listed[0].ID)
	assert.True(t, created.CreatedAt.Equal(listed[0].CreatedAt))

	require.NoError(t, client.RenameSession(ctx, created.ID, "renamed"))
	require.NoError(t, client.SetSessionGroup(ctx, created.ID, "team"))
	got, err := client.GetSession(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "renamed", got.Name)

	risk, err := client.CheckSessionRisk(ctx, created.ID)
	require.NoError(t, err)
	assert.IsType(t, sessionsvc.Risk{}, risk)

	options, err := client.SessionLaunchOptions(ctx)
	require.NoError(t, err)
	assert.NotNil(t, options)

	windowed, err := client.CreateSessionWithWindows(ctx, sessionsvc.NewSessionRequest{Name: "spawned", Remote: remote},
		[]multiplexer.WindowSpec{{Name: "agent", Command: "true"}})
	require.NoError(t, err)
	assert.Equal(t, "spawned", windowed.Name)

	require.NoError(t, client.DeleteSession(ctx, created.ID))
	_, err = client.GetSession(ctx, created.ID)
	require.ErrorIs(t, err, session.ErrNotFound)

	info, err := client.Hello(ctx)
	require.NoError(t, err)
	assert.Equal(t, "test", info.Version)
	health, err := client.Health(ctx)
	require.NoError(t, err)
	assert.True(t, health.DBOK)
	assert.Equal(t, 1, health.SessionsByState[string(session.StateActive)])
}

func TestClientGetsNotFoundForAMissingSession(t *testing.T) {
	_, err := newClient(t).GetSession(t.Context(), "missing")
	require.ErrorIs(t, err, session.ErrNotFound)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestCreateWithoutARemoteIsInvalid(t *testing.T) {
	client := newClient(t)
	_, err := client.CreateSession(t.Context(), sessionsvc.CreateOptions{Name: "x", SkipSpawn: true})
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	_, err = client.CreateSessionWithWindows(t.Context(), sessionsvc.NewSessionRequest{Name: "y"}, nil)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestConcurrentCreatesKeepTheirOwnProgress(t *testing.T) {
	client := newClient(t)
	names := []string{"alpha", "bravo", "charlie", "delta"}
	progress := make([]strings.Builder, len(names))

	var wg sync.WaitGroup
	for i, name := range names {
		wg.Go(func() {
			_, err := client.CreateSession(t.Context(), sessionsvc.CreateOptions{Name: name, Remote: remote, SkipSpawn: true, Progress: &progress[i]})
			assert.NoError(t, err)
		})
	}
	wg.Wait()

	for i, name := range names {
		out := progress[i].String()
		assert.Contains(t, out, "Session created: "+name+"\n")
		for _, other := range names {
			if other != name {
				assert.NotContains(t, out, "Session created: "+other+"\n")
			}
		}
	}
}
