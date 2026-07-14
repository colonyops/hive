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
	return New(tabs, manifest.ID, scope, "", w, h)
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

func fakeManifest() sources.Manifest {
	return sources.Manifest{
		ID:          "fake",
		DisplayName: "Fake Source",
		Capabilities: sources.Capabilities{
			FetchDetail: true,
		},
	}
}

// remoteManifest sets a tiny debounce so tests exercise the real search
// tick path without slow sleeps.
func remoteManifest() sources.Manifest {
	m := fakeManifest()
	m.Picker.Search.DebounceMS = 1
	return m
}

// enterSearch puts the picker into search mode.
func enterSearch(t *testing.T, p Picker) Picker {
	t.Helper()
	next, cmd := p.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	p = drainPicker(t, next, cmd)
	require.True(t, p.searchMode, "pressing / must enter search mode")
	return p
}

func TestPicker_SearchModeRespectsDisabledTab(t *testing.T) {
	tests := []struct {
		name           string
		searchDisabled bool
		wantSearchMode bool
	}{
		{name: "enabled tab enters search", wantSearchMode: true},
		{name: "disabled tab ignores slash", searchDisabled: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := remoteManifest()
			fake := newFakeTUISource(manifest, nil)
			p := newTestPicker(fake, manifest, "o/r", 80, 24)
			p.tabs[0].tab.SearchDisabled = tt.searchDisabled
			p = drainPicker(t, p, p.Init())

			next, cmd := p.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
			assert.Equal(t, tt.wantSearchMode, next.searchMode)
			if tt.searchDisabled {
				assert.Nil(t, cmd)
				assert.Nil(t, next.debounceSearch(next.activeState()), "disabled tabs never schedule refinement")
			}
		})
	}
}

