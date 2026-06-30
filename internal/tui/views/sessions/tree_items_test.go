package sessions

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/pkg/kv"
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
		{name: "pane", item: TreeItem{IsPaneItem: true}, want: false},
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

func TestRenderPanePrefixContinuesParentWindowLine(t *testing.T) {
	delegate := NewTreeDelegate()
	got := terminal.StripANSI(delegate.renderPane(TreeItem{
		IsPaneItem:   true,
		PaneID:       "%2",
		PaneTool:     "claude",
		PaneStatus:   terminal.StatusReady,
		PaneIsAgent:  true,
		IsLastInRepo: false,
		IsLastWindow: false,
	}, false))

	// Session not last → "│   "; window not last → "│   "; then connector.
	wantPrefix := "│   │   ├─"
	if got[:len(wantPrefix)] != wantPrefix {
		t.Fatalf("renderPane prefix = %q, want prefix %q", got, wantPrefix)
	}
}

func TestRenderPanePrefixStopsAtLastWindow(t *testing.T) {
	delegate := NewTreeDelegate()
	got := terminal.StripANSI(delegate.renderPane(TreeItem{
		IsPaneItem:   true,
		PaneID:       "%2",
		PaneTool:     "claude",
		PaneStatus:   terminal.StatusReady,
		PaneIsAgent:  true,
		IsLastInRepo: false,
		IsLastWindow: true,
	}, false))

	// Session not last → "│   "; window last → "    "; then connector.
	wantPrefix := "│       ├─"
	if got[:len(wantPrefix)] != wantPrefix {
		t.Fatalf("renderPane prefix = %q, want prefix %q", got, wantPrefix)
	}
}

func TestRenderSessionDoesNotShowAggregatePorts(t *testing.T) {
	statuses := kv.New[string, TerminalStatus]()
	statuses.Set("s1", TerminalStatus{Status: terminal.StatusReady, Ports: []int{3000, 8080}})
	delegate := NewTreeDelegate()
	delegate.TerminalStatuses = statuses
	delegate.PreviewMode = true
	item := TreeItem{Session: session.Session{ID: "s1", Name: "alpha", State: session.StateActive}}
	model := list.New([]list.Item{item}, delegate, 80, 24)

	got := terminal.StripANSI(delegate.renderSession(item, false, model, 0))
	if strings.Contains(got, ":3000") || strings.Contains(got, ":8080") {
		t.Fatalf("session row should not show aggregate ports: %q", got)
	}
}

func TestRenderWindowAndPaneShowPorts(t *testing.T) {
	statuses := kv.New[string, TerminalStatus]()
	statuses.Set("s1", TerminalStatus{Windows: []WindowStatus{{WindowIndex: "0", WindowName: "main", Status: terminal.StatusReady, HasAgent: true, Ports: []int{3000}}}})
	delegate := NewTreeDelegate()
	delegate.TerminalStatuses = statuses
	parent := session.Session{ID: "s1", Name: "alpha", State: session.StateActive}

	window := terminal.StripANSI(delegate.renderWindow(TreeItem{IsWindowItem: true, WindowIndex: "0", WindowName: "main", ParentSession: parent}, false))
	pane := terminal.StripANSI(delegate.renderPane(TreeItem{IsPaneItem: true, PaneID: "%2", PaneTool: "zsh", Ports: []int{8080}}, false))

	if !strings.Contains(window, ":3000") {
		t.Fatalf("window row should show ports: %q", window)
	}
	if !strings.Contains(pane, ":8080") {
		t.Fatalf("pane row should show ports: %q", pane)
	}
}

func TestDisplayPaneID(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "tmux pane id", in: "%1", want: "#1"},
		{name: "multi digit", in: "%12", want: "#12"},
		{name: "already display id", in: "#3", want: "#3"},
		{name: "empty", in: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := displayPaneID(tt.in); got != tt.want {
				t.Errorf("displayPaneID(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestTreeItemsSessions(t *testing.T) {
	items := []list.Item{
		TreeItem{IsHeader: true, RepoName: "repo1"},
		TreeItem{Session: session.Session{ID: "s1"}},
		TreeItem{IsRecycledPlaceholder: true},
		TreeItem{Session: session.Session{ID: "s2"}},
		TreeItem{IsWindowItem: true},
		TreeItem{IsPaneItem: true},
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
