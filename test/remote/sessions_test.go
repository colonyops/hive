//go:build remote

package remote

import (
	"testing"

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

func sessionIDs(sessions []Session) []string {
	ids := make([]string, 0, len(sessions))
	for _, s := range sessions {
		ids = append(ids, s.ID)
	}
	return ids
}

func TestProbeSeesASessionCreatedOnTheServer(t *testing.T) {
	created := serverA.CreateSession(t, "probe-me")

	probe := DecodeJSON[probeResult](t, NewClient(t).Hive("servers", "probe", "--json", serverA.Name).Ok(t))

	assert.Equal(t, serverA.Name, probe.Name)
	assert.True(t, probe.Health.DBOK)
	assert.True(t, probe.Health.TmuxOK)
	assert.Contains(t, probe.Sessions, probeSession{ID: created.ID, Name: "probe-me"})
}

func TestServersLsReachesBothServers(t *testing.T) {
	summaries := DecodeJSONLines[serverSummary](t, NewClient(t).Hive("servers", "ls", "--json").Ok(t))

	require.Len(t, summaries, 2)
	for _, s := range summaries {
		assert.True(t, s.Reachable, "%s: %s", s.Name, s.Error)
		assert.NotEmpty(t, s.Version, s.Name)
	}
}

func TestSessionAPIOverSSHChangesTheServer(t *testing.T) {
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
	// hive prints its errors to stdout.
	assert.Contains(t, gone.Stdout+gone.Stderr, session.ErrNotFound.Error())
}

func TestFixtureAliasesReachDistinctServers(t *testing.T) {
	onA := serverA.CreateSession(t, "twin")
	onB := serverB.CreateSession(t, "twin")

	idsOnA := sessionIDs(serverA.List(t))
	idsOnB := sessionIDs(serverB.List(t))
	assert.Contains(t, idsOnA, onA.ID)
	assert.NotContains(t, idsOnA, onB.ID)
	assert.Contains(t, idsOnB, onB.ID)
	assert.NotContains(t, idsOnB, onA.ID)
}

func TestAStoppedServerIsUnreachableUntilItStarts(t *testing.T) {
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
	assert.True(t, reachable()[serverB.Name].Reachable)
}
