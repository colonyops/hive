package stores

import (
	"context"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/todo"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTodoStore(t *testing.T) {
	ctx := context.Background()

	newStore := func(t *testing.T) *TodoStore {
		t.Helper()
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		t.Cleanup(func() { _ = database.Close() })
		return NewTodoStore(database)
	}

	t.Run("create and get", func(t *testing.T) {
		store := newStore(t)
		now := time.Now()

		err := store.Create(ctx, todo.Todo{
			ID:        "todo-1",
			SessionID: "sess-abc",
			Source:    todo.SourceAgent,
			Title:     "review PR",
			URI:       todo.ParseURI("session://abc123"),
			Status:    todo.StatusPending,
			CreatedAt: now,
			UpdatedAt: now,
		})
		require.NoError(t, err)

		got, err := store.Get(ctx, "todo-1")
		require.NoError(t, err)
		assert.Equal(t, "review PR", got.Title)
		assert.Equal(t, todo.StatusPending, got.Status)
		assert.Equal(t, todo.SourceAgent, got.Source)
		assert.Equal(t, "sess-abc", got.SessionID)
		assert.Equal(t, "session", got.URI.Scheme)
		assert.Equal(t, "abc123", got.URI.Value)
		assert.True(t, got.CompletedAt.IsZero())
	})

	t.Run("get not found", func(t *testing.T) {
		store := newStore(t)

		_, err := store.Get(ctx, "nonexistent")
		require.ErrorIs(t, err, todo.ErrNotFound)
	})

	t.Run("create validates source", func(t *testing.T) {
		store := newStore(t)
		now := time.Now()

		err := store.Create(ctx, todo.Todo{
			ID: "todo-bad", Source: todo.Source("invalid"),
			Title: "bad", Status: todo.StatusPending,
			CreatedAt: now, UpdatedAt: now,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid source")
	})

	t.Run("create validates status", func(t *testing.T) {
		store := newStore(t)
		now := time.Now()

		err := store.Create(ctx, todo.Todo{
			ID: "todo-bad", Source: todo.SourceAgent,
			Title: "bad", Status: todo.Status("nope"),
			CreatedAt: now, UpdatedAt: now,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid status")
	})

	t.Run("list returns newest first", func(t *testing.T) {
		store := newStore(t)
		base := time.Now()

		for i, title := range []string{"first", "second", "third"} {
			err := store.Create(ctx, todo.Todo{
				ID: title, Source: todo.SourceAgent,
				Title: title, Status: todo.StatusPending,
				URI:       todo.ParseURI("session://s" + title),
				CreatedAt: base.Add(time.Duration(i) * time.Second),
				UpdatedAt: base.Add(time.Duration(i) * time.Second),
			})
			require.NoError(t, err)
		}

		items, err := store.List(ctx, todo.ListFilter{})
		require.NoError(t, err)
		require.Len(t, items, 3)
		assert.Equal(t, "third", items[0].Title)
		assert.Equal(t, "second", items[1].Title)
		assert.Equal(t, "first", items[2].Title)
	})

	t.Run("list filter by status", func(t *testing.T) {
		store := newStore(t)
		now := time.Now()

		err := store.Create(ctx, todo.Todo{
			ID: "p1", Source: todo.SourceAgent,
			Title: "pending item", Status: todo.StatusPending,
			URI: todo.ParseURI("session://s1"), CreatedAt: now, UpdatedAt: now,
		})
		require.NoError(t, err)

		err = store.Create(ctx, todo.Todo{
			ID: "c1", Source: todo.SourceAgent,
			Title: "completed item", Status: todo.StatusCompleted,
			URI: todo.ParseURI("session://s2"), CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second),
		})
		require.NoError(t, err)

		pending := todo.StatusPending
		items, err := store.List(ctx, todo.ListFilter{Status: &pending})
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "pending item", items[0].Title)

		completed := todo.StatusCompleted
		items, err = store.List(ctx, todo.ListFilter{Status: &completed})
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "completed item", items[0].Title)
	})

	t.Run("list filter by scheme", func(t *testing.T) {
		store := newStore(t)
		now := time.Now()

		err := store.Create(ctx, todo.Todo{
			ID: "s1", Source: todo.SourceAgent,
			Title: "session todo", Status: todo.StatusPending,
			URI: todo.ParseURI("session://s1"), CreatedAt: now, UpdatedAt: now,
		})
		require.NoError(t, err)

		err = store.Create(ctx, todo.Todo{
			ID: "h1", Source: todo.SourceAgent,
			Title: "review PR", Status: todo.StatusPending,
			URI: todo.ParseURI("https://github.com/org/repo/pull/42"), CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second),
		})
		require.NoError(t, err)

		items, err := store.List(ctx, todo.ListFilter{Scheme: "session"})
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "session todo", items[0].Title)

		items, err = store.List(ctx, todo.ListFilter{Scheme: "https"})
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "review PR", items[0].Title)
	})

	t.Run("list filter by session ID", func(t *testing.T) {
		store := newStore(t)
		now := time.Now()

		err := store.Create(ctx, todo.Todo{
			ID: "a1", SessionID: "sess-a", Source: todo.SourceAgent,
			Title: "from session a", Status: todo.StatusPending,
			CreatedAt: now, UpdatedAt: now,
		})
		require.NoError(t, err)

		err = store.Create(ctx, todo.Todo{
			ID: "b1", SessionID: "sess-b", Source: todo.SourceAgent,
			Title: "from session b", Status: todo.StatusPending,
			CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second),
		})
		require.NoError(t, err)

		items, err := store.List(ctx, todo.ListFilter{SessionID: "sess-a"})
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "from session a", items[0].Title)
	})

	t.Run("update status", func(t *testing.T) {
		store := newStore(t)
		now := time.Now()

		err := store.Create(ctx, todo.Todo{
			ID: "u1", Source: todo.SourceAgent,
			Title: "to complete", Status: todo.StatusPending,
			URI: todo.ParseURI("session://s1"), CreatedAt: now, UpdatedAt: now,
		})
		require.NoError(t, err)

		err = store.Update(ctx, "u1", todo.StatusCompleted)
		require.NoError(t, err)

		got, err := store.Get(ctx, "u1")
		require.NoError(t, err)
		assert.Equal(t, todo.StatusCompleted, got.Status)
		assert.False(t, got.CompletedAt.IsZero(), "CompletedAt should be set")
	})

	t.Run("update validates status", func(t *testing.T) {
		store := newStore(t)
		now := time.Now()

		err := store.Create(ctx, todo.Todo{
			ID: "u2", Source: todo.SourceAgent,
			Title: "test", Status: todo.StatusPending,
			CreatedAt: now, UpdatedAt: now,
		})
		require.NoError(t, err)

		err = store.Update(ctx, "u2", todo.Status("invalid"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid status")
	})

	t.Run("delete", func(t *testing.T) {
		store := newStore(t)
		now := time.Now()

		err := store.Create(ctx, todo.Todo{
			ID: "d1", Source: todo.SourceAgent,
			Title: "delete me", Status: todo.StatusPending,
			URI: todo.ParseURI("session://s1"), CreatedAt: now, UpdatedAt: now,
		})
		require.NoError(t, err)

		require.NoError(t, store.Delete(ctx, "d1"))

		_, err = store.Get(ctx, "d1")
		require.ErrorIs(t, err, todo.ErrNotFound)
	})

	t.Run("count pending", func(t *testing.T) {
		store := newStore(t)
		now := time.Now()

		count, err := store.CountPending(ctx)
		require.NoError(t, err)
		assert.Equal(t, 0, count)

		err = store.Create(ctx, todo.Todo{
			ID: "cp1", Source: todo.SourceAgent,
			Title: "pending", Status: todo.StatusPending,
			CreatedAt: now, UpdatedAt: now,
		})
		require.NoError(t, err)

		err = store.Create(ctx, todo.Todo{
			ID: "cp2", Source: todo.SourceAgent,
			Title: "acknowledged", Status: todo.StatusAcknowledged,
			CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second),
		})
		require.NoError(t, err)

		count, err = store.CountPending(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("count open", func(t *testing.T) {
		store := newStore(t)
		now := time.Now()

		for i, status := range []todo.Status{todo.StatusPending, todo.StatusAcknowledged, todo.StatusCompleted} {
			err := store.Create(ctx, todo.Todo{
				ID: string(status), Source: todo.SourceAgent,
				Title: string(status), Status: status,
				CreatedAt: now.Add(time.Duration(i) * time.Millisecond),
				UpdatedAt: now.Add(time.Duration(i) * time.Millisecond),
			})
			require.NoError(t, err)
		}

		count, err := store.CountOpen(ctx)
		require.NoError(t, err)
		assert.Equal(t, 2, count, "pending + acknowledged = 2")
	})

	t.Run("count recent by session", func(t *testing.T) {
		store := newStore(t)
		now := time.Now()

		err := store.Create(ctx, todo.Todo{
			ID: "old", SessionID: "sess-1", Source: todo.SourceAgent,
			Title: "old", Status: todo.StatusPending,
			CreatedAt: now.Add(-time.Minute), UpdatedAt: now.Add(-time.Minute),
		})
		require.NoError(t, err)

		err = store.Create(ctx, todo.Todo{
			ID: "new", SessionID: "sess-1", Source: todo.SourceAgent,
			Title: "new", Status: todo.StatusPending,
			CreatedAt: now, UpdatedAt: now,
		})
		require.NoError(t, err)

		count, err := store.CountRecentBySession(ctx, "sess-1", now.Add(-30*time.Second))
		require.NoError(t, err)
		assert.Equal(t, 1, count)

		count, err = store.CountRecentBySession(ctx, "sess-1", now.Add(-2*time.Minute))
		require.NoError(t, err)
		assert.Equal(t, 2, count)

		count, err = store.CountRecentBySession(ctx, "sess-other", now.Add(-2*time.Minute))
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("empty URI stored and retrieved", func(t *testing.T) {
		store := newStore(t)
		now := time.Now()

		err := store.Create(ctx, todo.Todo{
			ID: "nourl", Source: todo.SourceAgent,
			Title: "no uri", Status: todo.StatusPending,
			CreatedAt: now, UpdatedAt: now,
		})
		require.NoError(t, err)

		got, err := store.Get(ctx, "nourl")
		require.NoError(t, err)
		assert.True(t, got.URI.IsZero())
	})

	t.Run("empty list returns empty slice", func(t *testing.T) {
		store := newStore(t)

		items, err := store.List(ctx, todo.ListFilter{})
		require.NoError(t, err)
		assert.Empty(t, items)
		assert.NotNil(t, items)
	})
}
