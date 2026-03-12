package stores

import (
	"context"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/hc"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptr[T any](v T) *T { return &v }

func TestHCStore_UpdateItem_TitleDesc(t *testing.T) {
	now := time.Now()

	type args struct {
		update hc.ItemUpdate
	}
	tests := []struct {
		name       string
		args       args
		wantTitle  string
		wantDesc   string
		wantStatus hc.Status
	}{
		{
			name:       "nil Title preserves existing",
			args:       args{update: hc.ItemUpdate{Title: nil}},
			wantTitle:  "original title",
			wantDesc:   "original desc",
			wantStatus: hc.StatusOpen,
		},
		{
			name:       "non-nil Title overwrites existing",
			args:       args{update: hc.ItemUpdate{Title: ptr("new title")}},
			wantTitle:  "new title",
			wantDesc:   "original desc",
			wantStatus: hc.StatusOpen,
		},
		{
			name:       "nil Desc preserves existing",
			args:       args{update: hc.ItemUpdate{Desc: nil}},
			wantTitle:  "original title",
			wantDesc:   "original desc",
			wantStatus: hc.StatusOpen,
		},
		{
			name:       "non-nil Desc overwrites existing",
			args:       args{update: hc.ItemUpdate{Desc: ptr("new desc")}},
			wantTitle:  "original title",
			wantDesc:   "new desc",
			wantStatus: hc.StatusOpen,
		},
		{
			name:       "empty string Desc clears description",
			args:       args{update: hc.ItemUpdate{Desc: ptr("")}},
			wantTitle:  "original title",
			wantDesc:   "",
			wantStatus: hc.StatusOpen,
		},
		{
			name:       "nil Status preserves existing",
			args:       args{update: hc.ItemUpdate{Status: nil}},
			wantTitle:  "original title",
			wantDesc:   "original desc",
			wantStatus: hc.StatusOpen,
		},
		{
			name: "all four fields updated together",
			args: args{update: hc.ItemUpdate{
				Title:  ptr("all updated title"),
				Desc:   ptr("all updated desc"),
				Status: ptr(hc.StatusDone),
			}},
			wantTitle:  "all updated title",
			wantDesc:   "all updated desc",
			wantStatus: hc.StatusDone,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
			require.NoError(t, err)
			defer func() { _ = database.Close() }()

			store := NewHCStore(database)

			epic := makeEpic("epic-1", now)
			item := hc.Item{
				ID:        "task-1",
				EpicID:    "epic-1",
				ParentID:  "epic-1",
				Title:     "original title",
				Desc:      "original desc",
				Type:      hc.ItemTypeTask,
				Status:    hc.StatusOpen,
				Depth:     1,
				CreatedAt: now,
				UpdatedAt: now,
			}
			require.NoError(t, store.CreateItems(ctx, []hc.Item{epic, item}))

			updated, err := store.UpdateItem(ctx, "task-1", tc.args.update)
			require.NoError(t, err)

			assert.Equal(t, tc.wantTitle, updated.Title)
			assert.Equal(t, tc.wantDesc, updated.Desc)
			assert.Equal(t, tc.wantStatus, updated.Status)
		})
	}
}

func TestHCStore_BulkUpdateStatus(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	store := NewHCStore(database)
	now := time.Now()

	epic := makeEpic("epic-1", now)
	taskOpen := makeItem("task-open", "epic-1", "epic-1", hc.StatusOpen, 1, now)
	taskInProgress := makeItem("task-in-progress", "epic-1", "epic-1", hc.StatusInProgress, 1, now)
	taskDone := makeItem("task-done", "epic-1", "epic-1", hc.StatusDone, 1, now)
	taskCancelled := makeItem("task-cancelled", "epic-1", "epic-1", hc.StatusCancelled, 1, now)

	require.NoError(t, store.CreateItems(ctx, []hc.Item{epic, taskOpen, taskInProgress, taskDone, taskCancelled}))

	require.NoError(t, store.BulkUpdateStatus(ctx, "epic-1", hc.StatusDone))

	assertItemStatus := func(id string, want hc.Status) {
		t.Helper()
		item, err := store.GetItem(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, want, item.Status, "item %s status", id)
	}

	assertItemStatus("task-open", hc.StatusDone)
	assertItemStatus("task-in-progress", hc.StatusDone)
	assertItemStatus("task-done", hc.StatusDone)           // was already done
	assertItemStatus("task-cancelled", hc.StatusCancelled) // not modified
}
