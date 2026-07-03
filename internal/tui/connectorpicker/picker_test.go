package connectorpicker

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/connectors"
	"github.com/colonyops/hive/internal/core/styles"
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
func drainPicker(t *testing.T, p Picker, cmd tea.Cmd) Picker {
	t.Helper()
	if cmd == nil {
		return p
	}
	msg := cmd()
	return applyPickerMsg(t, p, msg)
}

func applyPickerMsg(t *testing.T, p Picker, msg tea.Msg) Picker {
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
func typeKey(t *testing.T, p Picker, s string) Picker {
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

func TestPicker_LocalFilterDoesNotCallSearch(t *testing.T) {
	items := []connectors.Item{
		{ID: "1", Title: "alpha"},
		{ID: "2", Title: "beta"},
	}
	fake := newFakeTUIConnector(listManifest(), items)
	p := New(fake, listManifest(), "", 80, 24)
	p = drainPicker(t, p, p.Init())

	require.Equal(t, 1, fake.searchCallCount(), "initial load issues exactly one Search")
	require.Len(t, p.items, 2)

	p = typeKey(t, p, "alpha")

	assert.Equal(t, 1, fake.searchCallCount(), "local filtering must not call Search again")
	require.Len(t, p.items, 1)
	assert.Equal(t, "alpha", p.items[0].Title)
}

func TestPicker_RemoteSearchDebouncesAndCallsWithQuery(t *testing.T) {
	items := []connectors.Item{
		{ID: "1", Title: "alpha"},
		{ID: "2", Title: "beta"},
	}
	fake := newFakeTUIConnector(remoteManifest(), items)
	p := New(fake, remoteManifest(), "", 80, 24)
	p = drainPicker(t, p, p.Init())
	require.Equal(t, 1, fake.searchCallCount())

	p = typeKey(t, p, "beta")

	require.Equal(t, 2, fake.searchCallCount(), "remote mode issues one Search per settled query")
	assert.Equal(t, []string{"", "beta"}, fake.searchCalls)
	require.Len(t, p.items, 1)
	assert.Equal(t, "beta", p.items[0].Title)
}

func TestPicker_SelectionAndCancel(t *testing.T) {
	items := []connectors.Item{{ID: "1", Title: "alpha"}}
	fake := newFakeTUIConnector(listManifest(), items)
	p := New(fake, listManifest(), "", 80, 24)
	p = drainPicker(t, p, p.Init())

	next, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	p = drainPicker(t, next, cmd)

	result, ok := p.Selected()
	require.True(t, ok)
	assert.Equal(t, "1", result.Item.ID)
	assert.False(t, p.Cancelled())

	p2 := New(fake, listManifest(), "", 80, 24)
	p2 = drainPicker(t, p2, p2.Init())
	next2, cmd2 := p2.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	p2 = drainPicker(t, next2, cmd2)
	assert.True(t, p2.Cancelled())
	_, ok = p2.Selected()
	assert.False(t, ok)
}

func TestPicker_LazyDetailFetchIsCachedPerID(t *testing.T) {
	items := []connectors.Item{
		{ID: "1", Title: "alpha"},
		{ID: "2", Title: "beta"},
	}
	fake := newFakeTUIConnector(listManifest(), items)
	fake.detail["1"] = connectors.Detail{Markdown: &connectors.MarkdownDetail{Content: "alpha body"}}
	fake.detail["2"] = connectors.Detail{Markdown: &connectors.MarkdownDetail{Content: "beta body"}}

	p := New(fake, listManifest(), "", 80, 24)
	p = drainPicker(t, p, p.Init())

	assert.Equal(t, 1, fake.detailCallCount("1"), "cursor starts on first item, fetching its detail once")

	next, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	p = drainPicker(t, next, cmd)
	assert.Equal(t, 1, fake.detailCallCount("2"))

	next, cmd = p.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	p = drainPicker(t, next, cmd)
	assert.Equal(t, 1, fake.detailCallCount("1"), "revisiting item 1 must use the cache, not refetch")

	assert.Contains(t, terminal.StripANSI(p.detailVP.View()), "alpha body")
}

func TestPicker_ListRowShowsMetadataWithoutState(t *testing.T) {
	item := connectors.Item{
		ID:    "1278",
		Title: "feat: make expiring-exemptions endpoints public",
		Fields: map[string]any{
			"number": 1278,
			"state":  "OPEN",
			"author": "alice",
			"labels": []string{"api", "public"},
		},
	}
	p := New(newFakeTUIConnector(listManifest(), []connectors.Item{item}), listManifest(), "", 100, 30)

	row := terminal.StripANSI(p.renderRow(item, true, 48))

	assert.Contains(t, row, styles.IconSelector+" feat: make expiring-exemptions")
	assert.Contains(t, row, "#1278")
	assert.Contains(t, row, "@alice")
	assert.Contains(t, row, "[api]")
	assert.NotContains(t, row, "OPEN")
}

func TestPicker_EmptyResultsShowsMessageAndEnterIsNoop(t *testing.T) {
	fake := newFakeTUIConnector(listManifest(), nil)
	p := New(fake, listManifest(), "", 80, 24)
	p = drainPicker(t, p, p.Init())

	require.Empty(t, p.items)
	assert.Contains(t, p.renderList(40), "no results")

	next, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	p = drainPicker(t, next, cmd)

	_, ok := p.Selected()
	assert.False(t, ok, "enter on an empty list must not select anything")
}

func TestPicker_NoDetailItemShowsPlaceholder(t *testing.T) {
	manifest := listManifest()
	manifest.Capabilities.FetchDetail = false
	items := []connectors.Item{{ID: "1", Title: "alpha"}}
	fake := newFakeTUIConnector(manifest, items)

	p := New(fake, manifest, "", 80, 24)
	p = drainPicker(t, p, p.Init())

	assert.Equal(t, 0, fake.detailCallCount("1"), "FetchDetail must not be called when the manifest doesn't support it")
	assert.NotPanics(t, func() {
		out := terminal.StripANSI(p.detailVP.View())
		assert.Contains(t, out, "no detail available")
	})
}

func TestPicker_DetailFetchErrorRendersInPane(t *testing.T) {
	items := []connectors.Item{{ID: "1", Title: "alpha"}}
	fake := newFakeTUIConnector(listManifest(), items)
	fake.detailErr["1"] = fmt.Errorf("boom")

	p := New(fake, listManifest(), "", 80, 24)
	p = drainPicker(t, p, p.Init())

	out := terminal.StripANSI(p.detailVP.View())
	assert.Contains(t, out, "boom")
}

func TestPicker_SearchErrorIsShownAndNonFatal(t *testing.T) {
	fake := newFakeTUIConnector(listManifest(), nil)
	fake.searchErr = fmt.Errorf("gh: unauthenticated")

	p := New(fake, listManifest(), "", 80, 24)
	p = drainPicker(t, p, p.Init())

	assert.Contains(t, terminal.StripANSI(p.renderList(40)), "gh: unauthenticated")
	assert.NotPanics(t, func() {
		p.View()
	})
}

// TestPicker_ViewFitsTerminal is the regression test for the
// picker overflowing small terminals: the rendered view (including modal
// border, padding, and the help line's margin) must never exceed the
// terminal height the picker was sized for, and loading a long detail body
// must not change the frame height.
func TestPicker_ViewFitsTerminal(t *testing.T) {
	sizes := []struct{ width, height int }{
		{80, 24},
		{90, 24},
		{100, 30},
		{120, 38},
		{120, 50},
	}
	long := strings.Repeat("This is a long line of markdown detail content. ", 40) +
		"\n\n" + strings.Repeat("Another paragraph. ", 60)

	for _, size := range sizes {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			items := []connectors.Item{{ID: "1", Title: "Item one"}}
			fake := newFakeTUIConnector(listManifest(), items)
			fake.detail["1"] = connectors.Detail{Markdown: &connectors.MarkdownDetail{Content: long}}

			p := New(fake, listManifest(), "", size.width, size.height)
			before := lipgloss.Height(p.View())
			assert.LessOrEqual(t, before, size.height, "picker must fit the terminal before loading")

			p = drainPicker(t, p, p.Init())

			after := lipgloss.Height(p.View())
			assert.Equal(t, before, after, "loading detail must not change the frame height")
			assert.LessOrEqual(t, after, size.height, "picker must fit the terminal after loading")
		})
	}
}

