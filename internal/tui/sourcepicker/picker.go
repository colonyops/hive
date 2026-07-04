package sourcepicker

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/rs/zerolog/log"
	"github.com/sahilm/fuzzy"

	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/sources"
	"github.com/colonyops/hive/internal/tui/components"
)

// Fixed dialog sizing: the modal's overall width/height are a deterministic
// function of the terminal size, never of the current item/detail content.
const (
	sourcePickerMaxModalHeight = 28
	// sourcePickerMaxModalWidth caps the dialog on wide terminals. The
	// two-line card layout reflows instead of pinning table columns, so
	// the modal no longer needs to stretch to ~92% of the terminal.
	sourcePickerMaxModalWidth = 120
	sourcePickerModalMargin   = 2
	sourcePickerMinModalWidth = 72
	// sourcePickerChrome counts the fixed rows View renders around the
	// scrollable list area: border (2), tab bar (1), separator (1),
	// filter line (1), separator (1), and the help line including
	// ModalHelpStyle's MarginTop (2).
	sourcePickerChrome = 8
	// rowsPerItemCard is the terminal-row height of one card item: the
	// title line plus the status strip beneath it.
	rowsPerItemCard = 2
)

// defaultSearchDebounce is used when a remote-search manifest does not
// set its own DebounceMS: long enough to coalesce normal typing, short
// enough to feel immediate once the user pauses.
const defaultSearchDebounce = 300 * time.Millisecond

// sourcePickerGen issues a unique generation token per picker instance.
var sourcePickerGen atomic.Int64

// sourceItemsSource implements fuzzy.Source over item titles.
type sourceItemsSource []sources.Item

func (c sourceItemsSource) String(i int) string { return c[i].Title }
func (c sourceItemsSource) Len() int            { return len(c) }

// TabSource pairs a source with its configuration for one picker tab.
type TabSource struct {
	ID        string
	Source    sources.Source
	Manifest  sources.Manifest
	Templates sources.TemplateConfig
}

// tabState tracks the per-tab lifecycle: uninit → loading → loaded/error.
type tabState struct {
	tab TabSource

	initialized   bool
	loading       bool
	loadingMsg    string
	items         []sources.Item
	filteredItems []sources.Item
	searchErr     error
	searchedOnce  bool

	// cursor/scroll per tab so switching back preserves position.
	cursor       int
	scrollOffset int
	filterQuery  string
}

// Picker is a tabbed, searchable modal for browsing external sources.
// Each tab corresponds to a registered source (PRs, issues, etc.) with
// lazy initialization and per-tab result caching.
type Picker struct {
	gen int64

	tabs      []tabState
	activeTab int
	scope     string

	input      textinput.Model
	searchMode bool
	spinner    spinner.Model

	cancelled bool
	selected  bool

	// Sizing.
	width, height               int
	modalWidth, modalHeight     int
	contentWidth, contentHeight int
	innerWidth                  int // contentWidth minus horizontal padding (for padded sections)

	// Row styles, precomputed per size change so View doesn't rebuild
	// them for every visible row on every frame. The card variants are
	// identical but allow two lines for the title + status strip.
	selectedRowStyle  lipgloss.Style
	normalRowStyle    lipgloss.Style
	selectedCardStyle lipgloss.Style
	normalCardStyle   lipgloss.Style
}

// Result is the item selected by the user, if any. Source and Manifest
// let the parent fetch the item's detail (capability-gated) before
// rendering session templates.
type Result struct {
	Item      sources.Item
	SourceID  string
	Source    sources.Source
	Manifest  sources.Manifest
	Templates sources.TemplateConfig
}

// New constructs a tabbed picker. initialTab selects the initially active
// tab by source ID; if not found the first tab is used.
func New(tabSources []TabSource, initialTab, scope string, width, height int) Picker {
	input := textinput.New()
	input.Placeholder = "search..."
	input.Prompt = "/ "
	inputStyles := textinput.DefaultStyles(true)
	inputStyles.Focused.Prompt = styles.TextPrimaryStyle
	inputStyles.Cursor.Color = styles.ColorPrimary
	input.SetStyles(inputStyles)

	s := spinner.New()
	s.Spinner = spinner.Meter
	s.Style = lipgloss.NewStyle().Foreground(styles.ColorPrimary)

	modalWidth, modalHeight, contentWidth, innerW, contentHeight := computePickerDims(width, height)
	input.SetWidth(max(innerW-4, 10))

	tabs := make([]tabState, len(tabSources))
	activeIdx := 0
	for i, ts := range tabSources {
		tabs[i] = tabState{tab: ts}
		if ts.ID == initialTab {
			activeIdx = i
		}
	}

	p := Picker{
		gen:           sourcePickerGen.Add(1),
		tabs:          tabs,
		activeTab:     activeIdx,
		scope:         scope,
		input:         input,
		spinner:       s,
		width:         width,
		height:        height,
		modalWidth:    modalWidth,
		modalHeight:   modalHeight,
		contentWidth:  contentWidth,
		innerWidth:    innerW,
		contentHeight: contentHeight,
	}
	p.rebuildRowStyles()
	return p
}

