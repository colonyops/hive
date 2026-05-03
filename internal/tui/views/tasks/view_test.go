package tasks

import (
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/hc"
	"github.com/stretchr/testify/assert"
)

// newViewWithNodes creates a minimal View whose flatNodes slice is pre-populated.
// It does not use the service layer, making it suitable for pure unit tests.
func newViewWithNodes(nodes []FlatNode) *View {
	return &View{
		flatNodes:    nodes,
		comments:     make(map[string][]hc.Comment),
		blockers:     make(map[string][]hc.Item),
		statusFilter: FilterOpen,
		showPreview:  false,
		height:       24,
		width:        80,
	}
}

func makeNode(id string) FlatNode {
	return FlatNode{
		Node: &TreeNode{
			Item: hc.Item{
				ID:        id,
				Title:     "Item " + id,
				Type:      hc.ItemTypeTask,
				Status:    hc.StatusOpen,
				CreatedAt: time.Now(),
			},
			Expanded: false,
		},
		Depth:  0,
		IsLast: false,
	}
}

// --- SelectAtRow ---

func TestSelectAtRow_HeaderRow_NoOp(t *testing.T) {
	nodes := []FlatNode{makeNode("task-1"), makeNode("task-2"), makeNode("task-3")}
	v := newViewWithNodes(nodes)

	// contentY=0 is the repo header line — treeRow = -1 → no-op
	v.SelectAtRow(0, 0)
	assert.Equal(t, 0, v.cursor, "clicking repo header (contentY=0) should be no-op")
}

func TestSelectAtRow_ContentY1_SelectsFirst(t *testing.T) {
	nodes := []FlatNode{makeNode("task-1"), makeNode("task-2"), makeNode("task-3")}
	v := newViewWithNodes(nodes)

	// contentY=1 → treeRow=0, idx=scrollOffset+0=0
	v.SelectAtRow(0, 1)
	assert.Equal(t, 0, v.cursor, "contentY=1 should select flatNodes[0]")
}

func TestSelectAtRow_ContentY2_SelectsSecond(t *testing.T) {
	nodes := []FlatNode{makeNode("task-1"), makeNode("task-2"), makeNode("task-3")}
	v := newViewWithNodes(nodes)

	v.SelectAtRow(0, 2)
	assert.Equal(t, 1, v.cursor, "contentY=2 should select flatNodes[1]")
}

func TestSelectAtRow_BeyondNodes_NoOp(t *testing.T) {
	nodes := []FlatNode{makeNode("task-1")}
	v := newViewWithNodes(nodes)

	// contentY=5 → treeRow=4, idx=4 >= len(flatNodes)=1 → no-op
	v.SelectAtRow(0, 5)
	assert.Equal(t, 0, v.cursor, "clicking beyond flatNodes should be no-op")
}

func TestSelectAtRow_EmptyFlatNodes_NoOp(t *testing.T) {
	v := newViewWithNodes(nil)

	v.SelectAtRow(0, 1)
	assert.Equal(t, 0, v.cursor, "clicking with empty flatNodes should be no-op")
}

func TestSelectAtRow_ShowPreview_ClickInDetailPane_NoOp(t *testing.T) {
	nodes := []FlatNode{makeNode("task-1"), makeNode("task-2")}
	v := newViewWithNodes(nodes)
	v.showPreview = true
	v.width = 100
	// splitRatio=0 → default 30%; availWidth=100-1=99; treeWidth=max(99*30/100,25)=max(29,25)=29
	// x=50 >= treeWidth=29 → no-op

	v.SelectAtRow(50, 1)
	assert.Equal(t, 0, v.cursor, "click in detail pane should be no-op")
}

func TestSelectAtRow_ShowPreview_ClickInTree_Selects(t *testing.T) {
	nodes := []FlatNode{makeNode("task-1"), makeNode("task-2")}
	v := newViewWithNodes(nodes)
	v.showPreview = true
	v.width = 100
	// treeWidth=29; x=10 < 29 → allowed

	v.SelectAtRow(10, 2) // contentY=2 → treeRow=1, idx=1
	assert.Equal(t, 1, v.cursor, "click in tree pane should select item")
}
