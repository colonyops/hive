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

// --- UndoLast ---

func TestTodoPanel_UndoLast_AfterComplete(t *testing.T) {
	svc := newTodoPanelService(t)
	addHumanTodo(t, svc, "t1", "Write tests")

	p := newTestTodoPanel(t, svc)
	require.Len(t, p.items, 1)

	require.NoError(t, p.CompleteCurrent())
	assert.NotNil(t, p.undoEntry, "undoEntry should be set after complete")
	assert.Equal(t, "t1", p.undoEntry.itemID)
	assert.Equal(t, todo.StatusAcknowledged, p.undoEntry.prevStatus)

	require.NoError(t, p.UndoLast())
	assert.Nil(t, p.undoEntry, "undoEntry cleared after undo")

	result, err := svc.Get(context.Background(), "t1")
	require.NoError(t, err)
	assert.Equal(t, todo.StatusAcknowledged, result.Status)
}

func TestTodoPanel_UndoLast_AfterDismiss(t *testing.T) {
	svc := newTodoPanelService(t)
	addHumanTodo(t, svc, "t1", "Write tests")

	p := newTestTodoPanel(t, svc)
	require.Len(t, p.items, 1)

	require.NoError(t, p.DismissCurrent())
	assert.NotNil(t, p.undoEntry)
	assert.Equal(t, todo.StatusAcknowledged, p.undoEntry.prevStatus)

	require.NoError(t, p.UndoLast())
	assert.Nil(t, p.undoEntry)

	result, err := svc.Get(context.Background(), "t1")
	require.NoError(t, err)
	assert.Equal(t, todo.StatusAcknowledged, result.Status)
}

func TestTodoPanel_UndoLast_NoEntry(t *testing.T) {
	svc := newTodoPanelService(t)
	p := newTestTodoPanel(t, svc)

	err := p.UndoLast()
	assert.ErrorContains(t, err, "nothing to undo")
}

func TestTodoPanel_UndoLast_OnlyRetainsLastAction(t *testing.T) {
	svc := newTodoPanelService(t)
	addHumanTodo(t, svc, "t1", "First")
	addHumanTodo(t, svc, "t2", "Second")

	p := newTestTodoPanel(t, svc)
	require.Len(t, p.items, 2)

	// Complete first item
	require.NoError(t, p.CompleteCurrent())
	firstEntry := p.undoEntry

	// Dismiss second item — overwrites undo entry
	require.NoError(t, p.DismissCurrent())
	assert.NotEqual(t, firstEntry.itemID, p.undoEntry.itemID, "undo entry should be overwritten")

	// Undo only reverts the second action
	require.NoError(t, p.UndoLast())
	result, err := svc.Get(context.Background(), p.items[0].ID)
	require.NoError(t, err)
	assert.Equal(t, todo.StatusAcknowledged, result.Status)
}

func TestTodoPanel_UndoLast_CanUndoTwice(t *testing.T) {
	svc := newTodoPanelService(t)
	addHumanTodo(t, svc, "t1", "Write tests")

	p := newTestTodoPanel(t, svc)
	require.Len(t, p.items, 1)

	require.NoError(t, p.CompleteCurrent())
	require.NoError(t, p.UndoLast())

	// Item reappears after undo; complete again
	require.Len(t, p.items, 1)
	require.NoError(t, p.CompleteCurrent())
	require.NoError(t, p.UndoLast())

	result, err := svc.Get(context.Background(), "t1")
	require.NoError(t, err)
	assert.Equal(t, todo.StatusAcknowledged, result.Status)
}

// --- ReopenCurrent ---

func TestTodoPanel_ReopenCurrent_FromCompleted(t *testing.T) {
	svc := newTodoPanelService(t)
	addHumanTodo(t, svc, "t1", "Write tests")

	// Complete via service directly, bypassing panel undo
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

// --- CompleteCurrent undo capture ---

func TestTodoPanel_CompleteCurrent_ClearsUndoEntryOnAlreadyDone(t *testing.T) {
	svc := newTodoPanelService(t)
	addHumanTodo(t, svc, "t1", "Write tests")

	p := newTestTodoPanel(t, svc)
	require.Len(t, p.items, 1)

	// Complete once — captures undo
	require.NoError(t, p.CompleteCurrent())
	assert.NotNil(t, p.undoEntry)

	// Undo to get it back
	require.NoError(t, p.UndoLast())
	require.Len(t, p.items, 1)

	// Completing again should set a fresh undo entry
	require.NoError(t, p.CompleteCurrent())
	assert.NotNil(t, p.undoEntry)
	assert.Equal(t, "t1", p.undoEntry.itemID)
}

// --- Filter interaction ---

func TestTodoPanel_UndoLast_WorksAfterItemFiltered(t *testing.T) {
	svc := newTodoPanelService(t)
	addHumanTodo(t, svc, "t1", "Write tests")

	p := newTestTodoPanel(t, svc)
	require.Len(t, p.items, 1, "item should be visible before completion")

	require.NoError(t, p.CompleteCurrent())
	assert.Len(t, p.items, 0, "item disappears in Open filter after completion")

	// Undo should still work even though item is no longer visible
	require.NoError(t, p.UndoLast())
	assert.Len(t, p.items, 1, "item reappears after undo")

	result, err := svc.Get(context.Background(), "t1")
	require.NoError(t, err)
	assert.Equal(t, todo.StatusAcknowledged, result.Status)
}
