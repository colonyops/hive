package sourcepicker

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/sources"
)

// fakeTUISource is a test double for sources.Source that records
// call counts/arguments for Search and FetchDetail so picker tests can
// assert call-count and debouncing behavior.
type fakeTUISource struct {
	mu sync.Mutex

	manifest sources.Manifest
	items    []sources.Item
	detail   map[string]sources.Detail

	searchErr error
	detailErr map[string]error

	searchCalls []string // queries passed to Search, in call order
	detailCalls map[string]int
}

func newFakeTUISource(manifest sources.Manifest, items []sources.Item) *fakeTUISource {
	return &fakeTUISource{
		manifest:    manifest,
		items:       items,
		detail:      make(map[string]sources.Detail),
		detailErr:   make(map[string]error),
		detailCalls: make(map[string]int),
	}
}

func (f *fakeTUISource) Name() string                     { return "fake" }
func (f *fakeTUISource) Available(_ context.Context) bool { return true }
func (f *fakeTUISource) Initialize(_ context.Context) (sources.Manifest, error) {
	return f.manifest, nil
}

func (f *fakeTUISource) Search(_ context.Context, params sources.SearchParams) (sources.SearchResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.searchCalls = append(f.searchCalls, params.Query)
	if f.searchErr != nil {
		return sources.SearchResult{}, f.searchErr
	}
	if params.Query == "" {
		return sources.SearchResult{Items: f.items}, nil
	}
	var filtered []sources.Item
	for _, item := range f.items {
		if item.Title == params.Query {
			filtered = append(filtered, item)
		}
	}
	return sources.SearchResult{Items: filtered}, nil
}

func (f *fakeTUISource) FetchDetail(_ context.Context, params sources.FetchDetailParams) (sources.Detail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.detailCalls[params.ID]++
	if err, ok := f.detailErr[params.ID]; ok {
		return sources.Detail{}, err
	}
	return f.detail[params.ID], nil
}

var _ sources.Source = (*fakeTUISource)(nil)

// newTestPicker creates a Picker wrapping a single fake source for tests.
func newTestPicker(fake *fakeTUISource, manifest sources.Manifest, scope string, w, h int) Picker {
	tabs := []TabSource{{
		ID:        manifest.ID,
		Source:    fake,
		Manifest:  manifest,
		Templates: sources.TemplateConfig{},
	}}
	return New(tabs, manifest.ID, scope, w, h)
}

// drainPicker synchronously executes cmd and feeds resulting message(s) back
// into the picker's Update loop, following batches and follow-up commands
// until none remain. Spinner ticks are filtered out to prevent infinite loops.
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
	// Filter out spinner ticks to avoid infinite test loops.
	if _, ok := msg.(spinner.TickMsg); ok {
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

func listManifest() sources.Manifest {
	return sources.Manifest{
		ID:          "fake",
		DisplayName: "Fake Source",
		Capabilities: sources.Capabilities{
			FetchDetail: true,
		},
		Picker: sources.PickerManifest{
			Layout: sources.LayoutModeList,
			Search: sources.SearchManifest{
				Mode: sources.SearchModeLocal,
			},
		},
	}
}

// enterSearch puts the picker into search mode.
func enterSearch(t *testing.T, p Picker) Picker {
	t.Helper()
	next, cmd := p.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	p = drainPicker(t, next, cmd)
	require.True(t, p.searchMode, "pressing / must enter search mode")
	return p
}

// typeKey feeds each rune of s as a keystroke.
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

// activeTab returns the current tab's state for assertions.
func activeTab(p Picker) *tabState {
	return &p.tabs[p.activeTab]
}

func TestPicker_LocalFilterPreservesItems(t *testing.T) {
	items := []sources.Item{
		{ID: "1", Title: "alpha"},
		{ID: "2", Title: "beta"},
	}
	fake := newFakeTUISource(listManifest(), items)
	p := newTestPicker(fake, listManifest(), "", 80, 24)
	p = drainPicker(t, p, p.Init())

	tab := activeTab(p)
	require.Len(t, tab.filteredItems, 2)

	p = enterSearch(t, p)
	p = typeKey(t, p, "alpha")

	tab = activeTab(p)
	require.Len(t, tab.filteredItems, 1)
	assert.Equal(t, "alpha", tab.filteredItems[0].Title)
}

func TestPicker_SelectionAndCancel(t *testing.T) {
	items := []sources.Item{{ID: "1", Title: "alpha"}}
	fake := newFakeTUISource(listManifest(), items)
	p := newTestPicker(fake, listManifest(), "", 80, 24)
	p = drainPicker(t, p, p.Init())

	next, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	p = drainPicker(t, next, cmd)

	result, ok := p.Selected()
	require.True(t, ok)
	assert.Equal(t, "1", result.Item.ID)
	assert.False(t, p.Cancelled())

	// Cancel test
	p2 := newTestPicker(fake, listManifest(), "", 80, 24)
	p2 = drainPicker(t, p2, p2.Init())
	next2, cmd2 := p2.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	p2 = drainPicker(t, next2, cmd2)
	assert.True(t, p2.Cancelled())
	_, ok = p2.Selected()
	assert.False(t, ok)
}

func TestPicker_EmptyResultsShowsEmptyState(t *testing.T) {
	fake := newFakeTUISource(listManifest(), nil)
	p := newTestPicker(fake, listManifest(), "test-repo", 80, 24)
	p = drainPicker(t, p, p.Init())

	tab := activeTab(p)
	require.Empty(t, tab.filteredItems)

	body := terminal.StripANSI(p.renderBody())
	assert.Contains(t, body, "No open")

	next, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	p = drainPicker(t, next, cmd)

	_, ok := p.Selected()
	assert.False(t, ok, "enter on an empty list must not select anything")
}

func TestPicker_SearchErrorRendersInBody(t *testing.T) {
	fake := newFakeTUISource(listManifest(), nil)
	fake.searchErr = fmt.Errorf("gh: unauthenticated")

	p := newTestPicker(fake, listManifest(), "", 80, 24)
	p = drainPicker(t, p, p.Init())

	body := terminal.StripANSI(p.renderBody())
	assert.Contains(t, body, "unauthenticated")
	assert.NotPanics(t, func() { p.View() })
}

func TestPicker_ViewFitsTerminal(t *testing.T) {
	sizes := []struct{ width, height int }{
		{80, 24},
		{90, 24},
		{100, 30},
		{120, 38},
		{120, 50},
	}

	for _, size := range sizes {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			items := []sources.Item{{ID: "1", Title: "Item one"}}
			fake := newFakeTUISource(listManifest(), items)

			p := newTestPicker(fake, listManifest(), "", size.width, size.height)
			p = drainPicker(t, p, p.Init())

			viewHeight := lipgloss.Height(p.View())
			assert.LessOrEqual(t, viewHeight, size.height, "picker must fit the terminal")
		})
	}
}

