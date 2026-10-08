package hive_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/events"
	"github.com/colonyops/hive/pkg/executil/executiltest"
)

func runtimeOptions() hive.RuntimeOptions {
	return hive.RuntimeOptions{
		Executor:       &executiltest.Exec{},
		Mux:            stubMux{},
		Logger:         zerolog.Nop(),
		ScriptsVersion: "test",
	}
}

// settleGoroutines waits for goroutines left by earlier tests to exit, so the
// returned count is a baseline this test owns.
func settleGoroutines() int {
	baseline := runtime.NumGoroutine()
	for range 50 {
		time.Sleep(10 * time.Millisecond)
		current := runtime.NumGoroutine()
		if current >= baseline {
			return current
		}
		baseline = current
	}
	return baseline
}

// A plain loop rather than assert.Eventually: that helper runs its condition
// on a goroutine of its own, which the count would include.
func assertNoGoroutineLeak(t *testing.T, before int) {
	t.Helper()
	after := runtime.NumGoroutine()
	for range 250 {
		if after <= before {
			return
		}
		time.Sleep(20 * time.Millisecond)
		after = runtime.NumGoroutine()
	}
	buf := make([]byte, 1<<20)
	t.Log(string(buf[:runtime.Stack(buf, true)]))
	assert.LessOrEqual(t, after, before, "goroutine leaked")
}

func TestRuntimeOpenStartsTheBusAndExtractsScripts(t *testing.T) {
	cfg := loadConfig(t, t.TempDir())
	rt, err := hive.Open(t.Context(), cfg, runtimeOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = rt.Close() })

	require.NotNil(t, rt.Engine())
	assert.Same(t, cfg, rt.Engine().Config())
	assert.FileExists(t, filepath.Join(cfg.DataDir, "hive.db"))
	assert.DirExists(t, cfg.BinDir())

	delivered := make(chan struct{}, 1)
	rt.Engine().Bus().SubscribeRepoFocused(func(events.RepoFocusedPayload) { delivered <- struct{}{} })
	rt.Engine().Bus().PublishRepoFocused(events.RepoFocusedPayload{})
	select {
	case <-delivered:
	case <-time.After(5 * time.Second):
		t.Fatal("the event bus is not running")
	}
}

func TestRuntimeOpenFailureClosesWhatItOpened(t *testing.T) {
	t.Run("engine", func(t *testing.T) {
		cfg := loadConfig(t, t.TempDir())
		before := settleGoroutines()

		opts := runtimeOptions()
		opts.Mux = nil
		_, err := hive.Open(t.Context(), cfg, opts)

		var startup *hive.StartupError
		require.ErrorAs(t, err, &startup)
		assert.Equal(t, hive.StartupStepEngine, startup.Step)
		assertNoGoroutineLeak(t, before)
	})

	t.Run("database", func(t *testing.T) {
		cfg := loadConfig(t, t.TempDir())
		require.NoError(t, os.Mkdir(filepath.Join(cfg.DataDir, "hive.db"), 0o755))
		before := settleGoroutines()

		_, err := hive.Open(t.Context(), cfg, runtimeOptions())

		var startup *hive.StartupError
		require.ErrorAs(t, err, &startup)
		assert.Equal(t, hive.StartupStepDatabase, startup.Step)
		assertNoGoroutineLeak(t, before)
	})
}

func TestRuntimeCloseIsIdempotentAndStopsBackgroundWork(t *testing.T) {
	cfg := loadConfig(t, t.TempDir())
	before := settleGoroutines()

	rt, err := hive.Open(t.Context(), cfg, runtimeOptions())
	require.NoError(t, err)

	require.NoError(t, rt.Close())
	require.NoError(t, rt.Close())
	assertNoGoroutineLeak(t, before)
}

func TestRuntimeOpenRefusesACancelledContext(t *testing.T) {
	cfg := loadConfig(t, t.TempDir())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := hive.Open(ctx, cfg, runtimeOptions())

	require.ErrorIs(t, err, context.Canceled)
	assert.NoFileExists(t, filepath.Join(cfg.DataDir, "hive.db"))
}

func TestRuntimeCancelledContextStopsBackgroundWork(t *testing.T) {
	cfg := loadConfig(t, t.TempDir())
	before := settleGoroutines()
	ctx, cancel := context.WithCancel(t.Context())

	rt, err := hive.Open(ctx, cfg, runtimeOptions())
	require.NoError(t, err)
	cancel()

	done := make(chan error, 1)
	go func() { done <- rt.Close() }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return after the context was cancelled")
	}
	assertNoGoroutineLeak(t, before)
}

func TestRuntimeReloadKeepsTheDatabaseAndBus(t *testing.T) {
	dataDir := t.TempDir()
	rt, err := hive.Open(t.Context(), loadConfig(t, dataDir), runtimeOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = rt.Close() })

	engine := rt.Engine()
	bus := engine.Bus()
	database := rt.DB()
	require.NoError(t, engine.KV().Set(t.Context(), "k", "v"))

	reloaded := make(chan *config.Config, 1)
	bus.SubscribeConfigReloaded(func(p events.ConfigReloadedPayload) { reloaded <- p.Config })

	next := loadConfig(t, dataDir)
	next.GitPath = "/opt/git/bin/git"
	require.NoError(t, engine.Reload(next))

	assert.Same(t, engine, rt.Engine())
	assert.Same(t, bus, rt.Engine().Bus())
	assert.Same(t, database, rt.DB())
	assert.Same(t, next, rt.Engine().Config())
	select {
	case got := <-reloaded:
		assert.Same(t, next, got)
	case <-time.After(5 * time.Second):
		t.Fatal("config.reloaded was not delivered")
	}

	var got string
	require.NoError(t, rt.Engine().KV().Get(t.Context(), "k", &got))
	assert.Equal(t, "v", got)
}
