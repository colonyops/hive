package stores

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/todo"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTodoStore(t *testing.T) {
	ctx := context.Background()

	t.Run("create and get", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewTodoStore(database)

		now := time.Now()
		item := todo.Item{
			ID:          "test-id-1",
			Type:        todo.ItemTypeFileChange,
			Status:      todo.StatusPending,
			Title:       "Review plan.md",
			Description: "New plan file detected",
			FilePath:    "/repo/.hive/plans/plan.md",
			SessionID:   "session-abc",
			RepoRemote:  "https://github.com/org/repo",
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		require.NoError(t, store.Create(ctx, &item))

		got, err := store.Get(ctx, "test-id-1")
		require.NoError(t, err)
		assert.Equal(t, item.ID, got.ID)
		assert.Equal(t, todo.ItemTypeFileChange, got.Type)
		assert.Equal(t, todo.StatusPending, got.Status)
		assert.Equal(t, "Review plan.md", got.Title)
		assert.Equal(t, "New plan file detected", got.Description)
		assert.Equal(t, "/repo/.hive/plans/plan.md", got.FilePath)
		assert.Equal(t, "session-abc", got.SessionID)
		assert.Equal(t, "https://github.com/org/repo", got.RepoRemote)
	})

	t.Run("create generates ID when empty", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewTodoStore(database)

		item := todo.Item{
			Type:       todo.ItemTypeCustom,
			Title:      "Custom task",
			RepoRemote: "https://github.com/org/repo",
		}

		require.NoError(t, store.Create(ctx, &item))

		// Verify by listing all
		items, err := store.List(ctx, todo.ListFilter{})
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.NotEmpty(t, items[0].ID)
		assert.Equal(t, todo.StatusPending, items[0].Status)
	})

	t.Run("get not found", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewTodoStore(database)

		_, err = store.Get(ctx, "nonexistent")
		assert.ErrorIs(t, err, todo.ErrNotFound)
	})

	t.Run("list returns empty slice", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewTodoStore(database)

		items, err := store.List(ctx, todo.ListFilter{})
		require.NoError(t, err)
		assert.Empty(t, items)
		assert.NotNil(t, items)
	})

	t.Run("list filters by status", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewTodoStore(database)

		base := time.Now()
		for i, status := range []todo.Status{todo.StatusPending, todo.StatusCompleted, todo.StatusPending} {
			require.NoError(t, store.Create(ctx, &todo.Item{
				ID:         fmt.Sprintf("item-%d", i),
				Type:       todo.ItemTypeCustom,
				Status:     status,
				Title:      fmt.Sprintf("Task %d", i),
				RepoRemote: "https://github.com/org/repo",
				CreatedAt:  base.Add(time.Duration(i) * time.Second),
				UpdatedAt:  base.Add(time.Duration(i) * time.Second),
			}))
		}

		pending, err := store.List(ctx, todo.ListFilter{Status: todo.StatusPending})
		require.NoError(t, err)
		assert.Len(t, pending, 2)

		completed, err := store.List(ctx, todo.ListFilter{Status: todo.StatusCompleted})
		require.NoError(t, err)
		assert.Len(t, completed, 1)
	})

	t.Run("list filters by session", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewTodoStore(database)

		now := time.Now()
		require.NoError(t, store.Create(ctx, &todo.Item{
			ID: "s1", Type: todo.ItemTypeCustom, Status: todo.StatusPending,
			Title: "From session A", SessionID: "sess-a", RepoRemote: "repo",
			CreatedAt: now, UpdatedAt: now,
		}))
		require.NoError(t, store.Create(ctx, &todo.Item{
			ID: "s2", Type: todo.ItemTypeCustom, Status: todo.StatusPending,
			Title: "From session B", SessionID: "sess-b", RepoRemote: "repo",
			CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second),
		}))

		items, err := store.List(ctx, todo.ListFilter{SessionID: "sess-a"})
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "From session A", items[0].Title)
	})

	t.Run("list filters by repo", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewTodoStore(database)

		now := time.Now()
		require.NoError(t, store.Create(ctx, &todo.Item{
			ID: "r1", Type: todo.ItemTypeCustom, Status: todo.StatusPending,
			Title: "Repo A item", RepoRemote: "repo-a",
			CreatedAt: now, UpdatedAt: now,
		}))
		require.NoError(t, store.Create(ctx, &todo.Item{
			ID: "r2", Type: todo.ItemTypeCustom, Status: todo.StatusPending,
			Title: "Repo B item", RepoRemote: "repo-b",
			CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second),
		}))

		items, err := store.List(ctx, todo.ListFilter{RepoRemote: "repo-a"})
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "Repo A item", items[0].Title)
	})

	t.Run("update status", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewTodoStore(database)

		now := time.Now()
		require.NoError(t, store.Create(ctx, &todo.Item{
			ID: "upd-1", Type: todo.ItemTypeCustom, Status: todo.StatusPending,
			Title: "Pending task", RepoRemote: "repo",
			CreatedAt: now, UpdatedAt: now,
		}))

		require.NoError(t, store.UpdateStatus(ctx, "upd-1", todo.StatusCompleted))

		got, err := store.Get(ctx, "upd-1")
		require.NoError(t, err)
		assert.Equal(t, todo.StatusCompleted, got.Status)
		assert.True(t, got.UpdatedAt.After(now))
	})

	t.Run("update status not found", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewTodoStore(database)

		err = store.UpdateStatus(ctx, "nonexistent", todo.StatusCompleted)
		assert.ErrorIs(t, err, todo.ErrNotFound)
	})

	t.Run("dismiss by path", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewTodoStore(database)

		now := time.Now()
		require.NoError(t, store.Create(ctx, &todo.Item{
			ID: "d1", Type: todo.ItemTypeFileChange, Status: todo.StatusPending,
			Title: "File changed", FilePath: "/repo/plan.md", RepoRemote: "repo",
			CreatedAt: now, UpdatedAt: now,
		}))
		require.NoError(t, store.Create(ctx, &todo.Item{
			ID: "d2", Type: todo.ItemTypeFileChange, Status: todo.StatusPending,
			Title: "Other file", FilePath: "/repo/other.md", RepoRemote: "repo",
			CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second),
		}))

		require.NoError(t, store.DismissByPath(ctx, "/repo/plan.md"))

		got, err := store.Get(ctx, "d1")
		require.NoError(t, err)
		assert.Equal(t, todo.StatusDismissed, got.Status)

		// Other item unaffected
		got2, err := store.Get(ctx, "d2")
		require.NoError(t, err)
		assert.Equal(t, todo.StatusPending, got2.Status)
	})

	t.Run("deduplication by file path", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewTodoStore(database)

		now := time.Now()
		require.NoError(t, store.Create(ctx, &todo.Item{
			ID: "dup1", Type: todo.ItemTypeFileChange, Status: todo.StatusPending,
			Title: "First", FilePath: "/repo/plan.md", RepoRemote: "repo",
			CreatedAt: now, UpdatedAt: now,
		}))

		// Second pending item for same path should fail
		err = store.Create(ctx, &todo.Item{
			ID: "dup2", Type: todo.ItemTypeFileChange, Status: todo.StatusPending,
			Title: "Second", FilePath: "/repo/plan.md", RepoRemote: "repo",
			CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second),
		})
		require.ErrorIs(t, err, todo.ErrDuplicate)

		// After dismissing, a new pending item for the same path should succeed
		require.NoError(t, store.DismissByPath(ctx, "/repo/plan.md"))

		err = store.Create(ctx, &todo.Item{
			ID: "dup3", Type: todo.ItemTypeFileChange, Status: todo.StatusPending,
			Title: "Third", FilePath: "/repo/plan.md", RepoRemote: "repo",
			CreatedAt: now.Add(2 * time.Second), UpdatedAt: now.Add(2 * time.Second),
		})
		require.NoError(t, err)
	})

	t.Run("count pending", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewTodoStore(database)

		count, err := store.CountPending(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(0), count)

		now := time.Now()
		for i := range 3 {
			require.NoError(t, store.Create(ctx, &todo.Item{
				ID: fmt.Sprintf("cp-%d", i), Type: todo.ItemTypeCustom, Status: todo.StatusPending,
				Title: "Task", RepoRemote: "repo",
				CreatedAt: now.Add(time.Duration(i) * time.Millisecond),
				UpdatedAt: now.Add(time.Duration(i) * time.Millisecond),
			}))
		}

		count, err = store.CountPending(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(3), count)

		// Complete one, count should decrease
		require.NoError(t, store.UpdateStatus(ctx, "cp-0", todo.StatusCompleted))

		count, err = store.CountPending(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(2), count)
	})

	t.Run("count pending by session", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewTodoStore(database)

		now := time.Now()
		require.NoError(t, store.Create(ctx, &todo.Item{
			ID: "cps1", Type: todo.ItemTypeCustom, Status: todo.StatusPending,
			Title: "A", SessionID: "sess-x", RepoRemote: "repo",
			CreatedAt: now, UpdatedAt: now,
		}))
		require.NoError(t, store.Create(ctx, &todo.Item{
			ID: "cps2", Type: todo.ItemTypeCustom, Status: todo.StatusPending,
			Title: "B", SessionID: "sess-x", RepoRemote: "repo",
			CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second),
		}))
		require.NoError(t, store.Create(ctx, &todo.Item{
			ID: "cps3", Type: todo.ItemTypeCustom, Status: todo.StatusPending,
			Title: "C", SessionID: "sess-y", RepoRemote: "repo",
			CreatedAt: now.Add(2 * time.Second), UpdatedAt: now.Add(2 * time.Second),
		}))

		count, err := store.CountPendingBySession(ctx, "sess-x")
		require.NoError(t, err)
		assert.Equal(t, int64(2), count)

		count, err = store.CountPendingBySession(ctx, "sess-y")
		require.NoError(t, err)
		assert.Equal(t, int64(1), count)
	})

	t.Run("count custom by session since", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewTodoStore(database)

		base := time.Now()
		// Create 3 custom items at base, base+1h, base+2h
		for i := range 3 {
			require.NoError(t, store.Create(ctx, &todo.Item{
				ID: fmt.Sprintf("rate-%d", i), Type: todo.ItemTypeCustom, Status: todo.StatusPending,
				Title: "Rate limited", SessionID: "agent-1", RepoRemote: "repo",
				CreatedAt: base.Add(time.Duration(i) * time.Hour),
				UpdatedAt: base.Add(time.Duration(i) * time.Hour),
			}))
		}

		// Count since base+30min should return 2 (items at 1h and 2h)
		count, err := store.CountCustomBySessionSince(ctx, "agent-1", base.Add(30*time.Minute))
		require.NoError(t, err)
		assert.Equal(t, int64(2), count)

		// Count since base should return 3
		count, err = store.CountCustomBySessionSince(ctx, "agent-1", base)
		require.NoError(t, err)
		assert.Equal(t, int64(3), count)
	})

	t.Run("list ordered by created_at desc", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewTodoStore(database)

		base := time.Now()
		for i, title := range []string{"first", "second", "third"} {
			require.NoError(t, store.Create(ctx, &todo.Item{
				ID: fmt.Sprintf("ord-%d", i), Type: todo.ItemTypeCustom, Status: todo.StatusPending,
				Title: title, RepoRemote: "repo",
				CreatedAt: base.Add(time.Duration(i) * time.Second),
				UpdatedAt: base.Add(time.Duration(i) * time.Second),
			}))
		}

		items, err := store.List(ctx, todo.ListFilter{})
		require.NoError(t, err)
		require.Len(t, items, 3)
		assert.Equal(t, "third", items[0].Title)
		assert.Equal(t, "second", items[1].Title)
		assert.Equal(t, "first", items[2].Title)
	})
}