// TestPicker_StaleGenerationMessagesAreDropped guards against a
// late async result from a previously closed picker (possibly for a
// different scope) poisoning the current picker's caches.
func TestPicker_StaleGenerationMessagesAreDropped(t *testing.T) {
	items := []connectors.Item{{ID: "1", Title: "alpha"}}
	fake := newFakeTUIConnector(listManifest(), items)
	p := New(fake, listManifest(), "", 80, 24)
	p = drainPicker(t, p, p.Init())

	stale := connectorDetailResultMsg{
		Gen:    p.gen - 1,
		ID:     "1",
		Detail: connectors.Detail{Markdown: &connectors.MarkdownDetail{Content: "wrong repo body"}},
	}
	p = applyPickerMsg(t, p, stale)

	assert.NotContains(t, terminal.StripANSI(p.detailVP.View()), "wrong repo body")

	staleSearch := connectorSearchResultMsg{
		Gen:   p.gen - 1,
		Query: p.input.Value(),
		Items: []connectors.Item{{ID: "9", Title: "poisoned"}},
	}
	p = applyPickerMsg(t, p, staleSearch)
	require.Len(t, p.items, 1)
	assert.Equal(t, "alpha", p.items[0].Title)
}

// TestPicker_NonEditingKeyDoesNotResetOrResearch verifies that
// keys which don't change the query (e.g. left/right) neither reset the
// cursor nor re-issue a remote search.
func TestPicker_NonEditingKeyDoesNotResetOrResearch(t *testing.T) {
	items := []connectors.Item{
		{ID: "1", Title: "alpha"},
		{ID: "2", Title: "beta"},
	}
	fake := newFakeTUIConnector(remoteManifest(), items)
	p := New(fake, remoteManifest(), "", 80, 24)
	p = drainPicker(t, p, p.Init())
	require.Equal(t, 1, fake.searchCallCount())

	next, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	p = drainPicker(t, next, cmd)
	require.Equal(t, 1, p.cursor)

	next, cmd = p.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	p = drainPicker(t, next, cmd)

	assert.Equal(t, 1, p.cursor, "non-editing key must not reset the cursor")
	assert.Equal(t, 1, fake.searchCallCount(), "non-editing key must not re-issue a remote search")
}

