package kv_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/hay-kot/hive/internal/core/kv"
	"github.com/hay-kot/hive/internal/data/db"
	"github.com/hay-kot/hive/internal/data/stores"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestKV(t *testing.T) kv.KV {
	t.Helper()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	return stores.NewKVStore(database)
}

func TestTypedKV_SetAndGet(t *testing.T) {
	ctx := context.Background()
	store := newTestKV(t)
	typed := kv.Scoped[string](store, "test")

	require.NoError(t, typed.Set(ctx, "greeting", "hello"))

	got, err := typed.Get(ctx, "greeting")
	require.NoError(t, err)
	assert.Equal(t, "hello", got)
}

func TestTypedKV_ScopedPrefix(t *testing.T) {
	ctx := context.Background()
	store := newTestKV(t)

	// Two scoped stores with different namespaces
	alpha := kv.Scoped[int](store, "alpha")
	beta := kv.Scoped[int](store, "beta")

	require.NoError(t, alpha.Set(ctx, "count", 10))
	require.NoError(t, beta.Set(ctx, "count", 20))

	// Each scope sees its own value
	a, err := alpha.Get(ctx, "count")
	require.NoError(t, err)
	assert.Equal(t, 10, a)

	b, err := beta.Get(ctx, "count")
	require.NoError(t, err)
	assert.Equal(t, 20, b)

	// Raw store sees both with prefixed keys
	keys, err := store.ListKeys(ctx)
	require.NoError(t, err)
	assert.Contains(t, keys, "alpha:count")
	assert.Contains(t, keys, "beta:count")
}

func TestTypedKV_Delete(t *testing.T) {
	ctx := context.Background()
	store := newTestKV(t)
	typed := kv.Scoped[string](store, "ns")

	require.NoError(t, typed.Set(ctx, "key", "val"))
	require.NoError(t, typed.Delete(ctx, "key"))

	has, err := typed.Has(ctx, "key")
	require.NoError(t, err)
	assert.False(t, has)
}

func TestTypedKV_Has(t *testing.T) {
	ctx := context.Background()
	store := newTestKV(t)
	typed := kv.Scoped[int](store, "ns")

	has, err := typed.Has(ctx, "missing")
	require.NoError(t, err)
	assert.False(t, has)

	require.NoError(t, typed.Set(ctx, "exists", 1))
	has, err = typed.Has(ctx, "exists")
	require.NoError(t, err)
	assert.True(t, has)
}

func TestTypedKV_TTL(t *testing.T) {
	ctx := context.Background()
	store := newTestKV(t)
	typed := kv.Scoped[string](store, "ttl")

	require.NoError(t, typed.SetTTL(ctx, "temp", "gone", time.Millisecond))
	time.Sleep(5 * time.Millisecond)

	_, err := typed.Get(ctx, "temp")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestTypedKV_StructValue(t *testing.T) {
	ctx := context.Background()
	store := newTestKV(t)

	type Config struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}

	typed := kv.Scoped[Config](store, "config")
	require.NoError(t, typed.Set(ctx, "api", Config{Host: "localhost", Port: 8080}))

	got, err := typed.Get(ctx, "api")
	require.NoError(t, err)
	assert.Equal(t, "localhost", got.Host)
	assert.Equal(t, 8080, got.Port)
}
