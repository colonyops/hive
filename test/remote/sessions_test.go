//go:build remote

package remote

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/session"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
)

type serverSummary struct {
	Name      string `json:"name"`
	Reachable bool   `json:"reachable"`
	Version   string `json:"version"`
	Sessions  int    `json:"sessions"`
	Error     string `json:"error"`
}

type probeResult struct {
	Name   string `json:"name"`
	Health struct {
		DBOK   bool `json:"db_ok"`
		TmuxOK bool `json:"tmux_ok"`
	} `json:"health"`
	Sessions []probeSession `json:"sessions"`
}

type probeSession struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// sessionWithServer is the part of the session JSON that names a session
// among servers.
type sessionWithServer struct {
	Server string `json:"server"`
	ID     string `json:"id"`
	Name   string `json:"name"`
}

func sessionIDs(sessions []Session) []string {
	ids := make([]string, 0, len(sessions))
	for _, s := range sessions {
		ids = append(ids, s.ID)
	}
	return ids
}

func TestServersProbe_SeesASessionCreatedOnTheServer(t *testing.T) {
	created := serverA.CreateSession(t, "probe-me")

	probe := DecodeJSON[probeResult](t, NewClient(t).Hive("servers", "probe", "--json", serverA.Name).Ok(t))

	assert.Equal(t, serverA.Name, probe.Name)
	assert.True(t, probe.Health.DBOK)
	assert.True(t, probe.Health.TmuxOK)
	assert.Contains(t, probe.Sessions, probeSession{ID: created.ID, Name: "probe-me"})
}

func TestServersLs_ReachesBothServers(t *testing.T) {
	summaries := DecodeJSONLines[serverSummary](t, NewClient(t).Hive("servers", "ls", "--json").Ok(t))

	require.Len(t, summaries, 2)
	for _, s := range summaries {
		assert.True(t, s.Reachable, "%s: %s", s.Name, s.Error)
		assert.NotEmpty(t, s.Version, s.Name)
	}
}

func TestSessionAPI_ChangesTheServer(t *testing.T) {
	api := serverA.API(t)
	ctx := t.Context()

	created, err := api.CreateSession(ctx, sessionsvc.CreateOptions{Name: "lifecycle", Remote: tFixtureRepo})
	require.NoError(t, err)
	serverA.DeleteOnCleanup(t, created.ID)
	assert.Contains(t, sessionIDs(serverA.List(t)), created.ID)

	require.NoError(t, api.RenameSession(ctx, created.ID, "lifecycle-renamed"))
	require.NoError(t, api.SetSessionGroup(ctx, created.ID, "team"))
	shown := serverA.Show(t, created.ID)
	assert.Equal(t, "lifecycle-renamed", shown.Name)
	assert.Equal(t, "team", shown.Group)

	require.NoError(t, api.RecycleSession(ctx, created.ID, nil))
	assert.Equal(t, string(session.StateRecycled), serverA.Show(t, created.ID).State)

	require.NoError(t, api.DeleteSession(ctx, created.ID))
	gone := serverA.Hive(t, "session", "show", "--json", created.ID).Fails(t)
	assert.Contains(t, gone.Stderr, session.ErrNotFound.Error())
}

func TestFixture_AliasesReachDistinctServers(t *testing.T) {
	onA := serverA.CreateSession(t, "twin")
	onB := serverB.CreateSession(t, "twin")

	idsOnA := sessionIDs(serverA.List(t))
	idsOnB := sessionIDs(serverB.List(t))
	assert.Contains(t, idsOnA, onA.ID)
	assert.NotContains(t, idsOnA, onB.ID)
	assert.Contains(t, idsOnB, onB.ID)
	assert.NotContains(t, idsOnB, onA.ID)
}

