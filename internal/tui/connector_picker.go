package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/sahilm/fuzzy"

	"github.com/colonyops/hive/internal/connectors"
	"github.com/colonyops/hive/internal/core/styles"
)

// connectorPickerMinVisible is the floor on how many rows the left pane
// renders even on a very short terminal, mirroring RepoPicker's
// max(p.height/3, 5) convention.
const connectorPickerMinVisible = 5

// connectorItemsSource implements fuzzy.Source over item titles, for
// local-mode filtering (mirrors commandEntries in command_palette.go).
type connectorItemsSource []connectors.Item

func (c connectorItemsSource) String(i int) string { return c[i].Title }
func (c connectorItemsSource) Len() int            { return len(c) }

// ConnectorPicker is a two-pane modal: a searchable/filterable item list on
// the left and a detail pane on the right. It reuses the textinput +
// sahilm/fuzzy pattern from CommandPalette for local-mode filtering rather
// than a hand-rolled substring loop; remote-mode search debounces a real
// Connector.Search call per manifest configuration.
type ConnectorPicker struct {
	conn     connectors.Connector
	manifest connectors.Manifest
	scope    string

	input textinput.Model

	// loaded holds the full result set from the last completed Search call;
	// items is the currently displayed set (loaded, or a local-filtered
	// subset of it).
	loaded []connectors.Item
	items  []connectors.Item

	cursor       int
	scrollOffset int

	searchInFlight bool
	searchedOnce   bool
	searchErr      error

	// detailCache/detailErr/detailPending are keyed by item ID. Reads and
	// writes to these maps are safe from value-receiver methods because a
	// map is a reference type; only the map header itself must not be
	// reassigned, and it never is here.
	detailCache   map[string]connectors.Detail
	detailErr     map[string]error
	detailPending map[string]bool

	cancelled bool
	selected  bool

	width, height int
}

// ConnectorPickerResult is the item and detail selected by the user, if any.
type ConnectorPickerResult struct {
	Item   connectors.Item
	Detail connectors.Detail
}

// NewConnectorPicker constructs a picker for conn, using manifest to decide
// layout/columns/search behavior and scope to constrain Search/FetchDetail
// calls (e.g. a GitHub "owner/name" repo). width/height are the caller's
// current terminal dimensions (mirrors NewRepoPicker(repos, currentRepo,
// width, height)) so the picker renders at the real size instead of a fixed
// default that can overflow a small terminal/tmux pane.
func NewConnectorPicker(conn connectors.Connector, manifest connectors.Manifest, scope string, width, height int) ConnectorPicker {
	input := textinput.New()
	input.Placeholder = "search..."
	input.Prompt = "/ "
	input.Focus()
	inputStyles := textinput.DefaultStyles(true)
	inputStyles.Focused.Prompt = styles.TextPrimaryStyle
	inputStyles.Cursor.Color = styles.ColorPrimary
	input.SetStyles(inputStyles)
	input.SetWidth(40)

	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}

	return ConnectorPicker{
		conn:          conn,
		manifest:      manifest,
		scope:         scope,
		input:         input,
		detailCache:   make(map[string]connectors.Detail),
		detailErr:     make(map[string]error),
		detailPending: make(map[string]bool),
		width:         width,
		height:        height,
	}
}

// Init issues the initial (empty-query) Search call to populate the list.
func (p ConnectorPicker) Init() tea.Cmd {
	return p.startSearch("")
}

// startSearch marks a search in flight and returns the Cmd that performs it.
func (p *ConnectorPicker) startSearch(query string) tea.Cmd {
	p.searchInFlight = true
	p.searchErr = nil
	return connectorSearchCmd(p.conn, p.scope, query)
}

// SetSize updates the picker's rendering dimensions.
func (p *ConnectorPicker) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// Update handles key events and connector RPC result messages.
func (p ConnectorPicker) Update(msg tea.Msg) (ConnectorPicker, tea.Cmd) {
	switch m := msg.(type) {
	case tea.KeyPressMsg:
		return p.handleKey(m)
	case connectorSearchResultMsg:
		return p.handleSearchResult(m)
	case connectorSearchErrorMsg:
		return p.handleSearchError(m)
	case connectorSearchDebounceMsg:
		return p.handleDebounce(m)
	case connectorDetailResultMsg:
		return p.handleDetailResult(m)
	case connectorDetailErrorMsg:
		return p.handleDetailError(m)
	}
	return p, nil
}