// rebuildRowStyles recomputes the width-bound row styles. Selected rows
// get a left border accent and a full-width subtle highlight background;
// unselected rows get a two-space indent to keep alignment with the
// border+space of selected rows.
func (p *Picker) rebuildRowStyles() {
	width := p.innerWidth
	selected := lipgloss.NewStyle().
		Border(lipgloss.ThickBorder(), false, false, false, true).
		BorderForeground(styles.ColorPrimary).
		Background(styles.ColorSurfaceLow).
		Foreground(styles.ColorPrimary).
		Bold(true).
		PaddingLeft(1).
		Width(width).
		MaxWidth(width)
	normal := lipgloss.NewStyle().
		PaddingLeft(2).
		Width(width).
		MaxWidth(width)

	p.selectedRowStyle = selected.MaxHeight(1)
	p.normalRowStyle = normal.MaxHeight(1)
	p.selectedCardStyle = selected.MaxHeight(rowsPerItemCard)
	p.normalCardStyle = normal.MaxHeight(rowsPerItemCard)
}

func computePickerDims(width, height int) (modalWidth, modalHeight, contentWidth, innerWidth, contentHeight int) {
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}

	modalWidth = min(max(int(float64(width)*0.92), sourcePickerMinModalWidth), max(width-sourcePickerModalMargin, sourcePickerMinModalWidth))
	modalWidth = min(modalWidth, sourcePickerMaxModalWidth)
	modalHeight = min(max(height-sourcePickerModalMargin, sourcePickerChrome+3), sourcePickerMaxModalHeight)
	contentHeight = max(modalHeight-sourcePickerChrome, 3)

	// contentWidth excludes the border (2); innerWidth also excludes
	// the per-section horizontal padding (2).
	contentWidth = max(modalWidth-2, 20)
	innerWidth = max(contentWidth-2, 16)

	return modalWidth, modalHeight, contentWidth, innerWidth, contentHeight
}

// Init kicks off the initial tab: checks availability, initializes, and
// searches.
func (p *Picker) Init() tea.Cmd {
	return tea.Batch(p.spinner.Tick, p.initTab(p.activeTab))
}

// initTab starts the async Available+Initialize+Search pipeline for a tab.
func (p *Picker) initTab(idx int) tea.Cmd {
	tab := &p.tabs[idx]
	if tab.initialized {
		return nil
	}
	tab.loading = true
	tab.loadingMsg = fmt.Sprintf("Fetching %s...", tab.tab.Manifest.DisplayName)

	gen := p.gen
	conn := tab.tab.Source
	scope := p.scope
	sourceID := tab.tab.ID

	return func() tea.Msg {
		ctx := context.Background()
		if !conn.Available(ctx) {
			return sourceTabErrorMsg{Gen: gen, SourceID: sourceID, Err: fmt.Errorf("source %q is not available", sourceID)}
		}
		manifest, err := conn.Initialize(ctx)
		if err != nil {
			return sourceTabErrorMsg{Gen: gen, SourceID: sourceID, Err: fmt.Errorf("initialize: %w", err)}
		}
		result, err := conn.Search(ctx, sources.SearchParams{Scope: scope})
		if err != nil {
			return sourceTabErrorMsg{Gen: gen, SourceID: sourceID, Err: err}
		}
		return sourceTabReadyMsg{
			Gen:      gen,
			SourceID: sourceID,
			Manifest: manifest,
			Items:    result.Items,
		}
	}
}

// SetSize updates rendering dimensions.
func (p *Picker) SetSize(width, height int) {
	p.width = width
	p.height = height
	p.modalWidth, p.modalHeight, p.contentWidth, p.innerWidth, p.contentHeight = computePickerDims(width, height)
	p.input.SetWidth(max(p.innerWidth-4, 10))
	p.rebuildRowStyles()
}

// Update handles messages.
func (p Picker) Update(msg tea.Msg) (Picker, tea.Cmd) {
	switch m := msg.(type) {
	case tea.KeyPressMsg:
		return p.handleKey(m)
	case spinner.TickMsg:
		var cmd tea.Cmd
		p.spinner, cmd = p.spinner.Update(msg)
		return p, cmd
	case sourceTabReadyMsg:
		return p.handleTabReady(m)
	case sourceTabErrorMsg:
		return p.handleTabError(m)
	case sourceSearchResultMsg:
		return p.handleSearchResult(m)
	case sourceSearchErrorMsg:
		return p.handleSearchError(m)
	case sourceSearchDebounceMsg:
		return p.handleDebounce(m)
	}
	return p, nil
}

func (p Picker) handleTabReady(msg sourceTabReadyMsg) (Picker, tea.Cmd) {
	if msg.Gen != p.gen {
		return p, nil
	}
	idx := p.tabIndex(msg.SourceID)
	if idx < 0 {
		return p, nil
	}
	tab := &p.tabs[idx]
	tab.initialized = true
	tab.loading = false
	tab.searchedOnce = true
	tab.searchErr = nil
	tab.tab.Manifest = msg.Manifest
	tab.items = msg.Items
	tab.filteredItems = msg.Items
	tab.cursor = 0
	tab.scrollOffset = 0
	return p, nil
}