func TestPicker_DisabledTabDropsDebounceMessage(t *testing.T) {
	manifest := remoteManifest()
	fake := newFakeTUISource(manifest, nil)
	p := newTestPicker(fake, manifest, "o/r", 80, 24)
	p.tabs[0].tab.SearchDisabled = true
	p = drainPicker(t, p, p.Init())
	require.Equal(t, []string{""}, fake.searchCalls)

	msg := sourceSearchDebounceMsg{Gen: p.gen, SourceID: manifest.ID, Query: ""}
	next, cmd := p.Update(msg)
	assert.Nil(t, cmd)
	assert.False(t, activeTab(next).loading)
	assert.Equal(t, []string{""}, fake.searchCalls, "disabled tabs never dispatch remote refinement")
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

func TestPicker_RemoteSearchDispatchesDebouncedQuery(t *testing.T) {
	items := []sources.Item{
		{ID: "1", Title: "alpha"},
		{ID: "2", Title: "beta"},
	}
	fake := newFakeTUISource(remoteManifest(), items)
	p := newTestPicker(fake, remoteManifest(), "o/r", 80, 24)
	p = drainPicker(t, p, p.Init())
	require.Equal(t, []string{""}, fake.searchCalls, "init must issue the unfiltered search")

	p = enterSearch(t, p)
	p = typeKey(t, p, "beta")

	// Only the final query survives debouncing: intermediate ticks carry
	// stale queries and are dropped by handleDebounce.
	assert.Equal(t, []string{"", "beta"}, fake.searchCalls)

	tab := activeTab(p)
	require.Len(t, tab.filteredItems, 1)
	assert.Equal(t, "beta", tab.filteredItems[0].Title)
}

// TestPicker_StaleSearchResultsAreDropped covers both staleness guards:
// results from a previous picker instance (generation) and results for a
// query the user has since changed.
func TestPicker_StaleSearchResultsAreDropped(t *testing.T) {
	tests := []struct {
		name string
		msg  func(p Picker) sourceSearchResultMsg
	}{
		{
			name: "stale generation",
			msg: func(p Picker) sourceSearchResultMsg {
				return sourceSearchResultMsg{Gen: p.gen - 1, SourceID: "fake", Query: p.input.Value()}
			},
		},
		{
			name: "stale query",
			msg: func(p Picker) sourceSearchResultMsg {
				return sourceSearchResultMsg{Gen: p.gen, SourceID: "fake", Query: "outdated"}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := []sources.Item{{ID: "1", Title: "alpha"}}
			fake := newFakeTUISource(remoteManifest(), items)
			p := newTestPicker(fake, remoteManifest(), "o/r", 80, 24)
			p = drainPicker(t, p, p.Init())

			stale := tt.msg(p)
			stale.Items = []sources.Item{{ID: "9", Title: "poisoned"}}
			p = applyPickerMsg(t, p, stale)

			tab := activeTab(p)
			require.Len(t, tab.filteredItems, 1)
			assert.Equal(t, "alpha", tab.filteredItems[0].Title)
		})
	}
}

func TestPicker_SelectionAndCancel(t *testing.T) {
	items := []sources.Item{{ID: "1", Title: "alpha"}}
	fake := newFakeTUISource(fakeManifest(), items)
	p := newTestPicker(fake, fakeManifest(), "", 80, 24)
	p = drainPicker(t, p, p.Init())

	next, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	p = drainPicker(t, next, cmd)

	results, ok := p.Selected()
	require.True(t, ok)
	require.Len(t, results, 1, "enter without marks selects the cursor item")
	assert.Equal(t, "1", results[0].Item.ID)
	assert.False(t, p.Cancelled())

	// Cancel test
	p2 := newTestPicker(fake, fakeManifest(), "", 80, 24)
	p2 = drainPicker(t, p2, p2.Init())
	next2, cmd2 := p2.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	p2 = drainPicker(t, next2, cmd2)
	assert.True(t, p2.Cancelled())
	_, ok = p2.Selected()
	assert.False(t, ok)
}

// pressSpace toggles the mark on the cursor item.
func pressSpace(t *testing.T, p Picker) Picker {
	t.Helper()
	next, cmd := p.Update(tea.KeyPressMsg{Text: " ", Code: tea.KeySpace})
	return drainPicker(t, next, cmd)
}

func TestPicker_SpaceTogglesMark(t *testing.T) {
	items := []sources.Item{
		{ID: "1", Title: "alpha"},
		{ID: "2", Title: "beta"},
	}
	fake := newFakeTUISource(fakeManifest(), items)
	p := newTestPicker(fake, fakeManifest(), "", 80, 24)
	p = drainPicker(t, p, p.Init())

	p = pressSpace(t, p)
	tab := activeTab(p)
	require.Len(t, tab.marked, 1)
	assert.Equal(t, "1", tab.marked[0].ID)
	assert.Equal(t, 0, tab.cursor, "marking must not move the cursor")

	// Mark the second item, then move back and unmark the first.
	p = p.moveCursor(1)
	p = pressSpace(t, p)
	require.Len(t, activeTab(p).marked, 2)

	p = p.moveCursor(-1)
	p = pressSpace(t, p)
	tab = activeTab(p)
	require.Len(t, tab.marked, 1)
	assert.Equal(t, "2", tab.marked[0].ID, "unmarking removes only the toggled item")
}

func TestPicker_EnterSpawnsMarkedItems(t *testing.T) {
	items := []sources.Item{
		{ID: "1", Title: "alpha"},
		{ID: "2", Title: "beta"},
		{ID: "3", Title: "gamma"},
	}
	fake := newFakeTUISource(fakeManifest(), items)
	p := newTestPicker(fake, fakeManifest(), "", 80, 24)
	p = drainPicker(t, p, p.Init())

	p = pressSpace(t, p)
	p = p.moveCursor(1)
	p = pressSpace(t, p)

	next, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	p = drainPicker(t, next, cmd)

	results, ok := p.Selected()
	require.True(t, ok)
	require.Len(t, results, 2, "enter returns every marked item, not the cursor item")
	assert.Equal(t, "1", results[0].Item.ID)
	assert.Equal(t, "2", results[1].Item.ID)
}

func TestPicker_MarksSurviveRemoteSearch(t *testing.T) {
	items := []sources.Item{
		{ID: "1", Title: "alpha"},
		{ID: "2", Title: "beta"},
	}
	fake := newFakeTUISource(remoteManifest(), items)
	p := newTestPicker(fake, remoteManifest(), "o/r", 80, 24)
	p = drainPicker(t, p, p.Init())

	p = pressSpace(t, p) // mark "alpha"

	// Search narrows the list to "beta"; the mark on "alpha" must survive.
	p = enterSearch(t, p)
	p = typeKey(t, p, "beta")
	require.Len(t, activeTab(p).filteredItems, 1)

	next, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	p = drainPicker(t, next, cmd)

	results, ok := p.Selected()
	require.True(t, ok)
	require.Len(t, results, 1)
	assert.Equal(t, "1", results[0].Item.ID, "marked item spawns even when filtered out of view")
}

func TestPicker_MarksAcrossTabs(t *testing.T) {
	m1 := fakeManifest()
	m1.ID = "prs"
	m2 := fakeManifest()
	m2.ID = "issues"

	tabs := []TabSource{
		{ID: "prs", Source: newFakeTUISource(m1, []sources.Item{{ID: "p1", Title: "PR one"}}), Manifest: m1},
		{ID: "issues", Source: newFakeTUISource(m2, []sources.Item{{ID: "i1", Title: "Issue one"}}), Manifest: m2},
	}
	p := New(tabs, "prs", "", "", 80, 24)
	p = drainPicker(t, p, p.Init())

	p = pressSpace(t, p)

	next, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	p = drainPicker(t, next, cmd)
	p = pressSpace(t, p)

	next, cmd = p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	p = drainPicker(t, next, cmd)

	results, ok := p.Selected()
	require.True(t, ok)
	require.Len(t, results, 2)
	assert.Equal(t, "prs", results[0].SourceID)
	assert.Equal(t, "p1", results[0].Item.ID)
	assert.Equal(t, "issues", results[1].SourceID)
	assert.Equal(t, "i1", results[1].Item.ID)
}

func TestPicker_MarkCapRefusesFurtherMarks(t *testing.T) {
	items := make([]sources.Item, maxMarkedItems+2)
	for i := range items {
		items[i] = sources.Item{ID: fmt.Sprintf("%d", i+1), Title: fmt.Sprintf("item %d", i+1)}
	}
	fake := newFakeTUISource(fakeManifest(), items)
	p := newTestPicker(fake, fakeManifest(), "", 80, 40)
	p = drainPicker(t, p, p.Init())

	for range items {
		p = pressSpace(t, p)
		p = p.moveCursor(1)
	}
	assert.Equal(t, maxMarkedItems, p.totalMarked(), "marks stop at the cap")

	// Unmarking below the cap frees a slot.
	activeTab(p).cursor = 0
	p = pressSpace(t, p) // unmark first item
	assert.Equal(t, maxMarkedItems-1, p.totalMarked())
}

func TestPicker_MarkedRowRendersGlyphAndCount(t *testing.T) {
	items := []sources.Item{
		{ID: "1", Title: "alpha"},
		{ID: "2", Title: "beta"},
	}
	fake := newFakeTUISource(fakeManifest(), items)
	p := newTestPicker(fake, fakeManifest(), "", 80, 24)
	p = drainPicker(t, p, p.Init())

	p = pressSpace(t, p)

	list := terminal.StripANSI(p.renderList(activeTab(p)))
	require.Contains(t, list, markGlyph, "marked row must show the mark glyph")
	firstLine := strings.SplitN(list, "\n", 2)[0]
	assert.Less(t, strings.Index(firstLine, markGlyph), strings.Index(firstLine, "alpha"),
		"mark glyph sits in the gutter before the title")

	filterLine := terminal.StripANSI(p.renderFilterLine(activeTab(p)))
	assert.Contains(t, filterLine, markGlyph+" 1", "filter line shows the marked count")

	help := terminal.StripANSI(p.helpText())
	assert.Contains(t, help, "spawn 1", "enter hint reflects the marked count")
}

func TestPicker_EmptyResultsShowsEmptyState(t *testing.T) {
	fake := newFakeTUISource(fakeManifest(), nil)
	p := newTestPicker(fake, fakeManifest(), "test-repo", 80, 24)
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
	fake := newFakeTUISource(fakeManifest(), nil)
	fake.searchErr = fmt.Errorf("gh: unauthenticated")

	p := newTestPicker(fake, fakeManifest(), "", 80, 24)
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
			fake := newFakeTUISource(fakeManifest(), items)

			p := newTestPicker(fake, fakeManifest(), "", size.width, size.height)
			p = drainPicker(t, p, p.Init())

			viewHeight := lipgloss.Height(p.View())
			assert.LessOrEqual(t, viewHeight, size.height, "picker must fit the terminal")
		})
	}
}

