package hc

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextBlockString(t *testing.T) {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	block := ContextBlock{
		Epic: Item{
			ID:        "hc-epic1",
			Title:     "Ship Honeycomb Feature",
			Desc:      "Implement the honeycomb task management system for agent coordination.",
			Type:      ItemTypeEpic,
			Status:    StatusInProgress,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Counts: TaskCounts{
			Open:       3,
			InProgress: 1,
			Done:       2,
			Cancelled:  0,
		},
		MyTasks: []TaskWithComment{
			{
				Item: Item{
					ID:       "hc-task1",
					Title:    "Add domain types",
					Type:     ItemTypeTask,
					Status:   StatusInProgress,
					EpicID:   "hc-epic1",
					ParentID: "hc-epic1",
					Depth:    1,
				},
				LatestComment: Comment{
					ID:      "hc-cmt1",
					ItemID:  "hc-task1",
					Message: "Started with Item and Activity structs; need to add Store interface next.",
				},
			},
			{
				Item: Item{
					ID:       "hc-task2",
					Title:    "Add data migrations",
					Type:     ItemTypeTask,
					Status:   StatusOpen,
					EpicID:   "hc-epic1",
					ParentID: "hc-epic1",
					Depth:    1,
				},
			},
		},
		AllOpenTasks: []Item{
			{
				ID:       "hc-task1",
				Title:    "Add domain types",
				Type:     ItemTypeTask,
				Status:   StatusInProgress,
				EpicID:   "hc-epic1",
				ParentID: "hc-epic1",
				Depth:    1,
			},
			{
				ID:       "hc-task2",
				Title:    "Add data migrations",
				Type:     ItemTypeTask,
				Status:   StatusOpen,
				EpicID:   "hc-epic1",
				ParentID: "hc-epic1",
				Depth:    1,
			},
			{
				ID:       "hc-task3",
				Title:    "Wire service layer",
				Type:     ItemTypeTask,
				Status:   StatusOpen,
				EpicID:   "hc-epic1",
				ParentID: "hc-epic1",
				Depth:    1,
			},
		},
	}

	got := block.String()

	golden := filepath.Join("testdata", "context_string.golden")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		require.NoError(t, os.WriteFile(golden, []byte(got), 0o644))
	}

	want, err := os.ReadFile(golden)
	require.NoError(t, err, "golden file missing — run with UPDATE_GOLDEN=1 to create it")
	assert.Equal(t, string(want), got)
}