func (p Picker) handleTabError(msg sourceTabErrorMsg) (Picker, tea.Cmd) {
	if msg.Gen != p.gen {
		return p, nil
	}
	idx := p.tabIndex(msg.SourceID)
	if idx < 0 {
		return p, nil
	}
	tab := &p.tabs[idx]
	tab.loading = false
	tab.searchedOnce = true
	tab.searchErr = msg.Err
	tab.items = nil
	tab.filteredItems = nil
	return p, nil
}

func (p Picker) tabIndex(sourceID string) int {
	for i := range p.tabs {
		if p.tabs[i].tab.ID == sourceID {
			return i
		}
	}
	return -1
}

func (p Picker) activeState() *tabState {
	return &p.tabs[p.activeTab]
}

func (p Picker) handleKey(msg tea.KeyPressMsg) (Picker, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if p.searchMode {
			p.searchMode = false
			p.input.Blur()
			return p, nil
		}
		p.cancelled = true
		return p, nil
	case "enter":
		tab := p.activeState()
		if len(tab.filteredItems) > 0 && tab.cursor < len(tab.filteredItems) {
			p.selected = true
		}
		return p, nil
	case "tab":
		return p.switchTab(1)
	case "shift+tab":
		return p.switchTab(-1)
	case "up", "ctrl+k":
		return p.moveCursor(-1), nil
	case "down", "ctrl+j":
		return p.moveCursor(1), nil
	case "r":
		if !p.searchMode {
			return p.retryTab()
		}
	}

	if !p.searchMode {
		return p.handleNavigateKey(msg)
	}

	before := p.input.Value()
	var inputCmd tea.Cmd
	p.input, inputCmd = p.input.Update(msg)
	if p.input.Value() == before {
		return p, inputCmd
	}

	tab := p.activeState()
	if tab.tab.Manifest.Picker.Search.Mode == sources.SearchModeRemote {
		return p, tea.Batch(inputCmd, p.debounceSearch(tab))
	}
	p.applyLocalFilter()
	return p, inputCmd
}

// debounceSearch schedules a remote search for the active tab's current
// query after the manifest's debounce delay. The resulting message
// carries the query so stale timers (the user kept typing) are dropped
// in handleDebounce.
func (p Picker) debounceSearch(tab *tabState) tea.Cmd {
	delay := time.Duration(tab.tab.Manifest.Picker.Search.DebounceMS) * time.Millisecond
	if delay <= 0 {
		delay = defaultSearchDebounce
	}
	gen := p.gen
	sourceID := tab.tab.ID
	query := p.input.Value()
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return sourceSearchDebounceMsg{Gen: gen, SourceID: sourceID, Query: query}
	})
}

func (p Picker) handleNavigateKey(msg tea.KeyPressMsg) (Picker, tea.Cmd) {
	switch msg.String() {
	case "j":
		return p.moveCursor(1), nil
	case "k":
		return p.moveCursor(-1), nil
	case "/":
		p.searchMode = true
		return p, p.input.Focus()
	case "O":
		return p, p.openCurrentItemURL()
	}
	return p, nil
}

func (p Picker) switchTab(delta int) (Picker, tea.Cmd) {
	if len(p.tabs) <= 1 {
		return p, nil
	}

	// Save current filter state.
	p.activeState().filterQuery = p.input.Value()

	next := (p.activeTab + delta + len(p.tabs)) % len(p.tabs)
	p.activeTab = next

	// Restore the target tab's filter.
	tab := p.activeState()
	p.input.SetValue(tab.filterQuery)

	// Leave search mode on tab switch.
	p.searchMode = false
	p.input.Blur()

	if !tab.initialized && !tab.loading {
		return p, p.initTab(next)
	}
	return p, nil
}

func (p Picker) retryTab() (Picker, tea.Cmd) {
	tab := p.activeState()
	if tab.searchErr == nil {
		return p, nil
	}
	tab.searchErr = nil
	tab.searchedOnce = false
	tab.initialized = false
	return p, p.initTab(p.activeTab)
}

func (p Picker) moveCursor(delta int) Picker {
	tab := p.activeState()
	next := tab.cursor + delta
	if next >= 0 && next < len(tab.filteredItems) {
		tab.cursor = next
		p.clampScroll(tab)
	}
	return p
}

func (p Picker) applyLocalFilter() {
	tab := p.activeState()
	query := p.input.Value()
	if query == "" {
		tab.filteredItems = tab.items
	} else {
		matches := fuzzy.FindFrom(query, sourceItemsSource(tab.items))
		items := make([]sources.Item, len(matches))
		for i, match := range matches {
			items[i] = tab.items[match.Index]
		}
		tab.filteredItems = items
	}
	tab.cursor = 0
	tab.scrollOffset = 0
}

