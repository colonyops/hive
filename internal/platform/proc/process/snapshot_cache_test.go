package process

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type countingTake struct {
	calls int
	maps  []map[int][]int
}

func (c *countingTake) take() map[int][]int {
	m := c.maps[c.calls]
	c.calls++
	return m
}

func newTestCache(maxAge time.Duration, now *time.Time, take *countingTake) *SnapshotCache {
	c := NewSnapshotCache(maxAge)
	c.now = func() time.Time { return *now }
	c.take = take.take
	return c
}

func TestSnapshotReaderIsLazy(t *testing.T) {
	now := time.Unix(0, 0)
	take := &countingTake{maps: []map[int][]int{{1: {2}}}}
	cache := newTestCache(time.Second, &now, take)

	reader := cache.Reader(OSReader{})
	assert.Zero(t, take.calls, "building a reader must not read the process table")

	for range 3 {
		children, err := reader.Children(1)
		require.NoError(t, err)
		assert.Equal(t, []int{2}, children)
	}
	assert.Equal(t, 1, take.calls)
}

func TestSnapshotCacheReusesWithinMaxAge(t *testing.T) {
	now := time.Unix(0, 0)
	take := &countingTake{maps: []map[int][]int{{1: {2}}, {1: {3}}}}
	cache := newTestCache(5*time.Second, &now, take)

	children, _ := cache.Reader(OSReader{}).Children(1)
	assert.Equal(t, []int{2}, children)

	now = now.Add(4 * time.Second)
	children, _ = cache.Reader(OSReader{}).Children(1)
	assert.Equal(t, []int{2}, children)
	assert.Equal(t, 1, take.calls)

	now = now.Add(time.Second)
	children, _ = cache.Reader(OSReader{}).Children(1)
	assert.Equal(t, []int{3}, children, "a snapshot at maxAge is stale")
	assert.Equal(t, 2, take.calls)
}

func TestSnapshotCacheRetriesAFailedSnapshot(t *testing.T) {
	now := time.Unix(0, 0)
	take := &countingTake{maps: []map[int][]int{nil, {1: {2}}}}
	cache := newTestCache(time.Minute, &now, take)

	assert.Nil(t, cache.load())
	assert.Equal(t, map[int][]int{1: {2}}, cache.load())
	assert.Equal(t, 2, take.calls)
}
