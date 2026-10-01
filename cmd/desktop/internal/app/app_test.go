package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

// TestAppLifecycle is the cheapest proof that the wiring package main used to
// own survived the move intact: the core builds, starts, and unwinds.
//
// The goroutine assertion is the part that matters. Every background
// subsystem here — the producer, the output worker, retention, the config
// watchers, the event bus and its subscribers, the shared hive event bus —
// detaches a goroutine, and a Close that forgets one leaks it silently for
// the life of the process. Nothing else in the suite would notice.
//
// go.uber.org/goleak would say this more precisely; it is not in go.mod and
// one assertion does not justify a dependency.
type startupCompactorStub struct {
	result queries.CompactionResult
	err    error
	called bool
}

func (s *startupCompactorStub) Compact(context.Context, queries.CompactionPolicy) (queries.CompactionResult, error) {
	s.called = true
	return s.result, s.err
}

func TestStartupCompactionFailureDoesNotPreventStartup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "desktop-pipeline.db")
	require.NoError(t, os.WriteFile(path, []byte("database"), 0o600))
	var logs bytes.Buffer
	compactor := &startupCompactorStub{err: errors.New("vacuum unavailable")}

	compactPipelineStoreAtStartup(t.Context(), compactor, path, zerolog.New(&logs))

	assert.True(t, compactor.called)
	assert.Contains(t, logs.String(), "pipeline compaction failed; startup continuing")
	assert.Contains(t, logs.String(), "vacuum unavailable")
}

func TestStartupCompactionContinuesWhenFileSizeCannotBeMeasured(t *testing.T) {
	var logs bytes.Buffer
	compactor := &startupCompactorStub{}

	compactPipelineStoreAtStartup(t.Context(), compactor, filepath.Join(t.TempDir(), "missing.db"), zerolog.New(&logs))

	assert.True(t, compactor.called)
	assert.Contains(t, logs.String(), "measure size before maintenance")
	assert.Contains(t, logs.String(), "measure size after maintenance")
}

func TestAppLifecycle(t *testing.T) {
	root := t.TempDir()
	t.Setenv(settings.EnvDataDir, filepath.Join(root, "data"))
	t.Setenv("HIVE_CONFIG", filepath.Join(root, "hive.yaml"))
	t.Setenv(settings.EnvConfigDir, filepath.Join(root, "config"))
	// Mock mode has no live producer and no keychain access, which is what
	// makes this runnable anywhere.
	t.Setenv(settings.EnvMockMode, "feed")

	settle(t)
	before := runtime.NumGoroutine()

	core, err := New(t.Context(), Config{
		Settings: settings.DefaultSettings(),
		MockMode: settings.MockMode(),
		Logger:   zerolog.Nop(),
	})
	require.NoError(t, err)

	// The facade is whole: every service an adapter mounts is non-nil.
	require.NotNil(t, core.Inbox)
	require.NotNil(t, core.Flows)
	require.NotNil(t, core.Actions)
	require.NotNil(t, core.Settings)
	require.NotNil(t, core.System)
	require.NotNil(t, core.Webhooks)
	require.NotNil(t, core.GitHub)
	require.NotNil(t, core.Activity)
	require.NotNil(t, core.Jobs)
	require.NotNil(t, core.Prompts)
	require.NotNil(t, core.Perf)
	require.NotNil(t, core.Events)
	require.NotNil(t, core.Stores)
	require.NotNil(t, core.PipelineDB())

	require.NoError(t, core.Start(t.Context()))
	require.NoError(t, core.Close())

	// Close is idempotent: a failed Run calls shutdown and so does a clean
	// one, and on some paths both.
	require.NotPanics(t, func() { _ = core.Close() })

	// A plain loop rather than assert.Eventually: that helper evaluates its
	// condition on a goroutine of its own, which the count would include, so
	// the assertion could never pass.
	after := runtime.NumGoroutine()
	for range 500 {
		if after <= before {
			break
		}
		time.Sleep(20 * time.Millisecond)
		after = runtime.NumGoroutine()
	}
	if after > before {
		buf := make([]byte, 1<<20)
		t.Log(string(buf[:runtime.Stack(buf, true)]))
	}
	assert.LessOrEqual(t, after, before, "Close leaked a goroutine")
}

// settle waits for goroutines left over from earlier tests in this package to
// exit, so the baseline is this test's own.
func settle(t *testing.T) {
	t.Helper()
	baseline := runtime.NumGoroutine()
	for range 50 {
		time.Sleep(10 * time.Millisecond)
		current := runtime.NumGoroutine()
		if current >= baseline {
			return
		}
		baseline = current
	}
}

