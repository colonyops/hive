package lua

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/data/stores"
)

// kvHarness wires the shared luaHarness with a KVModule scoped under the
// production pluginName so namespacing assertions ("lua:foo") match.
func kvHarness(t *testing.T, store kv.KV, script string) *luaHarness {
	t.Helper()
	return newLuaHarness(t, script, &KVModule{Store: kv.Scoped[string](store, pluginName)})
}

func newRealKVStore(t *testing.T) kv.KV {
	t.Helper()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	return stores.NewKVStore(database)
}

func TestKVModule_RoundTrip(t *testing.T) {
	store := newRealKVStore(t)
	kvHarness(t, store, `
return function(hive)
  hive.kv.set("foo", "bar")
  assert(hive.kv.get("foo") == "bar", "expected foo=bar")
end
`)

	keys, err := store.ListKeys(context.Background())
	require.NoError(t, err)
	assert.Contains(t, keys, "lua:foo")
}

func TestKVModule_GetMissingReturnsNil(t *testing.T) {
	kvHarness(t, newRealKVStore(t), `
return function(hive)
  assert(hive.kv.get("missing") == nil, "expected nil for missing key")
end
`)
}

func TestKVModule_DeleteRemovesKey(t *testing.T) {
	store := newRealKVStore(t)
	kvHarness(t, store, `
return function(hive)
  hive.kv.set("foo", "bar")
  hive.kv.delete("foo")
  assert(hive.kv.get("foo") == nil, "expected nil after delete")
end
`)

	has, err := store.Has(context.Background(), "lua:foo")
	require.NoError(t, err)
	assert.False(t, has)
}

func TestKVModule_NamespacingIsolatesPlugins(t *testing.T) {
	store := newRealKVStore(t)
	require.NoError(t, store.Set(context.Background(), "other:foo", "v"))

	kvHarness(t, store, `
return function(hive)
  assert(hive.kv.get("foo") == nil, "lua plugin must not see other:foo")
end
`)
}

func TestKVModule_RejectsNonStringValue(t *testing.T) {
	kvHarness(t, newRealKVStore(t), `
return function(hive)
  local ok = pcall(hive.kv.set, "foo", {1, 2, 3})
  if ok then
    error("expected non-string value to be rejected")
  end
end
`)
}

func TestKVModule_OpsRejectInputs(t *testing.T) {
	storeErr := &errKV{err: errors.New("disk on fire")}
	cases := []struct {
		name   string
		store  kv.KV
		script string
	}{
		{"set empty key", newRealKVStore(t), `local ok = pcall(hive.kv.set, "", "v"); if ok then error("expected empty key") end`},
		{"get empty key", newRealKVStore(t), `local ok = pcall(hive.kv.get, ""); if ok then error("expected empty key") end`},
		{"delete empty key", newRealKVStore(t), `local ok = pcall(hive.kv.delete, ""); if ok then error("expected empty key") end`},
		{"set store error", storeErr, `local ok, err = pcall(hive.kv.set, "foo", "bar"); if ok or not string.find(tostring(err), "hive.kv.set", 1, true) then error("unexpected: " .. tostring(err)) end`},
		{"get store error", storeErr, `local ok, err = pcall(hive.kv.get, "foo"); if ok or not string.find(tostring(err), "hive.kv.get", 1, true) then error("unexpected: " .. tostring(err)) end`},
		{"delete store error", storeErr, `local ok, err = pcall(hive.kv.delete, "foo"); if ok or not string.find(tostring(err), "hive.kv.delete", 1, true) then error("unexpected: " .. tostring(err)) end`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kvHarness(t, tc.store, "return function(hive) "+tc.script+" end")
		})
	}
}

// errKV is a kv.KV whose every method returns the configured err.
type errKV struct {
	err error
}

func (e *errKV) Get(context.Context, string, any) error                   { return e.err }
func (e *errKV) Set(context.Context, string, any) error                   { return e.err }
func (e *errKV) SetTTL(context.Context, string, any, time.Duration) error { return e.err }
func (e *errKV) Delete(context.Context, string) error                     { return e.err }
func (e *errKV) Has(context.Context, string) (bool, error)                { return false, e.err }
func (e *errKV) ListKeys(context.Context) ([]string, error)               { return nil, e.err }
func (e *errKV) GetRaw(context.Context, string) (kv.Entry, error)         { return kv.Entry{}, e.err }