func TestServersLs_AStoppedServerIsUnreachableUntilItStarts(t *testing.T) {
	client := NewClient(t)
	reachable := func() map[string]serverSummary {
		out := map[string]serverSummary{}
		for _, s := range DecodeJSONLines[serverSummary](t, client.Hive("servers", "ls", "--json").Ok(t)) {
			out[s.Name] = s
		}
		return out
	}

	serverB.Stop(t)
	down := reachable()
	assert.True(t, down[serverA.Name].Reachable, "server-a stays reachable: %s", down[serverA.Name].Error)
	assert.False(t, down[serverB.Name].Reachable)
	assert.NotEmpty(t, down[serverB.Name].Error)

	serverB.Start(t)
	// The client may reach the restarted server before its daemon answers.
	assert.Eventually(t, func() bool {
		b := reachable()[serverB.Name]
		if !b.Reachable {
			t.Logf("server-b not reachable yet: %s", b.Error)
		}
		return b.Reachable
	}, 20*time.Second, 500*time.Millisecond)
}

func TestClientCLI_CreatesASessionOnTheServer(t *testing.T) {
	r := NewClient(t).HiveOn(serverA, "session", "create", "--json", "--background", "--remote", tFixtureRepo, "from-client").Ok(t)
	created := DecodeJSON[Session](t, r)
	serverA.DeleteOnCleanup(t, created.ID)

	shown := serverA.Show(t, created.ID)
	assert.Equal(t, "from-client", shown.Name)
	assert.Equal(t, tFixtureRepo, shown.Remote)
}

func TestClientCLI_RefusesASourceDirectoryForTheServer(t *testing.T) {
	failed := NewClient(t).HiveOn(serverA, "session", "create", "--background", "--remote", tFixtureRepo, "--source", ".", "with-source").Fails(t)

	assert.Contains(t, failed.Stderr, "--source")
	for _, s := range serverA.List(t) {
		assert.NotEqual(t, "with-source", s.Name)
	}
}

func TestClientCLI_ShowsAndListsServerSessions(t *testing.T) {
	created := serverA.CreateSession(t, "listed")
	client := NewClient(t)

	shown := DecodeJSON[Session](t, client.HiveOn(serverA, "session", "show", "--json", created.ID).Ok(t))
	assert.Equal(t, created.ID, shown.ID)
	assert.Equal(t, "listed", shown.Name)

	listed := DecodeJSONLines[Session](t, client.HiveOn(serverA, "session", "list", "--json").Ok(t))
	assert.Contains(t, sessionIDs(listed), created.ID)
}

func TestClientCLI_RenamesAndGroupsASession(t *testing.T) {
	created := serverA.CreateSession(t, "before")

	NewClient(t).HiveOn(serverA, "session", "update", created.ID, "--name", "after", "--group", "team").Ok(t)

	shown := serverA.Show(t, created.ID)
	assert.Equal(t, "after", shown.Name)
	assert.Equal(t, "team", shown.Group)
}

func TestClientCLI_RecyclesASession(t *testing.T) {
	created := serverA.CreateSession(t, "recycle-me")

	NewClient(t).HiveOn(serverA, "session", "recycle", created.ID).Ok(t)

	assert.Equal(t, string(session.StateRecycled), serverA.Show(t, created.ID).State)
}

func TestClientCLI_ANewSessionOnARecycledCloneIsANewSession(t *testing.T) {
	first := serverA.CreateSession(t, "first-task")
	serverA.Hive(t, "msg", "pub", "--topic", "agent."+first.ID+".inbox", "-m", "for first-task").Ok(t)
	client := NewClient(t)
	client.HiveOn(serverA, "session", "recycle", first.ID).Ok(t)

	created := client.HiveOn(serverA, "session", "create", "--json", "--background", "--remote", tFixtureRepo, "second-task").Ok(t)
	second := DecodeJSON[Session](t, created)
	serverA.DeleteOnCleanup(t, second.ID)

	assert.NotEqual(t, first.ID, second.ID)
	assert.Equal(t, first.Path, second.Path, "the clone is reused")
	gone := client.HiveOn(serverA, "session", "show", first.ID).Fails(t)
	assert.Contains(t, gone.Stderr, session.ErrNotFound.Error())

	inbox := serverA.Hive(t, "msg", "inbox", "--session", second.ID, "--all").Ok(t)
	assert.NotContains(t, inbox.Stdout, "for first-task")
}

func TestClientCLI_DeletesASession(t *testing.T) {
	created := serverA.CreateSession(t, "delete-me")

	NewClient(t).HiveOn(serverA, "session", "delete", created.ID).Ok(t)

	gone := serverA.Hive(t, "session", "show", "--json", created.ID).Fails(t)
	assert.Contains(t, gone.Stderr, session.ErrNotFound.Error())
}