func TestPicker_NavigateModeIsDefault(t *testing.T) {
	items := []sources.Item{
		{ID: "1", Title: "alpha"},
		{ID: "2", Title: "beta"},
		{ID: "3", Title: "gamma"},
	}
	fake := newFakeTUISource(fakeManifest(), items)
	p := newTestPicker(fake, fakeManifest(), "", 80, 24)
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

	fake1 := newFakeTUISource(fakeManifest(), items1)
	m1 := fakeManifest()
	m1.ID = "prs"
	m1.DisplayName = "Pull Requests"

	fake2 := newFakeTUISource(fakeManifest(), items2)
	m2 := fakeManifest()
	m2.ID = "issues"
	m2.DisplayName = "Issues"

	tabs := []TabSource{
		{ID: "prs", Source: fake1, Manifest: m1},
		{ID: "issues", Source: fake2, Manifest: m2},
	}
	p := New(tabs, "prs", "", "", 80, 24)
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
	m1 := fakeManifest()
	m1.ID = "prs"
	m1.DisplayName = "Pull Requests"
	m2 := fakeManifest()
	m2.ID = "issues"
	m2.DisplayName = "Issues"

	tabs := []TabSource{
		{ID: "prs", Source: newFakeTUISource(m1, nil), Manifest: m1},
		{ID: "issues", Source: newFakeTUISource(m2, nil), Manifest: m2},
	}
	p := New(tabs, "prs", "owner/repo", "", 80, 24)

	bar := terminal.StripANSI(p.renderTabBar())
	assert.Contains(t, bar, "Pull Requests")
	assert.Contains(t, bar, "Issues")
	assert.Contains(t, bar, "owner/repo")
}

