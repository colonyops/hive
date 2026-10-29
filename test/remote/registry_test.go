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

func newRegistry(t *testing.T) *servers.Registry {
	t.Helper()
	var configs []config.ServerConfig
	for _, s := range []Server{serverA, serverB} {
		configs = append(configs, config.ServerConfig{Name: s.Name, SSH: s.Name, SSHConfig: sshConfig})
	}
	r := servers.New(zerolog.Nop(), nil, configs)
	t.Cleanup(r.Close)
	return r
}

func TestRegistry_ReconnectsToAServerThatRestarts(t *testing.T) {
	r := newRegistry(t)
	b, ok := r.Server(serverB.Name)
	require.True(t, ok)
	require.Equal(t, server.StateConnected, b.Check(t.Context()).State)

	serverB.Stop(t)
	statuses := r.CheckAll(t.Context())
	assert.Equal(t, server.StateConnected, statuses[0].State, "server-a stays connected: %s", statuses[0].Error)
	assert.Equal(t, server.StateOffline, statuses[1].State)
	assert.NotEmpty(t, statuses[1].Error)

	serverB.Start(t)
	// The same client dials a new ssh connection, so the registry recovers
	// without a rebuild.
	assert.Eventually(t, func() bool {
		return b.Check(t.Context()).State == server.StateConnected
	}, 20*time.Second, 500*time.Millisecond)
	_, err := b.Client().ListSessions(t.Context())
	assert.NoError(t, err)
}