func TestPicker_StaleGenerationMessagesAreDropped(t *testing.T) {
	items := []sources.Item{{ID: "1", Title: "alpha"}}
	fake := newFakeTUISource(listManifest(), items)
	p := newTestPicker(fake, listManifest(), "", 80, 24)
	p = drainPicker(t, p, p.Init())

	staleSearch := sourceSearchResultMsg{
		Gen:      p.gen - 1,
		SourceID: "fake",
		Query:    p.input.Value(),
		Items:    []sources.Item{{ID: "9", Title: "poisoned"}},
	}
	p = applyPickerMsg(t, p, staleSearch)

	tab := activeTab(p)
	require.Len(t, tab.filteredItems, 1)
	assert.Equal(t, "alpha", tab.filteredItems[0].Title)
}

func TestPicker_NavigateModeIsDefault(t *testing.T) {
	items := []sources.Item{
		{ID: "1", Title: "alpha"},
		{ID: "2", Title: "beta"},
		{ID: "3", Title: "gamma"},
	}
	fake := newFakeTUISource(listManifest(), items)
	p := newTestPicker(fake, listManifest(), "", 80, 24)
	p = drainPicker(t, p, p.Init())
	require.False(t, p.searchMode, "picker must start in navigate mode")

	// j/k navigate
	next, cmd := p.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	p = drainPicker(t, next, cmd)
	assert.Equal(t, 1, activeTab(p).cursor, "j must move the cursor down")

	next, cmd = p.Update(tea.KeyPressMsg{Text: "k", Code: 'k'})
	p = drainPicker(t, next, cmd)
	assert.Equal(t, 0, activeTab(p).cursor, "k must move the cursor up")

	// Stray typing does not filter
	next, cmd = p.Update(tea.KeyPressMsg{Text: "x", Code: 'x'})
	p = drainPicker(t, next, cmd)
	assert.Empty(t, p.input.Value())
	assert.Len(t, activeTab(p).filteredItems, 3)

	// / enters search mode, esc leaves it
	p = enterSearch(t, p)
	p = typeKey(t, p, "beta")
	require.Len(t, activeTab(p).filteredItems, 1)

	next, cmd = p.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	p = drainPicker(t, next, cmd)
	assert.False(t, p.searchMode, "esc must leave search mode")
	assert.False(t, p.Cancelled(), "first esc must not cancel the picker")

	next, cmd = p.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	p = drainPicker(t, next, cmd)
	assert.True(t, p.Cancelled(), "esc in navigate mode must cancel")
}

