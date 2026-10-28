package hive_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/events"
	"github.com/colonyops/hive/internal/hive/hivetest"
	"github.com/colonyops/hive/pkg/executil/executiltest"
)

func runtimePorts(dataDir string) hive.Ports {
	return hive.Ports{
		Executor: &executiltest.Exec{},
		Mux:      hivetest.Mux{},
		DataDir:  dataDir,
		Logger:   zerolog.Nop(),
	}
}

func startRuntime(t *testing.T, ctx context.Context, cfg *config.Config) *hive.Runtime {
	t.Helper()
	rt, err := hive.StartRuntime(ctx, cfg, hive.RuntimeOptions{Ports: runtimePorts(cfg.DataDir), ScriptsVersion: "test"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = rt.Close() })
	return rt
}

// SQLite removes the WAL file when the last connection closes, so its absence
// shows hive.db was closed.
func walPath(cfg *config.Config) string { return cfg.DatabaseFile() + "-wal" }

func waitBackground(t *testing.T, rt *hive.Runtime) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		rt.WaitBackground()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runtime background work did not stop")
	}
}

func TestRuntimeStartsTheEngineOnAnOpenDatabaseAndBus(t *testing.T) {
	cfg := hivetest.LoadConfig(t, t.TempDir())
	rt := startRuntime(t, t.Context(), cfg)

	require.NotNil(t, rt.Engine())
	assert.FileExists(t, walPath(cfg))
	assert.FileExists(t, filepath.Join(cfg.DataDir, "bin", ".version"), "the bundled scripts are extracted")
	assert.Same(t, rt.Bus(), rt.Engine().Bus())

	_, err := rt.Engine().Sessions().ListSessions(t.Context())
	require.NoError(t, err)

	delivered := make(chan struct{}, 1)
	rt.Bus().SubscribeSessionDeleted(func(events.SessionDeletedPayload) { delivered <- struct{}{} })
	rt.Bus().PublishSessionDeleted(events.SessionDeletedPayload{SessionID: "s1"})
	select {
	case <-delivered:
	case <-time.After(time.Second):
		t.Fatal("the bus did not deliver an event")
	}
}

func TestRuntimeRejectsAProgramSuppliedDatabaseOrBus(t *testing.T) {
	cfg := hivetest.LoadConfig(t, t.TempDir())
	ports := runtimePorts(cfg.DataDir)
	ports.Bus = events.New(1)

	_, err := hive.StartRuntime(t.Context(), cfg, hive.RuntimeOptions{Ports: ports})
	require.Error(t, err)
}

func TestRuntimeDatabaseFailureReportsTheDatabaseStep(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(blocker, nil, 0o600))
	cfg := hivetest.LoadConfig(t, filepath.Join(blocker, "data"))

	_, err := hive.StartRuntime(t.Context(), cfg, hive.RuntimeOptions{Ports: runtimePorts(t.TempDir())})

	var startErr *hive.StartError
	require.ErrorAs(t, err, &startErr)
	assert.Equal(t, hive.StartStepDatabase, startErr.Step)
}

func TestRuntimeEngineFailureClosesTheDatabase(t *testing.T) {
	cfg := hivetest.LoadConfig(t, t.TempDir())
	ports := runtimePorts(cfg.DataDir)
	ports.Mux = nil

	rt, err := hive.StartRuntime(t.Context(), cfg, hive.RuntimeOptions{Ports: ports})

	assert.Nil(t, rt)
	var startErr *hive.StartError
	require.ErrorAs(t, err, &startErr)
	assert.Equal(t, hive.StartStepEngine, startErr.Step)
	assert.FileExists(t, cfg.DatabaseFile(), "the database step ran")
	assert.NoFileExists(t, walPath(cfg))
}

func TestRuntimeCloseIsIdempotent(t *testing.T) {
	cfg := hivetest.LoadConfig(t, t.TempDir())
	rt := startRuntime(t, t.Context(), cfg)

	require.NoError(t, rt.Close())
	require.NoError(t, rt.Close())

	waitBackground(t, rt)
	assert.NoFileExists(t, walPath(cfg))
	require.Error(t, rt.DB().Conn().PingContext(t.Context()))
}

func TestRuntimeDoesNotStartOnACanceledContext(t *testing.T) {
	cfg := hivetest.LoadConfig(t, t.TempDir())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := hive.StartRuntime(ctx, cfg, hive.RuntimeOptions{Ports: runtimePorts(cfg.DataDir)})

	require.ErrorIs(t, err, context.Canceled)
	assert.NoFileExists(t, cfg.DatabaseFile())
}

func TestRuntimeBackgroundWorkStopsWithItsContext(t *testing.T) {
	cfg := hivetest.LoadConfig(t, t.TempDir())
	ctx, cancel := context.WithCancel(t.Context())
	rt := startRuntime(t, ctx, cfg)

	cancel()

	waitBackground(t, rt)
	require.NoError(t, rt.Close())
}

func TestRuntimeReloadKeepsTheDatabaseAndBus(t *testing.T) {
	cfg := hivetest.LoadConfig(t, t.TempDir())
	rt := startRuntime(t, t.Context(), cfg)
	bus, sessions := rt.Bus(), rt.Engine().Sessions()
	require.NoError(t, rt.Engine().KV().Set(t.Context(), "before.reload", "kept"))

	next := hivetest.LoadConfig(t, cfg.DataDir)
	next.GitPath = "/opt/git/bin/git"
	require.NoError(t, rt.Engine().Reload(next))

	assert.Same(t, next, rt.Engine().Config())
	assert.NotSame(t, sessions, rt.Engine().Sessions())
	assert.Same(t, bus, rt.Engine().Bus())

	var got string
	require.NoError(t, rt.Engine().KV().Get(t.Context(), "before.reload", &got))
	assert.Equal(t, "kept", got)
	_, err := rt.Engine().Sessions().ListSessions(t.Context())
	require.NoError(t, err, "the rebuilt session service reads the runtime's database")
}
