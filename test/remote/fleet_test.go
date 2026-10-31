//go:build remote

package remote

import (
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/server"
	"github.com/colonyops/hive/internal/hive/hivetest"
	"github.com/colonyops/hive/internal/hive/servers"
	"github.com/colonyops/hive/internal/hive/sessionapi"
)

func newFleet(t *testing.T) *servers.Fleet {
	t.Helper()
	var configs []config.ServerConfig
	for _, s := range []Server{serverA, serverB} {
		configs = append(configs, config.ServerConfig{Name: s.Name, SSH: s.Name, SSHConfig: sshConfig})
	}
	fleet := servers.New(zerolog.Nop(), nil, configs, "")
	t.Cleanup(fleet.Close)
	return fleet
}

func TestFleet_ReconnectsToAServerThatRestarts(t *testing.T) {
	fleet := newFleet(t)
	b, ok := fleet.Server(serverB.Name)
	require.True(t, ok)
	require.Equal(t, server.StateConnected, b.Check(t.Context()).State)

	serverB.Stop(t)
	statuses := fleet.CheckAll(t.Context())
	assert.Equal(t, server.StateConnected, statuses[0].State, "server-a stays connected: %s", statuses[0].Error)
	assert.Equal(t, server.StateOffline, statuses[1].State)
	assert.NotEmpty(t, statuses[1].Error)

	serverB.Start(t)
	// The same client dials a new ssh connection, so the fleet recovers
	// without a rebuild.
	assert.Eventually(t, func() bool {
		return b.Check(t.Context()).State == server.StateConnected
	}, 20*time.Second, 500*time.Millisecond)
	_, err := b.Client().ListSessions(t.Context())
	assert.NoError(t, err)
}

func TestRouter_ManagesSessionsOnBothServersAndSurvivesOneDown(t *testing.T) {
	var configs []config.ServerConfig
	for _, s := range []Server{serverA, serverB} {
		configs = append(configs, config.ServerConfig{Name: s.Name, SSH: s.Name, SSHConfig: sshConfig})
	}
	local := sessionapi.NewLocal(hivetest.NewEngine(t, hivetest.Options{}).Engine)
	registry := servers.New(zerolog.Nop(), local, configs, "")
	t.Cleanup(registry.Close)
	router := servers.NewRouter(registry)
	onA := serverA.CreateSession(t, "routed-a")
	onB := serverB.CreateSession(t, "routed-b")

	listed, err := router.ListSessions(t.Context())
	require.NoError(t, err)
	ids := map[string]bool{}
	for _, s := range listed {
		ids[s.ID] = true
	}
	assert.True(t, ids[serverA.Name+":"+onA.ID])
	assert.True(t, ids[serverB.Name+":"+onB.ID])

	require.NoError(t, router.RenameSession(t.Context(), serverB.Name+":"+onB.ID, "routed-b2"))
	assert.Equal(t, "routed-b2", serverB.Show(t, onB.ID).Name)

	serverB.Stop(t)
	listed, err = router.ListSessions(t.Context())
	require.NoError(t, err, "an offline server does not fail the list")
	ids = map[string]bool{}
	for _, s := range listed {
		ids[s.ID] = true
	}
	assert.True(t, ids[serverA.Name+":"+onA.ID])
	assert.True(t, ids[serverB.Name+":"+onB.ID], "an offline server keeps the rows of its last list")
	states := router.States()
	assert.Equal(t, server.StateConnected, states[0].State)
	assert.Equal(t, server.StateOffline, states[1].State)
}