func TestPicker_TabSwitching(t *testing.T) {
	items1 := []sources.Item{{ID: "1", Title: "PR one"}}
	items2 := []sources.Item{{ID: "2", Title: "Issue one"}, {ID: "3", Title: "Issue two"}}

	fake1 := newFakeTUISource(listManifest(), items1)
	m1 := listManifest()
	m1.ID = "prs"
	m1.DisplayName = "Pull Requests"

	fake2 := newFakeTUISource(listManifest(), items2)
	m2 := listManifest()
	m2.ID = "issues"
	m2.DisplayName = "Issues"

	tabs := []TabSource{
		{ID: "prs", Source: fake1, Manifest: m1},
		{ID: "issues", Source: fake2, Manifest: m2},
	}
	p := New(tabs, "prs", "", 80, 24)
	p = drainPicker(t, p, p.Init())

	assert.Equal(t, 0, p.activeTab)
	tab := activeTab(p)
	require.True(t, tab.initialized)
	require.Len(t, tab.filteredItems, 1)

	// Tab switches to the next source
	next, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	p = drainPicker(t, next, cmd)
	assert.Equal(t, 1, p.activeTab)

	// Second tab should now be initialized
	tab2 := activeTab(p)
	require.True(t, tab2.initialized)
	require.Len(t, tab2.filteredItems, 2)

	// Tab back preserves first tab's state
	next, cmd = p.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	p = drainPicker(t, next, cmd)
	assert.Equal(t, 0, p.activeTab)

	tab = activeTab(p)
	require.True(t, tab.initialized)
	require.Len(t, tab.filteredItems, 1)
}

func TestPicker_TabBarRendering(t *testing.T) {
	m1 := listManifest()
	m1.ID = "prs"
	m1.DisplayName = "Pull Requests"
	m2 := listManifest()
	m2.ID = "issues"
	m2.DisplayName = "Issues"

	tabs := []TabSource{
		{ID: "prs", Source: newFakeTUISource(m1, nil), Manifest: m1},
		{ID: "issues", Source: newFakeTUISource(m2, nil), Manifest: m2},
	}
	p := New(tabs, "prs", "owner/repo", 80, 24)

	bar := terminal.StripANSI(p.renderTabBar())
	assert.Contains(t, bar, "Pull Requests")
	assert.Contains(t, bar, "Issues")
	assert.Contains(t, bar, "owner/repo")
}

func TestPicker_RetryOnError(t *testing.T) {
	fake := newFakeTUISource(listManifest(), nil)
	fake.searchErr = fmt.Errorf("rate limited")

	p := newTestPicker(fake, listManifest(), "", 80, 24)
	p = drainPicker(t, p, p.Init())

	tab := activeTab(p)
	require.Error(t, tab.searchErr)

	// Fix the error and retry
	fake.searchErr = nil
	fake.items = []sources.Item{{ID: "1", Title: "alpha"}}

	next, cmd := p.Update(tea.KeyPressMsg{Text: "r", Code: 'r'})
	p = drainPicker(t, next, cmd)

	tab = activeTab(p)
	require.NoError(t, tab.searchErr)
	require.Len(t, tab.filteredItems, 1)
}

// TestPicker_TableRowsNeverWrap verifies table rows with flex columns
// and long titles remain single-line.
func TestPicker_TableRowsNeverWrap(t *testing.T) {
	manifest := sources.Manifest{
		ID:          "fake-prs",
		DisplayName: "Fake Pull Requests",
		Picker: sources.PickerManifest{
			Layout:      sources.LayoutModeTable,
			HidePreview: true,
			Columns: []sources.Column{
				{Key: "number", Label: "#", Width: 6},
				{Key: "title", Label: "Title", Flex: 1},
				{Key: "author", Label: "Author", Width: 14},
			},
			Search: sources.SearchManifest{Mode: sources.SearchModeLocal},
		},
	}
	long := strings.Repeat("very long pull request title ", 10)
	item := sources.Item{ID: "1", Title: long, Fields: map[string]any{
		"number": 1, "title": long, "author": "someone",
	}}

	tab := &tabState{tab: TabSource{ID: "fake-prs", Manifest: manifest}}
	fake := newFakeTUISource(manifest, []sources.Item{item})
	p := newTestPicker(fake, manifest, "", 120, 40)
	p = drainPicker(t, p, p.Init())

	row := p.renderRow(item, true, tab, 0)
	assert.Equal(t, 1, lipgloss.Height(row), "table row must render as exactly one line")
}

func TestResolveColumnWidths(t *testing.T) {
	cols := []sources.Column{
		{Key: "number", Width: 6},
		{Key: "title", Flex: 1},
		{Key: "author", Width: 14},
	}
	widths := resolveSourceColumnWidths(cols, 100)
	assert.Equal(t, 6, widths[0])
	assert.Equal(t, 14, widths[2])
	assert.Equal(t, 78, widths[1])

	widths = resolveSourceColumnWidths([]sources.Column{{Key: "a"}, {Key: "b", Flex: 1}}, 40)
	assert.Equal(t, 12, widths[0])
	assert.Equal(t, 27, widths[1])

	widths = resolveSourceColumnWidths(cols, 20)
	assert.GreaterOrEqual(t, widths[1], sourceFlexColumnMinWidth)
}