// TestPicker_ClearingFilterRefreshesDetailPane verifies the detail
// pane follows the cursor back to item 0 when a local filter is cleared.
func TestPicker_ClearingFilterRefreshesDetailPane(t *testing.T) {
	items := []connectors.Item{
		{ID: "1", Title: "alpha", Detail: connectors.Detail{Markdown: &connectors.MarkdownDetail{Content: "alpha body"}}},
		{ID: "2", Title: "beta", Detail: connectors.Detail{Markdown: &connectors.MarkdownDetail{Content: "beta body"}}},
	}
	fake := newFakeTUIConnector(listManifest(), items)
	p := New(fake, listManifest(), "", 80, 24)
	p = drainPicker(t, p, p.Init())

	p = typeKey(t, p, "beta")
	require.Len(t, p.items, 1)
	require.Contains(t, terminal.StripANSI(p.detailVP.View()), "beta body")

	next, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	p = drainPicker(t, next, cmd)
	for range 3 {
		next, cmd = p.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
		p = drainPicker(t, next, cmd)
	}
	require.Empty(t, p.input.Value())

	require.Len(t, p.items, 2)
	assert.Equal(t, 0, p.cursor)
	assert.Contains(t, terminal.StripANSI(p.detailVP.View()), "alpha body", "detail pane must refresh to the item under the cursor")
}

// TestPicker_InitShowsSearching verifies the initial load renders
// a "searching..." status instead of a blank list (Init must set
// searchInFlight on the stored picker).
func TestPicker_InitShowsSearching(t *testing.T) {
	fake := newFakeTUIConnector(listManifest(), nil)
	p := New(fake, listManifest(), "", 80, 24)
	cmd := p.Init()
	require.NotNil(t, cmd)

	assert.Contains(t, terminal.StripANSI(p.renderList(40)), "searching...")
}

// TestPicker_TableRowsNeverWrap is the regression test for table
// rows with flex columns and long titles wrapping onto multiple lines and
// destroying the fixed-height layout: each rendered row must be exactly
// one line, and the row must span the pane width rather than the flex
// column collapsing to its 12-column default.
func TestPicker_TableRowsNeverWrap(t *testing.T) {
	manifest := connectors.Manifest{
		ID:          "fake-prs",
		DisplayName: "Fake Pull Requests",
		Picker: connectors.PickerManifest{
			Layout:      connectors.LayoutModeTable,
			HidePreview: true,
			Columns: []connectors.Column{
				{Key: "number", Label: "#", Width: 6},
				{Key: "title", Label: "Title", Flex: 1},
				{Key: "author", Label: "Author", Width: 14},
			},
			Search: connectors.SearchManifest{Mode: connectors.SearchModeLocal},
		},
	}
	long := strings.Repeat("very long pull request title ", 10)
	item := connectors.Item{ID: "1", Title: long, Fields: map[string]any{
		"number": 1, "title": long, "author": "someone",
	}}
	fake := newFakeTUIConnector(manifest, []connectors.Item{item})
	p := New(fake, manifest, "", 120, 40)
	p = drainPicker(t, p, p.Init())

	row := p.renderRow(item, true, p.listWidth)
	assert.Equal(t, 1, lipgloss.Height(row), "table row must render as exactly one line")
	assert.Greater(t, lipgloss.Width(row), p.listWidth/2, "flex column must expand into available width")
	assert.LessOrEqual(t, lipgloss.Width(row), p.listWidth, "row must not exceed the pane width")
}