// handleKey dispatches navigation/selection keys, then falls through to the
// text input for query editing.
func (p ConnectorPicker) handleKey(msg tea.KeyPressMsg) (ConnectorPicker, tea.Cmd) {
	switch msg.String() {
	case "esc":
		p.cancelled = true
		return p, nil
	case "enter":
		if len(p.items) > 0 && p.cursor < len(p.items) {
			p.selected = true
		}
		return p, nil
	case "up", "ctrl+k":
		if p.cursor > 0 {
			p.cursor--
			p.clampScroll()
		}
		return p, p.triggerDetailFetch()
	case "down", "ctrl+j":
		if p.cursor < len(p.items)-1 {
			p.cursor++
			p.clampScroll()
		}
		return p, p.triggerDetailFetch()
	}

	var inputCmd tea.Cmd
	p.input, inputCmd = p.input.Update(msg)
	return p.handleQueryChanged(inputCmd)
}

// handleQueryChanged reacts to a query edit according to the manifest's
// search mode: local mode re-filters already-loaded items in memory; remote
// mode schedules a debounced Search call.
func (p ConnectorPicker) handleQueryChanged(inputCmd tea.Cmd) (ConnectorPicker, tea.Cmd) {
	query := p.input.Value()

	if p.manifest.Picker.Search.Mode != connectors.SearchModeRemote {
		p.applyLocalFilter(query)
		return p, tea.Batch(inputCmd, p.triggerDetailFetch())
	}

	delay := connectorDebounceDelay(p.manifest)
	return p, tea.Batch(inputCmd, connectorDebounceCmd(query, delay))
}

// applyLocalFilter narrows p.items to fuzzy matches of query against
// p.loaded. An empty query resets to the full loaded set.
func (p *ConnectorPicker) applyLocalFilter(query string) {
	if query == "" {
		p.items = p.loaded
		p.cursor = 0
		p.scrollOffset = 0
		return
	}

	matches := fuzzy.FindFrom(query, connectorItemsSource(p.loaded))
	items := make([]connectors.Item, len(matches))
	for i, match := range matches {
		items[i] = p.loaded[match.Index]
	}
	p.items = items
	p.cursor = 0
	p.scrollOffset = 0
}

// handleDebounce issues the actual remote Search call once the debounce
// delay elapses, but only if the query hasn't changed since the debounce
// was scheduled (otherwise this tick is stale and is dropped).
func (p ConnectorPicker) handleDebounce(msg connectorSearchDebounceMsg) (ConnectorPicker, tea.Cmd) {
	if msg.Query != p.input.Value() {
		return p, nil
	}
	return p, p.startSearch(msg.Query)
}

// handleSearchResult applies a completed Search response, discarding it if
// the query it answers no longer matches the current input (a stale/
// superseded response).
func (p ConnectorPicker) handleSearchResult(msg connectorSearchResultMsg) (ConnectorPicker, tea.Cmd) {
	if msg.Query != p.input.Value() {
		return p, nil
	}
	p.searchInFlight = false
	p.searchedOnce = true
	p.searchErr = nil
	p.loaded = msg.Items
	p.items = msg.Items
	p.cursor = 0
	p.scrollOffset = 0
	return p, p.triggerDetailFetch()
}

// handleSearchError records a Search failure, discarding it if stale.
func (p ConnectorPicker) handleSearchError(msg connectorSearchErrorMsg) (ConnectorPicker, tea.Cmd) {
	if msg.Query != p.input.Value() {
		return p, nil
	}
	p.searchInFlight = false
	p.searchedOnce = true
	p.searchErr = msg.Err
	p.loaded = nil
	p.items = nil
	p.cursor = 0
	p.scrollOffset = 0
	return p, nil
}

// handleDetailResult caches a completed FetchDetail response.
func (p ConnectorPicker) handleDetailResult(msg connectorDetailResultMsg) (ConnectorPicker, tea.Cmd) {
	delete(p.detailPending, msg.ID)
	delete(p.detailErr, msg.ID)
	p.detailCache[msg.ID] = msg.Detail
	return p, nil
}

