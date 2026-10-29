package app

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/daemon"
	"github.com/colonyops/hive/internal/hive/daemonclient"
	"github.com/colonyops/hive/internal/hive/hivetest"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/pkg/executil/executiltest"
)

// runTestDaemon serves a daemon over its own hive.db, so anything the
// SessionsService reads or writes there can only have gone through the
// client.
func runTestDaemon(t *testing.T, mux *fakeMux) *daemonclient.Client {
	t.Helper()
	return daemonclient.New(daemonclient.Options{Dialer: daemonclient.UnixDialer(runTestDaemonSocket(t, mux)), Logger: zerolog.Nop()})
}

// runTestDaemonSocket is runTestDaemon's daemon, answered by its socket path.
func runTestDaemonSocket(t *testing.T, mux *fakeMux) string {
	t.Helper()
	dataDir := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		done <- daemon.Run(ctx, daemon.Options{
			Config:  hivetest.LoadConfig(t, dataDir),
			Ports:   hive.Ports{Executor: &executiltest.Exec{}, Mux: mux, DataDir: dataDir, Logger: zerolog.Nop()},
			Version: "test",
			OnReady: func(path string) { ready <- path },
		})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("daemon did not stop")
		}
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

func TestSessionsServiceRunsOverTheDaemon(t *testing.T) {
	daemonMux := &fakeMux{}
	client := runTestDaemon(t, daemonMux)
	created, err := client.CreateSession(t.Context(), sessionsvc.CreateOptions{Name: "review", Remote: "https://github.com/acme/site", SkipSpawn: true})
	require.NoError(t, err)

	local := newHiveHarness(t, engineOptions{})
	tmux := &fakeSessionTmux{}
	jobs := &fakeJobRunner{}
	svc := newSessionsService(SessionsDeps{Hive: local.engine, Sessions: client, Tmux: tmux, Jobs: jobs, Logger: zerolog.Nop()})

	listed, err := svc.ListSessions(t.Context())
	require.NoError(t, err)
	require.Len(t, listed, 1, "the session exists only in the daemon's hive.db")
	assert.Equal(t, created.ID, listed[0].ID)

	renamed, err := svc.RenameSession(t.Context(), created.ID, "renamed")
	require.NoError(t, err)
	assert.Equal(t, "renamed", renamed.Name)
	assert.Equal(t, [][2]string{{"review", "renamed"}}, tmux.renames, "Desktop renames tmux itself first")
	assert.Equal(t, [][2]string{{"review", "renamed"}}, daemonMux.renamed, "the daemon renamed the row and its tmux session")

	_, err = svc.DeleteSession(t.Context(), created.ID)
	require.NoError(t, err)
	require.NoError(t, jobs.err)
	remaining, err := client.ListSessions(t.Context())
	require.NoError(t, err)
	assert.Empty(t, remaining)
	assert.Empty(t, local.mux.renamed, "the local engine saw none of it")
}

func TestSessionsServiceEditsTagsOverTheDaemon(t *testing.T) {
	client := runTestDaemon(t, &fakeMux{})
	created, err := client.CreateSession(t.Context(), sessionsvc.CreateOptions{Name: "review", Remote: "https://github.com/acme/site", SkipSpawn: true})
	require.NoError(t, err)

	local := newHiveHarness(t, engineOptions{})
	svc := newSessionsService(SessionsDeps{Hive: local.engine, Sessions: client, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}, Logger: zerolog.Nop()})

	edited, err := svc.EditSession(t.Context(), created.ID, "review", []string{"backend"})
	require.NoError(t, err)
	assert.Equal(t, []string{"backend"}, edited.Tags)

	edited, err = svc.EditSessionTags(t.Context(), created.ID, []string{"urgent"}, []string{"backend"})
	require.NoError(t, err)
	assert.Equal(t, []string{"urgent"}, edited.Tags)

	stored, err := client.GetSession(t.Context(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"urgent"}, stored.Tags, "the tags live only in the daemon's hive.db")

	_, err = svc.EditSessionTags(t.Context(), created.ID, []string{"a,b"}, nil)
	assert.Equal(t, KindInvalid, KindOf(err))
}