const tSharedID = "clash1"

// sharedIDSessions puts a session with the same id on each server.
func sharedIDSessions(t *testing.T) (refA, refB string) {
	t.Helper()
	serverA.CreateSessionWithID(t, "on-a", tSharedID)
	serverB.CreateSessionWithID(t, "on-b", tSharedID)
	return serverA.Name + ":" + tSharedID, serverB.Name + ":" + tSharedID
}

func TestClientCLI_ActsOnTheSessionOnTheServerTheRefNames(t *testing.T) {
	refA, refB := sharedIDSessions(t)
	client := NewClient(t)

	shownA := DecodeJSON[sessionWithServer](t, client.Hive("session", "show", "--json", refA).Ok(t))
	assert.Equal(t, sessionWithServer{Server: serverA.Name, ID: tSharedID, Name: "on-a"}, shownA)
	shownB := DecodeJSON[sessionWithServer](t, client.Hive("session", "show", "--json", refB).Ok(t))
	assert.Equal(t, sessionWithServer{Server: serverB.Name, ID: tSharedID, Name: "on-b"}, shownB)

	client.Hive("session", "update", refB, "--name", "renamed-b").Ok(t)

	assert.Equal(t, "on-a", serverA.Show(t, tSharedID).Name)
	assert.Equal(t, "renamed-b", serverB.Show(t, tSharedID).Name)
}

func TestClientCLI_DeletesOnlyTheSessionTheRefNames(t *testing.T) {
	refA, _ := sharedIDSessions(t)

	NewClient(t).Hive("session", "delete", refA).Ok(t)

	gone := serverA.Hive(t, "session", "show", "--json", tSharedID).Fails(t)
	assert.Contains(t, gone.Stderr, session.ErrNotFound.Error())
	assert.Equal(t, "on-b", serverB.Show(t, tSharedID).Name)
}

func TestClientCLI_RecyclesOnlyTheSessionTheRefNames(t *testing.T) {
	_, refB := sharedIDSessions(t)

	NewClient(t).Hive("session", "recycle", refB).Ok(t)

	assert.Equal(t, string(session.StateActive), serverA.Show(t, tSharedID).State)
	assert.Equal(t, string(session.StateRecycled), serverB.Show(t, tSharedID).State)
}

func TestClientCLI_ChecksRiskOnTheServerTheRefNames(t *testing.T) {
	refA, refB := sharedIDSessions(t)
	serverA.Shell(t, "touch "+shellQuote(serverA.Show(t, tSharedID).Path+"/uncommitted")).Ok(t)
	client := NewClient(t)

	for _, action := range []string{"delete", "recycle"} {
		refused := client.Hive("session", action, refA).Fails(t)
		assert.Contains(t, refused.Stderr, "uncommitted changes", action)
	}
	assert.Equal(t, string(session.StateActive), serverA.Show(t, tSharedID).State)

	client.Hive("session", "recycle", refB).Ok(t)

	client.Hive("session", "delete", "--force", refA).Ok(t)
	gone := serverA.Hive(t, "session", "show", "--json", tSharedID).Fails(t)
	assert.Contains(t, gone.Stderr, session.ErrNotFound.Error())
}

func TestClientCLI_SessionInfoRefusesATargetServer(t *testing.T) {
	failed := NewClient(t).HiveOn(serverA, "session", "info", "--json").Fails(t)

	assert.Contains(t, failed.Stderr, "session info reads the session of this directory on this machine")
}

func TestClientCLI_ADeleteFailsOnAStuckDaemonWithoutAChange(t *testing.T) {
	created := serverA.CreateSession(t, "stuck-daemon")
	// The bracket keeps pkill from matching the shell that runs it.
	serverA.Shell(t, "pkill -STOP -f '[h]ive daemon run'").Ok(t)
	t.Cleanup(func() { serverA.Shell(t, "pkill -CONT -f '[h]ive daemon run'").Ok(t) })

	failed := NewClient(t).Hive("session", "delete", "--force", serverA.Name+":"+created.ID).Fails(t)

	assert.Contains(t, failed.Stderr, "nothing changed")
	serverA.Shell(t, "pkill -CONT -f '[h]ive daemon run'").Ok(t)
	assert.Equal(t, "stuck-daemon", serverA.Show(t, created.ID).Name)
}