// handleDetailError records a FetchDetail failure for the detail pane.
func (p ConnectorPicker) handleDetailError(msg connectorDetailErrorMsg) (ConnectorPicker, tea.Cmd) {
	delete(p.detailPending, msg.ID)
	p.detailErr[msg.ID] = msg.Err
	return p, nil
}

// triggerDetailFetch issues a lazy FetchDetail call for the currently
// highlighted item if the connector supports detail, the item doesn't
// already carry inline detail, and it isn't already cached or in flight.
func (p ConnectorPicker) triggerDetailFetch() tea.Cmd {
	if !p.manifest.Capabilities.FetchDetail {
		return nil
	}
	if p.cursor < 0 || p.cursor >= len(p.items) {
		return nil
	}

	item := p.items[p.cursor]
	if item.Detail.Kind() != connectors.DetailKindNone {
		return nil
	}
	if _, cached := p.detailCache[item.ID]; cached {
		return nil
	}
	if p.detailPending[item.ID] {
		return nil
	}

	p.detailPending[item.ID] = true
	return connectorFetchDetailCmd(p.conn, p.scope, item)
}

// clampScroll keeps the cursor within the visible scroll window.
func (p *ConnectorPicker) clampScroll() {
	visible := p.visibleCount()
	if p.cursor < p.scrollOffset {
		p.scrollOffset = p.cursor
	} else if p.cursor >= p.scrollOffset+visible {
		p.scrollOffset = p.cursor - visible + 1
	}
	maxOffset := max(len(p.items)-visible, 0)
	p.scrollOffset = min(max(p.scrollOffset, 0), maxOffset)
}

// visibleCount bounds how many rows the left pane renders at once; the
// remainder scrolls. It scales with the picker's height (mirrors
// RepoPicker.visibleCount: min(len(items), max(height/3, floor))) so a
// short terminal gets a shorter list instead of a modal that overflows the
// screen.
func (p ConnectorPicker) visibleCount() int {
	return min(len(p.items), max(p.height/3, connectorPickerMinVisible))
}

// paneWidths splits the picker's available width between the list and
// detail panes. It clamps to the actual terminal width (via p.width) rather
// than only growing from floors, so a narrow terminal shrinks the detail
// pane (down to a 20-column floor) before the modal overflows the screen.
func (p ConnectorPicker) paneWidths() (listWidth, detailWidth int) {
	total := p.width
	if total <= 0 {
		total = 80
	}
	// Reserve room for the inter-pane gap and modal frame/padding.
	available := max(total-6, 20)

	listWidth = max(available/3, p.minListWidth())
	detailWidth = available - listWidth
	if detailWidth < 20 {
		detailWidth = 20
		listWidth = max(available-detailWidth, 10)
	}
	return listWidth, detailWidth
}

// minListWidth returns the minimum left-pane width needed to render the
// manifest's table columns without truncation, or a sane default for list
// layout.
func (p ConnectorPicker) minListWidth() int {
	if p.manifest.Picker.Layout != connectors.LayoutModeTable || len(p.manifest.Picker.Columns) == 0 {
		return 28
	}
	total := 2 // leading cursor column
	for i, col := range p.manifest.Picker.Columns {
		width := col.Width
		if width <= 0 {
			width = 12
		}
		total += width
		if i > 0 {
			total++ // inter-column space
		}
	}
	return total
}

// Selected returns the highlighted item and its best-known detail, if the
// user pressed enter on a non-empty list.
func (p ConnectorPicker) Selected() (ConnectorPickerResult, bool) {
	if !p.selected || p.cursor < 0 || p.cursor >= len(p.items) {
		return ConnectorPickerResult{}, false
	}
	item := p.items[p.cursor]
	detail := item.Detail
	if detail.Kind() == connectors.DetailKindNone {
		if cached, ok := p.detailCache[item.ID]; ok {
			detail = cached
		}
	}
	return ConnectorPickerResult{Item: item, Detail: detail}, true
}

// Cancelled reports whether the user dismissed the picker.
func (p ConnectorPicker) Cancelled() bool {
	return p.cancelled
}

