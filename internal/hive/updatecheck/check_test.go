package updatecheck

import (
	"context"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/data/stores"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestKVStore(t *testing.T) kv.KV {
	t.Helper()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	return stores.NewKVStore(database)
}

func cacheRelease(t *testing.T, store kv.KV, tag string) {
	t.Helper()
	cache := kv.Scoped[ReleaseInfo](store, cacheNamespace)
	err := cache.SetTTL(context.Background(), cacheKey, ReleaseInfo{TagName: tag}, cacheTTL)
	require.NoError(t, err)
}

func withStubbedFetch(t *testing.T) *int {
	t.Helper()
	calls := 0
	prevFetch := fetchLatestReleaseJSON
	fetchLatestReleaseJSON = func(context.Context) ([]byte, error) {
		calls++
		return nil, nil
	}
	t.Cleanup(func() {
		fetchLatestReleaseJSON = prevFetch
	})
	return &calls
}

func TestCheck_DevVersion(t *testing.T) {
	result, err := Check(context.Background(), nil, "dev")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestCheck_EmptyVersion(t *testing.T) {
	result, err := Check(context.Background(), nil, "")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestCheck_InvalidVersion(t *testing.T) {
	result, err := Check(context.Background(), nil, "not-semver")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestCheck_CurrentIsLatest(t *testing.T) {
	store := newTestKVStore(t)
	cacheRelease(t, store, "v1.3.0")
	lookups := withStubbedFetch(t)

	result, err := Check(context.Background(), store, "v1.3.0")
	require.NoError(t, err)
	assert.Nil(t, result)
	assert.Equal(t, 0, *lookups)
}

func TestCheck_UpdateAvailable(t *testing.T) {
	store := newTestKVStore(t)
	cacheRelease(t, store, "v2.0.0")
	lookups := withStubbedFetch(t)

	result, err := Check(context.Background(), store, "v1.0.0")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "v1.0.0", result.Current)
	assert.Equal(t, "v2.0.0", result.Latest)
	assert.Equal(t, 0, *lookups)
}

func TestCheck_CurrentIsNewer(t *testing.T) {
	store := newTestKVStore(t)
	cacheRelease(t, store, "v1.0.0")
	lookups := withStubbedFetch(t)

	result, err := Check(context.Background(), store, "v2.0.0")
	require.NoError(t, err)
	assert.Nil(t, result)
	assert.Equal(t, 0, *lookups)
}

func TestCheck_CachedResult(t *testing.T) {
	store := newTestKVStore(t)
	cacheRelease(t, store, "v1.1.0")
	lookups := withStubbedFetch(t)

	result, err := Check(context.Background(), store, "v1.0.0")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "v1.1.0", result.Latest)
	assert.Equal(t, 0, *lookups)
}

func TestCheck_NormalizesVersionPrefix(t *testing.T) {
	store := newTestKVStore(t)
	cacheRelease(t, store, "v1.3.0")
	lookups := withStubbedFetch(t)

	result, err := Check(context.Background(), store, "1.2.3")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "v1.2.3", result.Current)
	assert.Equal(t, "v1.3.0", result.Latest)
	assert.Equal(t, 0, *lookups)
}

func TestCheck_InvalidCachedTag(t *testing.T) {
	store := newTestKVStore(t)
	cache := kv.Scoped[ReleaseInfo](store, cacheNamespace)
	err := cache.SetTTL(context.Background(), cacheKey, ReleaseInfo{TagName: "not-semver"}, time.Hour)
	require.NoError(t, err)

	result, checkErr := Check(context.Background(), store, "v1.0.0")
	require.NoError(t, checkErr)
	assert.Nil(t, result)
}
