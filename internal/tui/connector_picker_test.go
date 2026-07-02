package tui

import (
	"context"
	"fmt"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/connectors"
	"github.com/colonyops/hive/internal/core/terminal"
)

// fakeTUIConnector is a test double for connectors.Connector that records
// call counts/arguments for Search and FetchDetail so picker tests can
// assert call-count and debouncing behavior.
type fakeTUIConnector struct {
	mu sync.Mutex

	manifest connectors.Manifest
	items    []connectors.Item
	detail   map[string]connectors.Detail

	searchErr error
	detailErr map[string]error

	searchCalls []string // queries passed to Search, in call order
	detailCalls map[string]int
}

func newFakeTUIConnector(manifest connectors.Manifest, items []connectors.Item) *fakeTUIConnector {
	return &fakeTUIConnector{
		manifest:    manifest,
		items:       items,
		detail:      make(map[string]connectors.Detail),
		detailErr:   make(map[string]error),
		detailCalls: make(map[string]int),
	}
}

func (f *fakeTUIConnector) Name() string                     { return "fake" }
func (f *fakeTUIConnector) Available(_ context.Context) bool { return true }
func (f *fakeTUIConnector) Initialize(_ context.Context) (connectors.Manifest, error) {
	return f.manifest, nil
}

func (f *fakeTUIConnector) Search(_ context.Context, params connectors.SearchParams) (connectors.SearchResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.searchCalls = append(f.searchCalls, params.Query)
	if f.searchErr != nil {
		return connectors.SearchResult{}, f.searchErr
	}
	if params.Query == "" {
		return connectors.SearchResult{Items: f.items}, nil
	}
	var filtered []connectors.Item
	for _, item := range f.items {
		if item.Title == params.Query {
			filtered = append(filtered, item)
		}
	}
	return connectors.SearchResult{Items: filtered}, nil
}

func (f *fakeTUIConnector) FetchDetail(_ context.Context, params connectors.FetchDetailParams) (connectors.Detail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.detailCalls[params.ID]++
	if err, ok := f.detailErr[params.ID]; ok {
		return connectors.Detail{}, err
	}
	return f.detail[params.ID], nil
}

func (f *fakeTUIConnector) searchCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.searchCalls)
}

func (f *fakeTUIConnector) detailCallCount(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.detailCalls[id]
}

var _ connectors.Connector = (*fakeTUIConnector)(nil)

// drainPicker synchronously executes cmd and feeds resulting message(s) back
// into the picker's Update loop, following batches and follow-up commands
// until none remain. Debounce ticks execute in real time, so tests use small
// DebounceMS values to keep this fast.
func drainPicker(t *testing.T, p ConnectorPicker, cmd tea.Cmd) ConnectorPicker {
	t.Helper()
	if cmd == nil {
		return p
	}
	msg := cmd()
	return applyPickerMsg(t, p, msg)
}

func applyPickerMsg(t *testing.T, p ConnectorPicker, msg tea.Msg) ConnectorPicker {
	t.Helper()
	if msg == nil {
		return p
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, sub := range batch {
			p = drainPicker(t, p, sub)
		}
		return p
	}
	next, cmd := p.Update(msg)
	return drainPicker(t, next, cmd)
}

func listManifest() connectors.Manifest {
	return connectors.Manifest{
		ID:          "fake",
		DisplayName: "Fake Connector",
		Capabilities: connectors.Capabilities{
			FetchDetail: true,
		},
		Picker: connectors.PickerManifest{
			Layout: connectors.LayoutModeList,
			Search: connectors.SearchManifest{
				Mode: connectors.SearchModeLocal,
			},
		},
	}
}

func remoteManifest() connectors.Manifest {
	m := listManifest()
	m.Picker.Search.Mode = connectors.SearchModeRemote
	m.Picker.Search.DebounceMS = 5
	return m
}

