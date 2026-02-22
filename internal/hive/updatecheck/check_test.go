package updatecheck

import (
	"context"
	"fmt"
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

func TestCheckerCheck(t *testing.T) {
	tests := []struct {
		name           string
		currentVersion string
		cachedTag      string
		fetchedTag     string
		fetchErr       error
		want           *Result
		wantFetchCalls int
	}{
		{name: "dev version", currentVersion: "dev", wantFetchCalls: 0},
		{name: "empty version", currentVersion: "", wantFetchCalls: 0},
		{name: "invalid current version", currentVersion: "not-semver", wantFetchCalls: 0},
		{name: "current equals latest", currentVersion: "v1.3.0", cachedTag: "v1.3.0", wantFetchCalls: 0},
		{name: "update available from cache", currentVersion: "v1.0.0", cachedTag: "v2.0.0", want: &Result{Current: "v1.0.0", Latest: "v2.0.0"}, wantFetchCalls: 0},
		{name: "current is newer", currentVersion: "v2.0.0", cachedTag: "v1.0.0", wantFetchCalls: 0},
		{name: "normalizes version without v prefix", currentVersion: "1.2.3", cachedTag: "v1.3.0", want: &Result{Current: "v1.2.3", Latest: "v1.3.0"}, wantFetchCalls: 0},
		{name: "invalid cached tag", currentVersion: "v1.0.0", cachedTag: "not-semver", wantFetchCalls: 0},
		{name: "cache miss fetches remote", currentVersion: "v1.0.0", fetchedTag: "v1.1.0", want: &Result{Current: "v1.0.0", Latest: "v1.1.0"}, wantFetchCalls: 1},
		{name: "cache miss fetch error degrades", currentVersion: "v1.0.0", fetchErr: fmt.Errorf("network down"), wantFetchCalls: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newTestKVStore(t)
			checker := New(store, nil)

			if tt.cachedTag != "" {
				cacheRelease(t, store, tt.cachedTag)
			}

			fetchCalls := 0
			checker.fetchLatestReleaseJSON = func(context.Context) ([]byte, error) {
				fetchCalls++
				if tt.fetchErr != nil {
					return nil, tt.fetchErr
				}
				if tt.fetchedTag == "" {
					return []byte(`{"tag_name":""}`), nil
				}
				return []byte(fmt.Sprintf(`{"tag_name":%q}`, tt.fetchedTag)), nil
			}

			result, err := checker.Check(context.Background(), tt.currentVersion)
			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
			assert.Equal(t, tt.wantFetchCalls, fetchCalls)
		})
	}
}

func TestCheckerCheck_NilCheckerAndNilStore(t *testing.T) {
	var nilChecker *Checker
	result, err := nilChecker.Check(context.Background(), "v1.0.0")
	require.NoError(t, err)
	assert.Nil(t, result)

	checker := New(nil, nil)
	result, err = checker.Check(context.Background(), "v1.0.0")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestCheckerCheck_CacheEntryExpires(t *testing.T) {
	store := newTestKVStore(t)
	checker := New(store, nil)

	cache := kv.Scoped[ReleaseInfo](store, cacheNamespace)
	err := cache.SetTTL(context.Background(), cacheKey, ReleaseInfo{TagName: "v1.0.0"}, time.Millisecond)
	require.NoError(t, err)
	time.Sleep(5 * time.Millisecond)

	fetchCalls := 0
	checker.fetchLatestReleaseJSON = func(context.Context) ([]byte, error) {
		fetchCalls++
		return []byte(`{"tag_name":"v1.1.0"}`), nil
	}

	result, checkErr := checker.Check(context.Background(), "v1.0.0")
	require.NoError(t, checkErr)
	require.NotNil(t, result)
	assert.Equal(t, "v1.1.0", result.Latest)
	assert.Equal(t, 1, fetchCalls)
}
