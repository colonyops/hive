package sourcepicker

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync/atomic"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/rs/zerolog/log"
	"github.com/sahilm/fuzzy"

	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/sources"
)

// Fixed dialog sizing: the modal's overall width/height are a deterministic
// function of the terminal size, never of the current item/detail content.
const (
	sourcePickerMaxModalHeight = 36
	sourcePickerModalMargin    = 2
	sourcePickerMinModalWidth  = 72
	// sourcePickerChrome counts the fixed rows View renders around the
	// scrollable content area: border (2), vertical padding (0), tab
	// bar (1), separator line (1), blank line above help (1), and the
	// help line including ModalHelpStyle's MarginTop (2).
	sourcePickerChrome = 7
)

// sourcePickerGen issues a unique generation token per picker instance.
var sourcePickerGen atomic.Int64

// rowHighlightBg is a subtle background for the selected row — just a
// small lift above the terminal background so the row is visible without
// being distracting.
var rowHighlightBg = lipgloss.Color("#232433")

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
}

// Result is the item selected by the user, if any.
type Result struct {
	Item      sources.Item
	SourceID  string
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

	return Picker{
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
}

func computePickerDims(width, height int) (modalWidth, modalHeight, contentWidth, innerWidth, contentHeight int) {
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}

	modalWidth = min(max(int(float64(width)*0.92), sourcePickerMinModalWidth), max(width-sourcePickerModalMargin, sourcePickerMinModalWidth))
	modalHeight = min(max(height-sourcePickerModalMargin, sourcePickerChrome+3), sourcePickerMaxModalHeight)
	contentHeight = max(modalHeight-sourcePickerChrome, 3)

	// contentWidth excludes the border (2); innerWidth also excludes
	// the per-section horizontal padding (4).
	contentWidth = max(modalWidth-2, 20)
	innerWidth = max(contentWidth-4, 16)

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
		ctx := contextBackground()
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
	if p.input.Value() != before {
		p.applyLocalFilter()
	}
	return p, inputCmd
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

// handleSearchResult applies a completed remote Search response.
func (p Picker) handleSearchResult(msg sourceSearchResultMsg) (Picker, tea.Cmd) {
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
	if msg.Gen != p.gen || msg.Query != p.input.Value() {
		return p, nil
	}
	tab := p.activeState()
	tab.loading = true
	return p, sourceSearchCmd(p.gen, tab.tab.Source, tab.tab.ID, p.scope, msg.Query)
}

func (p *Picker) clampScroll(tab *tabState) {
	visible := p.listHeight()
	if tab.cursor < tab.scrollOffset {
		tab.scrollOffset = tab.cursor
	} else if tab.cursor >= tab.scrollOffset+visible {
		tab.scrollOffset = tab.cursor - visible + 1
	}
	maxOffset := max(len(tab.filteredItems)-visible, 0)
	tab.scrollOffset = min(max(tab.scrollOffset, 0), maxOffset)
}

func (p Picker) listHeight() int {
	// content area minus the two dividers and filter input row.
	return max(p.contentHeight-3, 1)
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
		Templates: tab.tab.Templates,
	}, true
}

// Cancelled reports whether the user dismissed the picker.
func (p Picker) Cancelled() bool {
	return p.cancelled
}

// --- View ---

func (p Picker) View() string {
	tabBar := p.renderTabBar()
	sep := styles.TextMutedStyle.Render(strings.Repeat("─", p.contentWidth))
	body := p.renderBody()
	help := styles.ModalHelpStyle.Render(p.helpText())

	// Pad sections that need inset; dividers span the full inner width.
	pad := lipgloss.NewStyle().Padding(0, 2)
	content := lipgloss.JoinVertical(lipgloss.Left,
		pad.Render(tabBar),
		sep,
		body,
		"",
		pad.Render(help),
	)

	// No padding on the modal itself so dividers reach the border edges.
	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.ColorPrimary)

	return modalStyle.Width(p.modalWidth).Render(content)
}

func (p Picker) renderTabBar() string {
	activeStyle := lipgloss.NewStyle().
		Background(styles.ColorSurface).
		Foreground(styles.ColorPrimary).
		Bold(true).
		Padding(0, 1)
	inactiveStyle := lipgloss.NewStyle().
		Foreground(styles.ColorMuted).
		Padding(0, 2)

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

	tabRow := lipgloss.JoinHorizontal(lipgloss.Center, parts...)

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
			styles.TextMutedStyle.Render("r to retry · tab to switch source"),
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

	// List state: filter input + dividers + item rows.
	pad := lipgloss.NewStyle().Padding(0, 2)
	filterLine := pad.Render(p.renderFilterLine(tab))
	div := styles.TextMutedStyle.Render(strings.Repeat("─", p.contentWidth))
	list := pad.Render(p.renderList(tab))

	return lipgloss.NewStyle().
		Width(p.contentWidth).
		Height(p.contentHeight).
		MaxHeight(p.contentHeight).
		Render(lipgloss.JoinVertical(lipgloss.Left, div, filterLine, div, list))
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
	placeholder := styles.TextPrimaryStyle.Render("/") + " " + styles.TextMutedStyle.Render(fmt.Sprintf("filter %s…", strings.ToLower(name)))

	countStr := styles.TextMutedStyle.Render(fmt.Sprintf("%d", len(tab.filteredItems)))

	gap := max(p.innerWidth-lipgloss.Width(placeholder)-lipgloss.Width(countStr), 1)
	return placeholder + strings.Repeat(" ", gap) + countStr
}

