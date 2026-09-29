package tasks

import (
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/hc"
)

func makeItem(id string, itemType hc.ItemType, parentID string, createdAt time.Time) hc.Item {
	return hc.Item{
		ID:        id,
		Title:     "Item " + id,
		Type:      itemType,
		Status:    hc.StatusOpen,
		ParentID:  parentID,
		Depth:     0,
		CreatedAt: createdAt,
	}
}

func TestBuildTree_EpicWithChildren(t *testing.T) {
	now := time.Now()
	items := []hc.Item{
		makeItem("epic-1", hc.ItemTypeEpic, "", now),
		makeItem("task-1", hc.ItemTypeTask, "epic-1", now),
		makeItem("task-2", hc.ItemTypeTask, "epic-1", now),
	}

	roots := buildTree(items)
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
	if roots[0].Item.ID != "epic-1" {
		t.Errorf("expected root to be epic-1, got %s", roots[0].Item.ID)
	}
	if len(roots[0].Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(roots[0].Children))
	}
	if roots[0].Children[0].Item.ID != "task-1" {
		t.Errorf("expected first child task-1, got %s", roots[0].Children[0].Item.ID)
	}
	if roots[0].Children[1].Item.ID != "task-2" {
		t.Errorf("expected second child task-2, got %s", roots[0].Children[1].Item.ID)
	}
}

func TestBuildTree_RootItemsWithoutParent(t *testing.T) {
	now := time.Now()
	items := []hc.Item{
		makeItem("task-1", hc.ItemTypeTask, "", now.Add(-time.Hour)),
		makeItem("task-2", hc.ItemTypeTask, "", now),
	}

	roots := buildTree(items)
	if len(roots) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(roots))
	}
	// Sorted by CreatedAt DESC, so task-2 (newer) comes first.
	if roots[0].Item.ID != "task-2" {
		t.Errorf("expected first root task-2, got %s", roots[0].Item.ID)
	}
	if roots[1].Item.ID != "task-1" {
		t.Errorf("expected second root task-1, got %s", roots[1].Item.ID)
	}
}

func TestBuildTree_EmptyInput(t *testing.T) {
	roots := buildTree(nil)
	if roots != nil {
		t.Errorf("expected nil, got %v", roots)
	}

	roots = buildTree([]hc.Item{})
	if roots != nil {
		t.Errorf("expected nil for empty slice, got %v", roots)
	}
}

func TestBuildTree_EpicsStartExpanded(t *testing.T) {
	now := time.Now()
	items := []hc.Item{
		makeItem("epic-1", hc.ItemTypeEpic, "", now),
		makeItem("task-1", hc.ItemTypeTask, "", now),
	}

	roots := buildTree(items)
	var epic, task *TreeNode
	for _, r := range roots {
		if r.Item.Type == hc.ItemTypeEpic {
			epic = r
		} else {
			task = r
		}
	}
	if epic == nil || !epic.Expanded {
		t.Error("expected epic to start expanded")
	}
	if task == nil || task.Expanded {
		t.Error("expected task to start collapsed")
	}
}

func TestCountByStatus(t *testing.T) {
	doneItem := hc.Item{Status: hc.StatusDone, Type: hc.ItemTypeTask}
	openItem := hc.Item{Status: hc.StatusOpen, Type: hc.ItemTypeTask}

	children := []*TreeNode{
		{Item: doneItem},
		{Item: openItem},
		{Item: doneItem},
	}

	done, total := countByStatus(children)
	if done != 2 {
		t.Errorf("expected done=2, got %d", done)
	}
	if total != 3 {
		t.Errorf("expected total=3, got %d", total)
	}
}

func TestCountByStatus_NestedChildren(t *testing.T) {
	// Parent with nested children — only leaves should be counted.
	children := []*TreeNode{
		{
			Item: hc.Item{Status: hc.StatusOpen, Type: hc.ItemTypeEpic},
			Children: []*TreeNode{
				{Item: hc.Item{Status: hc.StatusDone, Type: hc.ItemTypeTask}},
				{Item: hc.Item{Status: hc.StatusOpen, Type: hc.ItemTypeTask}},
			},
		},
		{Item: hc.Item{Status: hc.StatusDone, Type: hc.ItemTypeTask}},
	}

	done, total := countByStatus(children)
	if done != 2 {
		t.Errorf("expected done=2, got %d", done)
	}
	if total != 3 {
		t.Errorf("expected total=3, got %d", total)
	}
}

func TestFlattenTree_ExpandedEpic(t *testing.T) {
	now := time.Now()
	items := []hc.Item{
		makeItem("epic-1", hc.ItemTypeEpic, "", now),
		makeItem("task-1", hc.ItemTypeTask, "epic-1", now),
		makeItem("task-2", hc.ItemTypeTask, "epic-1", now),
	}

	roots := buildTree(items)
	// Epic should start expanded.
	flat := flattenTree(roots)

	if len(flat) != 3 {
		t.Fatalf("expected 3 flat nodes, got %d", len(flat))
	}
	if flat[0].Node.Item.ID != "epic-1" {
		t.Errorf("expected first node epic-1, got %s", flat[0].Node.Item.ID)
	}
	if flat[0].Depth != 0 {
		t.Errorf("expected depth 0 for epic, got %d", flat[0].Depth)
	}
	if flat[1].Depth != 1 {
		t.Errorf("expected depth 1 for task-1, got %d", flat[1].Depth)
	}
	if flat[2].Depth != 1 {
		t.Errorf("expected depth 1 for task-2, got %d", flat[2].Depth)
	}
}

func TestFlattenTree_CollapsedEpic(t *testing.T) {
	now := time.Now()
	items := []hc.Item{
		makeItem("epic-1", hc.ItemTypeEpic, "", now),
		makeItem("task-1", hc.ItemTypeTask, "epic-1", now),
		makeItem("task-2", hc.ItemTypeTask, "epic-1", now),
	}

	roots := buildTree(items)
	// Collapse the epic.
	roots[0].Expanded = false

	flat := flattenTree(roots)
	if len(flat) != 1 {
		t.Fatalf("expected 1 flat node for collapsed epic, got %d", len(flat))
	}
	if flat[0].Node.Item.ID != "epic-1" {
		t.Errorf("expected node epic-1, got %s", flat[0].Node.Item.ID)
	}
}

func TestFlattenTree_IsLastSetCorrectly(t *testing.T) {
	now := time.Now()
	items := []hc.Item{
		makeItem("epic-1", hc.ItemTypeEpic, "", now),
		makeItem("task-1", hc.ItemTypeTask, "epic-1", now),
		makeItem("task-2", hc.ItemTypeTask, "epic-1", now),
	}

	roots := buildTree(items)
	flat := flattenTree(roots)

	// epic-1 is the only root, so IsLast=true
	if !flat[0].IsLast {
		t.Error("expected epic-1 to be last root")
	}
	// task-1 is not last child
	if flat[1].IsLast {
		t.Error("expected task-1 to NOT be last child")
	}
	// task-2 is last child
	if !flat[2].IsLast {
		t.Error("expected task-2 to be last child")
	}
}
