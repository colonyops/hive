package tui

import (
	"testing"

	"github.com/hay-kot/hive/internal/core/session"
)

func TestTreeSelection_SaveRestore(t *testing.T) {
	sessA := session.Session{ID: "aaa"}
	sessB := session.Session{ID: "bbb"}

	items := []TreeItem{
		{IsHeader: true, RepoName: "repo1"},
		{Session: sessA, RepoPrefix: "repo1"},
		{IsWindowItem: true, ParentSession: sessA, WindowName: "claude", WindowIndex: "0", RepoPrefix: "repo1"},
		{IsWindowItem: true, ParentSession: sessA, WindowName: "aider", WindowIndex: "1", RepoPrefix: "repo1"},
		{Session: sessB, RepoPrefix: "repo1"},
		{IsRecycledPlaceholder: true, RepoPrefix: "repo1", RecycledCount: 2},
	}

	tests := []struct {
		name      string
		selIdx    int
		newItems  []TreeItem // if nil, use items
		wantIdx   int
		wantMatch string // description of what should match
	}{
		{
			name:      "session restores to same session",
			selIdx:    1,
			wantIdx:   1,
			wantMatch: "session aaa",
		},
		{
			name:      "window by name restores correctly",
			selIdx:    3, // aider window
			wantIdx:   3,
			wantMatch: "window aider",
		},
		{
			name:   "window by name after reorder",
			selIdx: 3, // aider window at index 3
			newItems: []TreeItem{
				{IsHeader: true, RepoName: "repo1"},
				{Session: sessA, RepoPrefix: "repo1"},
				{IsWindowItem: true, ParentSession: sessA, WindowName: "aider", WindowIndex: "1", RepoPrefix: "repo1"},
				{IsWindowItem: true, ParentSession: sessA, WindowName: "claude", WindowIndex: "0", RepoPrefix: "repo1"},
				{Session: sessB, RepoPrefix: "repo1"},
			},
			wantIdx:   2, // aider moved to index 2
			wantMatch: "window aider after reorder",
		},
		{
			name:   "window prefers index when names collide",
			selIdx: 2, // claude window index 0
			newItems: []TreeItem{
				{IsHeader: true, RepoName: "repo1"},
				{Session: sessA, RepoPrefix: "repo1"},
				{IsWindowItem: true, ParentSession: sessA, WindowName: "claude", WindowIndex: "1", RepoPrefix: "repo1"},
				{IsWindowItem: true, ParentSession: sessA, WindowName: "claude", WindowIndex: "0", RepoPrefix: "repo1"},
				{Session: sessB, RepoPrefix: "repo1"},
			},
			wantIdx:   3, // should restore to index 0, not first matching name
			wantMatch: "window index preferred over name",
		},
		{
			name:   "window falls back to index when name gone",
			selIdx: 3, // aider window
			newItems: []TreeItem{
				{IsHeader: true, RepoName: "repo1"},
				{Session: sessA, RepoPrefix: "repo1"},
				{IsWindowItem: true, ParentSession: sessA, WindowName: "claude", WindowIndex: "0", RepoPrefix: "repo1"},
				{IsWindowItem: true, ParentSession: sessA, WindowName: "codex", WindowIndex: "1", RepoPrefix: "repo1"},
				{Session: sessB, RepoPrefix: "repo1"},
			},
			wantIdx:   3, // index "1" is at position 3
			wantMatch: "window index fallback",
		},
		{
			name:   "window falls back to parent session when window gone",
			selIdx: 2, // claude window
			newItems: []TreeItem{
				{IsHeader: true, RepoName: "repo1"},
				{Session: sessA, RepoPrefix: "repo1"},
				{Session: sessB, RepoPrefix: "repo1"},
			},
			wantIdx:   1, // parent session aaa
			wantMatch: "session fallback",
		},
		{
			name:      "recycled placeholder restores",
			selIdx:    5,
			wantIdx:   5,
			wantMatch: "recycled repo1",
		},
		{
			name:   "recycled placeholder after count change",
			selIdx: 5,
			newItems: []TreeItem{
				{IsHeader: true, RepoName: "repo1"},
				{Session: sessA, RepoPrefix: "repo1"},
				{Session: sessB, RepoPrefix: "repo1"},
				{IsRecycledPlaceholder: true, RepoPrefix: "repo1", RecycledCount: 5},
			},
			wantIdx:   3,
			wantMatch: "recycled with different count",
		},
		{
			name:      "header selection preserves index",
			selIdx:    0,
			wantIdx:   0,
			wantMatch: "header by index",
		},
		{
			name:   "session gone falls to original index",
			selIdx: 4, // sessB
			newItems: []TreeItem{
				{IsHeader: true, RepoName: "repo1"},
				{Session: sessA, RepoPrefix: "repo1"},
				{IsRecycledPlaceholder: true, RepoPrefix: "repo1", RecycledCount: 2},
				{IsHeader: true, RepoName: "repo2"},
				{Session: session.Session{ID: "ccc"}, RepoPrefix: "repo2"},
			},
			wantIdx:   4, // clamped original index
			wantMatch: "original index fallback",
		},
		{
			name:   "index out of bounds falls to first non-header",
			selIdx: 4, // sessB
			newItems: []TreeItem{
				{IsHeader: true, RepoName: "repo1"},
				{Session: sessA, RepoPrefix: "repo1"},
			},
			wantIdx:   1, // first non-header
			wantMatch: "first non-header fallback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sel := saveTreeSelection(&items[tt.selIdx], tt.selIdx)

			target := items
			if tt.newItems != nil {
				target = tt.newItems
			}

			got := sel.restore(target)
			if got != tt.wantIdx {
				t.Fatalf("restore() = %d, want %d (%s)", got, tt.wantIdx, tt.wantMatch)
			}
		})
	}
}

func TestTreeSelection_NilItem(t *testing.T) {
	items := []TreeItem{
		{IsHeader: true, RepoName: "repo1"},
		{Session: session.Session{ID: "aaa"}, RepoPrefix: "repo1"},
	}

	sel := saveTreeSelection(nil, 1)
	got := sel.restore(items)
	if got != 1 {
		t.Fatalf("nil item should fall back to original index, got %d", got)
	}
}

func TestTreeSelection_EmptyItems(t *testing.T) {
	sel := saveTreeSelection(nil, 5)
	got := sel.restore(nil)
	if got != 0 {
		t.Fatalf("empty items should return 0, got %d", got)
	}
}
