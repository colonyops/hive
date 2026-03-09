package tui

import (
	"context"
	"testing"

	"github.com/colonyops/hive/internal/core/hc"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/data/stores"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/tui/views/tasks"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestHoneycombSvc(t *testing.T) *hive.HoneycombService {
	t.Helper()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	store := stores.NewHCStore(database)
	return hive.NewHoneycombService(store, zerolog.Nop())
}

func createTestItem(t *testing.T, svc *hive.HoneycombService, title string) hc.Item {
	t.Helper()
	// Epics don't require a parent; create one and use it as the item under test.
	epic, err := svc.CreateItem(context.Background(), "", hc.CreateItemInput{
		Title: title + " epic",
		Type:  hc.ItemTypeEpic,
	})
	require.NoError(t, err)
	return epic
}

// --- captureTaskUndo ---

func TestCaptureTaskUndo_NilTasksView(t *testing.T) {
	m := Model{}
	result := m.captureTaskUndo()
	assert.Nil(t, result.taskUndoEntry, "no undo entry when tasksView is nil")
}

func TestCaptureTaskUndo_NoSelectedItem(t *testing.T) {
	svc := newTestHoneycombSvc(t)
	view := tasks.New(svc, "", nil, nil, 0)
	m := Model{tasksView: view}

	result := m.captureTaskUndo()
	assert.Nil(t, result.taskUndoEntry, "no undo entry when no item is selected")
}

// --- taskUndoStatus ---

func TestTaskUndoStatus_NilEntry(t *testing.T) {
	m := Model{}
	result, cmd := m.taskUndoStatus()
	assert.Nil(t, cmd, "no cmd when nothing to undo")
	// Model returned is unchanged (no undo entry to clear)
	assert.Nil(t, result.(Model).taskUndoEntry)
}

func TestTaskUndoStatus_NilTasksView(t *testing.T) {
	m := Model{
		taskUndoEntry: &taskUndoEntry{itemID: "x", prevStatus: hc.StatusOpen},
	}
	result, cmd := m.taskUndoStatus()
	assert.Nil(t, cmd)
	// undo entry is preserved when tasksView is nil (no-op)
	_ = result
}

func TestTaskUndoStatus_NilService(t *testing.T) {
	// tasksView exists but has no service
	view := tasks.New(nil, "", nil, nil, 0)
	m := Model{
		tasksView:     view,
		taskUndoEntry: &taskUndoEntry{itemID: "x", prevStatus: hc.StatusOpen},
	}
	result, cmd := m.taskUndoStatus()
	assert.Nil(t, cmd)
	_ = result
}

func TestTaskUndoStatus_RevertsToPrevStatus(t *testing.T) {
	svc := newTestHoneycombSvc(t)
	item := createTestItem(t, svc, "My Task")

	// Simulate: item was open, we marked it done — undo should revert to open
	doneStatus := hc.StatusDone
	_, err := svc.UpdateItem(context.Background(), item.ID, hc.ItemUpdate{Status: &doneStatus})
	require.NoError(t, err)

	view := tasks.New(svc, "", nil, nil, 0)
	m := Model{
		tasksView: view,
		taskUndoEntry: &taskUndoEntry{
			itemID:     item.ID,
			prevStatus: hc.StatusOpen,
		},
	}

	_, cmd := m.taskUndoStatus()
	require.NotNil(t, cmd)

	// Execute the async command
	msg := cmd()
	actionMsg, ok := msg.(tasks.TaskActionCompleteMsg)
	require.True(t, ok, "expected TaskActionCompleteMsg, got %T", msg)
	require.NoError(t, actionMsg.Err)

	// Verify item is now open
	updated, err := svc.GetItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, hc.StatusOpen, updated.Status)
}

func TestTaskUndoStatus_ClearsEntryAfterDispatch(t *testing.T) {
	svc := newTestHoneycombSvc(t)
	item := createTestItem(t, svc, "My Task")

	view := tasks.New(svc, "", nil, nil, 0)
	m := Model{
		tasksView: view,
		taskUndoEntry: &taskUndoEntry{
			itemID:     item.ID,
			prevStatus: hc.StatusOpen,
		},
	}

	result, _ := m.taskUndoStatus()
	assert.Nil(t, result.(Model).taskUndoEntry, "undo entry cleared after dispatch")
}