func TestPicker_RetryOnError(t *testing.T) {
	fake := newFakeTUISource(fakeManifest(), nil)
	fake.searchErr = fmt.Errorf("rate limited")

	p := newTestPicker(fake, fakeManifest(), "", 80, 24)
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

func TestNumberColumnWidth_AlignsShortAndLongNumbers(t *testing.T) {
	items := []sources.Item{
		{ID: "9", Title: "Short", Fields: map[string]any{"number": 9}},
		{ID: "1315", Title: "Long", Fields: map[string]any{"number": 1315}},
	}
	w := numberColumnWidth(items)
	assert.Equal(t, 5, w) // "#1315"

	// The card title line pads "#<number>" to the shared width, so titles
	// start at the same column regardless of number width.
	p := newTestPicker(newFakeTUISource(fakeManifest(), nil), fakeManifest(), "test-repo", 90, 24)
	short := strings.SplitN(p.renderCardContent(items[0], false, false, 60, w), "\n", 2)[0]
	long := strings.SplitN(p.renderCardContent(items[1], false, false, 60, w), "\n", 2)[0]
	assert.Equal(t, strings.Index(short, "Short"), strings.Index(long, "Long"),
		"titles must start at the same column regardless of number width")
}

func TestPicker_ActiveFilterVisibleOutsideSearchMode(t *testing.T) {
	items := []sources.Item{
		{ID: "1", Title: "alpha"},
		{ID: "2", Title: "beta"},
	}
	fake := newFakeTUISource(fakeManifest(), items)
	p := newTestPicker(fake, fakeManifest(), "", 80, 24)
	p = drainPicker(t, p, p.Init())

	p = enterSearch(t, p)
	p = typeKey(t, p, "beta")

	// esc leaves search mode but keeps the filter applied; the filter
	// line must still show the active query so the reduced list is
	// explained.
	next, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	p = drainPicker(t, next, cmd)
	require.False(t, p.searchMode)
	require.Len(t, activeTab(p).filteredItems, 1)

	line := terminal.StripANSI(p.renderFilterLine(activeTab(p)))
	assert.Contains(t, line, "beta", "applied filter query must stay visible")
	assert.NotContains(t, line, "filter ", "placeholder must not mask an active filter")
}

func TestPicker_ScopeShownInTabBar(t *testing.T) {
	p := newTestPicker(
		newFakeTUISource(fakeManifest(), nil),
		fakeManifest(), "myorg/myrepo", 80, 24,
	)

	bar := terminal.StripANSI(p.renderTabBar())
	assert.Contains(t, bar, "myorg/myrepo")
}

// cardManifest is the two-line card layout used by the issues/PRs sources:
// title on line one, a status strip (ci · review · author · labels) beneath.
func cardManifest() sources.Manifest {
	return sources.Manifest{
		ID:          "fake-card",
		DisplayName: "Fake Card Source",
	}
}

func cardItems(n int) []sources.Item {
	items := make([]sources.Item, n)
	for i := range items {
		items[i] = sources.Item{
			ID:    fmt.Sprintf("%d", i+1),
			Title: fmt.Sprintf("card item number %d with enough words to fill a line", i+1),
			Fields: map[string]any{
				"number": i + 1, "author": "octocat", "review": "approved", "ci": "passing",
				"age": "3d", "linked_issue": 1, "assignee": "octocat", "assignee_count": 1,
			},
		}
	}
	return items
}

func TestPicker_CardRowIsTwoLines(t *testing.T) {
	manifest := cardManifest()
	item := sources.Item{ID: "1", Title: strings.Repeat("long title ", 20), Fields: map[string]any{
		"number": 1, "author": "octocat", "review": "changes requested", "ci": "failing",
		"labels": []string{"a", "b", "c"},
	}}

	p := newTestPicker(newFakeTUISource(manifest, []sources.Item{item}), manifest, "", 120, 40)
	p = drainPicker(t, p, p.Init())

	numWidth := numberColumnWidth([]sources.Item{item})
	for _, selected := range []bool{true, false} {
		for _, marked := range []bool{true, false} {
			row := p.renderRow(item, selected, marked, numWidth)
			assert.Equal(t, rowsPerItemCard, lipgloss.Height(row),
				"card row must render as exactly two lines (selected=%v marked=%v)", selected, marked)
		}
	}
}

func TestRenderCardContent_PlainIsANSIFree(t *testing.T) {
	manifest := cardManifest()
	p := newTestPicker(newFakeTUISource(manifest, nil), manifest, "test-repo", 90, 24)
	item := sources.Item{ID: "1", Title: "First reference item", Fields: map[string]any{
		"number": 1278, "author": "alice", "review": "approved", "ci": "passing",
		"labels": []string{"api", "public"}, "age": "3d", "linked_issue": 2,
		"assignee": "bob", "assignee_count": 2,
	}}

	plain := p.renderCardContent(item, false, false, 60, 5)
	styled := p.renderCardContent(item, true, false, 60, 5)

	assert.Equal(t, plain, terminal.StripANSI(plain), "plain card row must be ANSI-free")
	assert.Equal(t, plain, terminal.StripANSI(styled), "styled and plain must match once ANSI is stripped")
}

func TestPicker_CardLayoutFitsTerminal(t *testing.T) {
	sizes := []struct{ width, height int }{
		{80, 24}, {90, 24}, {100, 30}, {120, 38}, {120, 50}, {200, 60},
	}
	manifest := cardManifest()
	items := cardItems(40)
	for _, size := range sizes {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			p := newTestPicker(newFakeTUISource(manifest, items), manifest, "", size.width, size.height)
			p = drainPicker(t, p, p.Init())

			assert.LessOrEqual(t, lipgloss.Height(p.View()), size.height, "card picker must fit the terminal height")
			assert.LessOrEqual(t, lipgloss.Width(p.View()), size.width, "card picker must fit the terminal width")
		})
	}
}

