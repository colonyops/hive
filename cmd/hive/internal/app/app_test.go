package app

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hiveconfig "github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/servers"
	"github.com/colonyops/hive/internal/hive/sessionapi"
)

func newAppWithServers(local sessionapi.API) *App {
	fleet := servers.New(zerolog.Nop(), local, []hiveconfig.ServerConfig{{Name: "box", SSH: "box"}}, "")
	return &App{Servers: fleet, sessions: local}
}

func TestServerAPIResolvesServerNames(t *testing.T) {
	local := &struct{ sessionapi.API }{}
	a := newAppWithServers(local)

	api, err := a.ServerAPI(session.LocalServer)
	require.NoError(t, err)
	assert.Same(t, local, api)

	box, _ := a.Servers.Server("box")
	api, err = a.ServerAPI("box")
	require.NoError(t, err)
	assert.Same(t, box.Client(), api)

	_, err = a.ServerAPI("gone")
	assert.ErrorContains(t, err, `unknown server "gone"`)
}

func TestTargetServerKeepsLocalRefsOnThisMachine(t *testing.T) {
	local := &struct{ sessionapi.API }{}
	a := newAppWithServers(local)

	require.NoError(t, a.TargetServer("box"))

	assert.Equal(t, "box", a.DefaultServer())
	box, _ := a.Servers.Server("box")
	assert.Same(t, box.Client(), a.SessionAPI())
	api, err := a.ServerAPI(session.LocalServer)
	require.NoError(t, err)
	assert.Same(t, local, api)
}

func TestTargetServerLocalKeepsTheLocalSessions(t *testing.T) {
	local := &struct{ sessionapi.API }{}
	a := newAppWithServers(local)

	require.NoError(t, a.TargetServer(session.LocalServer))
	assert.Equal(t, session.LocalServer, a.DefaultServer())
}
