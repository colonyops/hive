package stores

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/hay-kot/hive/internal/data/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestKVStore(t *testing.T) *KVStore {
	t.Helper()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	return NewKVStore(database)
}

func TestKVStore_SetAndGet(t *testing.T) {
	ctx := context.Background()
	store := newTestKVStore(t)

	type payload struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	err := store.Set(ctx, "test-key", payload{Name: "hello", Value: 42})
	require.NoError(t, err)

	var got payload
	err = store.Get(ctx, "test-key", &got)
	require.NoError(t, err)
	assert.Equal(t, "hello", got.Name)
	assert.Equal(t, 42, got.Value)
}

func TestKVStore_GetNotFound(t *testing.T) {
	ctx := context.Background()
	store := newTestKVStore(t)

	var v string
	err := store.Get(ctx, "nonexistent", &v)
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestKVStore_SetOverwrite(t *testing.T) {
	ctx := context.Background()
	store := newTestKVStore(t)

	require.NoError(t, store.Set(ctx, "key", "first"))
	require.NoError(t, store.Set(ctx, "key", "second"))

	var got string
	require.NoError(t, store.Get(ctx, "key", &got))
	assert.Equal(t, "second", got)
}

func TestKVStore_Delete(t *testing.T) {
	ctx := context.Background()
	store := newTestKVStore(t)

	require.NoError(t, store.Set(ctx, "key", "value"))
	require.NoError(t, store.Delete(ctx, "key"))

	has, err := store.Has(ctx, "key")
	require.NoError(t, err)
	assert.False(t, has)
}

func TestKVStore_Has(t *testing.T) {
	ctx := context.Background()
	store := newTestKVStore(t)

	has, err := store.Has(ctx, "missing")
	require.NoError(t, err)
	assert.False(t, has)

	require.NoError(t, store.Set(ctx, "exists", true))
	has, err = store.Has(ctx, "exists")
	require.NoError(t, err)
	assert.True(t, has)
}

func TestKVStore_ListKeys(t *testing.T) {
	ctx := context.Background()
	store := newTestKVStore(t)

	require.NoError(t, store.Set(ctx, "b", 1))
	require.NoError(t, store.Set(ctx, "a", 2))
	require.NoError(t, store.Set(ctx, "c", 3))

	keys, err := store.ListKeys(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, keys)
}

func TestKVStore_GetRaw(t *testing.T) {
	ctx := context.Background()
	store := newTestKVStore(t)

	require.NoError(t, store.Set(ctx, "raw-test", map[string]int{"x": 1}))

	entry, err := store.GetRaw(ctx, "raw-test")
	require.NoError(t, err)
	assert.Equal(t, "raw-test", entry.Key)
	assert.Contains(t, string(entry.Value), `"x":1`)
	assert.Nil(t, entry.ExpiresAt)
	assert.False(t, entry.CreatedAt.IsZero())
}

func TestKVStore_TTLExpiry(t *testing.T) {
	ctx := context.Background()
	store := newTestKVStore(t)

	// Set with 1ms TTL
	require.NoError(t, store.SetTTL(ctx, "ephemeral", "gone", time.Millisecond))

	// Wait for it to expire
	time.Sleep(5 * time.Millisecond)

	// Get should return not found (lazy expiry)
	var v string
	err := store.Get(ctx, "ephemeral", &v)
	require.ErrorIs(t, err, sql.ErrNoRows)

	// Has should return false
	has, err := store.Has(ctx, "ephemeral")
	require.NoError(t, err)
	assert.False(t, has)
}

func TestKVStore_TTLNotExpired(t *testing.T) {
	ctx := context.Background()
	store := newTestKVStore(t)

	require.NoError(t, store.SetTTL(ctx, "alive", "here", time.Hour))

	var got string
	require.NoError(t, store.Get(ctx, "alive", &got))
	assert.Equal(t, "here", got)

	entry, err := store.GetRaw(ctx, "alive")
	require.NoError(t, err)
	assert.NotNil(t, entry.ExpiresAt)
}

func TestKVStore_SweepExpired(t *testing.T) {
	ctx := context.Background()
	store := newTestKVStore(t)

	// Create entries: one permanent, one expired
	require.NoError(t, store.Set(ctx, "permanent", "stays"))
	require.NoError(t, store.SetTTL(ctx, "expired", "goes", time.Millisecond))

	time.Sleep(5 * time.Millisecond)

	require.NoError(t, store.SweepExpired(ctx))

	keys, err := store.ListKeys(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"permanent"}, keys)
}

func TestKVStore_GetRawExpired(t *testing.T) {
	ctx := context.Background()
	store := newTestKVStore(t)

	require.NoError(t, store.SetTTL(ctx, "temp", "data", time.Millisecond))
	time.Sleep(5 * time.Millisecond)

	_, err := store.GetRaw(ctx, "temp")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}
