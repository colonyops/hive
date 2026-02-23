package hive

import (
	"context"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/todo"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/data/stores"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestTodoStore(t *testing.T) todo.Store {
	t.Helper()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	return stores.NewTodoStore(database)
}

func TestTodoLimiter_MaxPending(t *testing.T) {
	ctx := context.Background()
	store := newTestTodoStore(t)
	limiter := NewTodoLimiter(store, config.TodosLimiterConfig{
		MaxPending:          2,
		RateLimitPerSession: 0, // disabled
	})

	now := time.Now()
	for i := range 2 {
		require.NoError(t, store.Create(ctx, todo.Todo{
			ID: "t" + string(rune('0'+i)), Source: todo.SourceAgent,
			Title: "item", Status: todo.StatusPending,
			CreatedAt: now.Add(time.Duration(i) * time.Millisecond),
			UpdatedAt: now.Add(time.Duration(i) * time.Millisecond),
		}))
	}

	err := limiter.Check(ctx, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max pending")

	// Complete one → should allow again
	require.NoError(t, store.Update(ctx, "t0", todo.StatusCompleted))
	require.NoError(t, limiter.Check(ctx, ""))
}

func TestTodoLimiter_RateLimit(t *testing.T) {
	ctx := context.Background()
	store := newTestTodoStore(t)
	limiter := NewTodoLimiter(store, config.TodosLimiterConfig{
		MaxPending:          100,
		RateLimitPerSession: 15 * time.Second,
	})

	now := time.Now()
	require.NoError(t, store.Create(ctx, todo.Todo{
		ID: "t1", SessionID: "sess-1", Source: todo.SourceAgent,
		Title: "recent", Status: todo.StatusPending,
		CreatedAt: now, UpdatedAt: now,
	}))

	err := limiter.Check(ctx, "sess-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rate limited")

	// Different session should be fine
	require.NoError(t, limiter.Check(ctx, "sess-2"))

	// Empty session ID skips rate limit
	require.NoError(t, limiter.Check(ctx, ""))
}
