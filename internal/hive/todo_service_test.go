package hive

import (
	"context"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/eventbus"
	"github.com/colonyops/hive/internal/core/todo"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/data/stores"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestCfg() *config.Config {
	cfg := config.DefaultConfig()
	cfg.DataDir = "/tmp/test"
	return &cfg
}

func newTestBus(t *testing.T) *eventbus.EventBus {
	t.Helper()
	bus := eventbus.New(16)
	ctx, cancel := context.WithCancel(context.Background())
	go bus.Start(ctx)
	t.Cleanup(cancel)
	return bus
}

func newTestTodoService(t *testing.T) (*TodoService, todo.Store) {
	t.Helper()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })

	store := stores.NewTodoStore(database)
	bus := newTestBus(t)
	cfg := newTestCfg()
	logger := zerolog.Nop()

	svc := NewTodoService(store, bus, cfg, logger)
	return svc, store
}

func TestTodoLimiter(t *testing.T) {
	ctx := context.Background()

	t.Run("allows when under limits", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := stores.NewTodoStore(database)
		limiter := NewTodoLimiter(store, config.TodosLimiterConfig{
			MaxPending:          10,
			RateLimitPerSession: 15 * time.Second,
		})

		td := todo.Todo{
			ID:        "t1",
			SessionID: "sess-1",
			Source:    todo.SourceAgent,
			Status:    todo.StatusPending,
		}

		require.NoError(t, limiter.Check(ctx, td))
	})

	t.Run("rejects when max pending reached", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := stores.NewTodoStore(database)
		limiter := NewTodoLimiter(store, config.TodosLimiterConfig{
			MaxPending:          2,
			RateLimitPerSession: 0,
		})

		now := time.Now()
		for i, id := range []string{"t1", "t2"} {
			err := store.Create(ctx, todo.Todo{
				ID:        id,
				SessionID: "sess-1",
				Source:    todo.SourceAgent,
				URI:       todo.ParseRef("review://test.md"),
				Status:    todo.StatusPending,
				CreatedAt: now.Add(time.Duration(i) * time.Second),
				UpdatedAt: now.Add(time.Duration(i) * time.Second),
			})
			require.NoError(t, err)
		}

		td := todo.Todo{ID: "t3", SessionID: "sess-1"}
		err = limiter.Check(ctx, td)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "max pending")
	})

	t.Run("rejects when rate limited", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := stores.NewTodoStore(database)
		limiter := NewTodoLimiter(store, config.TodosLimiterConfig{
			MaxPending:          100,
			RateLimitPerSession: 15 * time.Second,
		})

		now := time.Now()
		err = store.Create(ctx, todo.Todo{
			ID:        "t1",
			SessionID: "sess-1",
			Source:    todo.SourceAgent,
			URI:       todo.ParseRef("review://test.md"),
			Status:    todo.StatusPending,
			CreatedAt: now,
			UpdatedAt: now,
		})
		require.NoError(t, err)

		td := todo.Todo{ID: "t2", SessionID: "sess-1"}
		err = limiter.Check(ctx, td)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "rate limited")
	})

	t.Run("rate limit allows different sessions", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := stores.NewTodoStore(database)
		limiter := NewTodoLimiter(store, config.TodosLimiterConfig{
			MaxPending:          100,
			RateLimitPerSession: 15 * time.Second,
		})

		now := time.Now()
		err = store.Create(ctx, todo.Todo{
			ID:        "t1",
			SessionID: "sess-1",
			Source:    todo.SourceAgent,
			URI:       todo.ParseRef("review://test.md"),
			Status:    todo.StatusPending,
			CreatedAt: now,
			UpdatedAt: now,
		})
		require.NoError(t, err)

		td := todo.Todo{ID: "t2", SessionID: "sess-2"}
		require.NoError(t, limiter.Check(ctx, td))
	})
}

