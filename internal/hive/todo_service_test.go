package hive

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/eventbus"
	"github.com/colonyops/hive/internal/core/eventbus/testbus"
	"github.com/colonyops/hive/internal/core/todo"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/data/stores"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestTodoService(t *testing.T) (*TodoService, *testbus.Bus) {
	t.Helper()

	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })

	store := stores.NewTodoStore(database)
	tb := testbus.New(t)
	log := zerolog.Nop()

	svc := NewTodoService(store, tb.EventBus, log)
	return svc, tb
}

func TestTodoService_HandleFileEvent(t *testing.T) {
	ctx := context.Background()

	t.Run("creates item from file with frontmatter", func(t *testing.T) {
		svc, tb := newTestTodoService(t)

		// Create a temp file with frontmatter
		dir := t.TempDir()
		filePath := filepath.Join(dir, "plan.md")
		err := os.WriteFile(filePath, []byte("---\nsession_id: sess-1\ntitle: My Plan\n---\n# Content\n"), 0o644)
		require.NoError(t, err)

		require.NoError(t, svc.HandleFileEvent(ctx, filePath, "https://github.com/org/repo"))

		items, err := svc.ListPending(ctx, todo.ListFilter{})
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "My Plan", items[0].Title)
		assert.Equal(t, "sess-1", items[0].SessionID)
		assert.Equal(t, todo.ItemTypeFileChange, items[0].Type)
		assert.Equal(t, filePath, items[0].FilePath)

		tb.AssertPublished(t, eventbus.EventTodoCreated)
	})

	t.Run("creates item from file without frontmatter", func(t *testing.T) {
		svc, _ := newTestTodoService(t)

		dir := t.TempDir()
		filePath := filepath.Join(dir, "notes.md")
		err := os.WriteFile(filePath, []byte("# Just notes\n"), 0o644)
		require.NoError(t, err)

		require.NoError(t, svc.HandleFileEvent(ctx, filePath, "repo"))

		items, err := svc.ListPending(ctx, todo.ListFilter{})
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "notes.md", items[0].Title)
		assert.Empty(t, items[0].SessionID)
	})

	t.Run("deduplicates pending items for same path", func(t *testing.T) {
		svc, _ := newTestTodoService(t)

		dir := t.TempDir()
		filePath := filepath.Join(dir, "plan.md")
		require.NoError(t, os.WriteFile(filePath, []byte("content"), 0o644))

		require.NoError(t, svc.HandleFileEvent(ctx, filePath, "repo"))
		require.NoError(t, svc.HandleFileEvent(ctx, filePath, "repo"))

		items, err := svc.ListPending(ctx, todo.ListFilter{})
		require.NoError(t, err)
		assert.Len(t, items, 1)
	})

	t.Run("handles missing file gracefully", func(t *testing.T) {
		svc, _ := newTestTodoService(t)

		require.NoError(t, svc.HandleFileEvent(ctx, "/nonexistent/file.md", "repo"))

		items, err := svc.ListPending(ctx, todo.ListFilter{})
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "file.md", items[0].Title)
	})
}