// TestAgentWorkspacesReloadPublishesEvenOnFailure exercises the watcher ->
// store -> events.AgentWorkspacesUpdated chain end to end, including that a
// broken manifest still publishes — the whole point of the
// publish-even-on-failure shape openAgentWorkspaces shares with openActions
// (app.go:624-628): the reload's own error is only logged, never used to skip
// the publish, so the UI still re-reads and sees the failure reflected in
// Statuses.
func TestAgentWorkspacesReloadPublishesEvenOnFailure(t *testing.T) {
	root := t.TempDir()
	t.Setenv(settings.EnvDataDir, filepath.Join(root, "data"))
	t.Setenv("HIVE_CONFIG", filepath.Join(root, "hive.yaml"))
	t.Setenv(settings.EnvConfigDir, filepath.Join(root, "config"))
	t.Setenv(settings.EnvMockMode, "feed")
	workspacesDir := filepath.Join(root, "workspaces")
	t.Setenv(settings.EnvAgentWorkspacesDir, workspacesDir)

	core, err := New(t.Context(), Config{
		Settings: settings.DefaultSettings(),
		MockMode: settings.MockMode(),
		Logger:   zerolog.Nop(),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = core.Close() })
	require.NoError(t, core.Start(t.Context()))

	require.NotNil(t, core.agentWorkspaceStore)
	require.NotNil(t, core.agentWorkspacesWatcher)

	updates := make(chan events.AgentWorkspacesUpdated, 8)
	cancel := events.Subscribe(t.Context(), core.Events, "test.agentworkspaces", events.Buffer(8), func(_ context.Context, e events.AgentWorkspacesUpdated) {
		updates <- e
	})
	t.Cleanup(cancel)

	// A broken manifest (an unknown field — the strict decode path) must
	// still make the watcher fire, and the store must isolate the failure to
	// this one workspace rather than erroring Reload itself.
	brokenDir := filepath.Join(workspacesDir, "broken")
	require.NoError(t, os.MkdirAll(brokenDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(brokenDir, "agent-workspace.yaml"), []byte("version: 1\nfoo: bar\n"), 0o600))

	select {
	case <-updates:
	case <-time.After(5 * time.Second):
		t.Fatal("no agent-workspaces:updated published after a broken manifest was written")
	}

	found := false
	for _, st := range core.agentWorkspaceStore.Statuses() {
		if st.Dir != "broken" {
			continue
		}
		found = true
		assert.False(t, st.Valid)
		require.Error(t, st.Err)
	}
	assert.True(t, found, "the broken workspace must still surface in Statuses rather than vanish")
}

// TestAgentWorkspacesUnavailableRootCreatesNothing proves the EnsureRoot
// contract (spec §14, phase 1's manual criteria) survives openAgentWorkspaces:
// a configured root whose parent does not exist (an unmounted volume, a
// signed-out iCloud Drive) is reported, never silently created — and nothing
// downstream (SeedDefaultsIfMissing, NewWatcher) may create it either.
func TestAgentWorkspacesUnavailableRootCreatesNothing(t *testing.T) {
	root := t.TempDir()
	t.Setenv(settings.EnvDataDir, filepath.Join(root, "data"))
	t.Setenv("HIVE_CONFIG", filepath.Join(root, "hive.yaml"))
	t.Setenv(settings.EnvConfigDir, filepath.Join(root, "config"))
	t.Setenv(settings.EnvMockMode, "feed")
	missingParent := filepath.Join(root, "no-such-parent")
	workspacesDir := filepath.Join(missingParent, "workspaces")
	t.Setenv(settings.EnvAgentWorkspacesDir, workspacesDir)

	core, err := New(t.Context(), Config{
		Settings: settings.DefaultSettings(),
		MockMode: settings.MockMode(),
		Logger:   zerolog.Nop(),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = core.Close() })
	require.NoError(t, core.Start(t.Context()))

	_, statErr := os.Stat(missingParent)
	assert.True(t, os.IsNotExist(statErr), "an unavailable root must not even create its missing parent")
	_, statErr = os.Stat(workspacesDir)
	assert.True(t, os.IsNotExist(statErr), "an unavailable root must not be created")

	require.NotNil(t, core.agentWorkspaceStore)
	assert.Empty(t, core.agentWorkspaceStore.Statuses())
	assert.Nil(t, core.agentWorkspacesWatcher, "no watcher may exist over a root that was never created")
}
