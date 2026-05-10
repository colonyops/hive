package lua

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/internal/hive/plugins"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	glua "github.com/yuin/gopher-lua"
)

func TestPluginAvailable(t *testing.T) {
	entry := filepath.Join(t.TempDir(), "init.lua")
	require.NoError(t, os.WriteFile(entry, []byte("return function() end\n"), 0o644))

	plugin := NewConfigPlugin(entry)
	assert.True(t, plugin.Available())

	missing := NewConfigPlugin(filepath.Join(t.TempDir(), "missing.lua"))
	assert.False(t, missing.Available())
}

func TestPluginInitReplacesPriorRunOnReinitialization(t *testing.T) {
	entry := filepath.Join(t.TempDir(), "init.lua")
	require.NoError(t, os.WriteFile(entry, []byte(`
return function(hive)
  hive.commands({ First = { sh = "echo first" } })
end
`), 0o644))

	plugin := NewConfigPlugin(entry)
	require.NoError(t, plugin.Init(context.Background()))
	t.Cleanup(func() { require.NoError(t, plugin.Close()) })

	require.Contains(t, plugin.Commands(), "First")

	require.NoError(t, os.WriteFile(entry, []byte(`
return function(hive)
  hive.commands({ Second = { sh = "echo second" } })
end
`), 0o644))

	require.NoError(t, plugin.Init(context.Background()))

	assert.NotContains(t, plugin.Commands(), "First", "stale command from prior init must not survive re-initialization")
	assert.Contains(t, plugin.Commands(), "Second")
}

func TestPluginInitDoesNotLeakCommandsOnFailure(t *testing.T) {
	entry := filepath.Join(t.TempDir(), "init.lua")
	require.NoError(t, os.WriteFile(entry, []byte(`
return function(hive)
  hive.commands({ Good = { sh = "echo good" } })
  error("boom")
end
`), 0o644))

	plugin := NewConfigPlugin(entry)
	err := plugin.Init(context.Background())
	require.Error(t, err)

	assert.Empty(t, plugin.Commands(), "Commands() must be empty after a failed Init")
}

func TestPlugin_Reinit_ClearsLuaSlot(t *testing.T) {
	entry := filepath.Join(t.TempDir(), "init.lua")
	require.NoError(t, os.WriteFile(entry, []byte(`
return function(hive)
  hive.commands({ Foo = { sh = "echo foo" } })
end
`), 0o644))

	plugin, set := newConfigPluginWithSet(entry)
	require.NoError(t, plugin.Init(context.Background()))
	require.Contains(t, set.Plugin("lua"), "Foo")

	// Re-write entry to register nothing.
	require.NoError(t, os.WriteFile(entry, []byte(`return function(hive) end`), 0o644))
	require.NoError(t, plugin.Init(context.Background()))
	t.Cleanup(func() { require.NoError(t, plugin.Close()) })

	// After re-init, the slot must not contain Foo from the prior run.
	assert.NotContains(t, set.Plugin("lua"), "Foo")
}

func TestPlugin_Close_ClearsSlotAfterDispatcherDrains(t *testing.T) {
	entry := filepath.Join(t.TempDir(), "init.lua")
	require.NoError(t, os.WriteFile(entry, []byte(`
return function(hive)
  hive.commands({ Foo = { sh = "echo foo" } })
end
`), 0o644))

	p, set := newConfigPluginWithSet(entry)
	require.NoError(t, p.Init(context.Background()))
	require.Contains(t, set.Plugin("lua"), "Foo")

	// Queue a MergePlugin on the dispatcher right before Close. The
	// dispatcher drains pending work during Close, so this work is
	// guaranteed to run; the bug fix ensures the slot is cleared *after*
	// the runtime has fully shut down, so any late writer cannot leave
	// stale commands behind.
	p.runtime.Submit(func(_ *glua.LState) {
		set.MergePlugin("lua", map[string]config.UserCommand{
			"LateRegistered": {Sh: "echo late"},
		})
	})

	require.NoError(t, p.Close())

	assert.Empty(t, set.Plugin("lua"), "slot must be empty after Close, regardless of in-flight dispatcher work")
}

// NewConfigPlugin builds a Plugin from a single entry file path. Shared by
// the lifecycle, runtime, and per-module tests in this package.
func NewConfigPlugin(entry string) *Plugin {
	return New(
		config.LuaPluginConfig{Entry: entry, DispatcherQueueSize: 64},
		newFakeKV(),
		plugins.NewWorkerPool(1),
		plugins.NewCommandSet(nil, nil),
		zerolog.Nop(),
	)
}

// newConfigPluginWithSet builds a Plugin and returns the shared CommandSet so
// tests can inspect plugin slot writes directly. Mirrors NewConfigPlugin but
// hands the set back to the caller.
func newConfigPluginWithSet(entry string) (*Plugin, *plugins.CommandSet) {
	set := plugins.NewCommandSet(nil, nil)
	p := New(
		config.LuaPluginConfig{Entry: entry, DispatcherQueueSize: 64},
		newFakeKV(),
		plugins.NewWorkerPool(1),
		set,
		zerolog.Nop(),
	)
	return p, set
}

// fakeKV is an in-memory kv.KV used purely for test wiring. It JSON-encodes
// values so semantics match the production KVStore: a Set followed by a Get
// into a string dest works the same way as it would over SQLite.
type fakeKV struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newFakeKV() *fakeKV {
	return &fakeKV{data: map[string][]byte{}}
}

func (f *fakeKV) Get(_ context.Context, key string, dest any) error {
	f.mu.Lock()
	raw, ok := f.data[key]
	f.mu.Unlock()
	if !ok {
		return fmt.Errorf("kv get %q: %w", key, sql.ErrNoRows)
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return fmt.Errorf("kv get %q unmarshal: %w", key, err)
	}
	return nil
}

func (f *fakeKV) Set(_ context.Context, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("kv set %q marshal: %w", key, err)
	}
	f.mu.Lock()
	f.data[key] = raw
	f.mu.Unlock()
	return nil
}

func (f *fakeKV) SetTTL(ctx context.Context, key string, value any, _ time.Duration) error {
	return f.Set(ctx, key, value)
}

func (f *fakeKV) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	delete(f.data, key)
	f.mu.Unlock()
	return nil
}

func (f *fakeKV) Has(_ context.Context, key string) (bool, error) {
	f.mu.Lock()
	_, ok := f.data[key]
	f.mu.Unlock()
	return ok, nil
}

func (f *fakeKV) ListKeys(_ context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	keys := make([]string, 0, len(f.data))
	for k := range f.data {
		keys = append(keys, k)
	}
	return keys, nil
}

func (f *fakeKV) GetRaw(_ context.Context, key string) (kv.Entry, error) {
	f.mu.Lock()
	raw, ok := f.data[key]
	f.mu.Unlock()
	if !ok {
		return kv.Entry{}, fmt.Errorf("kv get raw %q: %w", key, sql.ErrNoRows)
	}
	return kv.Entry{Key: key, Value: raw}, nil
}