func TestPicker_WideTerminalCapsModalWidth(t *testing.T) {
	manifest := cardManifest()
	p := newTestPicker(newFakeTUISource(manifest, cardItems(3)), manifest, "", 240, 60)
	p = drainPicker(t, p, p.Init())

	assert.Equal(t, sourcePickerMaxModalWidth, p.modalWidth,
		"on a very wide terminal the modal caps at the max width instead of ~92%")
}

func TestPicker_CardScrollFollowsCursorByItem(t *testing.T) {
	manifest := cardManifest()
	items := cardItems(12)
	// Small height keeps the item capacity below the item count so
	// navigating to the end must scroll the two-line rows.
	p := newTestPicker(newFakeTUISource(manifest, items), manifest, "", 100, 18)
	p = drainPicker(t, p, p.Init())

	tab := activeTab(p)
	require.Len(t, tab.filteredItems, len(items))

	capacity := p.itemCapacity()
	require.Positive(t, capacity)
	require.Less(t, capacity, len(items), "test needs more items than fit to force scrolling")

	for range len(items) - 1 {
		p = p.moveCursor(1)
	}

	tab = activeTab(p)
	assert.Equal(t, len(items)-1, tab.cursor, "cursor reaches the last item")
	assert.GreaterOrEqual(t, tab.cursor, tab.scrollOffset, "cursor stays at or below the top of the window")
	assert.Less(t, tab.cursor, tab.scrollOffset+capacity, "cursor stays within the visible window")
	assert.LessOrEqual(t, tab.scrollOffset, len(items)-capacity, "window never scrolls past the last page")
	assert.LessOrEqual(t, lipgloss.Height(p.renderList(tab)), p.listHeight(),
		"two-line rows must not overflow the fixed body height")
}
