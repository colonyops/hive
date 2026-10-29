package daemon

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/hive/gitstatus"
)

func TestGitStatusCache_ClientsWithinTheTTLShareOneRead(t *testing.T) {
	var reads [][]string
	cache := newGitStatusCache(func(_ context.Context, ids []string) (map[string]gitstatus.Status, error) {
		reads = append(reads, ids)
		out := map[string]gitstatus.Status{}
		for _, id := range ids {
			out[id] = gitstatus.Status{Branch: "branch-" + id, Resolved: true}
		}
		return out, nil
	})
	now := time.Unix(0, 0)
	cache.now = func() time.Time { return now }

	first, err := cache.get(t.Context(), []string{"a", "b"})
	require.NoError(t, err)
	assert.Equal(t, "branch-a", first["a"].Branch)

	now = now.Add(gitStatusTTL / 2)
	second, err := cache.get(t.Context(), []string{"a", "c"})
	require.NoError(t, err)
	assert.Equal(t, "branch-a", second["a"].Branch)
	assert.Equal(t, "branch-c", second["c"].Branch)

	now = now.Add(gitStatusTTL)
	_, err = cache.get(t.Context(), []string{"a"})
	require.NoError(t, err)

	assert.Equal(t, [][]string{{"a", "b"}, {"c"}, {"a"}}, reads)
}

func TestGitStatusCache_AFailedReadCachesNothing(t *testing.T) {
	fail := true
	cache := newGitStatusCache(func(_ context.Context, ids []string) (map[string]gitstatus.Status, error) {
		if fail {
			return nil, errors.New("database is locked")
		}
		return map[string]gitstatus.Status{ids[0]: {Resolved: true}}, nil
	})

	_, err := cache.get(t.Context(), []string{"a"})
	require.EqualError(t, err, "database is locked")

	fail = false
	got, err := cache.get(t.Context(), []string{"a"})
	require.NoError(t, err)
	assert.True(t, got["a"].Resolved)
}