// View renders the two-pane picker: item list on the left, detail on the
// right.
func (p ConnectorPicker) View() string {
	title := styles.ModalTitleStyle.Render(p.manifest.DisplayName)

	listWidth, detailWidth := p.paneWidths()

	left := lipgloss.JoinVertical(lipgloss.Left,
		p.input.View(),
		"",
		p.renderList(listWidth),
	)

	right := p.renderDetailPane(detailWidth)

	body := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(listWidth).Render(left),
		lipgloss.NewStyle().Width(detailWidth).Padding(0, 0, 0, 2).Render(right),
	)

	help := styles.ModalHelpStyle.Render("↑/↓ navigate  enter select  esc cancel")

	content := lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", help)
	return styles.ModalStyle.Width(listWidth + detailWidth + 6).Render(content)
}

// renderList renders the left-pane item rows, honoring the manifest's
// layout (list vs table) and current search/loading state.
func (p ConnectorPicker) renderList(width int) string {
	if p.searchErr != nil {
		return styles.TextErrorStyle.Render("search failed: " + p.searchErr.Error())
	}
	if p.searchInFlight {
		return styles.TextMutedStyle.Render("searching...")
	}
	if p.searchedOnce && len(p.items) == 0 {
		return styles.TextMutedStyle.Render("no results")
	}

	visible := p.visibleCount()
	var lines []string
	for i := range visible {
		idx := i + p.scrollOffset
		if idx >= len(p.items) {
			break
		}
		lines = append(lines, p.renderRow(p.items[idx], idx == p.cursor, width))
	}
	return strings.Join(lines, "\n")
}

// renderRow renders a single item row, using table columns when the
// manifest declares a table layout, or title/subtitle otherwise.
func (p ConnectorPicker) renderRow(item connectors.Item, selected bool, width int) string {
	cursor := "  "
	rowStyle := styles.TextForegroundStyle
	if selected {
		cursor = "▸ "
		rowStyle = styles.TextPrimaryBoldStyle
	}

	var text string
	if p.manifest.Picker.Layout == connectors.LayoutModeTable && len(p.manifest.Picker.Columns) > 0 {
		text = renderConnectorTableRow(item, p.manifest.Picker.Columns)
	} else {
		text = item.Title
		if item.Subtitle != "" {
			text = fmt.Sprintf("%s  %s", item.Title, styles.TextMutedStyle.Render(item.Subtitle))
		}
	}

	line := cursor + text
	return lipgloss.NewStyle().MaxWidth(width).Render(rowStyle.Render(line))
}

// renderConnectorTableRow renders an item's Fields as a single row aligned
// to the manifest's column widths.
func renderConnectorTableRow(item connectors.Item, columns []connectors.Column) string {
	var cells []string
	for _, col := range columns {
		value := connectorFieldString(item, col.Key)
		width := col.Width
		if width <= 0 {
			width = 12
		}
		cells = append(cells, lipgloss.NewStyle().Width(width).MaxWidth(width).Render(value))
	}
	return strings.Join(cells, " ")
}

// connectorFieldString resolves a column key against an item's well-known
// fields (id/title/subtitle) or its Fields map, returning "" if absent.
func connectorFieldString(item connectors.Item, key string) string {
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

// renderDetailPane renders the detail pane for the currently highlighted
// item, honoring inline item detail, cache, in-flight, and error states in
// that priority order.
func (p ConnectorPicker) renderDetailPane(width int) string {
	if len(p.items) == 0 || p.cursor < 0 || p.cursor >= len(p.items) {
		return styles.TextMutedStyle.Render("")
	}

	item := p.items[p.cursor]

	if item.Detail.Kind() != connectors.DetailKindNone {
		return renderConnectorDetail(item.Detail, width)
	}
	if err, ok := p.detailErr[item.ID]; ok {
		return styles.TextErrorStyle.Render("detail failed: " + err.Error())
	}
	if detail, ok := p.detailCache[item.ID]; ok {
		return renderConnectorDetail(detail, width)
	}
	if p.detailPending[item.ID] {
		return styles.TextMutedStyle.Render("loading detail...")
	}
	return renderConnectorDetail(connectors.Detail{}, width)
}

// connectorDebounceDelay resolves the remote-search debounce delay from the
// manifest, falling back to defaultConnectorSearchDebounce when unset.
func connectorDebounceDelay(manifest connectors.Manifest) time.Duration {
	if manifest.Picker.Search.DebounceMS <= 0 {
		return defaultConnectorSearchDebounce
	}
	return time.Duration(manifest.Picker.Search.DebounceMS) * time.Millisecond
}