func (p Picker) openCurrentItemURL() tea.Cmd {
	tab := p.activeState()
	if tab.cursor < 0 || tab.cursor >= len(tab.filteredItems) {
		return nil
	}
	uri := tab.filteredItems[tab.cursor].URI
	if uri == "" {
		return nil
	}
	return func() tea.Msg {
		if err := browserOpenCmd(uri).Run(); err != nil {
			log.Debug().Err(err).Str("uri", uri).Msg("source picker: open url failed")
		}
		return nil
	}
}

func browserOpenCmd(uri string) *exec.Cmd {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", uri)
	default:
		return exec.Command("xdg-open", uri)
	}
}

// handleSearchResult applies a completed remote Search response,
// dropping results whose query no longer matches what the user has
// typed (a newer search is already in flight or scheduled).
func (p Picker) handleSearchResult(msg sourceSearchResultMsg) (Picker, tea.Cmd) {
	if msg.Gen != p.gen {
		return p, nil
	}
	idx := p.tabIndex(msg.SourceID)
	if idx < 0 || msg.Query != p.tabQuery(idx) {
		return p, nil
	}
	tab := &p.tabs[idx]
	tab.loading = false
	tab.searchedOnce = true
	tab.searchErr = nil
	tab.items = msg.Items
	tab.filteredItems = msg.Items
	tab.cursor = 0
	tab.scrollOffset = 0
	return p, nil
}

func (p Picker) handleSearchError(msg sourceSearchErrorMsg) (Picker, tea.Cmd) {
	if msg.Gen != p.gen {
		return p, nil
	}
	idx := p.tabIndex(msg.SourceID)
	if idx < 0 {
		return p, nil
	}
	tab := &p.tabs[idx]
	tab.loading = false
	tab.searchedOnce = true
	tab.searchErr = msg.Err
	tab.items = nil
	tab.filteredItems = nil
	return p, nil
}

func (p Picker) handleDebounce(msg sourceSearchDebounceMsg) (Picker, tea.Cmd) {
	if msg.Gen != p.gen {
		return p, nil
	}
	idx := p.tabIndex(msg.SourceID)
	if idx < 0 || msg.Query != p.tabQuery(idx) {
		return p, nil
	}
	tab := &p.tabs[idx]
	tab.loading = true
	tab.loadingMsg = fmt.Sprintf("Searching %s...", tab.tab.Manifest.DisplayName)
	return p, sourceSearchCmd(p.gen, tab.tab.Source, tab.tab.ID, p.scope, msg.Query)
}

// tabQuery returns the tab's current effective search query: the live
// input for the active tab, the saved filter for background tabs.
func (p Picker) tabQuery(idx int) string {
	if idx == p.activeTab {
		return p.input.Value()
	}
	return p.tabs[idx].filterQuery
}

func (p *Picker) clampScroll(tab *tabState) {
	capacity := p.itemCapacity(tab)
	if tab.cursor < tab.scrollOffset {
		tab.scrollOffset = tab.cursor
	} else if tab.cursor >= tab.scrollOffset+capacity {
		tab.scrollOffset = tab.cursor - capacity + 1
	}
	maxOffset := max(len(tab.filteredItems)-capacity, 0)
	tab.scrollOffset = min(max(tab.scrollOffset, 0), maxOffset)
}

func (p Picker) listHeight() int {
	// The body is exactly the list area; all other rows are chrome.
	return max(p.contentHeight, 1)
}

// rowsPerItem is the number of terminal rows one item occupies in the
// given tab's layout: two for the card layout (title + status strip),
// one for the single-line list and table layouts.
func (p Picker) rowsPerItem(tab *tabState) int {
	if tab.tab.Manifest.Picker.Layout == sources.LayoutModeCard {
		return rowsPerItemCard
	}
	return 1
}

// itemCapacity is how many whole items fit in the list area for this tab,
// accounting for multi-row card layouts.
func (p Picker) itemCapacity(tab *tabState) int {
	return max(p.listHeight()/p.rowsPerItem(tab), 1)
}

// Selected returns the highlighted item if the user pressed enter.
func (p Picker) Selected() (Result, bool) {
	if !p.selected {
		return Result{}, false
	}
	tab := p.activeState()
	if tab.cursor < 0 || tab.cursor >= len(tab.filteredItems) {
		return Result{}, false
	}
	return Result{
		Item:      tab.filteredItems[tab.cursor],
		SourceID:  tab.tab.ID,
		Source:    tab.tab.Source,
		Manifest:  tab.tab.Manifest,
		Templates: tab.tab.Templates,
	}, true
}

// Cancelled reports whether the user dismissed the picker.
func (p Picker) Cancelled() bool {
	return p.cancelled
}

// --- View ---

func (p Picker) View() string {
	sep := styles.TextSurfaceStyle.Render(strings.Repeat("─", p.contentWidth))
	body := p.renderBody()
	help := styles.ModalHelpStyle.Render(p.helpText())

	// Pad sections that need inset; dividers span the full inner width.
	pad := lipgloss.NewStyle().Padding(0, 1)
	content := lipgloss.JoinVertical(lipgloss.Left,
		pad.Render(p.renderTabBar()),
		sep,
		pad.Render(p.renderFilterLine(p.activeState())),
		sep,
		body,
		pad.Render(help),
	)

	// No padding on the modal itself so dividers reach the border edges.
	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.ColorPrimary)

	return modalStyle.Width(p.modalWidth).Render(content)
}

