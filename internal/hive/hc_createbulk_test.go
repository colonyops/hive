package hive

import (
	"context"
	"testing"

	"github.com/colonyops/hive/internal/core/hc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateBulk_WithBlockers_ValidRefs(t *testing.T) {
	store := newFakeHCStore()
	svc := newTestHoneycombService(store)

	input := hc.CreateInput{
		Title: "Epic",
		Type:  hc.ItemTypeEpic,
		Children: []hc.CreateInput{
			{
				Ref:   "task-a",
				Title: "Task A",
				Type:  hc.ItemTypeTask,
			},
			{
				Ref:      "task-b",
				Title:    "Task B",
				Type:     hc.ItemTypeTask,
				Blockers: []string{"task-a"},
			},
		},
	}

	items, err := svc.CreateBulk(context.Background(), "owner/repo", input)
	require.NoError(t, err)
	assert.Len(t, items, 3)
}

func TestCreateBulk_WithBlockers_UnknownRef(t *testing.T) {
	store := newFakeHCStore()
	svc := newTestHoneycombService(store)

	input := hc.CreateInput{
		Title: "Epic",
		Type:  hc.ItemTypeEpic,
		Children: []hc.CreateInput{
			{
				Ref:      "task-a",
				Title:    "Task A",
				Type:     hc.ItemTypeTask,
				Blockers: []string{"nonexistent-ref"},
			},
		},
	}

	_, err := svc.CreateBulk(context.Background(), "owner/repo", input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown blocker ref")
}

func TestCreateBulk_WithBlockers_InBatchCycle(t *testing.T) {
	store := newFakeHCStore()
	svc := newTestHoneycombService(store)

	// A blocks B, B blocks A — cycle
	input := hc.CreateInput{
		Title: "Epic",
		Type:  hc.ItemTypeEpic,
		Children: []hc.CreateInput{
			{
				Ref:      "task-a",
				Title:    "Task A",
				Type:     hc.ItemTypeTask,
				Blockers: []string{"task-b"},
			},
			{
				Ref:      "task-b",
				Title:    "Task B",
				Type:     hc.ItemTypeTask,
				Blockers: []string{"task-a"},
			},
		},
	}

	_, err := svc.CreateBulk(context.Background(), "owner/repo", input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}
