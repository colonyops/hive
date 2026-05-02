package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newKVViewWithKeys creates a KVView preloaded with the given keys.
func newKVViewWithKeys(keys []string) *KVView {
	v := NewKVView()
	v.SetSize(100, 24)
	v.SetKeys(keys)
	return v
}

// --- SelectAtRow ---

func TestKVView_SelectAtRow_ClickInPreview_NoOp(t *testing.T) {
	v := newKVViewWithKeys([]string{"key-a", "key-b", "key-c"})
	// listWidth = int(100 * 0.20) = 20; x=50 >= 20 → no-op
	v.SelectAtRow(50, 1)
	assert.Equal(t, 0, v.cursor, "click in preview pane should be no-op")
}

func TestKVView_SelectAtRow_HeaderRow_NoOp(t *testing.T) {
	v := newKVViewWithKeys([]string{"key-a", "key-b", "key-c"})
	// headerRows=1 (no filter); listRow = contentY - 1 = -1 → no-op
	v.SelectAtRow(0, 0)
	assert.Equal(t, 0, v.cursor, "clicking header row (contentY=0) should be no-op")
}

func TestKVView_SelectAtRow_HappyPath(t *testing.T) {
	v := newKVViewWithKeys([]string{"key-a", "key-b", "key-c"})
	// headerRows=1; contentY=1 → listRow=0, idx=offset+0=0
	v.SelectAtRow(0, 1)
	assert.Equal(t, 0, v.cursor, "contentY=1 should select cursor=0")

	// contentY=2 → listRow=1, idx=1
	v.SelectAtRow(0, 2)
	assert.Equal(t, 1, v.cursor, "contentY=2 should select cursor=1")
}

func TestKVView_SelectAtRow_FilterActive_FilterLineIsNoOp(t *testing.T) {
	v := newKVViewWithKeys([]string{"alpha", "beta", "gamma"})
	v.StartFilter()
	v.AddFilterRune('a')

	// headerRows=2 (header + filter line); contentY=1 → listRow=-1 → no-op
	v.SelectAtRow(0, 1)
	assert.Equal(t, 0, v.cursor, "contentY=1 (filter line) should be no-op when filter active")
}

func TestKVView_SelectAtRow_FilterActive_ContentY2_SelectsFirst(t *testing.T) {
	v := newKVViewWithKeys([]string{"alpha", "beta", "gamma"})
	v.StartFilter()
	v.AddFilterRune('a') // matches "alpha" and "gamma" (2 items)

	require.NotEmpty(t, v.filtered, "filter should produce at least one match")

	// headerRows=2; contentY=2 → listRow=0, idx=0
	v.SelectAtRow(0, 2)
	assert.Equal(t, 0, v.cursor, "contentY=2 should select first filtered item")
}

func TestKVView_SelectAtRow_EmptyFiltered_NoOp(t *testing.T) {
	v := newKVViewWithKeys([]string{"alpha", "beta"})
	v.StartFilter()
	v.AddFilterRune('z') // no matches

	require.Empty(t, v.filtered, "filter should produce no matches")

	v.SelectAtRow(0, 1)
	assert.Equal(t, 0, v.cursor, "no-op when filtered list is empty")
}

func TestKVView_SelectAtRow_BeyondFiltered_NoOp(t *testing.T) {
	v := newKVViewWithKeys([]string{"key-a"})
	// filtered has 1 entry; contentY=10 → idx=9 >= 1 → no-op
	v.SelectAtRow(0, 10)
	assert.Equal(t, 0, v.cursor, "row beyond filtered items should be no-op")
}