func TestTodoService_HandleFileDelete(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestTodoService(t)

	dir := t.TempDir()
	filePath := filepath.Join(dir, "plan.md")
	require.NoError(t, os.WriteFile(filePath, []byte("content"), 0o644))
	require.NoError(t, svc.HandleFileEvent(ctx, filePath, "repo"))

	require.NoError(t, svc.HandleFileDelete(ctx, filePath))

	items, err := svc.ListPending(ctx, todo.ListFilter{})
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestTodoService_CreateCustom(t *testing.T) {
	ctx := context.Background()

	t.Run("creates custom item", func(t *testing.T) {
		svc, tb := newTestTodoService(t)

		err := svc.CreateCustom(ctx, todo.Item{
			Title:      "Please review PR #42",
			SessionID:  "sess-1",
			RepoRemote: "repo",
		})
		require.NoError(t, err)

		items, err := svc.ListPending(ctx, todo.ListFilter{})
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, todo.ItemTypeCustom, items[0].Type)
		assert.Equal(t, "Please review PR #42", items[0].Title)

		tb.AssertPublished(t, eventbus.EventTodoCreated)
	})

	t.Run("rate limits custom items per session", func(t *testing.T) {
		svc, _ := newTestTodoService(t)
		svc.rateLimit = 3
		svc.rateDuration = time.Hour

		for i := range 3 {
			err := svc.CreateCustom(ctx, todo.Item{
				Title:      "Task",
				SessionID:  "sess-1",
				RepoRemote: "repo",
				CreatedAt:  time.Now().Add(time.Duration(i) * time.Millisecond),
			})
			require.NoError(t, err)
		}

		// 4th should fail
		err := svc.CreateCustom(ctx, todo.Item{
			Title:      "Too many",
			SessionID:  "sess-1",
			RepoRemote: "repo",
		})
		require.ErrorIs(t, err, todo.ErrRateLimited)
	})

	t.Run("rate limit does not apply without session ID", func(t *testing.T) {
		svc, _ := newTestTodoService(t)
		svc.rateLimit = 1

		for i := range 3 {
			err := svc.CreateCustom(ctx, todo.Item{
				Title:      "Task",
				RepoRemote: "repo",
				CreatedAt:  time.Now().Add(time.Duration(i) * time.Millisecond),
			})
			require.NoError(t, err)
		}
	})
}

func TestTodoService_DismissAndComplete(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestTodoService(t)

	dir := t.TempDir()
	filePath := filepath.Join(dir, "plan.md")
	require.NoError(t, os.WriteFile(filePath, []byte("content"), 0o644))
	require.NoError(t, svc.HandleFileEvent(ctx, filePath, "repo"))

	items, err := svc.ListPending(ctx, todo.ListFilter{})
	require.NoError(t, err)
	require.Len(t, items, 1)

	// Dismiss
	require.NoError(t, svc.Dismiss(ctx, items[0].ID))

	pending, err := svc.ListPending(ctx, todo.ListFilter{})
	require.NoError(t, err)
	assert.Empty(t, pending)

	// Create another and complete it
	filePath2 := filepath.Join(dir, "plan2.md")
	require.NoError(t, os.WriteFile(filePath2, []byte("content"), 0o644))
	require.NoError(t, svc.HandleFileEvent(ctx, filePath2, "repo"))

	items, err = svc.ListPending(ctx, todo.ListFilter{})
	require.NoError(t, err)
	require.Len(t, items, 1)

	require.NoError(t, svc.Complete(ctx, items[0].ID))

	pending, err = svc.ListPending(ctx, todo.ListFilter{})
	require.NoError(t, err)
	assert.Empty(t, pending)
}

func TestTodoService_CompleteByPath(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestTodoService(t)

	dir := t.TempDir()
	path1 := filepath.Join(dir, "a.md")
	path2 := filepath.Join(dir, "b.md")
	require.NoError(t, os.WriteFile(path1, []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(path2, []byte("b"), 0o644))

	require.NoError(t, svc.HandleFileEvent(ctx, path1, "repo"))
	require.NoError(t, svc.HandleFileEvent(ctx, path2, "repo"))

	require.NoError(t, svc.CompleteByPath(ctx, path1))

	items, err := svc.ListPending(ctx, todo.ListFilter{})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, path2, items[0].FilePath)
}

func TestTodoService_CountPending(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestTodoService(t)

	count, err := svc.CountPending(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	dir := t.TempDir()
	for _, name := range []string{"a.md", "b.md"} {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte("---\nsession_id: s1\n---\n"), 0o644))
		require.NoError(t, svc.HandleFileEvent(ctx, p, "repo"))
	}

	count, err = svc.CountPending(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	count, err = svc.CountPendingBySession(ctx, "s1")
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	count, err = svc.CountPendingBySession(ctx, "s2")
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}
