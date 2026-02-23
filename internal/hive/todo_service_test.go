package hive

import (
	"context"
	"testing"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/todo"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestTodoService(t *testing.T) *TodoService {
	t.Helper()
	store := newTestTodoStore(t)
	limiter := NewTodoLimiter(store, config.TodosLimiterConfig{
		MaxPending:          100,
		RateLimitPerSession: 0, // disable rate limit for tests
	})
	return NewTodoService(store, limiter, zerolog.Nop())
}

func TestTodoService_Add(t *testing.T) {
	ctx := context.Background()
	svc := newTestTodoService(t)

	err := svc.Add(ctx, todo.Todo{
		ID:     "t1",
		Source: todo.SourceAgent,
		Title:  "review PR",
		URI:    todo.ParseURI("session://abc"),
	})
	require.NoError(t, err)

	got, err := svc.Get(ctx, "t1")
	require.NoError(t, err)
	assert.Equal(t, "review PR", got.Title)
	assert.Equal(t, todo.StatusPending, got.Status)
	assert.False(t, got.CreatedAt.IsZero())
}

func TestTodoService_Add_InvalidURI(t *testing.T) {
	ctx := context.Background()
	svc := newTestTodoService(t)

	err := svc.Add(ctx, todo.Todo{
		ID:     "t1",
		Source: todo.SourceAgent,
		Title:  "bad uri",
		URI:    todo.ParseURI("bare-string"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid URI")
}

func TestTodoService_Add_EmptyURI(t *testing.T) {
	ctx := context.Background()
	svc := newTestTodoService(t)

	err := svc.Add(ctx, todo.Todo{
		ID:     "t1",
		Source: todo.SourceAgent,
		Title:  "no uri",
	})
	require.NoError(t, err, "empty URI is allowed")
}

func TestTodoService_Lifecycle(t *testing.T) {
	ctx := context.Background()
	svc := newTestTodoService(t)

	require.NoError(t, svc.Add(ctx, todo.Todo{
		ID: "t1", Source: todo.SourceAgent, Title: "test",
	}))

	// Acknowledge
	require.NoError(t, svc.Acknowledge(ctx, "t1"))
	got, _ := svc.Get(ctx, "t1")
	assert.Equal(t, todo.StatusAcknowledged, got.Status)

	// Complete
	require.NoError(t, svc.Complete(ctx, "t1"))
	got, _ = svc.Get(ctx, "t1")
	assert.Equal(t, todo.StatusCompleted, got.Status)
}

func TestTodoService_CountPending(t *testing.T) {
	ctx := context.Background()
	svc := newTestTodoService(t)

	require.NoError(t, svc.Add(ctx, todo.Todo{
		ID: "t1", Source: todo.SourceAgent, Title: "pending",
	}))
	require.NoError(t, svc.Add(ctx, todo.Todo{
		ID: "t2", Source: todo.SourceAgent, Title: "also pending",
	}))

	count, err := svc.CountPending(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	require.NoError(t, svc.Complete(ctx, "t1"))
	count, err = svc.CountPending(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestTodoService_Add_RateLimited(t *testing.T) {
	ctx := context.Background()
	store := newTestTodoStore(t)
	limiter := NewTodoLimiter(store, config.TodosLimiterConfig{
		MaxPending:          100,
		RateLimitPerSession: 15_000_000_000, // 15s as Duration
	})
	svc := NewTodoService(store, limiter, zerolog.Nop())

	require.NoError(t, svc.Add(ctx, todo.Todo{
		ID: "t1", SessionID: "sess-1", Source: todo.SourceAgent, Title: "first",
	}))

	err := svc.Add(ctx, todo.Todo{
		ID: "t2", SessionID: "sess-1", Source: todo.SourceAgent, Title: "second",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rate limited")
}
