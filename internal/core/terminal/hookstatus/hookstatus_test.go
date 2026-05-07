package hookstatus_test

import (
	"context"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/core/terminal/hookstatus"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/data/stores"
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

func TestStore_WriteAndRead(t *testing.T) {
	ctx := context.Background()
	store := hookstatus.New(newTestKV(t))

	err := store.Write(ctx, "sess-1", "%0", terminal.StatusActive)
	require.NoError(t, err)

	got, ok := store.Read(ctx, "sess-1", "%0")
	require.True(t, ok)
	assert.Equal(t, terminal.StatusActive, got)
}

func TestStore_ReadMissing(t *testing.T) {
	ctx := context.Background()
	store := hookstatus.New(newTestKV(t))

	got, ok := store.Read(ctx, "sess-1", "%0")
	assert.False(t, ok)
	assert.Equal(t, terminal.Status(""), got)
}

func TestStore_Delete(t *testing.T) {
	ctx := context.Background()
	store := hookstatus.New(newTestKV(t))

	require.NoError(t, store.Write(ctx, "sess-1", "%0", terminal.StatusReady))
	require.NoError(t, store.Delete(ctx, "sess-1", "%0"))

	_, ok := store.Read(ctx, "sess-1", "%0")
	assert.False(t, ok)
}

func TestStore_IsFreshWithFreshEntry(t *testing.T) {
	ctx := context.Background()
	store := hookstatus.New(newTestKV(t))

	require.NoError(t, store.Write(ctx, "sess-1", "%0", terminal.StatusActive))

	// Entry was just written — should be fresh with a large maxAge.
	assert.True(t, store.IsFresh(ctx, "sess-1", "%0", 10*time.Minute))
}

func TestStore_IsFreshMissingEntry(t *testing.T) {
	ctx := context.Background()
	store := hookstatus.New(newTestKV(t))

	assert.False(t, store.IsFresh(ctx, "sess-1", "%0", 30*time.Second))
}

func TestStore_IsFreshExpired(t *testing.T) {
	ctx := context.Background()
	store := hookstatus.New(newTestKV(t))

	require.NoError(t, store.Write(ctx, "sess-1", "%0", terminal.StatusActive))

	// Use a tiny maxAge so the entry is immediately stale.
	assert.False(t, store.IsFresh(ctx, "sess-1", "%0", -1*time.Second))
}

func TestStore_LegacyWindowIndexKey(t *testing.T) {
	ctx := context.Background()
	store := hookstatus.New(newTestKV(t))

	// Write using a window index as the pane key (pre-Phase-2 sessions).
	require.NoError(t, store.Write(ctx, "sess-1", "0", terminal.StatusReady))

	got, ok := store.Read(ctx, "sess-1", "0")
	require.True(t, ok)
	assert.Equal(t, terminal.StatusReady, got)
}

func TestStore_IsolatedBySession(t *testing.T) {
	ctx := context.Background()
	store := hookstatus.New(newTestKV(t))

	require.NoError(t, store.Write(ctx, "sess-A", "%0", terminal.StatusActive))
	require.NoError(t, store.Write(ctx, "sess-B", "%0", terminal.StatusReady))

	a, ok := store.Read(ctx, "sess-A", "%0")
	require.True(t, ok)
	assert.Equal(t, terminal.StatusActive, a)

	b, ok := store.Read(ctx, "sess-B", "%0")
	require.True(t, ok)
	assert.Equal(t, terminal.StatusReady, b)
}