// typeKey feeds each rune of s into the picker as a keystroke without
// draining between characters, then drains all resulting commands
// afterward. This mirrors real usage where a debounced remote search should
// settle on the final query once the user stops typing: intermediate
// debounce ticks fire (since they run in real time) but are discarded by
// handleDebounce because their Query no longer matches the final input.
func typeKey(t *testing.T, p ConnectorPicker, s string) ConnectorPicker {
	t.Helper()
	var cmds []tea.Cmd
	for _, r := range s {
		next, cmd := p.Update(tea.KeyPressMsg{Text: string(r), Code: r})
		p = next
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	for _, cmd := range cmds {
		p = drainPicker(t, p, cmd)
	}
	return p
}

func TestConnectorPicker_LocalFilterDoesNotCallSearch(t *testing.T) {
	items := []connectors.Item{
		{ID: "1", Title: "alpha"},
		{ID: "2", Title: "beta"},
	}
	fake := newFakeTUIConnector(listManifest(), items)
	p := NewConnectorPicker(fake, listManifest(), "", 80, 24)
	p = drainPicker(t, p, p.Init())

	require.Equal(t, 1, fake.searchCallCount(), "initial load issues exactly one Search")
	require.Len(t, p.items, 2)

	p = typeKey(t, p, "alpha")

	assert.Equal(t, 1, fake.searchCallCount(), "local filtering must not call Search again")
	require.Len(t, p.items, 1)
	assert.Equal(t, "alpha", p.items[0].Title)
}

func TestConnectorPicker_RemoteSearchDebouncesAndCallsWithQuery(t *testing.T) {
	items := []connectors.Item{
		{ID: "1", Title: "alpha"},
		{ID: "2", Title: "beta"},
	}
	fake := newFakeTUIConnector(remoteManifest(), items)
	p := NewConnectorPicker(fake, remoteManifest(), "", 80, 24)
	p = drainPicker(t, p, p.Init())
	require.Equal(t, 1, fake.searchCallCount())

	p = typeKey(t, p, "beta")

	require.Equal(t, 2, fake.searchCallCount(), "remote mode issues one Search per settled query")
	assert.Equal(t, []string{"", "beta"}, fake.searchCalls)
	require.Len(t, p.items, 1)
	assert.Equal(t, "beta", p.items[0].Title)
}

func TestConnectorPicker_SelectionAndCancel(t *testing.T) {
	items := []connectors.Item{{ID: "1", Title: "alpha"}}
	fake := newFakeTUIConnector(listManifest(), items)
	p := NewConnectorPicker(fake, listManifest(), "", 80, 24)
	p = drainPicker(t, p, p.Init())

	next, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	p = drainPicker(t, next, cmd)

	result, ok := p.Selected()
	require.True(t, ok)
	assert.Equal(t, "1", result.Item.ID)
	assert.False(t, p.Cancelled())

	p2 := NewConnectorPicker(fake, listManifest(), "", 80, 24)
	p2 = drainPicker(t, p2, p2.Init())
	next2, cmd2 := p2.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	p2 = drainPicker(t, next2, cmd2)
	assert.True(t, p2.Cancelled())
	_, ok = p2.Selected()
	assert.False(t, ok)
}

func TestConnectorPicker_LazyDetailFetchIsCachedPerID(t *testing.T) {
	items := []connectors.Item{
		{ID: "1", Title: "alpha"},
		{ID: "2", Title: "beta"},
	}
	fake := newFakeTUIConnector(listManifest(), items)
	fake.detail["1"] = connectors.Detail{Markdown: &connectors.MarkdownDetail{Content: "alpha body"}}
	fake.detail["2"] = connectors.Detail{Markdown: &connectors.MarkdownDetail{Content: "beta body"}}

	p := NewConnectorPicker(fake, listManifest(), "", 80, 24)
	p = drainPicker(t, p, p.Init())

	assert.Equal(t, 1, fake.detailCallCount("1"), "cursor starts on first item, fetching its detail once")

	next, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	p = drainPicker(t, next, cmd)
	assert.Equal(t, 1, fake.detailCallCount("2"))

	next, cmd = p.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	p = drainPicker(t, next, cmd)
	assert.Equal(t, 1, fake.detailCallCount("1"), "revisiting item 1 must use the cache, not refetch")

	assert.Contains(t, terminal.StripANSI(p.renderDetailPane(40)), "alpha body")
}

func TestConnectorPicker_EmptyResultsShowsMessageAndEnterIsNoop(t *testing.T) {
	fake := newFakeTUIConnector(listManifest(), nil)
	p := NewConnectorPicker(fake, listManifest(), "", 80, 24)
	p = drainPicker(t, p, p.Init())

	require.Empty(t, p.items)
	assert.Contains(t, p.renderList(40), "no results")

	next, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	p = drainPicker(t, next, cmd)

	_, ok := p.Selected()
	assert.False(t, ok, "enter on an empty list must not select anything")
}

func TestConnectorPicker_NoDetailItemShowsPlaceholder(t *testing.T) {
	manifest := listManifest()
	manifest.Capabilities.FetchDetail = false
	items := []connectors.Item{{ID: "1", Title: "alpha"}}
	fake := newFakeTUIConnector(manifest, items)

	p := NewConnectorPicker(fake, manifest, "", 80, 24)
	p = drainPicker(t, p, p.Init())

	assert.Equal(t, 0, fake.detailCallCount("1"), "FetchDetail must not be called when the manifest doesn't support it")
	assert.NotPanics(t, func() {
		out := terminal.StripANSI(p.renderDetailPane(40))
		assert.Contains(t, out, "no detail available")
	})
}

func TestConnectorPicker_DetailFetchErrorRendersInPane(t *testing.T) {
	items := []connectors.Item{{ID: "1", Title: "alpha"}}
	fake := newFakeTUIConnector(listManifest(), items)
	fake.detailErr["1"] = fmt.Errorf("boom")

	p := NewConnectorPicker(fake, listManifest(), "", 80, 24)
	p = drainPicker(t, p, p.Init())

	out := terminal.StripANSI(p.renderDetailPane(40))
	assert.Contains(t, out, "boom")
}

func TestConnectorPicker_SearchErrorIsShownAndNonFatal(t *testing.T) {
	fake := newFakeTUIConnector(listManifest(), nil)
	fake.searchErr = fmt.Errorf("gh: unauthenticated")

	p := NewConnectorPicker(fake, listManifest(), "", 80, 24)
	p = drainPicker(t, p, p.Init())

	assert.Contains(t, terminal.StripANSI(p.renderList(40)), "gh: unauthenticated")
	assert.NotPanics(t, func() {
		p.View()
	})
}
