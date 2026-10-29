package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/server"
	"github.com/colonyops/hive/internal/hive/daemon"
	"github.com/colonyops/hive/internal/hive/daemon/wire"
	"github.com/colonyops/hive/internal/hive/daemonclient"
	"github.com/colonyops/hive/internal/hive/hivetest"
	"github.com/colonyops/hive/internal/hive/servers"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/platform/promptfile"
	tmuxcc "github.com/colonyops/hive/internal/platform/tmux/control"
)

type serverFixture struct {
	svc       *SessionsService
	box       *daemonclient.Client
	boxMux    *fakeMux
	boxTmux   *fakeSessionTmux
	boxWins   *fakeWindowSource
	localTmux *fakeSessionTmux
	jobs      *fakeJobRunner
}

// newServerFixture is a SessionsService whose server "box" is a test daemon
// reached without ssh, and whose server "down" never answers.
func newServerFixture(t *testing.T) serverFixture {
	t.Helper()
	f := serverFixture{boxMux: &fakeMux{}, boxTmux: &fakeSessionTmux{}, boxWins: &fakeWindowSource{}, localTmux: &fakeSessionTmux{}, jobs: &fakeJobRunner{}}
	socket := runTestDaemonSocket(t, f.boxMux, func(opts *daemon.Options) {
		opts.Ports.PaneSource = hivetest.PaneSource{}
		opts.Config.Agents.Default = "claude"
		opts.Config.Agents.Profiles = map[string]config.AgentProfile{"claude": {}, "aider": {}}
	})
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
		ServerTmux: func(context.Context, string) (serverSessionTmux, error) {
			return fakeServerTmux{f.boxTmux, f.boxWins}, nil
		},
	})
	return f
}

type fakeServerTmux struct {
	*fakeSessionTmux
	*fakeWindowSource
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

	git, err := f.svc.SessionGitStatus(t.Context(), ref)
	require.NoError(t, err)
	assert.True(t, git.Resolved, "the server reads its own checkout")
	assert.Equal(t, "site", git.Repo)

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
	assert.Equal(t, []string{"claude", "aider"}, opts.Agents)
	assert.Equal(t, "claude", opts.DefaultAgent)
	assert.Empty(t, opts.Workspaces, "a server session has no agent workspaces")

	_, err = f.svc.ServerLaunchOptions(t.Context(), "down")
	assert.Equal(t, KindUnavailable, KindOf(err))
}

func TestServerSessions_StatusPollAsksOnlyTheServersThatAnswered(t *testing.T) {
	f := newServerFixture(t)
	id := f.create(t, "review")
	f.boxWins.results = map[string][]tmuxcc.IndexedWindow{"review": {{ID: "@1", Index: "0", Name: "claude"}}}

	assert.Empty(t, f.svc.ServerSessionStatuses(t.Context()))
	assert.Empty(t, f.boxWins.seen, "no server is polled before the first list")

	f.svc.ListServerSessions(t.Context())
	require.Eventually(t, func() bool { return len(f.svc.ServerSessionStatuses(t.Context())) > 0 }, 5*time.Second, 10*time.Millisecond)

	items := f.svc.ServerSessionStatuses(t.Context())
	require.Len(t, items, 1, "the offline server is not asked")
	assert.Equal(t, "box:"+id, items[0].SessionID)
	assert.True(t, items[0].Running)
	assert.Contains(t, f.boxWins.seen, "review")
}

func TestServerSessions_StatusPollReportsNothingForAServerThatFailsIt(t *testing.T) {
	f := newServerFixture(t)
	f.create(t, "review")
	f.svc.ListServerSessions(t.Context())
	f.boxWins.err = errors.New("tmux is gone")

	assert.Empty(t, f.svc.ServerSessionStatuses(t.Context()))
}

func TestServerSessions_CreateOnAServer(t *testing.T) {
	f := newServerFixture(t)
	req := dispatch.CreateSessionRequest{Server: "box", Repository: "https://github.com/acme/site", Name: "review"}

	_, err := f.svc.CreateSession(t.Context(), req)
	require.NoError(t, err)
	require.NoError(t, f.jobs.err)
	onServer, err := f.box.ListSessions(t.Context())
	require.NoError(t, err)
	require.Len(t, onServer, 1)
	assert.Equal(t, "review", onServer[0].Name)
	local, err := f.svc.ListSessions(t.Context())
	require.NoError(t, err)
	assert.Empty(t, local)

	_, err = f.svc.CreateSession(t.Context(), req)
	require.NoError(t, err)
	require.ErrorContains(t, f.jobs.err, `already exists on server "box"`)
	draft, err := f.svc.FailedSessionDraft(t.Context())
	require.NoError(t, err)
	assert.Nil(t, draft.Failure, "a duplicate name is fixed in the field, not handed back as a failed form")
}

func TestServerSessions_CreateThatFailsKeepsTheServerInTheDraft(t *testing.T) {
	f := newServerFixture(t)

	_, err := f.svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{Server: "down", Repository: "https://github.com/acme/site", Name: "review"})
	require.NoError(t, err)
	require.Error(t, f.jobs.err)

	draft, err := f.svc.FailedSessionDraft(t.Context())
	require.NoError(t, err)
	require.NotNil(t, draft.Failure)
	assert.Equal(t, "down", draft.Server)
}

func TestServerSessions_AnUnreachableServerIsNotAMissingSession(t *testing.T) {
	f := newServerFixture(t)

	_, err := f.svc.SessionDetail(t.Context(), "down:s1")
	assert.Equal(t, KindUnavailable, KindOf(err))
	require.ErrorContains(t, err, "Connection refused")

	_, err = f.svc.EditSession(t.Context(), "down:s1", "renamed", nil)
	assert.Equal(t, KindUnavailable, KindOf(err))

	_, err = f.svc.SessionRisk(t.Context(), "down:s1")
	assert.Equal(t, KindUnavailable, KindOf(err))

	_, err = f.svc.SessionDetail(t.Context(), "box:missing")
	assert.Equal(t, KindNotFound, KindOf(err))
}

func TestServerSessions_ServersReportsEachCheck(t *testing.T) {
	f := newServerFixture(t)

	statuses := f.svc.Servers(t.Context())

	require.Len(t, statuses, 2)
	assert.Equal(t, server.StateConnected, statuses[0].State)
	assert.Equal(t, "test", statuses[0].Version)
	assert.Equal(t, server.StateOffline, statuses[1].State)
	assert.Contains(t, statuses[1].Error, "Connection refused")
}

func TestServerStateOf(t *testing.T) {
	incompatible := fmt.Errorf("hello: %w", &wire.IncompatibleError{DaemonVersion: "old"})
	assert.Equal(t, server.StateIncompatible, serverStateOf(incompatible))
	assert.Equal(t, server.StateOffline, serverStateOf(errors.New("exit status 255")))
}
