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
	"github.com/colonyops/hive/internal/hive/servers"
)

func newFleet(t *testing.T) *servers.Fleet {
	t.Helper()
	var configs []config.ServerConfig
	for _, s := range []Server{serverA, serverB} {
		configs = append(configs, config.ServerConfig{Name: s.Name, SSH: s.Name, SSHConfig: sshConfig})
	}
	fleet := servers.New(zerolog.Nop(), nil, configs)
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
