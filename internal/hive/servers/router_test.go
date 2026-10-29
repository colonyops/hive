package servers_test

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/server"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/daemon"
	"github.com/colonyops/hive/internal/hive/daemonclient"
	"github.com/colonyops/hive/internal/hive/hivetest"
	"github.com/colonyops/hive/internal/hive/servers"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/hive/sessionapi"
	"github.com/colonyops/hive/pkg/executil/executiltest"
)

// runDaemon serves a daemon over its own hive.db and returns its socket.
func runDaemon(t *testing.T) string {
	t.Helper()
	dataDir := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		done <- daemon.Run(ctx, daemon.Options{
			Config:  hivetest.LoadConfig(t, dataDir),
			Ports:   hive.Ports{Executor: &executiltest.Exec{}, Mux: hivetest.Mux{}, DataDir: dataDir, Logger: zerolog.Nop()},
			Version: "test",
			OnReady: func(path string) { ready <- path },
		})
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	select {
	case socket := <-ready:
		return socket
	case err := <-done:
		t.Fatalf("daemon exited before it listened: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not start listening")
	}
	return ""
}

// newRouter routes to a local API and the server "box", a test daemon
// reached without ssh. The server "down" never answers.
func newRouter(t *testing.T) (*servers.Router, sessionapi.API, sessionapi.API) {
	t.Helper()
	localSocket, boxSocket := runDaemon(t), runDaemon(t)
	dial := func(ctx context.Context, cfg config.ServerConfig, stderr io.Writer) (net.Conn, error) {
		if cfg.Name == "down" {
			_, _ = io.WriteString(stderr, "ssh: connect to host down port 22: Connection refused\n")
			return nil, errors.New("exit status 255")
		}
		var d net.Dialer
		return d.DialContext(ctx, "unix", boxSocket)
	}
	local := newClient(localSocket)
	registry := servers.New(zerolog.Nop(), local,
		[]config.ServerConfig{{Name: "box", SSH: "box"}, {Name: "down", SSH: "down"}}, "test", servers.WithDialer(dial))
	t.Cleanup(registry.Close)
	box, err := registry.API("box")
	require.NoError(t, err)
	return servers.NewRouter(registry), local, box
}

func create(t *testing.T, api sessionapi.API, name string) *session.Session {
	t.Helper()
	sess, err := api.CreateSession(t.Context(), sessionsvc.CreateOptions{Name: name, Remote: "https://github.com/acme/site", SkipSpawn: true})
	require.NoError(t, err)
	return sess
}

func TestRouter_ListsEveryServerWithQualifiedIDs(t *testing.T) {
	router, local, box := newRouter(t)
	here := create(t, local, "here")
	there := create(t, box, "there")

	sessions, err := router.ListSessions(t.Context())
	require.NoError(t, err)

	ids := map[string]string{}
	for _, s := range sessions {
		ids[s.Name] = s.ID
	}
	assert.Equal(t, map[string]string{"here": here.ID, "there": "box:" + there.ID}, ids)
	states := router.States()
	require.Len(t, states, 2)
	assert.Equal(t, server.StateConnected, states[0].State)
	assert.Equal(t, server.StateOffline, states[1].State)
	assert.ErrorContains(t, states[1].Err, "Connection refused")
}

func TestRouter_SendsACallToTheServerItsIDNames(t *testing.T) {
	router, local, box := newRouter(t)
	twinHere := create(t, local, "twin")
	twinThere := create(t, box, "twin")

	require.NoError(t, router.RenameSession(t.Context(), "box:"+twinThere.ID, "renamed"))

	got, err := router.GetSession(t.Context(), "box:"+twinThere.ID)
	require.NoError(t, err)
	assert.Equal(t, "renamed", got.Name)
	assert.Equal(t, "box:"+twinThere.ID, got.ID)
	stayed, err := local.GetSession(t.Context(), twinHere.ID)
	require.NoError(t, err)
	assert.Equal(t, "twin", stayed.Name)

	statuses, err := router.GitStatuses(t.Context(), []string{twinHere.ID, "box:" + twinThere.ID})
	require.NoError(t, err)
	assert.Contains(t, statuses, twinHere.ID)
	assert.Contains(t, statuses, "box:"+twinThere.ID)

	require.NoError(t, router.DeleteSession(t.Context(), "box:"+twinThere.ID))
	_, err = box.GetSession(t.Context(), twinThere.ID)
	require.ErrorIs(t, err, session.ErrNotFound)
}

func newClient(socket string) sessionapi.API {
	return daemonclient.New(daemonclient.Options{Dialer: daemonclient.UnixDialer(socket), Logger: zerolog.Nop()})
}
