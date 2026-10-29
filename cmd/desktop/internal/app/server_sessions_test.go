package app

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/server"
	"github.com/colonyops/hive/internal/hive/daemonclient"
	"github.com/colonyops/hive/internal/hive/servers"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/platform/promptfile"
)

type serverFixture struct {
	svc       *SessionsService
	box       *daemonclient.Client
	boxMux    *fakeMux
	boxTmux   *fakeSessionTmux
	localTmux *fakeSessionTmux
	jobs      *fakeJobRunner
}

// newServerFixture is a SessionsService whose server "box" is a test daemon
// reached without ssh, and whose server "down" never answers.
func newServerFixture(t *testing.T) serverFixture {
	t.Helper()
	f := serverFixture{boxMux: &fakeMux{}, boxTmux: &fakeSessionTmux{}, localTmux: &fakeSessionTmux{}, jobs: &fakeJobRunner{}}
	socket := runTestDaemonSocket(t, f.boxMux)
	f.box = daemonclient.New(daemonclient.Options{Dialer: daemonclient.UnixDialer(socket), Logger: zerolog.Nop()})
	dial := func(ctx context.Context, cfg config.ServerConfig, stderr io.Writer) (net.Conn, error) {
		if cfg.Name == "down" {
			_, _ = io.WriteString(stderr, "ssh: connect to host down port 22: Connection refused\n")
			return nil, errors.New("exit status 255")
		}
		var d net.Dialer
		return d.DialContext(ctx, "unix", socket)
	}
	fleet := servers.New(zerolog.Nop(), nil, []config.ServerConfig{{Name: "box", SSH: "box"}, {Name: "down", SSH: "down"}}, "test", servers.WithDialer(dial))
	t.Cleanup(fleet.Close)

	local := newHiveHarness(t, engineOptions{})
	f.svc = newSessionsService(SessionsDeps{
		Hive: local.engine, Tmux: f.localTmux, Jobs: f.jobs, Logger: zerolog.Nop(),
		Servers: func() *servers.Fleet { return fleet },
		ServerTmux: func(_ context.Context, name string) (sessionTmux, error) {
			require.Equal(t, "box", name)
			return f.boxTmux, nil
		},
	})
	return f
}

func (f serverFixture) create(t *testing.T, name string) string {
	t.Helper()
	created, err := f.box.CreateSession(t.Context(), sessionsvc.CreateOptions{Name: name, Remote: "https://github.com/acme/site", SkipSpawn: true})
	require.NoError(t, err)
	return created.ID
}

func TestServerSessions_ListEachServerWithoutFailingOnAnOfflineOne(t *testing.T) {
	f := newServerFixture(t)
	id := f.create(t, "review")

	lists := f.svc.ListServerSessions(t.Context())

	require.Len(t, lists, 2)
	assert.Equal(t, "box", lists[0].Server)
	assert.Equal(t, server.StateConnected, lists[0].State)
	require.Len(t, lists[0].Sessions, 1)
	assert.Equal(t, id, lists[0].Sessions[0].ID)
	assert.Equal(t, "down", lists[1].Server)
	assert.Equal(t, server.StateOffline, lists[1].State)
	assert.Contains(t, lists[1].Error, "Connection refused")
	assert.Empty(t, lists[1].Sessions)
}

func TestServerSessions_ActOnTheServerOfAQualifiedID(t *testing.T) {
	f := newServerFixture(t)
	id := f.create(t, "review")
	ref := "box:" + id

	detail, err := f.svc.SessionDetail(t.Context(), ref)
	require.NoError(t, err)
	assert.Equal(t, "review", detail.Name)

	edited, err := f.svc.EditSession(t.Context(), ref, "renamed", []string{"remote"})
	require.NoError(t, err)
	assert.Equal(t, "renamed", edited.Name)
	assert.Equal(t, []string{"remote"}, edited.Tags)
	assert.Equal(t, [][2]string{{"review", "renamed"}}, f.boxTmux.renames, "the server's tmux session follows the rename")
	assert.Empty(t, f.localTmux.renames, "the local tmux is not touched")

	risk, err := f.svc.SessionRisk(t.Context(), ref)
	require.NoError(t, err)
	assert.False(t, risk.UncommittedChanges)

	_, err = f.svc.DeleteSession(t.Context(), ref)
	require.NoError(t, err)
	require.NoError(t, f.jobs.err)
	remaining, err := f.box.ListSessions(t.Context())
	require.NoError(t, err)
	assert.Empty(t, remaining)
}

func TestServerSessions_RefuseWhatNeedsTheLocalCheckout(t *testing.T) {
	f := newServerFixture(t)
	ref := "box:" + f.create(t, "review")

	assert.Equal(t, KindUnavailable, KindOf(f.svc.OpenSessionInEditor(t.Context(), ref)))
	assert.Equal(t, KindUnavailable, KindOf(f.svc.RevealSession(t.Context(), ref)))
	_, err := f.svc.StartTmuxSession(t.Context(), "box:review")
	assert.Equal(t, KindUnavailable, KindOf(err))

	status, err := f.svc.SessionGitStatus(t.Context(), ref)
	require.NoError(t, err)
	assert.False(t, status.Resolved, "a server's checkout is not read from here")

	_, err = f.svc.SessionDetail(t.Context(), "gone:"+ref)
	assert.Equal(t, KindNotFound, KindOf(err))
}

func TestServerSessions_CreateOnAServerRefusesALocalOnlyRequest(t *testing.T) {
	f := newServerFixture(t)

	_, err := f.svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{Server: "box", Workspace: "alerts", Name: "chat"})
	assert.Equal(t, KindInvalid, KindOf(err), "an agent workspace runs on this machine")

	_, err = f.svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{
		Server: "box", Repository: "https://github.com/acme/site", Name: "long",
		Prompt: strings.Repeat("x", promptfile.ThresholdBytes+1),
	})
	assert.Equal(t, KindInvalid, KindOf(err), "a long prompt goes through a local file")

	_, err = f.svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{Server: "gone", Repository: "r", Name: "x"})
	assert.Equal(t, KindNotFound, KindOf(err))
}

func TestServerSessions_LaunchOptionsComeFromTheServer(t *testing.T) {
	f := newServerFixture(t)

	opts, err := f.svc.ServerLaunchOptions(t.Context(), "box")
	require.NoError(t, err)
	assert.Empty(t, opts.Workspaces, "a server session has no agent workspaces")

	_, err = f.svc.ServerLaunchOptions(t.Context(), "down")
	assert.Equal(t, KindUnavailable, KindOf(err))
}