func (p Picker) renderList(tab *tabState) string {
	rowHeight := p.listHeight()
	lines := make([]string, 0, rowHeight)

	visible := min(len(tab.filteredItems), rowHeight)
	for i := range visible {
		idx := i + tab.scrollOffset
		if idx >= len(tab.filteredItems) {
			break
		}
		item := tab.filteredItems[idx]
		selected := idx == tab.cursor
		line := p.renderRow(item, selected, tab)
		lines = append(lines, line)
	}

	for len(lines) < rowHeight {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (p Picker) renderRow(item sources.Item, selected bool, tab *tabState) string {
	width := p.innerWidth

	// For table layout sources, use the manifest columns.
	if tab.tab.Manifest.Picker.Layout == sources.LayoutModeTable && len(tab.tab.Manifest.Picker.Columns) > 0 {
		line := renderSourceTableRow(item, tab.tab.Manifest.Picker.Columns, max(width-1, 1), selected)
		return p.applyRowStyle(line, selected, width)
	}

	// Single-line list layout.
	return p.renderSingleLineRow(item, selected, width)
}

// applyRowStyle wraps content with selected/normal styling: selected rows
// get a left border accent and surface background; unselected rows get a
// two-space indent to keep alignment with the border+space of selected rows.
func (p Picker) applyRowStyle(content string, selected bool, width int) string {
	if selected {
		return lipgloss.NewStyle().
			Border(lipgloss.ThickBorder(), false, false, false, true).
			BorderForeground(styles.ColorPrimary).
			Background(rowHighlightBg).
			PaddingLeft(1).
			Width(width).
			MaxWidth(width).
			MaxHeight(1).
			Render(content)
	}
	return lipgloss.NewStyle().
		PaddingLeft(2).
		Width(width).
		MaxWidth(width).
		MaxHeight(1).
		Render(content)
}

func (p Picker) renderSingleLineRow(item sources.Item, selected bool, width int) string {
	var parts []string

	// CI status icon if present.
	if ciStatus := sourceFieldString(item, "ci_status"); ciStatus != "" {
		parts = append(parts, statusIcon(ciStatus))
	}

	// Number.
	numStyle := styles.TextMutedStyle
	if selected {
		numStyle = styles.TextPrimaryStyle
	}
	if number := sourceFieldString(item, "number"); number != "" {
		parts = append(parts, numStyle.Render("#"+number))
	}

	// Title.
	titleStyle := styles.TextForegroundStyle
	if selected {
		titleStyle = lipgloss.NewStyle().
			Foreground(styles.ColorPrimary).
			Bold(true)
	}
	parts = append(parts, titleStyle.Render(item.Title))

	// Labels (first 2).
	labels := sourceFieldStringSlice(item, "labels")
	for i, label := range labels {
		if i >= 2 {
			break
		}
		parts = append(parts, styles.TextSecondaryStyle.Render("["+label+"]"))
	}

	// Right-aligned metadata: author.
	var rightParts []string
	if author := sourceFieldString(item, "author"); author != "" {
		rightParts = append(rightParts, styles.TextMutedStyle.Render("@"+author))
	}

	left := strings.Join(parts, " ")
	right := strings.Join(rightParts, "  ")

	// Build the inner content, then wrap with the row style.
	innerWidth := max(width-2, 10) // account for the left border+space / padding
	leftWidth := ansi.StringWidth(left)
	rightWidth := ansi.StringWidth(right)

	var row string
	if right == "" {
		row = ansi.Truncate(left, innerWidth, "…")
	} else {
		gap := max(innerWidth-leftWidth-rightWidth, 1)
		if leftWidth+1+rightWidth > innerWidth {
			available := max(innerWidth-rightWidth-1, 10)
			left = ansi.Truncate(left, available, "…")
			gap = max(innerWidth-ansi.StringWidth(left)-rightWidth, 1)
		}
		row = left + strings.Repeat(" ", gap) + right
	}

	return p.applyRowStyle(row, selected, width)
}

func (p Picker) helpText() string {
	tab := p.activeState()

	if tab.loading {
		return helpLine("tab", "switch source", "esc", "close")
	}
	if tab.searchErr != nil {
		return helpLine("r", "retry", "tab", "switch source", "esc", "close")
	}
	if p.searchMode {
		return helpLine("↑↓", "navigate", "enter", "select", "esc", "done")
	}
	return helpLine("tab", "switch source", "/", "filter", "j/k", "navigate", "enter", "select", "esc", "close")
}

// helpLine builds a help string from alternating key/description pairs.
func helpLine(pairs ...string) string {
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		key := styles.TextForegroundBoldStyle.Render(pairs[i])
		parts = append(parts, key+" "+pairs[i+1])
	}
	return strings.Join(parts, " · ")
}

// --- Table helpers (preserved from original) ---

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

func renderSourceTableRow(item sources.Item, columns []sources.Column, width int, selected bool) string {
	widths := resolveSourceColumnWidths(columns, width)
	cells := make([]string, 0, len(columns))
	for i, col := range columns {
		w := max(widths[i], 1)
		value := sourceFieldString(item, col.Key)
		if col.Key == "number" && value != "" {
			value = "#" + value
		}
		style := tableCellStyle(col.Key, value)
		if selected {
			style = styles.TextPrimaryBoldStyle
		}
		value = ansi.Truncate(statusIcon(value)+value, w, "…")
		cells = append(cells, lipgloss.NewStyle().Width(w).MaxHeight(1).Render(style.Render(value)))
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
		return styles.TextPrimaryStyle
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