func (p Picker) renderTabBar() string {
	// Active and inactive tabs MUST have identical horizontal padding:
	// each tab occupies the same width regardless of which one is
	// active, so switching tabs never shifts the bar layout.
	activeStyle := lipgloss.NewStyle().
		Background(styles.ColorSurface).
		Foreground(styles.ColorPrimary).
		Bold(true).
		Padding(0, 1)
	inactiveStyle := lipgloss.NewStyle().
		Foreground(styles.ColorMuted).
		Padding(0, 1)

	var parts []string
	for _, tab := range p.tabs {
		name := tab.tab.Manifest.DisplayName
		if name == "" {
			name = tab.tab.ID
		}
		if tab.tab.ID == p.tabs[p.activeTab].tab.ID {
			parts = append(parts, activeStyle.Render(name))
		} else {
			parts = append(parts, inactiveStyle.Render(name))
		}
	}

	tabRow := strings.Join(parts, " ")

	// Repo context badge on the right using the git branch icon.
	badge := ""
	if p.scope != "" {
		badge = styles.TextPrimaryStyle.Render(styles.IconGitBranch + " " + p.scope)
	}

	if badge != "" {
		tabRowWidth := lipgloss.Width(tabRow)
		badgeWidth := lipgloss.Width(badge)
		gap := max(p.innerWidth-tabRowWidth-badgeWidth, 1)
		return tabRow + strings.Repeat(" ", gap) + badge
	}
	return tabRow
}

func (p Picker) renderBody() string {
	tab := p.activeState()

	// Loading state: centered spinner with primary color.
	if tab.loading {
		spinnerLine := p.spinner.View()
		return p.renderCenteredState(
			spinnerLine,
			"",
			styles.TextPrimaryStyle.Render(tab.loadingMsg),
		)
	}

	// Error state: centered error.
	if tab.searchErr != nil {
		return p.renderCenteredState(
			styles.TextErrorStyle.Render("[!]"),
			styles.TextErrorStyle.Render(tab.searchErr.Error()),
			components.KeyHints(
				components.HelpEntry{Key: "r", Desc: "retry"},
				components.HelpEntry{Key: "tab", Desc: "switch source"},
			),
		)
	}

	// Empty state: centered message.
	if tab.searchedOnce && len(tab.filteredItems) == 0 {
		name := tab.tab.Manifest.DisplayName
		if name == "" {
			name = tab.tab.ID
		}
		return p.renderCenteredState(
			styles.TextMutedStyle.Render("○"),
			fmt.Sprintf("%s %s.",
				styles.TextMutedStyle.Render("No open "+strings.ToLower(name)+" in"),
				styles.TextPrimaryStyle.Render(p.scope),
			),
			"",
		)
	}

	// List state: item rows only; the filter line and dividers are
	// rendered by View as fixed chrome.
	pad := lipgloss.NewStyle().Padding(0, 1)
	list := pad.Render(p.renderList(tab))

	return lipgloss.NewStyle().
		Width(p.contentWidth).
		Height(p.contentHeight).
		MaxHeight(p.contentHeight).
		Render(list)
}

func (p Picker) renderCenteredState(icon, message, hint string) string {
	lines := []string{icon, message}
	if hint != "" {
		lines = append(lines, hint)
	}
	block := lipgloss.JoinVertical(lipgloss.Center, lines...)

	return lipgloss.Place(
		p.contentWidth, p.contentHeight,
		lipgloss.Center, lipgloss.Center,
		block,
	)
}

func (p Picker) renderFilterLine(tab *tabState) string {
	if p.searchMode {
		return p.input.View()
	}

	name := tab.tab.Manifest.DisplayName
	if name == "" {
		name = tab.tab.ID
	}
	// Keep an applied filter visible outside search mode so a reduced
	// item list is never unexplained.
	var placeholder string
	if query := p.input.Value(); query != "" {
		placeholder = styles.TextPrimaryStyle.Render("/") + " " + styles.TextForegroundStyle.Render(query)
	} else {
		placeholder = styles.TextPrimaryStyle.Render("/") + " " + styles.TextMutedStyle.Render(fmt.Sprintf("filter %s…", strings.ToLower(name)))
	}

	countStr := styles.TextMutedStyle.Render(fmt.Sprintf("%d", len(tab.filteredItems)))

	gap := max(p.innerWidth-lipgloss.Width(placeholder)-lipgloss.Width(countStr), 1)
	return placeholder + strings.Repeat(" ", gap) + countStr
}

