package servers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/server"
	"github.com/colonyops/hive/internal/domain/session"
)

// failingSSH puts an ssh on PATH that prints line to stderr and exits.
func failingSSH(t *testing.T, line string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\necho '" + line + "' >&2\nexit 255\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ssh"), []byte(script), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestFleetFindsServersInConfigOrder(t *testing.T) {
	fleet := New(zerolog.Nop(), nil, []config.ServerConfig{{Name: "b", SSH: "b"}, {Name: "a", SSH: "a"}})

	require.Len(t, fleet.Servers(), 2)
	assert.Equal(t, "b", fleet.Servers()[0].Config().Name)
	a, ok := fleet.Server("a")
	require.True(t, ok)
	assert.Equal(t, server.StateConnecting, a.Status().State)
	_, ok = fleet.Server("missing")
	assert.False(t, ok)
}

func TestCheckReportsAnOfflineServerWithTheSSHError(t *testing.T) {
	failingSSH(t, "ssh: connect to host box port 22: Connection refused")
	fleet := New(zerolog.Nop(), nil, []config.ServerConfig{{Name: "box", SSH: "box"}})

	statuses := fleet.CheckAll(t.Context())

	require.Len(t, statuses, 1)
	assert.Equal(t, server.StateOffline, statuses[0].State)
	assert.Contains(t, statuses[0].Error, "Connection refused")
	box, _ := fleet.Server("box")
	assert.Equal(t, statuses[0], box.Status())
}

func TestCheckDropsTheSSHErrorOfAnEarlierDial(t *testing.T) {
	failingSSH(t, "first failure")
	fleet := New(zerolog.Nop(), nil, []config.ServerConfig{{Name: "box", SSH: "box"}})
	box, _ := fleet.Server("box")
	require.Contains(t, box.Check(t.Context()).Error, "first failure")

	failingSSH(t, "")
	status := box.Check(t.Context())

	assert.Equal(t, server.StateOffline, status.State)
	assert.NotContains(t, status.Error, "first failure")
}

func TestAPIResolvesServerNames(t *testing.T) {
	fleet := New(zerolog.Nop(), nil, []config.ServerConfig{{Name: "box", SSH: "box"}})

	box, ok := fleet.Server("box")
	require.True(t, ok)
	api, err := fleet.API("box")
	require.NoError(t, err)
	assert.Same(t, box.Client(), api)

	_, err = fleet.API("gone")
	require.ErrorIs(t, err, ErrUnknownServer)
	assert.Contains(t, err.Error(), `"gone"`)

	_, err = fleet.API(session.LocalServer)
	assert.Error(t, err, "a fleet without a local API reports it")
}
