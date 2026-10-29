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
)

// failingSSH puts an ssh on PATH that prints line to stderr and exits.
func failingSSH(t *testing.T, line string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\necho '" + line + "' >&2\nexit 255\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ssh"), []byte(script), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestRegistryFindsServersInConfigOrder(t *testing.T) {
	r := New(zerolog.Nop(), nil, []config.ServerConfig{{Name: "b", SSH: "b"}, {Name: "a", SSH: "a"}})

	require.Len(t, r.Servers(), 2)
	assert.Equal(t, "b", r.Servers()[0].Config().Name)
	a, ok := r.Server("a")
	require.True(t, ok)
	assert.Equal(t, server.StateConnecting, a.Status().State)
	_, ok = r.Server("missing")
	assert.False(t, ok)
}

func TestCheckReportsAnOfflineServerWithTheSSHError(t *testing.T) {
	failingSSH(t, "ssh: connect to host box port 22: Connection refused")
	r := New(zerolog.Nop(), nil, []config.ServerConfig{{Name: "box", SSH: "box"}})

	statuses := r.CheckAll(t.Context())

	require.Len(t, statuses, 1)
	assert.Equal(t, server.StateOffline, statuses[0].State)
	assert.Contains(t, statuses[0].Error, "Connection refused")
	box, _ := r.Server("box")
	assert.Equal(t, statuses[0], box.Status())
}

func TestCheckDropsTheSSHErrorOfAnEarlierDial(t *testing.T) {
	failingSSH(t, "first failure")
	r := New(zerolog.Nop(), nil, []config.ServerConfig{{Name: "box", SSH: "box"}})
	box, _ := r.Server("box")
	require.Contains(t, box.Check(t.Context()).Error, "first failure")

	failingSSH(t, "")
	status := box.Check(t.Context())

	assert.Equal(t, server.StateOffline, status.State)
	assert.NotContains(t, status.Error, "first failure")
}
