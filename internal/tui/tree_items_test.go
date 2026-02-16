package tui

import (
	"testing"

	"charm.land/bubbles/v2/list"
	"github.com/colonyops/hive/internal/core/session"
)

func TestIsSession(t *testing.T) {
	tests := []struct {
		name string
		item TreeItem
		want bool
	}{
		{name: "header", item: TreeItem{IsHeader: true}, want: false},
		{name: "recycled", item: TreeItem{IsRecycledPlaceholder: true}, want: false},
		{name: "window", item: TreeItem{IsWindowItem: true}, want: false},
		{name: "session", item: TreeItem{Session: session.Session{ID: "a"}}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.item.IsSession(); got != tt.want {
				t.Errorf("IsSession() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTreeItemsAll(t *testing.T) {
	items := []list.Item{
		TreeItem{IsHeader: true, RepoName: "repo1"},
		TreeItem{Session: session.Session{ID: "s1"}},
		TreeItem{IsRecycledPlaceholder: true},
	}

	var got []TreeItem
	for _, ti := range TreeItemsAll(items) {
		got = append(got, ti)
	}

	if len(got) != 3 {
		t.Fatalf("TreeItemsAll yielded %d items, want 3", len(got))
	}
	if !got[0].IsHeader {
		t.Error("first item should be header")
	}
	if got[1].Session.ID != "s1" {
		t.Error("second item should be session s1")
	}
	if !got[2].IsRecycledPlaceholder {
		t.Error("third item should be recycled placeholder")
	}
}

func TestTreeItemsSessions(t *testing.T) {
	items := []list.Item{
		TreeItem{IsHeader: true, RepoName: "repo1"},
		TreeItem{Session: session.Session{ID: "s1"}},
		TreeItem{IsRecycledPlaceholder: true},
		TreeItem{Session: session.Session{ID: "s2"}},
		TreeItem{IsWindowItem: true},
	}

	var indices []int
	var ids []string
	for i, ti := range TreeItemsSessions(items) {
		indices = append(indices, i)
		ids = append(ids, ti.Session.ID)
	}

	if len(ids) != 2 {
		t.Fatalf("TreeItemsSessions yielded %d items, want 2", len(ids))
	}
	if ids[0] != "s1" || ids[1] != "s2" {
		t.Errorf("got IDs %v, want [s1 s2]", ids)
	}
	if indices[0] != 1 || indices[1] != 3 {
		t.Errorf("got indices %v, want [1 3]", indices)
	}
}