func (p Picker) renderList(tab *tabState) string {
	totalRows := p.listHeight()
	perItem := p.rowsPerItem(tab)
	capacity := p.itemCapacity(tab)

	numWidth := numberColumnWidth(tab.filteredItems)
	visible := min(len(tab.filteredItems)-tab.scrollOffset, capacity)

	lines := make([]string, 0, totalRows)
	for i := range visible {
		idx := tab.scrollOffset + i
		item := tab.filteredItems[idx]
		selected := idx == tab.cursor
		// A card row is two lines joined by "\n"; a list/table row is one.
		lines = append(lines, p.renderRow(item, selected, tab, numWidth))
	}

	// Pad with blank lines so the body box keeps a fixed height. Each
	// rendered item already contributes perItem display lines.
	for rendered := visible * perItem; rendered < totalRows; rendered++ {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// renderRow renders one list row.
//
// Selected rows are composed as PLAIN text (no inner ANSI sequences) and
// then painted once by applyRowStyle. This is deliberate: any styled cell
// inside the row ends with an SGR reset, and that reset terminates the
// highlight background for the rest of the line — producing a row where
// only the trailing padding is highlighted. Do not "restore" per-cell
// styling on selected rows unless every cell and every padding space
// carries the highlight background itself.
func (p Picker) renderRow(item sources.Item, selected bool, tab *tabState, numWidth int) string {
	innerWidth := max(p.innerWidth-2, 10) // account for the left border+space / padding
	manifest := tab.tab.Manifest.Picker

	if manifest.Layout == sources.LayoutModeCard {
		content := p.renderCardContent(item, !selected, innerWidth, numWidth)
		if selected {
			return p.selectedCardStyle.Render(content)
		}
		return p.normalCardStyle.Render(content)
	}

	var content string
	if manifest.Layout == sources.LayoutModeTable && len(manifest.Columns) > 0 {
		content = renderSourceTableRow(item, manifest.Columns, innerWidth, !selected)
	} else {
		content = p.renderSingleLineContent(item, !selected, innerWidth, numWidth)
	}
	if selected {
		return p.selectedRowStyle.Render(content)
	}
	return p.normalRowStyle.Render(content)
}

// numberColumnWidth returns the display width of the widest "#<number>"
// among items, so list-layout rows can pad numbers to a common column and
// keep titles vertically aligned.
func numberColumnWidth(items []sources.Item) int {
	widest := 0
	for i := range items {
		if number := sourceFieldString(items[i], "number"); number != "" {
			widest = max(widest, len(number)+1)
		}
	}
	return widest
}

// renderSingleLineContent composes a list-layout row. When styled is
// false the result contains no ANSI sequences (used for selected rows —
// see renderRow). numWidth is the shared "#<number>" column width; zero
// disables padding.
func (p Picker) renderSingleLineContent(item sources.Item, styled bool, innerWidth, numWidth int) string {
	var parts []string

	// CI status icon if present.
	if ciStatus := sourceFieldString(item, "ci_status"); ciStatus != "" {
		parts = append(parts, statusIcon(ciStatus))
	}

	// Number, padded to the shared column width so titles align.
	if number := sourceFieldString(item, "number"); number != "" {
		num := "#" + number
		if pad := numWidth - len(num); pad > 0 {
			num += strings.Repeat(" ", pad)
		}
		if styled {
			num = styles.TextMutedStyle.Render(num)
		}
		parts = append(parts, num)
	}

	// Title.
	title := item.Title
	if styled {
		title = styles.TextForegroundStyle.Render(title)
	}
	parts = append(parts, title)

	// Labels (first 2).
	labels := sourceFieldStringSlice(item, "labels")
	for i, label := range labels {
		if i >= 2 {
			break
		}
		tag := "[" + label + "]"
		if styled {
			tag = styles.TextSecondaryStyle.Render(tag)
		}
		parts = append(parts, tag)
	}

	// Right-aligned metadata: author.
	right := ""
	if author := sourceFieldString(item, "author"); author != "" {
		right = "@" + author
		if styled {
			right = styles.TextMutedStyle.Render(right)
		}
	}

	left := strings.Join(parts, " ")
	leftWidth := ansi.StringWidth(left)
	rightWidth := ansi.StringWidth(right)

	if right == "" {
		return ansi.Truncate(left, innerWidth, "…")
	}
	gap := max(innerWidth-leftWidth-rightWidth, 1)
	if leftWidth+1+rightWidth > innerWidth {
		available := max(innerWidth-rightWidth-1, 10)
		left = ansi.Truncate(left, available, "…")
		gap = max(innerWidth-ansi.StringWidth(left)-rightWidth, 1)
	}
	return left + strings.Repeat(" ", gap) + right
}

// renderCardContent composes a two-line card row. Line 1 is the number and
// title (left) with the author right-aligned; line 2 is the metadata strip
// (left, aligned under the title) with labels right-aligned. When styled is
// false the result carries no ANSI (used for selected rows — see the
// comment on renderRow).
func (p Picker) renderCardContent(item sources.Item, styled bool, innerWidth, numWidth int) string {
	line1 := p.renderCardLine1(item, styled, innerWidth, numWidth)

	indent := numWidth + 1 // align the meta strip under the title, past "#<n> "
	line2 := strings.Repeat(" ", indent) + p.renderCardLine2(item, styled, max(innerWidth-indent, 1))
	return line1 + "\n" + line2
}

// renderCardLine1 renders "#<number> <title>" on the left with the author
// right-aligned. The title truncates first to protect the author.
func (p Picker) renderCardLine1(item sources.Item, styled bool, innerWidth, numWidth int) string {
	prefix := ""
	if number := sourceFieldString(item, "number"); number != "" {
		num := "#" + number
		if pad := numWidth - len(num); pad > 0 {
			num += strings.Repeat(" ", pad)
		}
		if styled {
			num = styles.TextMutedStyle.Render(num)
		}
		prefix = num + " "
	}
	// Bold the title so each item's first line anchors its pair against the
	// dimmer meta line, separating otherwise-blended rows without a spacer.
	title := item.Title
	if styled {
		title = styles.TextForegroundStyle.Bold(true).Render(title)
	}

	right := ""
	if author := sourceFieldString(item, "author"); author != "" {
		right = "@" + author
		if styled {
			right = styles.TextMutedStyle.Render(right)
		}
	}
	return composeLR(prefix+title, right, innerWidth)
}

// renderCardLine2 renders the metadata strip on the left with labels
// right-aligned within width.
func (p Picker) renderCardLine2(item sources.Item, styled bool, width int) string {
	return composeLR(renderCardMeta(item, styled), renderCardLabels(item, styled), width)
}

// renderCardMeta builds the metadata strip in a fixed order: CI and review
// (colored) lead for PRs, then age, linked PR/issue, and assignee (all
// muted). Each element renders only when its field is present, so issues —
// which carry no ci/review/linked_issue — naturally lead with age.
func renderCardMeta(item sources.Item, styled bool) string {
	var parts []string

	if ci := sourceFieldString(item, "ci"); ci != "" {
		if cell := ciMetaCell(ci, styled); cell != "" {
			parts = append(parts, cell)
		}
	}
	if review := sourceFieldString(item, "review"); review != "" {
		cell := review
		if styled {
			cell = tableCellStyle("review", review).Render(cell)
		}
		parts = append(parts, cell)
	}
	if age := sourceFieldString(item, "age"); age != "" {
		parts = append(parts, muted(age, styled))
	}
	if n := sourceFieldInt(item, "linked_pr"); n > 0 {
		ref := metaIcon(styles.IconGitPR, linkedRef(n, sourceFieldInt(item, "linked_pr_count")))
		parts = append(parts, muted(ref, styled))
	}
	if n := sourceFieldInt(item, "linked_issue"); n > 0 {
		ref := metaIcon(styles.IconLink, linkedRef(n, sourceFieldInt(item, "linked_issue_count")))
		parts = append(parts, muted(ref, styled))
	}
	if assignee := sourceFieldString(item, "assignee"); assignee != "" {
		val := "@" + assignee
		if more := sourceFieldInt(item, "assignee_count") - 1; more > 0 {
			val += fmt.Sprintf(" +%d", more)
		}
		parts = append(parts, muted(metaIcon(styles.IconPerson, val), styled))
	}

	sep := " · "
	if styled {
		sep = " " + styles.TextMutedStyle.Render("·") + " "
	}
	return strings.Join(parts, sep)
}

// metaIcon renders a nerd-icon element with uniform spacing: the glyph with
// the constant's built-in padding trimmed, then a single space and the
// value when one is present. This keeps icon spacing consistent across the
// meta strip regardless of how much padding each icon constant carries.
func metaIcon(icon, value string) string {
	icon = strings.TrimRight(icon, " ")
	if value == "" {
		return icon
	}
	return icon + " " + value
}

// linkedRef renders "#<n>" for a cross-reference number, plus " +k" when
// more than one reference exists.
func linkedRef(number, count int) string {
	ref := fmt.Sprintf("#%d", number)
	if count > 1 {
		ref += fmt.Sprintf(" +%d", count-1)
	}
	return ref
}

// renderCardLabels renders up to two labels as secondary "[name]" tags.
func renderCardLabels(item sources.Item, styled bool) string {
	var tags []string
	for i, label := range sourceFieldStringSlice(item, "labels") {
		if i >= 2 {
			break
		}
		tag := "[" + label + "]"
		if styled {
			tag = styles.TextSecondaryStyle.Render(tag)
		}
		tags = append(tags, tag)
	}
	return strings.Join(tags, " ")
}

// ciMetaCell renders a CI status as a colored circle glyph alone — the
// glyph plus color carries the state, so the "passing"/"failing" label is
// dropped. Unknown states render nothing.
func ciMetaCell(ci string, styled bool) string {
	var icon string
	switch strings.ToLower(strings.TrimSpace(ci)) {
	case "passing":
		icon = styles.IconCircleCheck
	case "failing":
		icon = styles.IconCircleX
	case "pending":
		icon = styles.IconCircle
	default:
		return ""
	}
	icon = strings.TrimRight(icon, " ")
	if styled {
		icon = tableCellStyle("ci", ci).Render(icon)
	}
	return icon
}

// muted renders s in the muted style when styled, else returns it verbatim.
func muted(s string, styled bool) string {
	if styled {
		return styles.TextMutedStyle.Render(s)
	}
	return s
}

// composeLR places left and right text on one line of the given width:
// right is right-aligned and left truncates with an ellipsis when the two
// would overlap.
func composeLR(left, right string, width int) string {
	if right == "" {
		return ansi.Truncate(left, width, "…")
	}
	rightW := ansi.StringWidth(right)
	if ansi.StringWidth(left)+1+rightW > width {
		left = ansi.Truncate(left, max(width-rightW-1, 1), "…")
	}
	gap := max(width-ansi.StringWidth(left)-rightW, 1)
	return left + strings.Repeat(" ", gap) + right
}

// sourceFieldInt reads an integer item field, tolerating the numeric types
// JSON and Go maps may carry.
func sourceFieldInt(item sources.Item, key string) int {
	if item.Fields == nil {
		return 0
	}
	switch v := item.Fields[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}

func (p Picker) helpText() string {
	tab := p.activeState()

	if tab.loading {
		return components.KeyHints(
			components.HelpEntry{Key: "tab", Desc: "switch source"},
			components.HelpEntry{Key: "esc", Desc: "close"},
		)
	}
	if tab.searchErr != nil {
		return components.KeyHints(
			components.HelpEntry{Key: "r", Desc: "retry"},
			components.HelpEntry{Key: "tab", Desc: "switch source"},
			components.HelpEntry{Key: "esc", Desc: "close"},
		)
	}
	if p.searchMode {
		return components.KeyHints(
			components.HelpEntry{Key: "↑↓", Desc: "navigate"},
			components.HelpEntry{Key: "enter", Desc: "select"},
			components.HelpEntry{Key: "esc", Desc: "done"},
		)
	}
	return components.KeyHints(
		components.HelpEntry{Key: "tab", Desc: "switch source"},
		components.HintFilter,
		components.HintNav,
		components.HelpEntry{Key: "enter", Desc: "select"},
		components.HelpEntry{Key: "O", Desc: "open"},
		components.HelpEntry{Key: "esc", Desc: "close"},
	)
}

// --- Table helpers ---

const sourceFlexColumnMinWidth = 8

func resolveSourceColumnWidths(columns []sources.Column, total int) []int {
	widths := make([]int, len(columns))
	remaining := total - max(len(columns)-1, 0)
	flexTotal := 0
	for i, col := range columns {
		switch {
		case col.Flex > 0:
			flexTotal += col.Flex
		case col.Width > 0:
			widths[i] = col.Width
			remaining -= col.Width
		default:
			widths[i] = 12
			remaining -= 12
		}
	}
	if flexTotal == 0 {
		return widths
	}
	for i, col := range columns {
		if col.Flex > 0 {
			widths[i] = max(remaining*col.Flex/flexTotal, sourceFlexColumnMinWidth)
		}
	}
	return widths
}

// renderSourceTableRow composes a table-layout row. When styled is false
// the result contains no ANSI sequences (used for selected rows — see
// renderRow); cell padding is plain spaces either way so widths are
// identical in both modes.
func renderSourceTableRow(item sources.Item, columns []sources.Column, width int, styled bool) string {
	widths := resolveSourceColumnWidths(columns, width)
	cells := make([]string, 0, len(columns))
	for i, col := range columns {
		w := max(widths[i], 1)
		raw := sourceFieldString(item, col.Key)
		value := raw
		if col.Key == "number" && value != "" {
			value = "#" + value
		}
		value = ansi.Truncate(statusIcon(raw)+value, w, "…")
		value += strings.Repeat(" ", max(w-ansi.StringWidth(value), 0))
		if styled {
			value = tableCellStyle(col.Key, raw).Render(value)
		}
		cells = append(cells, value)
	}
	return strings.Join(cells, " ")
}

func statusIcon(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "passing":
		return "✓ "
	case "failing":
		return "✗ "
	case "pending":
		return "● "
	}
	return ""
}

func tableCellStyle(key, value string) lipgloss.Style {
	switch key {
	case "number", "id", "index":
		// Match the list layout (issues): numbers are muted so titles
		// carry the visual weight.
		return styles.TextMutedStyle
	case "author":
		return styles.TextMutedStyle
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "approved", "open", "success", "passing":
		return styles.TextSuccessStyle
	case "changes requested", "closed", "failure", "failed", "failing":
		return styles.TextErrorStyle
	case "review required", "pending":
		return styles.TextWarningStyle
	case "draft":
		return styles.TextMutedStyle
	case "merged":
		return styles.TextSecondaryStyle
	}
	return styles.TextForegroundStyle
}

func sourceFieldString(item sources.Item, key string) string {
	switch key {
	case "id":
		return item.ID
	case "title":
		return item.Title
	case "subtitle":
		return item.Subtitle
	}
	if item.Fields == nil {
		return ""
	}
	if v, ok := item.Fields[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func sourceFieldStringSlice(item sources.Item, key string) []string {
	if item.Fields == nil {
		return nil
	}
	value, ok := item.Fields[key]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		values := make([]string, 0, len(typed))
		for _, v := range typed {
			if s := strings.TrimSpace(fmt.Sprintf("%v", v)); s != "" {
				values = append(values, s)
			}
		}
		return values
	default:
		if s := strings.TrimSpace(fmt.Sprintf("%v", value)); s != "" {
			return []string{s}
		}
		return nil
	}
}