func TestClientCLI_NamesAStoppedServerInTheError(t *testing.T) {
	serverB.Stop(t)

	failed := NewClient(t).Hive("session", "show", "--json", serverB.Name+":abc123").Fails(t)

	assert.Empty(t, strings.TrimSpace(failed.Stdout), "stdout is for JSON")
	assert.Contains(t, failed.Stderr, serverB.Name)
	assert.Contains(t, failed.Stderr, "hive servers ls")
}

func TestClientCLI_TheRiskCheckOutlastsASlowGit(t *testing.T) {
	created := serverA.CreateSession(t, "slow-git")
	serverA.Shell(t, "touch "+shellQuote(created.Path+"/uncommitted")).Ok(t)
	// The risk check runs git at least twice, so this passes the 10s read limit.
	serverA.Root(t, `printf '#!/bin/sh\nsleep 6\nexec /usr/bin/git "$@"\n' >/usr/local/bin/git; chmod 755 /usr/local/bin/git`).Ok(t)
	t.Cleanup(func() { serverA.Root(t, "rm -f /usr/local/bin/git").Ok(t) })

	refused := NewClient(t).Hive("session", "delete", serverA.Name+":"+created.ID).Fails(t)

	assert.Contains(t, refused.Stderr, "uncommitted changes")
}

func TestClientCLI_NamesTheServerWhenItsDaemonDiesDuringACreate(t *testing.T) {
	serverA.Root(t, `printf '#!/bin/sh\nsleep 6\nexec /usr/bin/git "$@"\n' >/usr/local/bin/git; chmod 755 /usr/local/bin/git`).Ok(t)
	t.Cleanup(func() { serverA.Root(t, "rm -f /usr/local/bin/git").Ok(t) })
	time.AfterFunc(3*time.Second, func() {
		// The bracket keeps pkill from matching the shell that runs it.
		serverA.Shell(t, "pkill -KILL -f '[h]ive daemon run'")
	})

	failed := NewClient(t).HiveOn(serverA, "session", "create", "--background", "--remote", tFixtureRepo, "daemon-dies").Fails(t)

	assert.Contains(t, failed.Stderr, "lost the connection to "+serverA.Name)
	assert.Contains(t, failed.Stderr, "may still finish")
}

func TestClientCLI_WorksWhenTheServerShellPrintsText(t *testing.T) {
	created := serverB.CreateSession(t, "noisy-shell")
	serverB.Shell(t, `sed -i '1i echo Welcome to box' ~/.bashrc`).Ok(t)
	t.Cleanup(func() { serverB.Shell(t, `sed -i '1d' ~/.bashrc`).Ok(t) })
	client := NewClient(t)

	shown := DecodeJSON[sessionWithServer](t, client.Hive("session", "show", "--json", serverB.Name+":"+created.ID).Ok(t))
	assert.Equal(t, "noisy-shell", shown.Name)

	client.Hive("session", "update", serverB.Name+":"+created.ID, "--name", "still-works").Ok(t)
	listed := DecodeJSONLines[sessionWithServer](t, client.HiveOn(serverB, "session", "list", "--json").Ok(t))
	assert.Contains(t, listed, sessionWithServer{Server: serverB.Name, ID: created.ID, Name: "still-works"})
}

func TestClientCLI_RefusesARefToAServerNotInTheConfig(t *testing.T) {
	failed := NewClient(t).Hive("session", "show", "gone:abc123").Fails(t)

	assert.Contains(t, failed.Stderr, `unknown server "gone"`)
}

func TestClientCLI_ListsSessionsWithTheirServer(t *testing.T) {
	created := serverA.CreateSession(t, "labeled")

	listed := DecodeJSONLines[sessionWithServer](t, NewClient(t).HiveOn(serverA, "session", "list", "--json").Ok(t))

	assert.Contains(t, listed, sessionWithServer{Server: serverA.Name, ID: created.ID, Name: "labeled"})
}
