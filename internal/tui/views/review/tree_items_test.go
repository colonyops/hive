package review

import (
	"testing"

	"charm.land/bubbles/v2/list"
)

func TestIsDocument(t *testing.T) {
	tests := []struct {
		name string
		item TreeItem
		want bool
	}{
		{name: "header", item: TreeItem{IsHeader: true}, want: false},
		{name: "document", item: TreeItem{Document: Document{RelPath: "a.md"}}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.item.IsDocument(); got != tt.want {
				t.Errorf("IsDocument() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTreeItemsAll(t *testing.T) {
	items := []list.Item{
		TreeItem{IsHeader: true, HeaderName: "Plans"},
		TreeItem{Document: Document{RelPath: "a.md"}},
		TreeItem{Document: Document{RelPath: "b.md"}},
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
}

func TestTreeItemsDocuments(t *testing.T) {
	items := []list.Item{
		TreeItem{IsHeader: true, HeaderName: "Plans"},
		TreeItem{Document: Document{RelPath: "a.md"}},
		TreeItem{IsHeader: true, HeaderName: "Research"},
		TreeItem{Document: Document{RelPath: "b.md"}},
	}

	var paths []string
	var indices []int
	for i, ti := range TreeItemsDocuments(items) {
		indices = append(indices, i)
		paths = append(paths, ti.Document.RelPath)
	}

	if len(paths) != 2 {
		t.Fatalf("TreeItemsDocuments yielded %d items, want 2", len(paths))
	}
	if paths[0] != "a.md" || paths[1] != "b.md" {
		t.Errorf("got paths %v, want [a.md b.md]", paths)
	}
	if indices[0] != 1 || indices[1] != 3 {
		t.Errorf("got indices %v, want [1 3]", indices)
	}
}

func TestTreeItemsFirstDocument(t *testing.T) {
	t.Run("with documents", func(t *testing.T) {
		items := []list.Item{
			TreeItem{IsHeader: true},
			TreeItem{Document: Document{RelPath: "a.md"}},
		}
		if got := TreeItemsFirstDocument(items); got != 1 {
			t.Errorf("TreeItemsFirstDocument() = %d, want 1", got)
		}
	})

	t.Run("only headers", func(t *testing.T) {
		items := []list.Item{
			TreeItem{IsHeader: true},
		}
		if got := TreeItemsFirstDocument(items); got != -1 {
			t.Errorf("TreeItemsFirstDocument() = %d, want -1", got)
		}
	})

	t.Run("empty", func(t *testing.T) {
		if got := TreeItemsFirstDocument(nil); got != -1 {
			t.Errorf("TreeItemsFirstDocument() = %d, want -1", got)
		}
	})
}
