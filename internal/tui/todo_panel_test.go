package tui

import (
	"context"
	"testing"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/eventbus"
	"github.com/colonyops/hive/internal/core/todo"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/data/stores"
	"github.com/colonyops/hive/internal/hive"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTodoPanelService(t *testing.T) *hive.TodoService {
	t.Helper()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })

	store := stores.NewTodoStore(database)
	bus := eventbus.New(16)
	ctx, cancel := context.WithCancel(context.Background())
	go bus.Start(ctx)
	t.Cleanup(cancel)

	cfg := config.DefaultConfig()
	cfg.DataDir = t.TempDir()
	return hive.NewTodoService(store, bus, &cfg, zerolog.Nop())
}

func addHumanTodo(t *testing.T, svc *hive.TodoService, id, title string) todo.Todo {
	t.Helper()
	td, err := todo.NewHumanTodo(id, title, todo.Ref{})
	require.NoError(t, err)
	created, err := svc.Add(context.Background(), td)
	require.NoError(t, err)
	return created
}

func newTestTodoPanel(t *testing.T, svc *hive.TodoService) *TodoPanel {
	t.Helper()
	return NewTodoPanel(svc, 120, 40)
}

// --- ReopenCurrent ---

func TestTodoPanel_ReopenCurrent_FromCompleted(t *testing.T) {
	svc := newTodoPanelService(t)
	addHumanTodo(t, svc, "t1", "Write tests")

	// Complete via service directly
	_, err := svc.Complete(context.Background(), "t1")
	require.NoError(t, err)

	// Switch to All filter to see completed item
	p := newTestTodoPanel(t, svc)
	p.filter = todoFilterAll
	p.applyFilter()
	require.Len(t, p.items, 1)
	assert.Equal(t, todo.StatusCompleted, p.items[0].Status)

	require.NoError(t, p.ReopenCurrent())

	result, err := svc.Get(context.Background(), "t1")
	require.NoError(t, err)
	assert.Equal(t, todo.StatusAcknowledged, result.Status)
}

func TestTodoPanel_ReopenCurrent_FromDismissed(t *testing.T) {
	svc := newTodoPanelService(t)
	addHumanTodo(t, svc, "t1", "Write tests")

	_, err := svc.Dismiss(context.Background(), "t1")
	require.NoError(t, err)

	p := newTestTodoPanel(t, svc)
	p.filter = todoFilterAll
	p.applyFilter()
	require.Len(t, p.items, 1)

	require.NoError(t, p.ReopenCurrent())

	result, err := svc.Get(context.Background(), "t1")
	require.NoError(t, err)
	assert.Equal(t, todo.StatusAcknowledged, result.Status)
}

func TestTodoPanel_ReopenCurrent_NoOpOnOpen(t *testing.T) {
	svc := newTodoPanelService(t)
	addHumanTodo(t, svc, "t1", "Write tests")

	p := newTestTodoPanel(t, svc)
	require.Len(t, p.items, 1)
	assert.Equal(t, todo.StatusAcknowledged, p.items[0].Status)

	require.NoError(t, p.ReopenCurrent())

	// Status unchanged
	result, err := svc.Get(context.Background(), "t1")
	require.NoError(t, err)
	assert.Equal(t, todo.StatusAcknowledged, result.Status)
}

func TestTodoPanel_ReopenCurrent_EmptyList(t *testing.T) {
	svc := newTodoPanelService(t)
	p := newTestTodoPanel(t, svc)

	// Should not panic or error with empty list
	require.NoError(t, p.ReopenCurrent())
}