func TestTableCellStyle(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		want  lipgloss.Style
	}{
		{"number column is muted", "number", "1315", styles.TextMutedStyle},
		{"id column is muted", "id", "42", styles.TextMutedStyle},
		{"author column is muted", "author", "alice", styles.TextMutedStyle},
		{"approved is success", "review", "approved", styles.TextSuccessStyle},
		{"open state is success", "state", "OPEN", styles.TextSuccessStyle},
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

func TestSourceTableRowColors(t *testing.T) {
	columns := []sources.Column{
		{Key: "number", Width: 6},
		{Key: "title", Flex: 1},
		{Key: "review", Width: 18},
	}
	item := sources.Item{ID: "10", Title: "Add feature", Fields: map[string]any{
		"number": 10, "title": "Add feature", "review": "approved",
	}}

	styled := renderSourceTableRow(item, columns, 60, true)
	plain := renderSourceTableRow(item, columns, 60, false)

	// Styled cells keep their semantic colors (padding is inside the
	// styled span so widths match the plain variant).
	assert.Contains(t, styled, styles.TextMutedStyle.Render("#10   "))
	assert.Contains(t, styled, styles.TextSuccessStyle.Render("approved"+strings.Repeat(" ", 10)))

	// The plain variant is used for selected rows: it must contain no
	// ANSI sequences at all, otherwise embedded SGR resets terminate the
	// full-row highlight background mid-line. Text layout must be
	// identical between the two variants.
	assert.Equal(t, plain, terminal.StripANSI(plain), "plain table row must be ANSI-free")
	assert.Equal(t, plain, terminal.StripANSI(styled))
}

func TestStatusIcon(t *testing.T) {
	assert.Equal(t, "✓ ", statusIcon("passing"))
	assert.Equal(t, "✗ ", statusIcon("failing"))
	assert.Equal(t, "● ", statusIcon("pending"))
	assert.Empty(t, statusIcon("approved"))
	assert.Empty(t, statusIcon(""))
}

func TestSourceTableRowCIIcons(t *testing.T) {
	columns := []sources.Column{
		{Key: "title", Flex: 1},
		{Key: "ci", Width: 10},
	}
	item := sources.Item{ID: "1", Title: "Add feature", Fields: map[string]any{
		"title": "Add feature", "ci": "failing",
	}}

	row := renderSourceTableRow(item, columns, 50, true)
	assert.Contains(t, row, styles.TextErrorStyle.Render("✗ failing "))

	plain := renderSourceTableRow(item, columns, 50, false)
	assert.Contains(t, plain, "✗ failing")
	assert.Equal(t, plain, terminal.StripANSI(plain), "plain table row must be ANSI-free")
}

func TestRenderSingleLineContent_PlainIsANSIFree(t *testing.T) {
	p := newTestPicker(newFakeTUISource(listManifest(), nil), listManifest(), "test-repo", 90, 24)
	item := sources.Item{ID: "1", Title: "First reference item", Fields: map[string]any{
		"number": 1278, "author": "alice", "labels": []string{"api", "public"}, "ci_status": "passing",
	}}

	plain := p.renderSingleLineContent(item, false, 60, 5)
	styled := p.renderSingleLineContent(item, true, 60, 5)

	assert.Equal(t, plain, terminal.StripANSI(plain), "plain list row must be ANSI-free")
	assert.Equal(t, plain, terminal.StripANSI(styled))
}

func TestNumberColumnWidth_AlignsShortAndLongNumbers(t *testing.T) {
	items := []sources.Item{
		{ID: "9", Title: "Short", Fields: map[string]any{"number": 9}},
		{ID: "1315", Title: "Long", Fields: map[string]any{"number": 1315}},
	}
	w := numberColumnWidth(items)
	assert.Equal(t, 5, w) // "#1315"

	p := newTestPicker(newFakeTUISource(listManifest(), nil), listManifest(), "test-repo", 90, 24)
	short := p.renderSingleLineContent(items[0], false, 60, w)
	long := p.renderSingleLineContent(items[1], false, 60, w)
	assert.Equal(t, strings.Index(short, "Short"), strings.Index(long, "Long"),
		"titles must start at the same column regardless of number width")
}

func TestPicker_ScopeShownInTabBar(t *testing.T) {
	p := newTestPicker(
		newFakeTUISource(listManifest(), nil),
		listManifest(), "myorg/myrepo", 80, 24,
	)

	bar := terminal.StripANSI(p.renderTabBar())
	assert.Contains(t, bar, "myorg/myrepo")
}