// TestResolveColumnWidths covers the flex-width math directly.
func TestResolveColumnWidths(t *testing.T) {
	cols := []connectors.Column{
		{Key: "number", Width: 6},
		{Key: "title", Flex: 1},
		{Key: "author", Width: 14},
	}
	widths := resolveConnectorColumnWidths(cols, 100)
	assert.Equal(t, 6, widths[0])
	assert.Equal(t, 14, widths[2])
	// total(100) - fixed(20) - separators(2) = 78 for the flex column.
	assert.Equal(t, 78, widths[1])

	// Column with neither Width nor Flex defaults to 12.
	widths = resolveConnectorColumnWidths([]connectors.Column{{Key: "a"}, {Key: "b", Flex: 1}}, 40)
	assert.Equal(t, 12, widths[0])
	assert.Equal(t, 27, widths[1]) // 40 - 12 - 1 separator

	// Flex columns never collapse below the floor on tiny panes.
	widths = resolveConnectorColumnWidths(cols, 20)
	assert.GreaterOrEqual(t, widths[1], connectorFlexColumnMinWidth)
}

// TestPicker_HidePreviewSinglePane verifies the single-pane mode:
// the list gets the full content width, no divider or detail viewport is
// rendered, and the view still fits the terminal.
func TestPicker_HidePreviewSinglePane(t *testing.T) {
	manifest := listManifest()
	manifest.Capabilities.FetchDetail = false
	manifest.Picker.HidePreview = true

	items := []connectors.Item{{ID: "1", Title: "alpha"}}
	fake := newFakeTUIConnector(manifest, items)
	p := New(fake, manifest, "", 100, 30)
	p = drainPicker(t, p, p.Init())

	assert.Equal(t, 0, p.detailWidth, "no detail pane in single-pane mode")
	view := terminal.StripANSI(p.View())
	for _, line := range strings.Split(view, "\n") {
		// The modal border contributes two "│" per line; a pane divider
		// would add a third.
		assert.LessOrEqual(t, strings.Count(line, "│"), 2, "no divider in single-pane mode: %q", line)
	}
	assert.NotContains(t, view, "ctrl+u/d", "help must not advertise detail scrolling")
	assert.LessOrEqual(t, lipgloss.Height(p.View()), 30, "single-pane view must fit the terminal")
}

// TestTableCellStyle covers the semantic column/value → style mapping.
func TestTableCellStyle(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		want  lipgloss.Style
	}{
		{"number column is primary", "number", "1315", styles.TextPrimaryStyle},
		{"id column is primary", "id", "42", styles.TextPrimaryStyle},
		{"author column is muted", "author", "alice", styles.TextMutedStyle},
		{"approved is success", "review", "approved", styles.TextSuccessStyle},
		{"open state is success (case-insensitive)", "state", "OPEN", styles.TextSuccessStyle},
		{"changes requested is error", "review", "changes requested", styles.TextErrorStyle},
		{"closed is error", "state", "CLOSED", styles.TextErrorStyle},
		{"review required is warning", "review", "review required", styles.TextWarningStyle},
		{"draft is muted", "review", "draft", styles.TextMutedStyle},
		{"merged is secondary", "state", "MERGED", styles.TextSecondaryStyle},
		{"plain title is foreground", "title", "fix a bug", styles.TextForegroundStyle},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tableCellStyle(tt.key, tt.value)
			assert.Equal(t, tt.want.Render(tt.value), got.Render(tt.value))
		})
	}
}

// TestConnectorTableRowColors verifies semantic colors appear on unselected
// rows and that the selected row renders as a uniform primary-bold bar.
func TestConnectorTableRowColors(t *testing.T) {
	columns := []connectors.Column{
		{Key: "number", Width: 6},
		{Key: "title", Flex: 1},
		{Key: "review", Width: 18},
	}
	item := connectors.Item{ID: "10", Title: "Add feature", Fields: map[string]any{
		"number": 10, "title": "Add feature", "review": "approved",
	}}

	unselected := renderConnectorTableRow(item, columns, 60, false)
	assert.Contains(t, unselected, styles.TextPrimaryStyle.Render("10"), "number cell must use the primary accent")
	assert.Contains(t, unselected, styles.TextSuccessStyle.Render("approved"), "approved must render in the success color")

	selected := renderConnectorTableRow(item, columns, 60, true)
	assert.Contains(t, selected, styles.TextPrimaryBoldStyle.Render("approved"), "selected row must be a uniform primary-bold bar")
	assert.NotContains(t, selected, styles.TextSuccessStyle.Render("approved"))
}
