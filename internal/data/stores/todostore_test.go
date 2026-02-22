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

func newTestTodo(id string) todo.Todo {
	now := time.Now()
	return todo.Todo{
		ID:        id,
		SessionID: "sess-1",
		Source:    todo.SourceAgent,
		Category:  todo.CategoryReview,
		Title:     "Test todo " + id,
		Ref:       "test.md",
		Status:    todo.StatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestTodoStore(t *testing.T) {
	ctx := context.Background()

	t.Run("create and get", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewTodoStore(database)
		td := newTestTodo("t1")

		require.NoError(t, store.Create(ctx, td))

		got, err := store.Get(ctx, "t1")
		require.NoError(t, err)
		assert.Equal(t, "t1", got.ID)
		assert.Equal(t, "sess-1", got.SessionID)
		assert.Equal(t, todo.SourceAgent, got.Source)
		assert.Equal(t, todo.CategoryReview, got.Category)
		assert.Equal(t, "Test todo t1", got.Title)
		assert.Equal(t, "test.md", got.Ref)
		assert.Equal(t, todo.StatusPending, got.Status)
		assert.True(t, got.CompletedAt.IsZero())
	})

	t.Run("create rejects invalid source", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewTodoStore(database)
		td := newTestTodo("t1")
		td.Source = "invalid"

		err = store.Create(ctx, td)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid source")
	})

	t.Run("create rejects invalid category", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewTodoStore(database)
		td := newTestTodo("t1")
		td.Category = "invalid"

		err = store.Create(ctx, td)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid category")
	})

	t.Run("update status to completed", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewTodoStore(database)
		td := newTestTodo("t1")
		require.NoError(t, store.Create(ctx, td))

		require.NoError(t, store.Update(ctx, "t1", todo.StatusCompleted))

		got, err := store.Get(ctx, "t1")
		require.NoError(t, err)
		assert.Equal(t, todo.StatusCompleted, got.Status)
		assert.False(t, got.CompletedAt.IsZero())
	})

	t.Run("update status to acknowledged keeps CompletedAt zero", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewTodoStore(database)
		td := newTestTodo("t1")
		require.NoError(t, store.Create(ctx, td))

		require.NoError(t, store.Update(ctx, "t1", todo.StatusAcknowledged))

		got, err := store.Get(ctx, "t1")
		require.NoError(t, err)
		assert.Equal(t, todo.StatusAcknowledged, got.Status)
		assert.True(t, got.CompletedAt.IsZero())
	})

	t.Run("list all", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewTodoStore(database)

		now := time.Now()
		for i, id := range []string{"t1", "t2", "t3"} {
			td := newTestTodo(id)
			td.CreatedAt = now.Add(time.Duration(i) * time.Second)
			td.UpdatedAt = td.CreatedAt
			require.NoError(t, store.Create(ctx, td))
		}

		items, err := store.List(ctx, todo.ListFilter{})
		require.NoError(t, err)
		require.Len(t, items, 3)
		// Newest first
		assert.Equal(t, "t3", items[0].ID)
		assert.Equal(t, "t2", items[1].ID)
		assert.Equal(t, "t1", items[2].ID)
	})

	t.Run("list by status", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewTodoStore(database)

		now := time.Now()
		for i, id := range []string{"t1", "t2", "t3"} {
			td := newTestTodo(id)
			td.CreatedAt = now.Add(time.Duration(i) * time.Second)
			td.UpdatedAt = td.CreatedAt
			require.NoError(t, store.Create(ctx, td))
		}

		require.NoError(t, store.Update(ctx, "t2", todo.StatusCompleted))

		status := todo.StatusPending
		items, err := store.List(ctx, todo.ListFilter{Status: &status})
		require.NoError(t, err)
		require.Len(t, items, 2)
	})

	t.Run("list by session", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewTodoStore(database)

		td1 := newTestTodo("t1")
		td1.SessionID = "sess-a"
		td2 := newTestTodo("t2")
		td2.SessionID = "sess-b"
		require.NoError(t, store.Create(ctx, td1))
		require.NoError(t, store.Create(ctx, td2))

		items, err := store.List(ctx, todo.ListFilter{SessionID: "sess-a"})
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "t1", items[0].ID)
	})

	t.Run("count pending", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewTodoStore(database)

		count, err := store.CountPending(ctx)
		require.NoError(t, err)
		assert.Equal(t, 0, count)

		require.NoError(t, store.Create(ctx, newTestTodo("t1")))
		require.NoError(t, store.Create(ctx, newTestTodo("t2")))

		count, err = store.CountPending(ctx)
		require.NoError(t, err)
		assert.Equal(t, 2, count)

		require.NoError(t, store.Update(ctx, "t1", todo.StatusCompleted))

		count, err = store.CountPending(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("count recent by session", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewTodoStore(database)

		now := time.Now()
		td := newTestTodo("t1")
		td.CreatedAt = now
		td.UpdatedAt = now
		require.NoError(t, store.Create(ctx, td))

		count, err := store.CountRecentBySession(ctx, "sess-1", now.Add(-time.Minute))
		require.NoError(t, err)
		assert.Equal(t, 1, count)

		count, err = store.CountRecentBySession(ctx, "sess-1", now.Add(time.Minute))
		require.NoError(t, err)
		assert.Equal(t, 0, count)

		count, err = store.CountRecentBySession(ctx, "other-sess", now.Add(-time.Minute))
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("delete", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewTodoStore(database)
		require.NoError(t, store.Create(ctx, newTestTodo("t1")))

		require.NoError(t, store.Delete(ctx, "t1"))

		items, err := store.List(ctx, todo.ListFilter{})
		require.NoError(t, err)
		assert.Empty(t, items)
	})

	t.Run("empty list returns empty slice", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewTodoStore(database)

		items, err := store.List(ctx, todo.ListFilter{})
		require.NoError(t, err)
		assert.Empty(t, items)
		assert.NotNil(t, items)
	})
}