func TestTodoService(t *testing.T) {
	ctx := context.Background()

	t.Run("add creates todo and publishes event", func(t *testing.T) {
		svc, store := newTestTodoService(t)

		td, err := todo.NewTodo("t1", "Review something", todo.SourceAgent, todo.ParseRef("review://test.md"))
		require.NoError(t, err)

		require.NoError(t, svc.Add(ctx, td))

		got, err := store.Get(ctx, "t1")
		require.NoError(t, err)
		assert.Equal(t, "t1", got.ID)
		assert.Equal(t, todo.StatusPending, got.Status)
		assert.Equal(t, "Review something", got.Title)
	})

	t.Run("NewTodo rejects invalid URI", func(t *testing.T) {
		_, err := todo.NewTodo("t1", "Bad URI", todo.SourceAgent, todo.ParseRef("bare-string"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid URI")
	})

	t.Run("add rejects when limited", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := stores.NewTodoStore(database)
		bus := newTestBus(t)

		cfg := newTestCfg()
		cfg.Todos.Limiter.MaxPending = 1

		svc := NewTodoService(store, bus, cfg, zerolog.Nop())

		td1, err := todo.NewTodo("t1", "First", todo.SourceAgent, todo.ParseRef("review://doc.md"))
		require.NoError(t, err)
		require.NoError(t, svc.Add(ctx, td1))

		td2, err := todo.NewTodo("t2", "Second", todo.SourceAgent, todo.ParseRef("review://doc2.md"))
		require.NoError(t, err)
		err = svc.Add(ctx, td2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "rejected")
	})

	t.Run("acknowledge updates status", func(t *testing.T) {
		svc, store := newTestTodoService(t)

		td, err := todo.NewTodo("t1", "Test", todo.SourceAgent, todo.ParseRef("review://doc.md"))
		require.NoError(t, err)
		require.NoError(t, svc.Add(ctx, td))

		require.NoError(t, svc.Acknowledge(ctx, "t1"))

		got, err := store.Get(ctx, "t1")
		require.NoError(t, err)
		assert.Equal(t, todo.StatusAcknowledged, got.Status)
	})

	t.Run("complete updates status", func(t *testing.T) {
		svc, store := newTestTodoService(t)

		td, err := todo.NewTodo("t1", "Test", todo.SourceAgent, todo.ParseRef("review://doc.md"))
		require.NoError(t, err)
		require.NoError(t, svc.Add(ctx, td))

		require.NoError(t, svc.Complete(ctx, "t1"))

		got, err := store.Get(ctx, "t1")
		require.NoError(t, err)
		assert.Equal(t, todo.StatusCompleted, got.Status)
		assert.False(t, got.CompletedAt.IsZero())
	})

	t.Run("dismiss updates status", func(t *testing.T) {
		svc, store := newTestTodoService(t)

		td, err := todo.NewTodo("t1", "Test", todo.SourceAgent, todo.ParseRef("review://doc.md"))
		require.NoError(t, err)
		require.NoError(t, svc.Add(ctx, td))

		require.NoError(t, svc.Dismiss(ctx, "t1"))

		got, err := store.Get(ctx, "t1")
		require.NoError(t, err)
		assert.Equal(t, todo.StatusDismissed, got.Status)
	})

	t.Run("count pending", func(t *testing.T) {
		svc, _ := newTestTodoService(t)

		count, err := svc.CountPending(ctx)
		require.NoError(t, err)
		assert.Equal(t, 0, count)

		td, err := todo.NewTodo("t1", "Test", todo.SourceAgent, todo.ParseRef("review://doc.md"))
		require.NoError(t, err)
		require.NoError(t, svc.Add(ctx, td))

		count, err = svc.CountPending(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("list with scheme filter", func(t *testing.T) {
		svc, _ := newTestTodoService(t)

		svc.limiter.rateLimitDur = 0

		td1, err := todo.NewTodo("t1", "First", todo.SourceAgent, todo.ParseRef("review://doc.md"))
		require.NoError(t, err)
		require.NoError(t, svc.Add(ctx, td1))

		td2, err := todo.NewTodo("t2", "Second", todo.SourceAgent, todo.ParseRef("session://sess-1"))
		require.NoError(t, err)
		require.NoError(t, svc.Add(ctx, td2))

		items, err := svc.List(ctx, todo.ListFilter{Scheme: "review"})
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "t1", items[0].ID)
	})
}
